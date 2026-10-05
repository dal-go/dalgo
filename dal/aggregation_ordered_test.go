package dal

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/dal-go/record"
)

func invoiceQuery(columns ...Column) StructuredQuery {
	return From(NewRootCollectionRef("Invoice", "")).NewQuery().GroupBy(Field("CustomerId")).
		SelectColumns(append([]Column{{Expression: Field("CustomerId")}}, columns...)...)
}

func firstAndLast(order []OrderExpression) StructuredQuery {
	return invoiceQuery(
		Column{Alias: "first_total", Expression: NewOrderedAggregate(FIRST, order, Field("Total"))},
		Column{Alias: "last_total", Expression: NewOrderedAggregate(LAST, order, Field("Total"))})
}

func TestOrderedAggregateWorkedExampleOnTheHashAndStreamingStrategies(t *testing.T) {
	byDate := orderedBy(AscendingField("InvoiceDate"))
	byDateThenIdDesc := orderedBy(AscendingField("InvoiceDate"), DescendingField("InvoiceId"))
	for name, tc := range map[string]struct {
		query StructuredQuery
		want  []map[string]any
	}{
		// Customer 1 has two invoices on 2024-01-02: the tie is broken by Total, NULL before 50.
		// Customer 2's invoice 14 has no date, so it sorts first and its Total is the first.
		"by date": {firstAndLast(byDate), []map[string]any{
			{"CustomerId": 1.0, "first_total": 100.0, "last_total": 50.0},
			{"CustomerId": 2.0, "first_total": 75.0, "last_total": 200.0}}},
		// With the invoice id descending, the last invoice of customer 1 is invoice 11, whose Total is NULL.
		"by date then id descending": {firstAndLast(byDateThenIdDesc), []map[string]any{
			{"CustomerId": 1.0, "first_total": 100.0, "last_total": nil},
			{"CustomerId": 2.0, "first_total": 75.0, "last_total": 200.0}}},
	} {
		for strategy, capabilities := range map[string]QueryCapabilities{
			"hash":      {},
			"streaming": {OrderBy: true, GroupKeyOrder: true},
		} {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reversed=%v", name, strategy, reversed), func(t *testing.T) {
					plan, err := PlanAggregation(tc.query, capabilities)
					if err != nil || (strategy == "hash") != (plan.Strategy == AggregationHash) || (strategy == "streaming") != (plan.Strategy == AggregationStreaming) {
						t.Fatalf("plan = %#v, err = %v", plan, err)
					}
					backend := &orderedStub{caps: capabilities, rows: map[string][]record.Record{"Invoice": invoiceRecords(workedInvoices, reversed)}}
					reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), tc.query)
					if err != nil {
						t.Fatal(err)
					}
					if rows := sortedByCustomer(readOrderedRows(t, reader)); !reflect.DeepEqual(rows, tc.want) {
						t.Fatalf("rows = %v, want %v", rows, tc.want)
					}
				})
			}
		}
	}
}

