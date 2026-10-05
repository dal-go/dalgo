package dal

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	plus2 := time.FixedZone("plus2", 2*3600)
	// X is a timestamp and every key ties, so X decides, by instant; rows of one instant are
	// told apart by their text, so the answer does not depend on the order rows arrive in.
	// Arrays and objects that DALgo prints alike are told apart by their JSON encoding.
	for name, tc := range map[string]struct {
		x           []any
		first, last any
	}{
		// 10:00+02:00 is 08:00Z, which is earlier than 09:00Z.
		"two instants": {
			[]any{time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 10, 0, 0, 0, plus2)},
			"2026-01-01T10:00:00+02:00", "2026-01-01T09:00:00Z"},
		"one instant written two ways": {
			[]any{time.Date(2026, 1, 1, 10, 0, 0, 0, plus2), time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)},
			"2026-01-01T08:00:00Z", "2026-01-01T10:00:00+02:00"},
		// Both print as [a b]; the encodings are ["a b"] and ["a","b"].
		"two arrays that print alike": {
			[]any{[]any{"a b"}, []any{"a", "b"}},
			[]any{"a b"}, []any{"a", "b"}},
		// Both print as map[a:1]; the encodings are {"a":"1"} and {"a":1}.
		"two objects that print alike": {
			[]any{map[string]any{"a": "1"}, map[string]any{"a": 1}},
			map[string]any{"a": "1"}, map[string]any{"a": 1.0}},
	} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reversed=%v", name, reversed), func(t *testing.T) {
				rows := make([]record.Record, len(tc.x))
				for i, x := range tc.x {
					rows[i] = joinTestRecord("T", fmt.Sprint(i), map[string]any{"k": 1, "x": x})
				}
				if reversed {
					rows[0], rows[1] = rows[1], rows[0]
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
				want := []map[string]any{{"first": tc.first, "last": tc.last}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("rows = %v, want %v", got, want)
				}
			})
		}
	}
}

// A field a provider's row holds under the reserved name is removed from the row. A row a
// store with free field names returns may hold one; a query that holds an ordered aggregate
// does not read it, on every in-memory path, and a query that holds none returns the field
// as it always did.
func TestOrderedAggregateRemovesAFieldOfTheReservedNameFromAProvidersRow(t *testing.T) {
	forged := func(id, customer, date, total int, forgedDate any) record.Record {
		return joinTestRecord("Invoice", fmt.Sprint(id), map[string]any{
			"CustomerId": customer, "InvoiceDate": date, "Total": total,
			sortValuesKey: map[string]any{"InvoiceDate": forgedDate, "Total": forgedDate},
		})
	}
	// Left alone, the first row would sort last: its forged date is the largest of the group.
	rows := []record.Record{forged(1, 1, 1, 10, 99), forged(2, 1, 2, 20, nil)}
	for _, path := range timestampPaths([]string{"CustomerId", "last_total", "first_total"}, orderedLastAndFirstColumns) {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reversed=%v", path.name, reversed), func(t *testing.T) {
				input := append([]record.Record(nil), rows...)
				if reversed {
					input[0], input[1] = input[1], input[0]
				}
				got := path.run(t, input)
				if len(got) != 1 || got[0]["last_total"] != 20.0 || got[0]["first_total"] != 20.0 {
					t.Fatalf("rows = %v, want the answers of the plain dates: last 20, first by date descending 20", got)
				}
			})
		}
	}
}

// A query that holds no ordered aggregate returns a field of the reserved name from a generic join as it
// returns any other field.
func TestJoinWithoutOrderedAggregateReturnsAFieldOfTheReservedName(t *testing.T) {
	invoice, customer := NewRootCollectionRef("Invoice", "i"), NewRootCollectionRef("Customer", "c")
	q := From(invoice).Join(NewJoinedSource(customer, JoinInner, joinOn("i", "CustomerId", "c", "Id"))).NewQuery().SelectIntoRecord(nil)
	row := joinTestRecord("Invoice", "1", map[string]any{"CustomerId": 1, "Total": 10, sortValuesKey: "mine"})
	stub := &orderedStub{rows: map[string][]record.Record{"Invoice": {row}, "Customer": customerRecords()}}
	reader, err := NewDB(stub).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	records, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %v, err = %v", records, err)
	}
	if got := records[0].Data().(map[string]any)[sortValuesKey]; got != "mine" {
		t.Fatalf("the field of the reserved name = %v, want it returned as it always was", got)
	}
}

