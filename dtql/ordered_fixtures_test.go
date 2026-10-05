package dtql

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

const orderedFixtureDir = "testdata/ordered-aggregates"

type orderedFixtureSuite struct {
	SchemaVersion int    `json:"schemaVersion"`
	Schema        string `json:"schema"`
	Dataset       string `json:"dataset"`
	Cases         []struct {
		Name, Input, Rows, Error string
		Strategy                 map[string]string `json:"strategy"`
	} `json:"cases"`
}

type orderedFixtureBackend struct {
	dal.Backend
	tables map[string][]record.Record
	caps   dal.QueryCapabilities
}

// The memory adapter's ordinary record shape preserves timestamp columns on
// unprojected reads; the fixture declares them explicitly for that path.
type orderedInvoiceRecord struct {
	InvoiceId   float64    `json:"InvoiceId"`
	CustomerId  float64    `json:"CustomerId"`
	InvoiceDate *time.Time `json:"InvoiceDate"`
	Total       *float64   `json:"Total"`
}

type orderedTypedRecord struct {
	G          float64        `json:"G"`
	KText      string         `json:"KText"`
	KNumber    float64        `json:"KNumber"`
	KBool      bool           `json:"KBool"`
	KTimestamp time.Time      `json:"KTimestamp"`
	XNumber    float64        `json:"XNumber"`
	XText      string         `json:"XText"`
	XBool      bool           `json:"XBool"`
	XTimestamp time.Time      `json:"XTimestamp"`
	XUUID      string         `json:"XUUID"`
	XEnum      string         `json:"XEnum"`
	XCitext    string         `json:"XCitext"`
	XJSON      map[string]any `json:"XJSON"`
}

type orderedOffsetRecord struct {
	G float64   `json:"G"`
	K time.Time `json:"K"`
	X string    `json:"X"`
}

func (b orderedFixtureBackend) QueryCapabilities() dal.QueryCapabilities { return b.caps }

