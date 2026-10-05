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

func joinedInvoiceQuery(order []OrderExpression) StructuredQuery {
	from := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id")))
	return from.NewQuery().GroupBy(NewFieldRef("c", "Id")).SelectColumns(
		Column{Alias: "CustomerId", Expression: NewFieldRef("c", "Id")},
		Column{Alias: "last_total", Expression: NewOrderedAggregate(LAST, order, NewFieldRef("i", "Total"))})
}

func joinedStub(caps QueryCapabilities, decline error) *orderedJoinStub {
	invoices := instantRecords(timestampDatasets[0].rows, false)
	return &orderedJoinStub{
		orderedStub: &orderedStub{caps: caps, rows: map[string][]record.Record{"Invoice": invoices, "Customer": customerRecords()}},
		decline:     decline,
	}
}

func TestOrderedAggregateOverAJoinThePlannerAsksTheAdapterOnlyWhenItRunsTheOrder(t *testing.T) {
	ctx := context.Background()
	q := joinedInvoiceQuery(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))))
	wantRows := []map[string]any{{"CustomerId": 1.0, "last_total": 20.0}}
	runsFirstAndLast := QueryCapabilities{GroupBy: true, Aggregate: AggregateCapabilities{First: true, Last: true, OrderBy: true}}
	noOrder := QueryCapabilities{GroupBy: true, Aggregate: AggregateCapabilities{First: true, Last: true}}
	refusal := fmt.Errorf("a type with no server form: %w", ErrNotSupported)

	t.Run("an adapter that accepts every join but reports no Aggregate.OrderBy is not asked, and the order is kept", func(t *testing.T) {
		stub := joinedStub(noOrder, nil)
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); !reflect.DeepEqual(rows, wantRows) || stub.accepted != 0 || stub.native != 0 || stub.plain != 2 {
			t.Fatalf("rows = %v, accepted = %d, native = %d, plain = %d", rows, stub.accepted, stub.native, stub.plain)
		}
	})
	t.Run("an adapter with no capabilities at all is not asked either", func(t *testing.T) {
		stub := joinedStub(QueryCapabilities{}, nil)
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil || stub.accepted != 0 {
			t.Fatalf("err = %v, accepted = %d", err, stub.accepted)
		}
		if rows := readOrderedRows(t, reader); !reflect.DeepEqual(rows, wantRows) {
			t.Fatalf("rows = %v", rows)
		}
	})
	t.Run("an adapter that runs the order is asked and answers natively", func(t *testing.T) {
		stub := joinedStub(runsFirstAndLast, nil)
		stub.nativeRows = []record.Record{joinTestRecord("Invoice", "n", map[string]any{"native": true})}
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); !reflect.DeepEqual(rows, []map[string]any{{"native": true}}) || stub.accepted != 1 || stub.native != 1 || stub.plain != 0 {
			t.Fatalf("rows = %v, accepted = %d, native = %d, plain = %d", rows, stub.accepted, stub.native, stub.plain)
		}
	})
	t.Run("an adapter that declines the join leaves it to DALgo", func(t *testing.T) {
		stub := joinedStub(runsFirstAndLast, errors.New("declined"))
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); !reflect.DeepEqual(rows, wantRows) || stub.accepted != 1 || stub.native != 0 {
			t.Fatalf("rows = %v, accepted = %d, native = %d", rows, stub.accepted, stub.native)
		}
	})
	t.Run("an adapter that accepted the join and then refuses it as not supported leaves it to DALgo", func(t *testing.T) {
		stub := joinedStub(runsFirstAndLast, nil)
		stub.nativeErr = refusal
		reader, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRows(t, reader); !reflect.DeepEqual(rows, wantRows) || stub.native != 1 || stub.plain != 2 {
			t.Fatalf("rows = %v, native = %d, plain = %d", rows, stub.native, stub.plain)
		}
	})
	t.Run("another error of the native join read is the read's error", func(t *testing.T) {
		stub := joinedStub(runsFirstAndLast, nil)
		stub.nativeErr = errors.New("server is down")
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, q); err != stub.nativeErr || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("a query with no ordered aggregate asks the adapter as before", func(t *testing.T) {
		stub := joinedStub(QueryCapabilities{}, nil)
		sum := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))).
			NewQuery().SelectColumns(SumAs(NewFieldRef("i", "Total"), "sum"))
		stub.nativeErr = refusal
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, sum); err != refusal || stub.accepted != 1 || stub.plain != 0 {
			t.Fatalf("err = %v, accepted = %d, plain = %d", err, stub.accepted, stub.plain)
		}
	})
	t.Run("a join that is not valid is refused even when the adapter is not asked", func(t *testing.T) {
		stub := joinedStub(noOrder, nil)
		clash := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "i"), JoinInner, joinOn("i", "CustomerId", "i", "Id"))).
			NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(LAST, orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("i", "Total"))})
		if _, err := NewDB(stub).ExecuteQueryToRecordsReader(ctx, clash); err == nil || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
	t.Run("the same query through the recordset reader", func(t *testing.T) {
		stub := joinedStub(noOrder, nil)
		reader, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil || stub.accepted != 0 || stub.nativeSets != 0 {
			t.Fatalf("err = %v, accepted = %d, nativeSets = %d", err, stub.accepted, stub.nativeSets)
		}
		if rows := readOrderedRecordset(t, reader, "CustomerId", "last_total"); !reflect.DeepEqual(rows, wantRows) {
			t.Fatalf("rows = %v", rows)
		}

		stub = joinedStub(runsFirstAndLast, nil)
		reader, err = NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil || reader.Recordset().Name() != "native" || stub.accepted != 1 || stub.nativeSets != 1 || stub.plain != 0 {
			t.Fatalf("err = %v, accepted = %d, nativeSets = %d, plain = %d", err, stub.accepted, stub.nativeSets, stub.plain)
		}

		stub = joinedStub(runsFirstAndLast, nil)
		stub.nativeErr = refusal
		reader, err = NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if rows := readOrderedRecordset(t, reader, "CustomerId", "last_total"); !reflect.DeepEqual(rows, wantRows) || stub.nativeSets != 1 || stub.plain != 2 {
			t.Fatalf("rows = %v, nativeSets = %d, plain = %d", rows, stub.nativeSets, stub.plain)
		}

		stub = joinedStub(runsFirstAndLast, nil)
		stub.nativeErr = errors.New("server is down")
		if _, err := NewDB(stub).ExecuteQueryToRecordsetReader(ctx, q); err != stub.nativeErr || stub.plain != 0 {
			t.Fatalf("err = %v, plain = %d", err, stub.plain)
		}
	})
}