// The last step of the comparison tells apart answers the engine's comparison ties.
func TestCompareOrderedTuplesTellsApartAnswersThatCompareEqual(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	for name, tc := range map[string]struct {
		a, b any
		want int
	}{
		"-0 before 0":                          {negativeZero, 0.0, -1},
		"0 after -0":                           {0.0, negativeZero, 1},
		"the same array":                       {[]any{"a"}, []any{"a"}, 0},
		"an array that prints like another":    {[]any{"a b"}, []any{"a", "b"}, -1},
		"the other way round":                  {[]any{"a", "b"}, []any{"a b"}, 1},
		"an object keyed in a different order": {map[string]any{"a": 1.0, "b": 2.0}, map[string]any{"b": 2.0, "a": 1.0}, 0},
		// NaN cannot be encoded; its printed form and type tell it apart from a value that can.
		"a value that cannot be encoded":      {math.NaN(), 1.0, 1},
		"a value that cannot be encoded, too": {1.0, math.NaN(), -1},
		"nothing and nothing":                 {nil, nil, 0},
	} {
		t.Run(name, func(t *testing.T) {
			got := compareOrderedTuples(nil, nil, nil, tc.a, nil, nil, tc.b)
			if got != tc.want {
				t.Fatalf("compareOrderedTuples = %d, want %d", got, tc.want)
			}
		})
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

// embeddedStamps is embedded in a row, so encoding/json writes its fields as fields of the row.
type embeddedStamps struct {
	Created time.Time `json:"created"`
}

// embeddedRow is a provider row held as a struct with an embedded struct, by value.
type embeddedRow struct {
	embeddedStamps
	Group int `json:"g"`
	Value int `json:"v"`
}

func TestOrderedAggregateReadsTimestampsOfAnEmbeddedStruct(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(500 * time.Millisecond)
	records := []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("T", "1"), embeddedRow{embeddedStamps{early}, 1, 1}),
		record.NewRecordWithData(record.NewKeyWithID("T", "2"), &embeddedRow{embeddedStamps{late}, 1, 2}),
	}
	q := From(NewRootCollectionRef("T", "")).NewQuery().GroupBy(Field("g")).SelectColumns(
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("created")), Field("v"))})
	reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": records}}).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	// The later instant is the one written with a fraction, which text puts first.
	if got := readOrderedRows(t, reader); !reflect.DeepEqual(got, []map[string]any{{"last": 2.0}}) {
		t.Fatalf("rows = %v", got)
	}
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
// aggregates were added, on every path, whatever the order the rows arrive in: the
// text of the plain read, and not a sort value. The expected rows were measured on
// the release before the change.
func TestQueriesWithoutAnOrderedAggregateReturnTheRowsTheyDid(t *testing.T) {
	columns := func(key, total FieldRef) []Column {
		return []Column{
			{Alias: "min_at", Expression: NewAggregate(MIN, false, key)},
			{Alias: "max_at", Expression: NewAggregate(MAX, false, key)},
			{Alias: "sum_total", Expression: NewAggregate(SUM, false, total)},
		}
	}
	rows := []instantRow{
		{1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 10},
		{1, time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), 20},
	}
	want := []map[string]any{{"CustomerId": 1.0, "min_at": "2026-01-01T00:00:00Z", "max_at": "2026-01-01T00:00:01Z", "sum_total": 30.0}}
	for _, path := range timestampPaths([]string{"CustomerId", "min_at", "max_at", "sum_total"}, columns) {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reversed=%v", path.name, reversed), func(t *testing.T) {
				if got := path.run(t, instantRecords(rows, reversed)); !reflect.DeepEqual(got, want) {
					t.Fatalf("rows = %v, want %v", got, want)
				}
			})
		}
	}
}

