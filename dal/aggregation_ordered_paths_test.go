package dal

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/record"
)

func lastByDateQuery(from FromSource, group, key, x FieldRef) StructuredQuery {
	return from.NewQuery().GroupBy(group).SelectColumns(append([]Column{{Alias: "CustomerId", Expression: group}}, orderedLastAndFirstColumns(key, x)...)...)
}

// orderedLastAndFirstColumns select the last Total by InvoiceDate and the first Total by
// InvoiceDate descending, which are the same row when the dates differ.
func orderedLastAndFirstColumns(key, total FieldRef) []Column {
	return []Column{
		{Alias: "last_total", Expression: NewOrderedAggregate(LAST, orderedBy(Ascending(key)), total)},
		{Alias: "first_total", Expression: NewOrderedAggregate(FIRST, orderedBy(Descending(key)), total)},
	}
}

func TestOrderedAggregateTimestampKeysOnEveryInMemoryPath(t *testing.T) {
	for _, path := range timestampPaths([]string{"CustomerId", "last_total", "first_total"}, orderedLastAndFirstColumns) {
		for _, dataset := range timestampDatasets {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reversed=%v", path.name, dataset.name, reversed), func(t *testing.T) {
					rows := path.run(t, instantRecords(dataset.rows, reversed))
					if len(rows) != 1 {
						t.Fatalf("rows = %v", rows)
					}
					if rows[0]["last_total"] != dataset.want {
						t.Fatalf("last by InvoiceDate = %v, want %v", rows[0]["last_total"], dataset.want)
					}
					// first(Total ORDER BY InvoiceDate DESC) is the last row's Total too, as the keys do not tie.
					if rows[0]["first_total"] != dataset.want {
						t.Fatalf("first by InvoiceDate descending = %v, want %v", rows[0]["first_total"], dataset.want)
					}
				})
			}
		}
	}
}

func TestOrderedAggregateAnswerIsReturnedAsItIsAndBreaksTiesByInstant(t *testing.T) {
	// X is a timestamp. Every key ties, so X decides, by instant: 10:00+02:00 is 08:00Z.
	rows := []record.Record{
		joinTestRecord("T", "1", map[string]any{"k": 1, "x": time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}),
		joinTestRecord("T", "2", map[string]any{"k": 1, "x": time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("plus2", 2*3600))}),
	}
	q := From(NewRootCollectionRef("T", "")).NewQuery().SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("k")), Field("x"))},
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("k")), Field("x"))})
	reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": rows}}).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	got := readOrderedRows(t, reader)
	// The answer is returned as a plain read returns it: the text of the instant in the zone it was written.
	want := []map[string]any{{"first": "2026-01-01T10:00:00+02:00", "last": "2026-01-01T09:00:00Z"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

// structRow is a provider row held as a struct, with a timestamp by value, by
// pointer and nested.
type structRow struct {
	Group int        `json:"g"`
	At    time.Time  `json:"at"`
	Later *time.Time `json:"later"`
	Meta  struct {
		When time.Time `json:"when"`
	} `json:"meta"`
	Value int `json:"v"`
}

func TestOrderedAggregateReadsTimestampsOfStructsPointersAndNestedFields(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(500 * time.Millisecond)
	a, b := structRow{Group: 1, At: early, Later: &early, Value: 1}, structRow{Group: 1, At: late, Later: &late, Value: 2}
	a.Meta.When, b.Meta.When = early, late
	c := structRow{Group: 1, At: late, Later: nil, Value: 3}
	c.Meta.When = early
	records := []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("T", "1"), a),
		record.NewRecordWithData(record.NewKeyWithID("T", "2"), &b),
	}
	for _, key := range []string{"at", "later", "meta.when"} {
		q := From(NewRootCollectionRef("T", "")).NewQuery().GroupBy(Field("g")).SelectColumns(
			Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField(key)), Field("v"))})
		reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": records}}).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if got := readOrderedRows(t, reader); !reflect.DeepEqual(got, []map[string]any{{"last": 2.0}}) {
			t.Fatalf("key %q: rows = %v", key, got)
		}
	}
	// A nil pointer is not a timestamp: it is a NULL key, which is first.
	records = append(records, record.NewRecordWithData(record.NewKeyWithID("T", "3"), c))
	q := From(NewRootCollectionRef("T", "")).NewQuery().GroupBy(Field("g")).SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("later")), Field("v"))})
	reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": records}}).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got := readOrderedRows(t, reader); !reflect.DeepEqual(got, []map[string]any{{"first": 3.0}}) {
		t.Fatalf("rows = %v", got)
	}
}