func (b orderedFixtureBackend) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q, ok := query.(dal.StructuredQuery)
	if !ok || q.From() == nil || q.From().Base() == nil || len(q.From().Joins()) != 0 || dal.HasAggregation(q) {
		return nil, errors.New("ordered fixture backend received a non-leaf read")
	}
	rows := b.tables[q.From().Base().Name()]
	if condition := q.Where(); condition != nil {
		comparison, ok := condition.(dal.Comparison)
		if !ok {
			return nil, fmt.Errorf("unsupported fixture leaf condition %T", condition)
		}
		field, fieldOK := comparison.Left.(dal.FieldRef)
		constant, constantOK := comparison.Right.(dal.Constant)
		if !fieldOK || !constantOK {
			return nil, errors.New("fixture leaf condition must compare a field with a constant")
		}
		filtered := make([]record.Record, 0, len(rows))
		for _, row := range rows {
			value := row.Data().(map[string]any)[field.Name()]
			match := false
			switch comparison.Operator {
			case dal.Equal:
				match = fmt.Sprint(value) == fmt.Sprint(constant.Value)
			case dal.GreaterThen:
				v, vok := value.(float64)
				c, cok := constant.Value.(int)
				match = vok && cok && v > float64(c)
			default:
				return nil, fmt.Errorf("unsupported fixture leaf operator %s", comparison.Operator)
			}
			if match {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if orders := q.OrderBy(); len(orders) > 0 {
		// The streaming executor may trust a provider's claimed group-key order.
		// This fake supplies it, even when fixture arrival interleaves groups.
		for _, order := range orders {
			if _, ok := order.Expression().(dal.FieldRef); !ok {
				return nil, fmt.Errorf("unsupported fixture source order %T", order.Expression())
			}
		}
		rows = append([]record.Record(nil), rows...)
		sort.SliceStable(rows, func(i, j int) bool {
			left, right := rows[i].Data().(map[string]any), rows[j].Data().(map[string]any)
			for _, order := range orders {
				key := order.Expression().(dal.FieldRef).Name()
				comparison := orderedFixtureCompare(left[key], right[key])
				if comparison != 0 {
					if order.Descending() {
						return comparison > 0
					}
					return comparison < 0
				}
			}
			return false
		})
	}
	if len(rows) == 0 {
		return dal.EmptyReader{}, nil
	}
	return dal.NewRecordsReader(rows), nil
}

func orderedFixtureCompare(left, right any) int {
	if left == nil {
		if right == nil {
			return 0
		}
		return -1
	}
	if right == nil {
		return 1
	}
	switch l := left.(type) {
	case float64:
		r := right.(float64)
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
		return 0
	case string:
		return strings.Compare(l, right.(string))
	case bool:
		r := right.(bool)
		if l == r {
			return 0
		}
		if !l {
			return -1
		}
		return 1
	case time.Time:
		r := right.(time.Time)
		if l.Before(r) {
			return -1
		}
		if l.After(r) {
			return 1
		}
		return 0
	default:
		return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
	}
}

func TestOrderedFixtureBackendMakesInterleavedGroupsContiguous(t *testing.T) {
	rows := []record.Record{}
	for i, group := range []float64{2, 1, 2, 1} {
		rows = append(rows, record.NewRecordWithData(record.NewKeyWithID("items", i), map[string]any{"G": group, "seq": i}))
	}
	backend := orderedFixtureBackend{tables: map[string][]record.Record{"items": rows}}
	for name, tc := range map[string]struct {
		order dal.OrderExpression
		want  []float64
	}{
		"ascending":  {dal.AscendingField("G"), []float64{1, 1, 2, 2}},
		"descending": {dal.DescendingField("G"), []float64{2, 2, 1, 1}},
	} {
		t.Run(name, func(t *testing.T) {
			q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().OrderBy(tc.order).SelectKeysOnly(0)
			reader, err := backend.ExecuteQueryToRecordsReader(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			got := orderedFixtureRows(t, reader)
			for i, group := range tc.want {
				if got[i]["G"] != group {
					t.Fatalf("ordered groups = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func orderedFixtureReadJSON(t *testing.T, file string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(orderedFixtureDir, file))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func orderedFixtureRecords(t *testing.T, reversed, timestampsAsText bool) map[string][]record.Record {
	t.Helper()
	var schema struct {
		Tables map[string]map[string]string `json:"tables"`
	}
	var dataset struct {
		Tables map[string][]map[string]any `json:"tables"`
	}
	orderedFixtureReadJSON(t, "schema.json", &schema)
	orderedFixtureReadJSON(t, "dataset.json", &dataset)
	tables := make(map[string][]record.Record, len(schema.Tables))
	for name, fields := range schema.Tables {
		for i, row := range dataset.Tables[name] {
			data := make(map[string]any, len(row))
			for field, value := range row {
				if fields[field] == "timestamp" && value != nil && !timestampsAsText {
					instant, err := time.Parse(time.RFC3339Nano, value.(string))
					if err != nil {
						t.Fatal(err)
					}
					data[field] = instant
				} else {
					data[field] = value
				}
			}
			tables[name] = append(tables[name], record.NewRecordWithData(record.NewKeyWithID(name, i), data))
		}
		if reversed {
			rows := tables[name]
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	return tables
}

func orderedFixtureRows(t *testing.T, reader dal.RecordsReader) []map[string]any {
	t.Helper()
	records, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]any, len(records))
	for i, row := range records {
		data, ok := row.Data().(map[string]any)
		if !ok {
			t.Fatalf("result row %d is %T", i, row.Data())
		}
		for key := range data {
			if strings.HasPrefix(key, "\x00") {
				t.Fatalf("result row %d holds reserved key %q", i, key)
			}
		}
		rows[i] = data
	}
	return rows
}

func orderedFixtureNormalized(t *testing.T, rows []map[string]any) []map[string]any {
	t.Helper()
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var normalized []map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatal(err)
	}
	if normalized == nil {
		return []map[string]any{}
	}
	return normalized
}

func TestOrderedAggregateFixtureManifestAndTypedDataset(t *testing.T) {
	var manifest subqueryFixtureManifest
	var suite orderedFixtureSuite
	orderedFixtureReadJSON(t, "manifest.json", &manifest)
	orderedFixtureReadJSON(t, "suite.json", &suite)
	if manifest.SchemaVersion != 1 || suite.SchemaVersion != 1 || len(suite.Cases) < 30 {
		t.Fatalf("ordered fixture version/count: manifest=%d, suite=%d, cases=%d", manifest.SchemaVersion, suite.SchemaVersion, len(suite.Cases))
	}
	entries, err := os.ReadDir(orderedFixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries)-1 != len(manifest.Files) {
		t.Fatalf("manifest tracks %d files, directory has %d", len(manifest.Files), len(entries)-1)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "manifest.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(orderedFixtureDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != manifest.Files[entry.Name()] {
			t.Fatalf("%s digest = %s, manifest = %s", entry.Name(), got, manifest.Files[entry.Name()])
		}
	}
	for _, file := range []string{suite.Schema, suite.Dataset, "suite.json"} {
		if manifest.Files[file] == "" {
			t.Fatalf("unmanaged suite file %q", file)
		}
	}
	for _, fixture := range suite.Cases {
		if fixture.Name == "" || fixture.Input == "" || (fixture.Rows == "") == (fixture.Error == "") {
			t.Fatalf("incomplete fixture: %#v", fixture)
		}
		for _, file := range []string{fixture.Input, fixture.Rows, fixture.Error} {
			if file != "" && manifest.Files[file] == "" {
				t.Fatalf("unmanaged fixture file %q", file)
			}
		}
		if fixture.Rows != "" {
			for _, engine := range []string{"memory", "sqlite", "postgres"} {
				if fixture.Strategy[engine] != "native" && fixture.Strategy[engine] != "dalgo" {
					t.Fatalf("%s has no %s strategy", fixture.Name, engine)
				}
			}
		}
	}
	tables := orderedFixtureRecords(t, false, false)
	if _, ok := tables["Offset"][0].Data().(map[string]any)["K"].(time.Time); !ok {
		t.Fatal("timestamp loader did not preserve time.Time")
	}
}

// A text-only timestamp loader would pass many ordinary date examples while
// misordering values with different offsets. This control deliberately makes
// the two loader representations produce different answers.
func TestOrderedAggregateFixtureTimestampLoaderHasAStringRedControl(t *testing.T) {
	input, err := os.ReadFile(filepath.Join(orderedFixtureDir, "timestamp-key-offsets.dtql.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := Deserialize(input)
	if err != nil {
		t.Fatal(err)
	}
	run := func(timestampsAsText bool) any {
		backend := orderedFixtureBackend{tables: orderedFixtureRecords(t, false, timestampsAsText)}
		reader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		rows := orderedFixtureRows(t, reader)
		if len(rows) != 1 {
			t.Fatalf("rows = %#v", rows)
		}
		return rows[0]["first_x"]
	}
	if got := run(false); got != "earlier" {
		t.Fatalf("typed timestamp answer = %v, want earlier", got)
	}
	if got := run(true); got != "later" {
		t.Fatalf("string loader control = %v, want later", got)
	}
}

func TestOrderedAggregateDocumentFixtures(t *testing.T) {
	var suite orderedFixtureSuite
	orderedFixtureReadJSON(t, "suite.json", &suite)
	for _, fixture := range suite.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(orderedFixtureDir, fixture.Input))
			if err != nil {
				t.Fatal(err)
			}
			q, parseErr := Deserialize(input)
			if fixture.Error != "" {
				var diagnostic struct {
					Contains string `json:"contains"`
				}
				orderedFixtureReadJSON(t, fixture.Error, &diagnostic)
				if parseErr == nil {
					_, parseErr = dal.ExecuteRecursiveQuery(context.Background(), fixtureLeafExecutor{tables: orderedFixtureRecords(t, false, false)}, q)
				}
				if parseErr == nil || !strings.Contains(parseErr.Error(), diagnostic.Contains) {
					t.Fatalf("error = %v, want %q", parseErr, diagnostic.Contains)
				}
				return
			}
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			var want []map[string]any
			orderedFixtureReadJSON(t, fixture.Rows, &want)
			if want == nil {
				want = []map[string]any{}
			}
			if fixture.Name == "unaliased-long-name" {
				want[0][q.Columns()[0].Expression.String()] = want[0]["__expression_string__"]
				delete(want[0], "__expression_string__")
				if len(q.Columns()[0].Expression.String()) <= 63 {
					t.Fatalf("unaliased aggregate label is only %d bytes", len(q.Columns()[0].Expression.String()))
				}
			}
			for _, reversed := range []bool{false, true} {
				modes := []string{"hash", "streaming", "memory", "recursive"}
				if fixture.Name == "first-last-by-date" || fixture.Name == "timestamp-key" || fixture.Name == "timestamp-key-offsets" {
					modes = append(modes, "memory-columnar")
				}
				if fixture.Name == "joined" {
					modes = []string{"generic-join", "recursive", "federated"}
				} else if fixture.Name == "with-subquery" {
					modes = []string{"recursive", "federated"}
				}
				for _, mode := range modes {
					t.Run(fmt.Sprintf("%s/reversed=%v", mode, reversed), func(t *testing.T) {
						tables := orderedFixtureRecords(t, reversed, false)
						var reader dal.RecordsReader
						var err error
						var progress []dal.FederatedProgress
						switch mode {
						case "hash", "streaming":
							caps := dal.QueryCapabilities{}
							if mode == "streaming" {
								caps.OrderBy, caps.GroupKeyOrder = true, true
							}
							plan, planErr := dal.PlanAggregation(q, caps)
							if planErr != nil {
								t.Fatal(planErr)
							}
							wantStrategy := dal.AggregationHash
							if mode == "streaming" && len(q.GroupBy()) > 0 {
								wantStrategy = dal.AggregationStreaming
							}
							if plan.Strategy != wantStrategy {
								t.Fatalf("strategy = %s, want %s", plan.Strategy, wantStrategy)
							}
							reader, err = dal.NewDB(orderedFixtureBackend{tables: tables, caps: caps}).ExecuteQueryToRecordsReader(context.Background(), q)
						case "memory", "memory-columnar":
							storage := []dalgo2memory.CollectionOption{}
							if mode == "memory-columnar" {
								storage = append(storage, dalgo2memory.WithColumnarStorage())
							}
							db := dalgo2memory.New(dalgo2memory.FirestoreProfile(), dalgo2memory.WithSchema(true,
								dalgo2memory.WithCollection[orderedInvoiceRecord]("Invoice", nil, storage...),
								dalgo2memory.WithCollection[orderedTypedRecord]("Typed", nil, storage...),
								dalgo2memory.WithCollection[orderedOffsetRecord]("Offset", nil, storage...),
							))
							err = db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
								for _, rows := range tables {
									for _, row := range rows {
										if err := tx.Set(ctx, row); err != nil {
											return err
										}
									}
								}
								return nil
							})
							if err == nil {
								reader, err = db.ExecuteQueryToRecordsReader(context.Background(), q)
							}
						case "recursive":
							reader, err = dal.ExecuteRecursiveQuery(context.Background(), orderedFixtureBackend{tables: tables}, q)
						case "generic-join":
							reader, err = dal.NewDB(orderedFixtureBackend{tables: tables}).ExecuteQueryToRecordsReader(context.Background(), q)
						case "federated":
							reader, err = dal.ExecuteFederatedQueryWithOptions(context.Background(), q, func(_ context.Context, database string) (dal.QueryExecutor, error) {
								if database != "orders" && database != "customers" {
									return nil, fmt.Errorf("unknown fixture database %q", database)
								}
								return orderedFixtureBackend{tables: tables}, nil
							}, dal.FederatedQueryOptions{OnProgress: func(event dal.FederatedProgress) { progress = append(progress, event) }})
						}
						if err != nil {
							t.Fatal(err)
						}
						got := orderedFixtureNormalized(t, orderedFixtureRows(t, reader))
						if mode == "federated" {
							firstDatabase := "customers" // indexed dimension first: streaming hash join
							if fixture.Name == "with-subquery" {
								firstDatabase = "orders" // recursive outer source first
							}
							if len(progress) == 0 || progress[0].Phase != "download" || progress[0].Database != firstDatabase {
								t.Fatalf("federated path progress = %#v; first database should be %s", progress, firstDatabase)
							}
						}
						if fixture.Name == "joined" {
							sort.Slice(got, func(i, j int) bool { return got[i]["CustomerId"].(float64) < got[j]["CustomerId"].(float64) })
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("rows = %#v, want %#v", got, want)
						}
					})
				}
			}
		})
	}
}