// A bad key is refused by every validator that walks a query, so that a walk
// that misses the order keys of an aggregate fails this test.
func TestOrderedAggregateWithABadKeyIsRefusedByEveryValidatorThatWalksAQuery(t *testing.T) {
	ghost := orderedBy(Ascending(NewFieldRef("ghost", "InvoiceDate")))
	q := joinedInvoiceQuery(ghost)

	if err := ValidateAggregation(q); err == nil || !strings.Contains(err.Error(), `unknown source "ghost"`) {
		t.Fatalf("ValidateAggregation: %v", err)
	}
	if _, err := PlanAggregation(q, QueryCapabilities{}); err == nil {
		t.Fatal("PlanAggregation planned it")
	}
	if err := ValidateQueryScope(q); err == nil || !strings.Contains(err.Error(), `unknown alias "ghost"`) {
		t.Fatalf("ValidateQueryScope: %v", err)
	}
	var diagnostic *JoinValidationError
	_, err := NewDB(joinedStub(QueryCapabilities{}, nil)).ExecuteQueryToRecordsReader(context.Background(), q)
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_scope" || !strings.Contains(diagnostic.Path, "orderBy[0]") {
		t.Fatalf("the join engine: %v", err)
	}
	if free := queryFreeReferences(q, map[uintptr]bool{}); !free["ghost"] {
		t.Fatalf("free references = %v", free)
	}
	if _, err := PlanRecursiveQuery(q); err == nil {
		t.Fatal("PlanRecursiveQuery planned it")
	}
	// A key that is a subquery is a subquery to every walk that looks for one.
	nested := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: Field("Id")})
	withSubquery := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(
		Column{Alias: "v", Expression: NewOrderedAggregate(LAST, orderedBy(Ascending(NewQueryExpression(nested, "n"))), Field("Total"))})
	if !HasSubquery(withSubquery) {
		t.Fatal("HasSubquery does not see a subquery in an order key")
	}
	// A bad key does not make a plan of a query that reads a derived source.
	noSubquery := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(
		Column{Alias: "v", Expression: NewOrderedAggregate(LAST, orderedBy(Ascending(Field("InvoiceDate"))), Field("Total"))})
	if HasSubquery(noSubquery) {
		t.Fatal("HasSubquery sees a subquery that is not there")
	}
}