func TestOrderedAggregateOnTheWholeResultAndOnAnEmptyOne(t *testing.T) {
	byDate := orderedBy(AscendingField("InvoiceDate"))
	whole := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "first_total", Expression: NewOrderedAggregate(FIRST, byDate, Field("Total"))},
		Column{Alias: "last_total", Expression: NewOrderedAggregate(LAST, byDate, Field("Total"))})
	run := func(q StructuredQuery, invoices []invoice) []map[string]any {
		backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoiceRecords(invoices, false)}}
		reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		return readOrderedRows(t, reader)
	}
	// The whole table is one group: the invoice with no date comes first, and the last date is 2024-01-03.
	if got, want := run(whole, workedInvoices), []map[string]any{{"first_total": 75.0, "last_total": 200.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	// No rows and no grouping is one row of NULL; with a grouping it is no row.
	if got, want := run(whole, nil), []map[string]any{{"first_total": nil, "last_total": nil}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty rows = %v, want %v", got, want)
	}
	if got := run(firstAndLast(byDate), nil); len(got) != 0 {
		t.Fatalf("empty grouped rows = %v", got)
	}
}

func TestOrderedAggregateRulesOfTheOrder(t *testing.T) {
	// Each case is one group of rows (key, X) and the first and last X by the key.
	type pair struct{ key, x any }
	run := func(t *testing.T, descending bool, rows ...pair) (first, last any) {
		t.Helper()
		records := make([]record.Record, len(rows))
		for i, row := range rows {
			records[i] = joinTestRecord("T", strconv.Itoa(i), map[string]any{"k": row.key, "x": row.x})
		}
		order := AscendingField("k")
		if descending {
			order = DescendingField("k")
		}
		q := From(NewRootCollectionRef("T", "")).NewQuery().SelectColumns(
			Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(order), Field("x"))},
			Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(order), Field("x"))})
		backend := &orderedStub{rows: map[string][]record.Record{"T": records}}
		reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		rows2 := readOrderedRows(t, reader)
		return rows2[0]["first"], rows2[0]["last"]
	}
	check := func(t *testing.T, gotFirst, gotLast, wantFirst, wantLast any) {
		t.Helper()
		if gotFirst != wantFirst || gotLast != wantLast {
			t.Fatalf("first, last = %v, %v; want %v, %v", gotFirst, gotLast, wantFirst, wantLast)
		}
	}

	t.Run("a NULL key sorts first ascending and last descending", func(t *testing.T) {
		first, last := run(t, false, pair{nil, "none"}, pair{1, "one"}, pair{2, "two"})
		check(t, first, last, "none", "two")
		first, last = run(t, true, pair{nil, "none"}, pair{1, "one"}, pair{2, "two"})
		check(t, first, last, "two", "none")
	})
	t.Run("the chosen row has X NULL: the answer is NULL and no row is skipped", func(t *testing.T) {
		first, last := run(t, false, pair{1, nil}, pair{2, "two"})
		check(t, first, last, nil, "two")
		first, last = run(t, false, pair{1, "one"}, pair{2, nil})
		check(t, first, last, "one", nil)
	})
	t.Run("a tie is broken by X ascending, NULL first", func(t *testing.T) {
		first, last := run(t, false, pair{1, "b"}, pair{1, "a"}, pair{1, "c"})
		check(t, first, last, "a", "c")
		first, last = run(t, false, pair{1, "b"}, pair{1, nil})
		check(t, first, last, nil, "b")
	})
	t.Run("last is not first descending when keys tie", func(t *testing.T) {
		rows := []pair{{1, "a"}, {2, "x"}, {2, "z"}}
		_, last := run(t, false, rows...)
		first, _ := run(t, true, rows...)
		check(t, last, first, "z", "x")
	})
	t.Run("text compares by code point", func(t *testing.T) {
		first, last := run(t, false, pair{"a", 1}, pair{"B", 2}, pair{"é", 3}, pair{"Z", 4})
		check(t, first, last, 2.0, 3.0) // B Z a é
	})
	t.Run("false is before true", func(t *testing.T) {
		first, last := run(t, false, pair{true, "yes"}, pair{false, "no"})
		check(t, first, last, "no", "yes")
	})
	t.Run("numbers compare as float64, so two whole numbers past 2^53 tie and X decides", func(t *testing.T) {
		first, last := run(t, false, pair{int64(9007199254740993), "b"}, pair{int64(9007199254740992), "a"})
		check(t, first, last, "a", "b")
	})
	t.Run("a number key of mixed kinds", func(t *testing.T) {
		first, last := run(t, false, pair{int8(3), "c"}, pair{2.5, "b"}, pair{uint16(1), "a"})
		check(t, first, last, "a", "c")
	})
}

