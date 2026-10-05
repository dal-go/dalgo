package dal

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// orderedBy lists the keys of an aggregate's order.
func orderedBy(keys ...OrderExpression) []OrderExpression { return keys }

// orderedStub is a provider for the ordered aggregate tests. It serves plain rows,
// answers a native (aggregate) read as it is told to, and counts what it is asked.
type orderedStub struct {
	Backend
	rows       map[string][]record.Record
	caps       QueryCapabilities
	nativeErr  error
	nativeRows []record.Record
	plainErr   error
	native     int
	plain      int
	nativeSets int
	plainReads []StructuredQuery
}

func (b *orderedStub) QueryCapabilities() QueryCapabilities { return b.caps }

func (b *orderedStub) ExecuteQueryToRecordsReader(_ context.Context, q Query) (RecordsReader, error) {
	structured := q.(StructuredQuery)
	if HasAggregation(structured) {
		b.native++
		if b.nativeErr != nil {
			return nil, b.nativeErr
		}
		return NewRecordsReader(b.nativeRows), nil
	}
	b.plain++
	b.plainReads = append(b.plainReads, structured)
	if b.plainErr != nil {
		return nil, b.plainErr
	}
	return NewRecordsReader(b.rows[structured.From().Base().Name()]), nil
}

func (b *orderedStub) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	b.nativeSets++
	if b.nativeErr != nil {
		return nil, b.nativeErr
	}
	return &aggregationRecordsetReader{recordset: recordset.NewColumnarRecordset("native")}, nil
}

// orderedJoinStub also accepts or declines a join, as a provider with a native join does.
type orderedJoinStub struct {
	*orderedStub
	decline  error
	accepted int
}

func (b *orderedJoinStub) CanExecuteJoin(context.Context, StructuredQuery) error {
	b.accepted++
	return b.decline
}

// orderedCapabilities are those of a provider that runs grouping and ordered aggregates natively.
var orderedCapabilities = QueryCapabilities{GroupBy: true, Having: true, OrderBy: true, Aggregate: AggregateCapabilities{First: true, Last: true, OrderBy: true}}

// invoice is a row of the Invoice table of the ordered aggregate tests.
type invoice struct {
	id, customer int
	date, total  any
}

func (i invoice) record() record.Record {
	return joinTestRecord("Invoice", strconv.Itoa(i.id), map[string]any{"InvoiceId": i.id, "CustomerId": i.customer, "InvoiceDate": i.date, "Total": i.total})
}

// workedInvoices is the table of the worked example.
var workedInvoices = []invoice{
	{10, 1, "2024-01-01", 100},
	{11, 1, "2024-01-02", nil},
	{13, 1, "2024-01-02", 50},
	{12, 2, "2024-01-03", 200},
	{14, 2, nil, 75},
}

func invoiceRecords(invoices []invoice, reversed bool) []record.Record {
	records := make([]record.Record, len(invoices))
	for i, row := range invoices {
		records[i] = row.record()
	}
	if reversed {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
	}
	return records
}

// readOrderedRows reads a result and fails when a row holds a reserved key.
func readOrderedRows(t *testing.T, reader RecordsReader) []map[string]any {
	t.Helper()
	records, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]any, len(records))
	for i, rec := range records {
		rows[i] = rec.Data().(map[string]any)
		for name := range rows[i] {
			if strings.HasPrefix(name, "\x00") {
				t.Fatalf("a returned row holds the reserved key %q", name)
			}
		}
	}
	return rows
}

// readOrderedRecordset reads a recordset result into rows.
func readOrderedRecordset(t *testing.T, reader RecordsetReader, columns ...string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for {
		row, rs, err := reader.Next()
		if err == ErrNoMoreRecords {
			return rows
		}
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]any{}
		for _, column := range columns {
			value, err := row.GetValueByName(column, rs)
			if err != nil {
				t.Fatal(err)
			}
			values[column] = value
		}
		rows = append(rows, values)
	}
}

func sortedByCustomer(rows []map[string]any) []map[string]any {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i]["CustomerId"].(float64) < rows[j]["CustomerId"].(float64) })
	return rows
}

// instantRow is a row of Invoice whose date is a timestamp the provider returns
// as a time.Time.
type instantRow struct {
	customer int
	at       time.Time
	total    int
}

func instantRecords(rows []instantRow, reversed bool) []record.Record {
	records := make([]record.Record, len(rows))
	for i, row := range rows {
		records[i] = joinTestRecord("Invoice", fmt.Sprint(i), map[string]any{"CustomerId": row.customer, "InvoiceDate": row.at, "Total": row.total})
	}
	if reversed {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
	}
	return records
}

func customerRecords() []record.Record {
	return []record.Record{joinTestRecord("Customer", "1", map[string]any{"Id": 1, "Name": "Ann"})}
}

// timestampDataset is a table of timestamps the provider returns as time.Time.
type timestampDataset struct {
	name string
	rows []instantRow
	want float64 // the last Total by InvoiceDate
}

var timestampDatasets = []timestampDataset{
	{
		name: "whole and fractional seconds",
		rows: []instantRow{
			{1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 10},
			{1, time.Date(2026, 1, 1, 0, 0, 0, 500_000_000, time.UTC), 20},
		},
		want: 20,
	},
	{
		// 10:00+02:00 is 08:00Z, which is earlier than 09:00Z.
		name: "two zone offsets",
		rows: []instantRow{
			{1, time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("plus2", 2*3600)), 5},
			{1, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), 7},
		},
		want: 7,
	},
}