func TestOrderedAggregateWalkersToleratANilKey(t *testing.T) {
	// A nil key, and a key with no expression, are refused by validation; a walk that
	// runs before it must not panic on them.
	for name, order := range map[string][]OrderExpression{
		"nil key":        {nil},
		"no expression":  {Ascending(nil)},
		"a good one too": {nil, AscendingField("InvoiceDate")},
	} {
		q := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(
			Column{Alias: "v", Expression: NewOrderedAggregate(LAST, order, Field("Total"))})
		t.Run(name, func(t *testing.T) {
			if got := collectSourceFields(q); len(got) == 0 {
				t.Fatalf("fields = %v", got)
			}
			if HasSubquery(q) {
				t.Fatal("HasSubquery")
			}
			if err := validateQueryScope(q, map[string]bool{}, "", map[uintptr]bool{}); err != nil {
				t.Fatal(err)
			}
			queryFreeReferences(q, map[uintptr]bool{})
			orderedAggregateFieldRefs(q)
			if !holdsOrderedAggregate(q) {
				t.Fatal("holdsOrderedAggregate")
			}
			e := &joinExecution{q: q, aliases: []string{"i"}, fields: map[string][]string{}}
			if err := e.validateQueryFields(); err != nil {
				t.Fatal(err)
			}
			if _, err := PlanAggregation(q, QueryCapabilities{}); err == nil {
				t.Fatal("PlanAggregation accepted a bad key")
			}
		})
	}
}

