package dtql

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// fixtureLeafExecutor deliberately supports only ordinary source scans. It
// proves that recursive evaluation never delegates a nested query to a
// provider as a native subquery or JOIN.
type fixtureLeafExecutor struct{ tables map[string][]record.Record }

func (e fixtureLeafExecutor) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q, ok := query.(dal.StructuredQuery)
	if !ok || q.From() == nil || q.From().Base() == nil || len(q.From().Joins()) != 0 {
		return nil, errors.New("fixture executor received a non-leaf query")
	}
	return dal.NewRecordsReader(e.tables[q.From().Base().Name()]), nil
}

func (fixtureLeafExecutor) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("fixture executor does not support recordsets")
}

func TestSubqueryFixturesExecuteThroughLeafScans(t *testing.T) {
	const directory = "testdata/subqueries"
	fixtureData, err := os.ReadFile(filepath.Join(directory, "suite.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite subqueryFixtureSuite
	if err := json.Unmarshal(fixtureData, &suite); err != nil {
		t.Fatal(err)
	}
	datasetData, err := os.ReadFile(filepath.Join(directory, suite.Dataset))
	if err != nil {
		t.Fatal(err)
	}
	var dataset struct {
		Tables map[string][]map[string]any `json:"tables"`
	}
	if err := json.Unmarshal(datasetData, &dataset); err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]record.Record, len(dataset.Tables))
	for name, rows := range dataset.Tables {
		for i, data := range rows {
			tables[name] = append(tables[name], record.NewRecordWithData(record.NewKeyWithID(name, i), data))
		}
	}
	executor := fixtureLeafExecutor{tables: tables}
	for _, fixture := range suite.Cases {
		if fixture.Input == "" || fixture.Rows == "" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(directory, fixture.Input))
			if err != nil {
				t.Fatal(err)
			}
			query, err := Deserialize(input)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := dal.ExecuteRecursiveQuery(context.Background(), executor, query)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			records, err := dal.ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]map[string]any, len(records))
			for i, row := range records {
				got[i] = row.Data().(map[string]any)
			}
			wantData, err := os.ReadFile(filepath.Join(directory, fixture.Rows))
			if err != nil {
				t.Fatal(err)
			}
			var want []map[string]any
			if err := json.Unmarshal(wantData, &want); err != nil {
				t.Fatal(err)
			}
			// The fixture is JSON, so normalize aggregate values such as int counts
			// through the same JSON number representation before comparing.
			normalized, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(normalized, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("rows = %#v, want %#v", got, want)
			}
		})
	}
}