// timestampPath runs one in-memory path over the rows of Invoice and returns its result.
type timestampPath struct {
	name string
	run  func(t *testing.T, invoices []record.Record) []map[string]any
}

// timestampPaths lists the in-memory paths a query can take. The query each one runs
// groups the rows of Invoice by customer and selects CustomerId and the columns that
// columns builds from the key (InvoiceDate) and the argument (Total) of the path;
// names are the output columns a recordset result is read by.
func timestampPaths(names []string, columns func(key, total FieldRef) []Column) []timestampPath {
	ctx := context.Background()
	invoice := NewRootCollectionRef("Invoice", "i")
	customer := NewRootCollectionRef("Customer", "c")
	onCustomer := joinOn("i", "CustomerId", "c", "Id")
	dbInvoice := NewDatabaseCollectionRef("orders", "", "Invoice", "i")
	dbCustomer := NewDatabaseCollectionRef("customers", "", "Customer", "c")
	build := func(from FromSource, group, key, total FieldRef) StructuredQuery {
		return from.NewQuery().GroupBy(group).SelectColumns(append([]Column{{Alias: "CustomerId", Expression: group}}, columns(key, total)...)...)
	}
	read := func(t *testing.T, reader RecordsReader, err error) []map[string]any {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return readOrderedRows(t, reader)
	}
	federated := func(t *testing.T, q StructuredQuery, invoices []record.Record) []map[string]any {
		reader, err := ExecuteFederatedQuery(ctx, q, func(_ context.Context, database string) (QueryExecutor, error) {
			if database == "orders" {
				return federatedStub{rows: invoices}, nil
			}
			return federatedStub{rows: customerRecords()}, nil
		})
		return read(t, reader, err)
	}
	oneSource := func(caps QueryCapabilities) func(*testing.T, []record.Record) []map[string]any {
		return func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(NewRootCollectionRef("Invoice", "")), Field("CustomerId"), Field("InvoiceDate"), Field("Total"))
			backend := &orderedStub{caps: caps, rows: map[string][]record.Record{"Invoice": invoices}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(ctx, q)
			return read(t, reader, err)
		}
	}
	joined := func(base, other RecordsetSource) func(*testing.T, []record.Record) []map[string]any {
		return func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(base).Join(NewJoinedSource(other, JoinInner, onCustomer)), NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
			backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoices, "Customer": customerRecords()}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(ctx, q)
			return read(t, reader, err)
		}
	}
	stream := func(base, other CollectionRef) func(*testing.T, []record.Record) []map[string]any {
		return func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(base).Join(NewJoinedSource(other, JoinInner, onCustomer)), NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
			if !canStreamFederatedAggregate(q) {
				t.Fatal("this is not the stream")
			}
			return federated(t, q, invoices)
		}
	}
	return []timestampPath{
		{"one source, hash", oneSource(QueryCapabilities{})},
		{"one source, streaming", oneSource(QueryCapabilities{OrderBy: true, GroupKeyOrder: true})},
		{"one source, recordset reader", func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(NewRootCollectionRef("Invoice", "")), Field("CustomerId"), Field("InvoiceDate"), Field("Total"))
			backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoices}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsetReader(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			return readOrderedRecordset(t, reader, names...)
		}},
		{"federated, one source (recursive executor)", func(t *testing.T, invoices []record.Record) []map[string]any {
			return federated(t, build(From(dbInvoice), Field("CustomerId"), Field("InvoiceDate"), Field("Total")), invoices)
		}},
		{"federated, flat hash join (stream), key on the streamed source", stream(dbInvoice, dbCustomer)},
		{"federated, flat hash join (stream), key on the joined source", func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(dbCustomer).Join(NewJoinedSource(dbInvoice, JoinInner, onCustomer)), NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
			if !canStreamFederatedAggregate(q) {
				t.Fatal("this is not the stream")
			}
			return federated(t, q, invoices)
		}},
		{"generic join, key on the base source", joined(invoice, customer)},
		{"generic join, key on the joined source", joined(customer, invoice)},
		{"generic join, unqualified fields of the base source", func(t *testing.T, invoices []record.Record) []map[string]any {
			q := build(From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)), NewFieldRef("c", "Id"), Field("InvoiceDate"), Field("Total"))
			backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoices, "Customer": customerRecords()}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(ctx, q)
			return read(t, reader, err)
		}},
		{"subquery in where", func(t *testing.T, invoices []record.Record) []map[string]any {
			exists := From(customer).NewQuery().Where(NewComparison(NewFieldRef("c", "Id"), Equal, NewFieldRef("i", "CustomerId"))).
				SelectColumns(Column{Expression: NewFieldRef("c", "Id")})
			q := From(invoice).NewQuery().Where(NewExistsCondition(exists)).GroupBy(NewFieldRef("i", "CustomerId")).SelectColumns(
				append([]Column{{Alias: "CustomerId", Expression: NewFieldRef("i", "CustomerId")}}, columns(NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))...)...)
			backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoices, "Customer": customerRecords()}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(ctx, q)
			return read(t, reader, err)
		}},
	}
}