// An aggregate is named by its text before validation refuses it, for an output
// column that has no alias, so the text of one whose order holds a key that is
// missing, or has no expression, is written and the query is refused as invalid.
func TestOrderedAggregateWithAMissingKeyHasATextAndIsRefusedWhenItIsRead(t *testing.T) {
	total := NewFieldRef("i", "Total")
	for name, tc := range map[string]struct {
		order []OrderExpression
		want  string
	}{
		"a missing key":                        {[]OrderExpression{nil}, "LAST(i.Total ORDER BY <missing key>)"},
		"a key with no expression":             {[]OrderExpression{Ascending(nil)}, "LAST(i.Total ORDER BY <missing key>)"},
		"a key with no expression, descending": {[]OrderExpression{Descending(nil)}, "LAST(i.Total ORDER BY <missing key>)"},
		"a good key after a missing one":       {[]OrderExpression{nil, DescendingField("InvoiceDate")}, "LAST(i.Total ORDER BY <missing key>, InvoiceDate DESC)"},
	} {
		t.Run(name, func(t *testing.T) {
			aggregate := NewOrderedAggregate(LAST, tc.order, total)
			if got := aggregate.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
			from := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id")))
			q := from.NewQuery().GroupBy(NewFieldRef("c", "Id")).SelectColumns(
				Column{Alias: "CustomerId", Expression: NewFieldRef("c", "Id")},
				Column{Expression: aggregate})
			_, err := NewDB(joinedStub(QueryCapabilities{}, nil)).ExecuteQueryToRecordsReader(context.Background(), q)
			if err == nil || !strings.Contains(err.Error(), "an aggregate order key must be a field") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOrderedAggregateSortValueHelpers(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("a sort value is fixed-width UTC text", func(t *testing.T) {
		for instant, want := range map[time.Time]string{
			early:                      "2026-01-01T00:00:00.000000000Z",
			early.Add(time.Nanosecond): "2026-01-01T00:00:00.000000001Z",
			time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("plus2", 7200)): "2026-01-01T08:00:00.000000000Z",
			time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC):                          "0000-01-01T00:00:00.000000000Z",
			time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC):          "9999-12-31T23:59:59.999999999Z",
		} {
			got, err := timestampSortValue(instant)
			if err != nil || got != want {
				t.Fatalf("timestampSortValue(%v) = %q, %v; want %q", instant, got, err, want)
			}
		}
		if _, err := timestampSortValue(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
			t.Fatal("year 10000 was accepted")
		}
		if _, err := timestampSortValue(time.Date(-1, 12, 31, 0, 0, 0, 0, time.UTC)); err == nil {
			t.Fatal("year -1 was accepted")
		}
	})

	t.Run("buildSortValues keeps only timestamps", func(t *testing.T) {
		raw := map[string]any{"at": early, "name": "x", "nothing": nil, "number": 1, "pointer": &early}
		got, err := buildSortValues(raw, []string{"at", "name", "nothing", "number", "pointer", "absent"})
		if err != nil || !reflect.DeepEqual(got, map[string]any{"at": "2026-01-01T00:00:00.000000000Z", "pointer": "2026-01-01T00:00:00.000000000Z"}) {
			t.Fatalf("got %v, %v", got, err)
		}
		if got, err := buildSortValues(raw, []string{"name"}); got != nil || err != nil {
			t.Fatalf("got %v, %v", got, err)
		}
		if got, err := buildSortValues(nil, []string{"at"}); got != nil || err != nil {
			t.Fatalf("got %v, %v", got, err)
		}
		if _, err := buildSortValues(map[string]any{"at": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}, []string{"at"}); err == nil {
			t.Fatal("a timestamp outside the years was accepted")
		}
	})

	t.Run("sortValue reads through the sort value of a source row, then through the value", func(t *testing.T) {
		single := map[string]any{"at": "2026-01-01T00:00:00Z", "name": "x", sortValuesKey: map[string]any{"at": "SORT"}}
		if got := sortValue(Field("at"), single); got != "SORT" {
			t.Fatalf("got %v", got)
		}
		if got := sortValue(Field("name"), single); got != "x" {
			t.Fatalf("got %v", got)
		}
		joined := map[string]any{
			joinBaseKey: "i",
			joinSourcesKey: map[string]any{
				"i": map[string]any{"at": "text", sortValuesKey: map[string]any{"at": "SORT-I"}},
				"c": map[string]any{"at": "text-c"},
			},
		}
		for field, want := range map[FieldRef]any{
			NewFieldRef("i", "at"): "SORT-I",
			NewFieldRef("", "at"):  "SORT-I", // no source: the base source
			NewFieldRef("c", "at"): "text-c", // a source row with no sort values
			NewFieldRef("z", "at"): nil,      // no row for that source
		} {
			if got := sortValue(field, joined); got != want {
				t.Fatalf("sortValue(%v) = %v, want %v", field, got, want)
			}
		}
	})

	t.Run("evalSortKey evaluates a key that is not a field as a scalar", func(t *testing.T) {
		row := map[string]any{"a": 2.0, sortValuesKey: map[string]any{"a": "SORT"}}
		if got, err := evalSortKey(Field("a"), row); err != nil || got != "SORT" {
			t.Fatalf("got %v, %v", got, err)
		}
		if got, err := evalSortKey(Binary(Field("a"), Add, NewConstant(1)), row); err != nil || got != 3.0 {
			t.Fatalf("got %v, %v", got, err)
		}
		if _, err := evalSortKey(aggregationCoverageExpression{"bad"}, row); err == nil {
			t.Fatal("a key that is no expression was accepted")
		}
	})

	t.Run("the fields that need a sort value", func(t *testing.T) {
		byDate := orderedBy(Ascending(NewFieldRef("i", "InvoiceDate")), AscendingField("Name"))
		q := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, joinOn("i", "CustomerId", "c", "Id"))).NewQuery().SelectColumns(
			Column{Alias: "a", Expression: NewOrderedAggregate(LAST, byDate, NewFieldRef("c", "Name"))},
			Column{Alias: "b", Expression: NewOrderedAggregate(FIRST, byDate, Binary(NewFieldRef("i", "Total"), Add, NewConstant(1)))},
			Column{Alias: "c", Expression: NewAggregate(MAX, false, NewFieldRef("i", "Ignored"))})
		// "Name" has no source in the second key, so it is a field of the base source i.
		want := map[string][]string{"i": {"InvoiceDate", "Name"}, "c": {"Name"}}
		if got := orderedAggregateSortFields(q); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		if got := orderedAggregateSortNames(q); !reflect.DeepEqual(got, []string{"InvoiceDate", "Name"}) {
			t.Fatalf("got %v", got)
		}
		plain := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(MaxAs(Field("Total"), "m"))
		if got := orderedAggregateSortFields(plain); len(got) != 0 || got == nil {
			t.Fatalf("got %v", got)
		}
		if got := orderedAggregateSortNames(plain); got != nil {
			t.Fatalf("got %v", got)
		}
		noSource := structuredQuery{columns: []Column{{Alias: "a", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("d")), Field("x"))}}}
		if got := orderedAggregateSortFields(noSource); !reflect.DeepEqual(got, map[string][]string{"": {"d", "x"}}) {
			t.Fatalf("got %v", got)
		}
		nilBase := structuredQuery{from: From(nil), columns: noSource.columns}
		if got := orderedAggregateSortFields(nilBase); !reflect.DeepEqual(got, map[string][]string{"": {"d", "x"}}) {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("tuples are compared by the keys in their directions, then by the argument, then by the answer", func(t *testing.T) {
		order := orderedBy(AscendingField("a"), DescendingField("b"))
		for name, tc := range map[string]struct {
			a, b             []any
			aTie, bTie       any
			aAnswer, bAnswer any
			want             int
		}{
			"first key decides":      {[]any{1.0, 1.0}, []any{2.0, 0.0}, "x", "y", "p", "q", -1},
			"second key is reversed": {[]any{1.0, 1.0}, []any{1.0, 2.0}, "x", "y", "p", "q", 1},
			"the argument decides":   {[]any{1.0, 1.0}, []any{1.0, 1.0}, "x", "y", "q", "p", -1},
			"NULL argument is first": {[]any{1.0, 1.0}, []any{1.0, 1.0}, nil, "y", "q", "p", -1},
			"the answer decides":     {[]any{1.0, 1.0}, []any{1.0, 1.0}, "x", "x", "b", "a", 1},
			"equal":                  {[]any{1.0, 1.0}, []any{1.0, 1.0}, "x", "x", "a", "a", 0},
		} {
			if got := compareOrderedTuples(order, tc.a, tc.aTie, tc.aAnswer, tc.b, tc.bTie, tc.bAnswer); got != tc.want {
				t.Fatalf("%s: got %d, want %d", name, got, tc.want)
			}
		}
	})
}