func TestOrderedAggregateNativeAndFallbackThroughTheRecordsReader(t *testing.T) {
	ctx := context.Background()
	byDate := orderedBy(AscendingField("InvoiceDate"))
	q := firstAndLast(byDate)
	wanted := []map[string]any{
		{"CustomerId": 1.0, "first_total": 100.0, "last_total": 50.0},
		{"CustomerId": 2.0, "first_total": 75.0, "last_total": 200.0},
	}
	newStub := func() *orderedStub {
		return &orderedStub{caps: orderedCapabilities, rows: map[string][]record.Record{"Invoice": invoiceRecords(workedInvoices, true)}}
	}
	refusal := fmt.Errorf("column InvoiceDate has a type with no server form: %w", ErrNotSupported)

	t.Run("a native answer is returned as it is, with no second read", func(t *testing.T) {
		stub := newStub()
		stub.nativeRows = []record.Record{joinTestRecord("Invoice", "n", map[string]any{"CustomerId": 9})}
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); len(rows) != 1 || rows[0]["CustomerId"] != 9 || stub.native != 1 || stub.plain != 0 {
			t.Fatalf("rows = %v, native = %d, plain = %d", rows, stub.native, stub.plain)
		}
	})
	t.Run("a refusal as not supported is computed by DALgo from one plain read", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = refusal
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := sortedByCustomer(readOrderedRows(t, reader)); !reflect.DeepEqual(rows, wanted) || stub.native != 1 || stub.plain != 1 {
			t.Fatalf("rows = %v, native = %d, plain = %d", rows, stub.native, stub.plain)
		}
		// The plain read asks for every field the aggregation reads, the order keys among them, and
		// none of the result-stage operations.
		var asked []string
		for _, column := range stub.plainReads[0].Columns() {
			asked = append(asked, column.Expression.String())
		}
		sort.Strings(asked)
		if !reflect.DeepEqual(asked, []string{"CustomerId", "InvoiceDate", "Total"}) || len(stub.plainReads[0].GroupBy()) != 0 || stub.plainReads[0].Limit() != 0 {
			t.Fatalf("the plain read asked for %v", asked)
		}
	})
	t.Run("any other error is returned and there is no second read", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = errors.New("server is down")
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q); err != stub.nativeErr || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("when the plain read fails too, the first refusal is returned", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = refusal
		stub.plainErr = errors.New("the plain read failed")
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q); err != refusal || stub.plain != 1 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("a query with no ordered aggregate keeps the refusal it gets today", func(t *testing.T) {
		stub := newStub()
		stub.caps = QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}
		stub.nativeErr = refusal
		count := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(Count())
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, count); err != refusal || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("the entry point a transaction shares with a database reads the same way", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = refusal
		reader, err := executePlannedRecords(ctx, stub, q, stub.caps, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rows := sortedByCustomer(readOrderedRows(t, reader)); !reflect.DeepEqual(rows, wanted) {
			t.Fatalf("rows = %v", rows)
		}
	})
}

func TestOrderedAggregateNativeAndFallbackThroughTheRecordsetReader(t *testing.T) {
	ctx := context.Background()
	q := firstAndLast(orderedBy(AscendingField("InvoiceDate")))
	refusal := fmt.Errorf("a type with no server form: %w", ErrNotSupported)
	newStub := func() *orderedStub {
		return &orderedStub{caps: orderedCapabilities, rows: map[string][]record.Record{"Invoice": invoiceRecords(workedInvoices, false)}}
	}

	t.Run("a native answer is returned as it is", func(t *testing.T) {
		stub := newStub()
		reader, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil || reader.Recordset().Name() != "native" || stub.nativeSets != 1 || stub.plain != 0 {
			t.Fatalf("reader = %v, err = %v, nativeSets = %d, plain = %d", reader, err, stub.nativeSets, stub.plain)
		}
	})
	t.Run("a refusal as not supported is computed by DALgo from one plain read", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = refusal
		reader, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		rows := readOrderedRecordset(t, reader, "CustomerId", "first_total", "last_total")
		sort.SliceStable(rows, func(i, j int) bool { return rows[i]["CustomerId"].(float64) < rows[j]["CustomerId"].(float64) })
		want := []map[string]any{
			{"CustomerId": 1.0, "first_total": 100.0, "last_total": 50.0},
			{"CustomerId": 2.0, "first_total": 75.0, "last_total": 200.0},
		}
		if !reflect.DeepEqual(rows, want) || stub.nativeSets != 1 || stub.plain != 1 || stub.native != 0 {
			t.Fatalf("rows = %v, nativeSets = %d, plain = %d, native = %d", rows, stub.nativeSets, stub.plain, stub.native)
		}
	})
	t.Run("any other error is returned and there is no second read", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = errors.New("server is down")
		if _, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q); err != stub.nativeErr || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("when the plain read fails too, the first refusal is returned", func(t *testing.T) {
		stub := newStub()
		stub.nativeErr = refusal
		stub.plainErr = errors.New("the plain read failed")
		if _, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q); err != refusal {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a plan that is not native is computed by DALgo", func(t *testing.T) {
		stub := newStub()
		stub.caps = QueryCapabilities{}
		reader, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil || stub.nativeSets != 0 || stub.plain != 1 || reader.Recordset().RowsCount() != 2 {
			t.Fatalf("err = %v, nativeSets = %d, plain = %d", err, stub.nativeSets, stub.plain)
		}
	})
	t.Run("a plain read that fails is the error of a plan that is not native", func(t *testing.T) {
		stub := newStub()
		stub.caps = QueryCapabilities{}
		stub.plainErr = errors.New("the plain read failed")
		if _, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q); err != stub.plainErr {
			t.Fatalf("err = %v", err)
		}
	})
}

