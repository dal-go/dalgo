package dalgo2memory

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

// ExecuteQueryToRecordsetReader executes a structured query and exposes its
// result as a columnar recordset — the read path DataTug uses for tabular
// query results. It reuses the records pipeline (WHERE, GROUP BY, HAVING,
// column projection, joins, ORDER BY, LIMIT/OFFSET all already applied there),
// then pivots the resulting rows into columns.
//
// Columns are derived from the query's explicit SELECT columns when present
// (projection and GROUP BY aggregation); otherwise from the sorted union of
// keys across the result rows (e.g. SELECT *). Values are stored in untyped
// (any) columns, matching the adapter's schemaless JSON storage. A non-
// structured query is not supported.
func (s session) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	q, ok := query.(dal.StructuredQuery)
	if !ok {
		return nil, dal.ErrNotSupported
	}
	reader, err := s.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil {
		return nil, err
	}
	return buildRecordsetReader(ctx, q, reader, options...)
}

// buildRecordsetReader drains a RecordsReader and pivots its rows into a
// columnar recordset reader. Split out so the read/convert error paths are
// directly testable with an injected reader.
func buildRecordsetReader(ctx context.Context, q dal.StructuredQuery, reader dal.RecordsReader, options ...recordset.Option) (dal.RecordsetReader, error) {
	records, err := dal.ReadAllToRecords(ctx, reader)
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]any, len(records))
	for i, rec := range records {
		m, err := recordToMap(rec)
		if err != nil {
			return nil, err
		}
		rows[i] = m
	}

	colNames := recordsetColumnNames(q, rows)
	cols := make([]recordset.Column[any], len(colNames))
	for i, name := range colNames {
		cols[i] = inferColumn(name, rows)
	}
	rs := recordset.NewColumnarRecordset(recordsetName(q, options...), cols...)
	for _, m := range rows {
		row := rs.NewRow()
		for _, name := range colNames {
			if v, ok := m[name]; ok {
				// The column exists and is not computed; a typed column only
				// exists when every row holds that exact type (see inferColumn),
				// and an any-column accepts anything — so SetValue cannot fail.
				_ = row.SetValueByName(name, v, rs)
			}
		}
	}
	return &columnarReader{rs: rs}, nil
}

// inferColumn picks a column type for a named field from its values across all
// rows. It returns a typed column (float64/string/bool/int) only when every row
// holds a present, non-nil value of one consistent kind; anything nullable,
// absent, mixed, or of another kind falls back to an untyped (any) column so
// nulls stay representable. The adapter is schemaless, so the type is inferred
// from values rather than a declared schema.
func inferColumn(name string, rows []map[string]any) recordset.Column[any] {
	var t reflect.Type
	for _, m := range rows {
		v, ok := m[name]
		if !ok || v == nil {
			return recordset.NewTypedColumn[any](name, nil)
		}
		vt := reflect.TypeOf(v)
		if t == nil {
			t = vt
		} else if t != vt {
			return recordset.NewTypedColumn[any](name, nil)
		}
	}
	switch {
	case t == nil: // no rows
		return recordset.NewTypedColumn[any](name, nil)
	case t.Kind() == reflect.Float64:
		return recordset.NewColumn[float64](name, 0)
	case t.Kind() == reflect.String:
		return recordset.NewColumn[string](name, "")
	case t.Kind() == reflect.Bool:
		return recordset.NewColumn[bool](name, false)
	case t.Kind() == reflect.Int:
		return recordset.NewColumn[int](name, 0)
	default:
		return recordset.NewTypedColumn[any](name, nil)
	}
}

// recordToMap renders a result record's data as a map keyed by column name.
// Projected/aggregated records already carry a map; a keys-only record exposes
// its key under "ID"; any other shape (e.g. a typed struct from a SELECT * with
// IntoRecord) is converted via a JSON round-trip.
func recordToMap(rec dal.Record) (map[string]any, error) {
	switch d := rec.Data().(type) {
	case map[string]any:
		return d, nil
	case *map[string]any:
		if d == nil {
			return map[string]any{}, nil
		}
		return *d, nil
	case nil:
		return map[string]any{"ID": fmt.Sprint(rec.Key().ID)}, nil
	default:
		b, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		if m == nil {
			m = map[string]any{}
		}
		return m, nil
	}
}

// recordsetColumnNames returns the ordered column names: the query's explicit
// SELECT columns when present, otherwise the sorted union of keys across rows.
func recordsetColumnNames(q dal.StructuredQuery, rows []map[string]any) []string {
	if cols := q.Columns(); len(cols) > 0 {
		names := make([]string, len(cols))
		for i, c := range cols {
			names[i] = columnOutKey(c)
		}
		return names
	}
	seen := make(map[string]bool)
	for _, m := range rows {
		for k := range m {
			seen[k] = true
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// recordsetName resolves the recordset name from the WithName option, falling
// back to the query's base recordset name.
func recordsetName(q dal.StructuredQuery, options ...recordset.Option) string {
	if name := recordset.NewOptions(options...).Name(); name != "" {
		return name
	}
	return q.From().Base().Name()
}

// columnarReader is a dal.RecordsetReader over a fully-built ColumnarRecordset:
// the recordset is populated up front and Next walks its rows in order.
type columnarReader struct {
	rs  *recordset.ColumnarRecordset
	pos int
}

var _ dal.RecordsetReader = (*columnarReader)(nil)

func (r *columnarReader) Recordset() recordset.Recordset { return r.rs }

func (r *columnarReader) Next() (recordset.Row, recordset.Recordset, error) {
	if r.pos >= r.rs.RowsCount() {
		return nil, r.rs, dal.ErrNoMoreRecords
	}
	row := r.rs.GetRow(r.pos)
	r.pos++
	return row, r.rs, nil
}

func (r *columnarReader) Cursor() (string, error) { return "", nil }

func (r *columnarReader) Close() error { return nil }