// A source that gives no schema metadata has its field names read from its first
// row. The sort values a row carries are not fields of the source, so no name read
// that way is the reserved key, and the rows the stream hands on still carry them.
func TestFederatedStreamDoesNotNameTheReservedKeyAsAFieldOfASource(t *testing.T) {
	invoices := instantRecords(timestampDatasets[0].rows, false)
	q := lastByDateQuery(From(NewDatabaseCollectionRef("orders", "", "Invoice", "i")).Join(NewJoinedSource(NewDatabaseCollectionRef("customers", "", "Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))),
		NewFieldRef("c", "Id"), NewFieldRef("i", "InvoiceDate"), NewFieldRef("i", "Total"))
	routed := federatedQueryExecutor{resolve: func(_ context.Context, database string) (QueryExecutor, error) {
		if database == "orders" {
			return federatedStub{rows: invoices}, nil
		}
		return federatedStub{rows: customerRecords()}, nil
	}}
	stream, err := newFederatedJoinStream(context.Background(), q, routed, FederatedQueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	row, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	names := stream.execution.fields["i"]
	if want := []string{"CustomerId", "InvoiceDate", "Total"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("fields of the streamed source = %v, want %v", names, want)
	}
	sources := row.Data().(map[string]any)[joinSourcesKey].(map[string]any)
	if values, ok := sources["i"].(map[string]any)[sortValuesKey].(map[string]any); !ok || values["InvoiceDate"] == nil {
		t.Fatalf("the streamed row carries no sort values: %v", sources["i"])
	}
}

// A query DALgo computes after the provider refused it as not supported meets the
// bounds of DALgo's engine, and the caller is told which bound, not the refusal.
func TestOrderedAggregateBoundsHoldOnThePathAfterARefusal(t *testing.T) {
	ctx := context.Background()
	refusal := fmt.Errorf("a column type with no server form: %w", ErrNotSupported)
	rowsOf := func(n int) []record.Record {
		rows := make([]record.Record, n)
		for i := range rows {
			rows[i] = record.NewRecordWithData(record.NewKeyWithID("Invoice", i), map[string]any{"CustomerId": i, "InvoiceDate": i, "Total": i})
		}
		return rows
	}
	byDate := orderedBy(AscendingField("InvoiceDate"))

	t.Run("one source: more state than the engine keeps", func(t *testing.T) {
		stub := &orderedStub{caps: orderedCapabilities, nativeErr: refusal, rows: map[string][]record.Record{"Invoice": rowsOf(defaultMaxAggregationGroups + 1)}}
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, firstAndLast(byDate))
		if err == nil {
			_, err = ReadAllToRecords(ctx, reader)
		}
		if err == nil || errors.Is(err, ErrNotSupported) || !strings.Contains(err.Error(), "retained aggregation byte limit") || stub.native != 1 || stub.plain != 1 {
			t.Fatalf("err = %v, native = %d, plain = %d", err, stub.native, stub.plain)
		}
	})

	joined := func() (*orderedJoinStub, StructuredQuery) {
		stub := &orderedJoinStub{orderedStub: &orderedStub{caps: orderedCapabilities, nativeErr: refusal,
			rows: map[string][]record.Record{"Invoice": rowsOf(maxJoinRows + 1), "Customer": customerRecords()}}}
		return stub, joinedInvoiceQuery(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))))
	}
	wantBound := func(t *testing.T, err error) {
		t.Helper()
		var diagnostic *JoinValidationError
		if !errors.As(err, &diagnostic) || errors.Is(err, ErrNotSupported) || !strings.Contains(err.Error(), "relation scan exceeds row or byte bound") {
			t.Fatalf("error = %v", err)
		}
	}
	t.Run("a join through the records reader: more rows than the join engine reads", func(t *testing.T) {
		stub, q := joined()
		_, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		wantBound(t, err)
		if stub.native != 1 {
			t.Fatalf("native = %d", stub.native)
		}
	})
	t.Run("a join through the recordset reader: more rows than the join engine reads", func(t *testing.T) {
		stub, q := joined()
		_, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		wantBound(t, err)
		if stub.nativeSets != 1 {
			t.Fatalf("nativeSets = %d", stub.nativeSets)
		}
	})
}