// An ordered aggregate stands wherever an aggregate stands: in HAVING, in ORDER BY
// and inside arithmetic.
func TestOrderedAggregateInHavingOrderByAndArithmetic(t *testing.T) {
	byDate := orderedBy(AscendingField("InvoiceDate"))
	last := NewOrderedAggregate(LAST, byDate, Field("Total"))
	first := NewOrderedAggregate(FIRST, byDate, Field("Total"))
	run := func(q StructuredQuery) []map[string]any {
		t.Helper()
		backend := &orderedStub{rows: map[string][]record.Record{"Invoice": invoiceRecords(workedInvoices, false)}}
		reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		return readOrderedRows(t, reader)
	}
	grouped := func() IQueryBuilder {
		return From(NewRootCollectionRef("Invoice", "")).NewQuery().GroupBy(Field("CustomerId"))
	}
	customer := Column{Expression: Field("CustomerId")}

	// HAVING keeps the customer whose last Total is over 60: customer 2, whose last Total is 200.
	having := grouped().Having(NewComparison(last, GreaterThen, NewConstant(60))).SelectColumns(customer)
	if got, want := run(having), []map[string]any{{"CustomerId": 2.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("HAVING: rows = %v, want %v", got, want)
	}
	// ORDER BY the last Total, descending: customer 2 (200) before customer 1 (50).
	ordered := grouped().OrderBy(Descending(last)).SelectColumns(customer)
	if got, want := run(ordered), []map[string]any{{"CustomerId": 2.0}, {"CustomerId": 1.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ORDER BY: rows = %v, want %v", got, want)
	}
	// Arithmetic over two ordered aggregates: the last Total less the first.
	span := grouped().OrderBy(AscendingField("CustomerId")).SelectColumns(customer, Column{Alias: "span", Expression: Binary(last, Subtract, first)})
	want := []map[string]any{{"CustomerId": 1.0, "span": -50.0}, {"CustomerId": 2.0, "span": 125.0}}
	if got := run(span); !reflect.DeepEqual(got, want) {
		t.Fatalf("arithmetic: rows = %v, want %v", got, want)
	}
	// The same aggregate named twice, and two that differ only in their order, are two states.
	byDateDesc := NewOrderedAggregate(LAST, orderedBy(DescendingField("InvoiceDate")), Field("Total"))
	both := grouped().OrderBy(AscendingField("CustomerId")).SelectColumns(customer,
		Column{Alias: "a", Expression: last}, Column{Alias: "b", Expression: byDateDesc}, Column{Alias: "c", Expression: last})
	want = []map[string]any{
		{"CustomerId": 1.0, "a": 50.0, "b": 100.0, "c": 50.0},
		{"CustomerId": 2.0, "a": 200.0, "b": 75.0, "c": 200.0},
	}
	if got := run(both); !reflect.DeepEqual(got, want) {
		t.Fatalf("two orders: rows = %v, want %v", got, want)
	}
}

// Rows removed by WHERE are not candidates.
func TestOrderedAggregateReadsOnlyTheRowsThatPassWhere(t *testing.T) {
	q := joinedInvoiceQuery(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))))
	filtered := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))).
		NewQuery().Where(NewComparison(NewFieldRef("i", "Total"), LessThen, NewConstant(15))).GroupBy(NewFieldRef("c", "Id")).
		SelectColumns(q.Columns()...)
	for name, tc := range map[string]struct {
		q    StructuredQuery
		want float64
	}{"every row": {q, 20}, "rows below 15": {filtered, 10}} {
		reader, err := NewDB(joinedStub(QueryCapabilities{}, nil)).ExecuteQueryToRecordsReader(context.Background(), tc.q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); len(rows) != 1 || rows[0]["last_total"] != tc.want {
			t.Fatalf("%s: rows = %v, want %v", name, rows, tc.want)
		}
	}
}