func TestOrderedAggregateRefusesATimestampOutsideTheSupportedYears(t *testing.T) {
	const want = "an aggregate order key holds a timestamp outside the supported years"
	// The last one is a valid JSON timestamp, in a zone where it is still the year 9999,
	// whose instant is in the year 10000.
	for _, instant := range []time.Time{
		time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("minus5", -5*3600)),
	} {
		rows := []record.Record{joinTestRecord("Invoice", "1", map[string]any{"CustomerId": 1, "InvoiceDate": instant, "Total": 1})}
		backend := &orderedStub{rows: map[string][]record.Record{"Invoice": rows, "Customer": customerRecords()}}
		one := lastByDateQuery(From(NewRootCollectionRef("Invoice", "")), Field("CustomerId"), Field("InvoiceDate"), Field("Total"))
		joined := lastByDateQuery(From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))),
			NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
		for name, q := range map[string]StructuredQuery{"one source": one, "join": joined} {
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
			if err == nil {
				_, err = ReadAllToRecords(context.Background(), reader)
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s, %v: error = %v", name, instant, err)
			}
		}
	}
	// The streaming strategy and the federated stream meet the year too.
	rows := []record.Record{joinTestRecord("Invoice", "1", map[string]any{"CustomerId": 1, "InvoiceDate": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), "Total": 1})}
	streaming := &orderedStub{caps: QueryCapabilities{OrderBy: true, GroupKeyOrder: true}, rows: map[string][]record.Record{"Invoice": rows}}
	one := lastByDateQuery(From(NewRootCollectionRef("Invoice", "")), Field("CustomerId"), Field("InvoiceDate"), Field("Total"))
	reader, err := NewDB(streaming).ExecuteQueryToRecordsReader(context.Background(), one)
	if err == nil {
		_, err = ReadAllToRecords(context.Background(), reader)
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("streaming: error = %v", err)
	}
	stream := lastByDateQuery(From(NewDatabaseCollectionRef("orders", "", "Invoice", "i")).Join(NewJoinedSource(NewDatabaseCollectionRef("customers", "", "Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))),
		NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
	reader, err = ExecuteFederatedQuery(context.Background(), stream, func(_ context.Context, database string) (QueryExecutor, error) {
		if database == "orders" {
			return federatedStub{rows: rows}, nil
		}
		return federatedStub{rows: customerRecords()}, nil
	})
	if err == nil {
		_, err = ReadAllToRecords(context.Background(), reader)
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("federated stream: error = %v", err)
	}
}

// A join over 10,001 rows with an ordered aggregate, computed by DALgo's engine,
// meets the bound of the generic engine.
func TestOrderedAggregateOverTenThousandAndOneJoinedRowsMeetsTheExistingBound(t *testing.T) {
	rows := make([]record.Record, maxJoinRows+1)
	for i := range rows {
		rows[i] = joinTestRecord("Invoice", fmt.Sprint(i), map[string]any{"CustomerId": 1, "InvoiceDate": time.Date(2026, 1, 1, 0, 0, 0, i, time.UTC), "Total": i})
	}
	q := lastByDateQuery(From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))),
		NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
	stub := &orderedStub{rows: map[string][]record.Record{"Invoice": rows, "Customer": customerRecords()}}
	_, err := NewDB(stub).ExecuteQueryToRecordsReader(context.Background(), q)
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || !strings.Contains(err.Error(), "relation scan exceeds row or byte bound") {
		t.Fatalf("error = %v", err)
	}
}

func TestOrderedAggregateSortValuesAreChargedToTheJoinBudgetOnlyWhenTheQueryNeedsThem(t *testing.T) {
	invoices := instantRecords(timestampDatasets[0].rows, false)
	backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoices, "Customer": customerRecords()}}
	run := func(aggregate AggregateFunc) *joinExecution {
		from := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id")))
		q := from.NewQuery().GroupBy(NewFieldRef("c", "Id")).SelectColumns(
			Column{Alias: "CustomerId", Expression: NewFieldRef("c", "Id")},
			Column{Alias: "v", Expression: aggregate})
		e := &joinExecution{ctx: context.Background(), q: q, executor: backend, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}, fields: map[string][]string{}, keyRefs: map[string][]joinKeyReference{}}
		reader, err := e.execute()
		if err != nil {
			t.Fatal(err)
		}
		readOrderedRows(t, reader)
		return e
	}
	total := NewFieldRef("i", "Total")
	plain := run(NewAggregate(LAST, false, total))
	ordered := run(NewOrderedAggregate(LAST, orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), total))

	// A query without an ordered aggregate carries no sort value and charges the bytes it charged before:
	// the golden figure was measured on the release before this change.
	for _, scanned := range plain.scans["i"] {
		if _, ok := scanned.data[sortValuesKey]; ok {
			t.Fatalf("a row of a query without an ordered aggregate carries sort values: %v", scanned.data)
		}
	}
	const bytesBefore = 1267
	if plain.bytes != bytesBefore {
		t.Fatalf("bytes charged = %d, want %d", plain.bytes, bytesBefore)
	}
	// The sort values count towards the 16 MiB of the join engine.
	for _, scanned := range ordered.scans["i"] {
		if _, ok := scanned.data[sortValuesKey]; !ok {
			t.Fatalf("a row of a query with an ordered aggregate carries no sort values: %v", scanned.data)
		}
	}
	if _, ok := ordered.scans["c"][0].data[sortValuesKey]; ok {
		t.Fatal("a source whose fields no ordered aggregate reads carries sort values")
	}
	if ordered.bytes <= plain.bytes {
		t.Fatalf("bytes charged with sort values = %d, without = %d", ordered.bytes, plain.bytes)
	}
}