func TestOrderedAggregateStateIsChargedToTheRetainedByteBudget(t *testing.T) {
	order := orderedBy(AscendingField("k"))
	aggregate := NewOrderedAggregate(FIRST, order, Field("x"))
	q := From(NewRootCollectionRef("T", "")).NewQuery().SelectColumns(Column{Alias: "v", Expression: aggregate})
	newReader := func() (*localAggregationReader, *localGroup, *aggregateState) {
		r := newLocalAggregationReader(context.Background(), q, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
		group, err := r.newGroup("implicit", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		return r, group, group.states[aggregate.String()]
	}
	row := func(k, x any) map[string]any { return map[string]any{"k": k, "x": x} }

	r, group, state := newReader()
	before := r.retainedBytes
	if err := r.updateGroup(group, row("zzzzzzzzzz", "xxxxxxxxxx")); err != nil {
		t.Fatal(err)
	}
	// The tuple is the answer, the key and the argument, each as JSON.
	wantBytes := len(`"xxxxxxxxxx"`) + len(`"zzzzzzzzzz"`) + len(`"xxxxxxxxxx"`)
	if state.valueBytes != wantBytes || r.retainedBytes-before != wantBytes || group.bytes != before+wantBytes {
		t.Fatalf("valueBytes = %d, retained grew by %d, want %d", state.valueBytes, r.retainedBytes-before, wantBytes)
	}
	// A later row that is not earlier changes nothing; an earlier and smaller one replaces the tuple and gives bytes back.
	if err := r.updateGroup(group, row("zzzzzzzzzz", "yyyyyyyyyy")); err != nil || state.value != "xxxxxxxxxx" {
		t.Fatalf("value = %v, err = %v", state.value, err)
	}
	if err := r.updateGroup(group, row("a", "b")); err != nil {
		t.Fatal(err)
	}
	if state.value != "b" || state.valueBytes != len(`"b"`)+len(`"a"`)+len(`"b"`) || r.retainedBytes-before != state.valueBytes {
		t.Fatalf("value = %v, valueBytes = %d, retained grew by %d", state.value, state.valueBytes, r.retainedBytes-before)
	}

	// A tuple that does not fit is the read's error and leaves the state as it was.
	r, group, state = newReader()
	r.retainedBytes = defaultMaxAggregationBytes
	if err := r.updateGroup(group, row("k", "x")); err == nil || state.hasValue {
		t.Fatalf("err = %v, hasValue = %v", err, state.hasValue)
	}

	// A key that is no field and no expression is an error of the update; validation refuses it first.
	bad := NewOrderedAggregate(LAST, orderedBy(Ascending(aggregationCoverageExpression{"bad"})), Field("x"))
	r = newLocalAggregationReader(context.Background(), q, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	group = &localGroup{states: map[string]*aggregateState{bad.String(): {expression: bad}}, keyValues: map[string]any{}}
	if err := r.updateGroup(group, row("k", "x")); err == nil {
		t.Fatal("a key that is no expression was accepted")
	}
}

func TestOrderedAggregateNormalizedRowErrors(t *testing.T) {
	q := From(NewRootCollectionRef("T", "")).NewQuery().SelectColumns(
		Column{Alias: "v", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("at")), Field("x"))})
	r := newLocalAggregationReader(context.Background(), q, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	if !reflect.DeepEqual(r.sortFields, []string{"at", "x"}) {
		t.Fatalf("sortFields = %v", r.sortFields)
	}
	row, err := r.normalizedRow(joinTestRecord("T", "1", map[string]any{"at": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "x": 1}))
	if err != nil || !reflect.DeepEqual(row[sortValuesKey], map[string]any{"at": "2026-01-01T00:00:00.000000000Z"}) {
		t.Fatalf("row = %v, err = %v", row, err)
	}
	if row, err = r.normalizedRow(joinTestRecord("T", "1", map[string]any{"at": "text", "x": 1})); err != nil || row[sortValuesKey] != nil {
		t.Fatalf("a row with no timestamp carries sort values: %v, %v", row, err)
	}
	if _, err = r.normalizedRow(joinTestRecord("T", "1", map[string]any{"at": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)})); err == nil {
		t.Fatal("a timestamp outside the years was accepted")
	}
	if _, err = r.normalizedRow(joinTestRecord("T", "1", map[string]any{"at": make(chan int)})); err == nil {
		t.Fatal("a row that is not JSON was accepted")
	}
}

// An ordered aggregate stands where an aggregate stands: the select list, HAVING
// and ORDER BY. In WHERE it is refused, and a key that is not a field is refused on
// a join a provider would run natively, before any provider is asked to read. A
// join condition holds field comparisons only, so it cannot hold one at all.
func TestOrderedAggregateIsRefusedOutsideTheSelectListHavingAndOrderByBeforeAnythingIsRead(t *testing.T) {
	ctx := context.Background()
	const placement = "an aggregate with an order cannot stand in where"
	byDate := orderedBy(Ascending(NewFieldRef("i", "InvoiceDate")))
	ordered := NewOrderedAggregate(LAST, byDate, NewFieldRef("i", "Total"))
	inWhere := NewComparison(ordered, GreaterThen, NewConstant(1))
	invoice, customer := NewRootCollectionRef("Invoice", "i"), NewRootCollectionRef("Customer", "c")
	onCustomer := joinOn("i", "CustomerId", "c", "Id")
	plainColumns := []Column{{Alias: "n", Expression: Count().Expression}}

	oneSource := From(invoice).NewQuery().Where(inWhere).SelectColumns(Column{Expression: NewFieldRef("i", "Total")})
	oneSourceGrouped := From(invoice).NewQuery().Where(inWhere).GroupBy(NewFieldRef("i", "CustomerId")).SelectColumns(
		Column{Expression: NewFieldRef("i", "CustomerId")}, Column{Alias: "v", Expression: ordered})
	joinedWhere := From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().Where(inWhere).SelectColumns(plainColumns...)
	joinedOn := From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer, inWhere)).NewQuery().SelectColumns(plainColumns...)
	badKey := NewOrderedAggregate(LAST, orderedBy(Ascending(NewConstant(1))), NewFieldRef("i", "Total"))
	joinedBadKey := From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().GroupBy(NewFieldRef("c", "Id")).SelectColumns(
		Column{Alias: "CustomerId", Expression: NewFieldRef("c", "Id")}, Column{Alias: "v", Expression: badKey})

	t.Run("ValidateAggregation", func(t *testing.T) {
		for name, q := range map[string]StructuredQuery{"a grouped query": oneSourceGrouped} {
			if err := ValidateAggregation(q); err == nil || !strings.Contains(err.Error(), placement) {
				t.Fatalf("%s: error = %v", name, err)
			}
		}
		// An aggregate with no order stands in WHERE as it did: nothing is added for it.
		plain := From(invoice).NewQuery().Where(NewComparison(NewAggregate(LAST, false, NewFieldRef("i", "Total")), GreaterThen, NewConstant(1))).SelectColumns(Column{Expression: NewFieldRef("i", "Total")})
		if err := ValidateAggregation(plain); err != nil {
			t.Fatal(err)
		}
	})

	for name, tc := range map[string]struct {
		q    StructuredQuery
		want string
	}{
		"one source":               {oneSource, placement},
		"one source, grouped":      {oneSourceGrouped, placement},
		"a join, in where":         {joinedWhere, placement},
		"a join, in its condition": {joinedOn, "only == is supported"},
	} {
		for label, caps := range map[string]QueryCapabilities{"no capabilities": {}, "capabilities": orderedCapabilities} {
			t.Run(name+"/"+label, func(t *testing.T) {
				stub := &orderedJoinStub{orderedStub: &orderedStub{caps: caps, rows: map[string][]record.Record{"Invoice": instantRecords(timestampDatasets[0].rows, false), "Customer": customerRecords()}}}
				db := NewDB(stub)
				if _, err := db.ExecuteQueryToRecordsReader(ctx, tc.q); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("records reader: error = %v", err)
				}
				if _, err := db.ExecuteQueryToRecordsetReader(ctx, tc.q); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("recordset reader: error = %v", err)
				}
				if stub.native+stub.plain+stub.nativeSets != 0 {
					t.Fatalf("native = %d, plain = %d, nativeSets = %d reads reached the provider, want 0", stub.native, stub.plain, stub.nativeSets)
				}
			})
		}
	}

	// A provider that accepts the join and runs the order is still not handed a key that is not a field.
	t.Run("a join the provider would run natively, with a key that is not a field", func(t *testing.T) {
		stub := &orderedJoinStub{orderedStub: &orderedStub{caps: orderedCapabilities, rows: map[string][]record.Record{"Invoice": instantRecords(timestampDatasets[0].rows, false), "Customer": customerRecords()}}}
		db := NewDB(stub)
		if _, err := db.ExecuteQueryToRecordsReader(ctx, joinedBadKey); err == nil || !strings.Contains(err.Error(), "an aggregate order key must be a field") {
			t.Fatalf("records reader: error = %v", err)
		}
		if _, err := db.ExecuteQueryToRecordsetReader(ctx, joinedBadKey); err == nil || !strings.Contains(err.Error(), "an aggregate order key must be a field") {
			t.Fatalf("recordset reader: error = %v", err)
		}
		if stub.accepted == 0 || stub.native+stub.nativeSets != 0 {
			t.Fatalf("accepted = %d, native = %d, nativeSets = %d: the provider was asked to accept the join and must not be asked to read it", stub.accepted, stub.native, stub.nativeSets)
		}
	})
}