// A query without an ordered aggregate returns the rows it returned before ordered
// aggregates were added, on every path, whatever the order the rows arrive in. The
// expected rows were measured on the release before the change.
func TestQueriesWithoutAnOrderedAggregateReturnTheRowsTheyDid(t *testing.T) {
	columns := func(key, total FieldRef) []Column {
		return []Column{
			{Alias: "min_at", Expression: NewAggregate(MIN, false, key)},
			{Alias: "max_at", Expression: NewAggregate(MAX, false, key)},
			{Alias: "sum_total", Expression: NewAggregate(SUM, false, total)},
		}
	}
	want := map[string][]map[string]any{
		"whole and fractional seconds": {{"CustomerId": 1.0, "min_at": "2026-01-01T00:00:00.5Z", "max_at": "2026-01-01T00:00:00Z", "sum_total": 30.0}},
		"two zone offsets":             {{"CustomerId": 1.0, "min_at": "2026-01-01T09:00:00Z", "max_at": "2026-01-01T10:00:00+02:00", "sum_total": 12.0}},
	}
	for _, path := range timestampPaths([]string{"CustomerId", "min_at", "max_at", "sum_total"}, columns) {
		for _, dataset := range timestampDatasets {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reversed=%v", path.name, dataset.name, reversed), func(t *testing.T) {
					if got := path.run(t, instantRecords(dataset.rows, reversed)); !reflect.DeepEqual(got, want[dataset.name]) {
						t.Fatalf("rows = %v, want %v", got, want[dataset.name])
					}
				})
			}
		}
	}
}
