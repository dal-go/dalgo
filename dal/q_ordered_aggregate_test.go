package dal

import (
	"strings"
	"testing"
)

func TestNewOrderedAggregateModel(t *testing.T) {
	order := orderedBy(AscendingField("InvoiceDate"), DescendingField("InvoiceId"))
	aggregate := NewOrderedAggregate("last", order, Field("Total"))

	if aggregate.FuncName() != LAST {
		t.Fatalf("name = %q", aggregate.FuncName())
	}
	if args := aggregate.FuncArgs(); len(args) != 1 || args[0].String() != "Total" {
		t.Fatalf("args = %v", args)
	}
	ordered, ok := aggregate.(OrderedAggregateFunc)
	if !ok {
		t.Fatal("an aggregate with an order does not implement OrderedAggregateFunc")
	}
	if got := ordered.AggregateOrder(); len(got) != 2 || got[0].String() != "InvoiceDate" || got[1].String() != "InvoiceId DESC" {
		t.Fatalf("order = %v", got)
	}
	if want := "LAST(Total ORDER BY InvoiceDate, InvoiceId DESC)"; aggregate.String() != want {
		t.Fatalf("String() = %q, want %q", aggregate.String(), want)
	}
	if aggregateDistinct(aggregate) {
		t.Fatal("an ordered aggregate reports DISTINCT")
	}

	// The constructor keeps its own copy of the order and of the arguments.
	order[0] = DescendingField("Other")
	if got := ordered.AggregateOrder(); got[0].String() != "InvoiceDate" {
		t.Fatalf("the order was not copied: %v", got)
	}
}

func TestOrderedAggregateWithoutAnOrderIsTheUnorderedAggregate(t *testing.T) {
	plain := NewAggregate(FIRST, false, Field("Total"))
	if _, ok := plain.(OrderedAggregateFunc); !ok {
		t.Fatal("the concrete aggregate carries the optional interface")
	}
	if got := plain.(OrderedAggregateFunc).AggregateOrder(); len(got) != 0 {
		t.Fatalf("NewAggregate carries an order: %v", got)
	}
	for _, empty := range [][]OrderExpression{nil, {}} {
		same := NewOrderedAggregate(FIRST, empty, Field("Total"))
		if same.String() != plain.String() || same.String() != "FIRST(Total)" {
			t.Fatalf("String() = %q, want %q", same.String(), plain.String())
		}
	}
}

func TestOrderedAggregateRulesOfValidation(t *testing.T) {
	invoice := NewRootCollectionRef("Invoice", "")
	derived := NewQuerySource(From(invoice).NewQuery().SelectColumns(Column{Expression: Field("Total")}), "d")
	byDate := orderedBy(AscendingField("InvoiceDate"))
	last := func(order []OrderExpression, arg Expression) Column {
		return Column{Expression: NewOrderedAggregate(LAST, order, arg), Alias: "v"}
	}

	valid := map[string]StructuredQuery{
		"one source, no group": From(invoice).NewQuery().SelectColumns(last(byDate, Field("Total"))),
		"grouped": From(invoice).NewQuery().GroupBy(Field("CustomerId")).
			SelectColumns(Column{Expression: Field("CustomerId")}, last(orderedBy(AscendingField("InvoiceDate"), DescendingField("InvoiceId")), Field("Total"))),
		"x is arithmetic": From(invoice).NewQuery().SelectColumns(last(byDate, Binary(Field("Total"), Multiply, NewConstant(2)))),
		"key is qualified": From(NewRootCollectionRef("Invoice", "i")).NewQuery().
			SelectColumns(last(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("i", "Total"))),
		"inside arithmetic": From(invoice).NewQuery().SelectColumns(Column{Alias: "span", Expression: Binary(
			NewOrderedAggregate(LAST, byDate, Field("Total")), Subtract, NewOrderedAggregate(FIRST, byDate, Field("Total")))}),
		"in having and order by": From(invoice).NewQuery().GroupBy(Field("CustomerId")).
			Having(NewComparison(NewOrderedAggregate(LAST, byDate, Field("Total")), GreaterThen, NewConstant(1))).
			OrderBy(Ascending(NewOrderedAggregate(FIRST, byDate, Field("Total")))).
			SelectColumns(Column{Expression: Field("CustomerId")}),
		"joined stored sources": From(NewRootCollectionRef("Invoice", "i")).
			Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner, NewComparison(NewFieldRef("i", "CustomerId"), Equal, NewFieldRef("c", "Id")))).
			NewQuery().GroupBy(NewFieldRef("c", "Name")).
			SelectColumns(Column{Expression: NewFieldRef("c", "Name")}, last(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("i", "Total"))),
		"a derived source that the aggregate does not read": From(NewRootCollectionRef("Invoice", "i")).
			Join(NewJoinedSource(derived, JoinInner, NewComparison(NewFieldRef("i", "CustomerId"), Equal, NewFieldRef("d", "Total")))).
			NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("i", "Total"))),
		"a query with no source at all": structuredQuery{columns: []Column{last(byDate, Field("Total"))}},
	}
	for name, q := range valid {
		t.Run("valid "+name, func(t *testing.T) {
			if err := ValidateAggregation(q); err != nil {
				t.Fatal(err)
			}
		})
	}

	invalidKeys := []struct {
		name  string
		order []OrderExpression
		want  string
	}{
		{"constant", orderedBy(Ascending(NewConstant(1))), "an aggregate order key must be a field"},
		{"param", orderedBy(Ascending(NewParam("p"))), "an aggregate order key must be a field"},
		{"arithmetic", orderedBy(Ascending(Binary(Field("a"), Add, Field("b")))), "an aggregate order key must be a field"},
		{"aggregate", orderedBy(Ascending(NewAggregate(MAX, false, Field("a")))), "an aggregate order key must be a field"},
		{"subquery", orderedBy(Ascending(NewQueryExpression(From(invoice).NewQuery().SelectColumns(Column{Expression: Field("a")}), "q"))), "an aggregate order key must be a field"},
		{"nil item", orderedBy(nil), "an aggregate order key must be a field"},
		{"nil expression", orderedBy(Ascending(nil)), "an aggregate order key must be a field"},
		{"a field by pointer", orderedBy(Ascending(&FieldRef{})), "an aggregate order key must be a field"},
	}
	for _, tc := range invalidKeys {
		t.Run("invalid key "+tc.name, func(t *testing.T) {
			q := From(invoice).NewQuery().SelectColumns(last(tc.order, Field("Total")))
			err := ValidateAggregation(q)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, function := range []string{COUNT, SUM, AVERAGE, MIN, MAX} {
		t.Run("order on "+function, func(t *testing.T) {
			q := From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(function, byDate, Field("Total"))})
			err := ValidateAggregation(q)
			if err == nil || !strings.Contains(err.Error(), "aggregate orderBy is supported only for FIRST and LAST") {
				t.Fatalf("error = %v", err)
			}
		})
	}

	derivedFrom := func(joined ...JoinedSource) FromSource {
		base := From(derived)
		for _, join := range joined {
			base = base.Join(join)
		}
		return base
	}
	storedJoin := NewJoinedSource(NewRootCollectionRef("Invoice", "i"), JoinInner, NewComparison(NewFieldRef("d", "Total"), Equal, NewFieldRef("i", "Total")))
	for name, tc := range map[string]struct {
		q    StructuredQuery
		want string
	}{
		"key from a derived source": {
			derivedFrom(storedJoin).NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("d", "Total"))), NewFieldRef("i", "Total"))),
			`an ordered aggregate reads fields of stored sources only: source "d" is a query`},
		"x from a derived source": {
			derivedFrom(storedJoin).NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("d", "Total"))),
			`an ordered aggregate reads fields of stored sources only: source "d" is a query`},
		"an unqualified key beside a derived source": {
			derivedFrom(storedJoin).NewQuery().SelectColumns(last(orderedBy(AscendingField("InvoiceDate")), NewFieldRef("i", "Total"))),
			`an ordered aggregate reads fields of stored sources only: field "InvoiceDate" has no source and source "d" is a query`},
		"an unqualified x beside a derived source": {
			derivedFrom(storedJoin).NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), Field("Total"))),
			`an ordered aggregate reads fields of stored sources only: field "Total" has no source and source "d" is a query`},
		"a derived source on a join edge": {
			From(invoice).Join(NewJoinedSource(derived, JoinInner, NewComparison(NewFieldRef("Invoice", "Total"), Equal, NewFieldRef("d", "Total")))).
				NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("d", "Total"))), NewFieldRef("Invoice", "Total"))),
			`an ordered aggregate reads fields of stored sources only: source "d" is a query`},
		"a source that is not in the query": {
			From(invoice).NewQuery().SelectColumns(last(orderedBy(Ascending(NewFieldRef("other", "InvoiceDate"))), Field("Total"))),
			`an ordered aggregate reads fields of stored sources only: unknown source "other"`},
		"an x from a source that is not in the query": {
			From(invoice).NewQuery().SelectColumns(last(orderedBy(AscendingField("InvoiceDate")), NewFieldRef("other", "Total"))),
			`an ordered aggregate reads fields of stored sources only: unknown source "other"`},
		"in having": {
			From(invoice).NewQuery().GroupBy(Field("CustomerId")).
				Having(NewComparison(NewOrderedAggregate(LAST, orderedBy(Ascending(NewFieldRef("other", "InvoiceDate"))), Field("Total")), GreaterThen, NewConstant(1))).
				SelectColumns(Column{Expression: Field("CustomerId")}),
			`unknown source "other"`},
		"in order by": {
			From(invoice).NewQuery().GroupBy(Field("CustomerId")).
				OrderBy(Ascending(NewOrderedAggregate(LAST, orderedBy(Ascending(NewFieldRef("other", "InvoiceDate"))), Field("Total")))).
				SelectColumns(Column{Expression: Field("CustomerId")}),
			`unknown source "other"`},
		"a query with no source": {
			structuredQuery{columns: []Column{last(orderedBy(Ascending(NewFieldRef("other", "d"))), Field("Total"))}},
			`unknown source "other"`},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateAggregation(tc.q)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	// An ordered aggregate stands where an aggregate stands: not in GROUP BY and not inside another aggregate.
	ordered := NewOrderedAggregate(LAST, byDate, Field("Total"))
	for name, tc := range map[string]struct {
		q    StructuredQuery
		want string
	}{
		"in group by":            {structuredQuery{from: From(invoice), groupBy: []Expression{ordered}}, "contains an aggregate"},
		"inside another":         {From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewAggregate(SUM, false, ordered)}), "nested aggregates are not supported"},
		"inside another ordered": {From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(FIRST, byDate, ordered)}), "nested aggregates are not supported"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAggregation(tc.q); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	// DISTINCT stays refused for FIRST and LAST, with the order or without it.
	distinctFirst := From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: distinctOrdered{NewOrderedAggregate(FIRST, byDate, Field("Total"))}})
	if err := ValidateAggregation(distinctFirst); err == nil || !strings.Contains(err.Error(), "DISTINCT is not supported for FIRST") {
		t.Fatalf("error = %v", err)
	}
}

// distinctOrdered reports DISTINCT on an ordered aggregate, which no DALgo
// constructor builds.
type distinctOrdered struct{ AggregateFunc }

func (distinctOrdered) IsDistinct() bool { return true }
func (d distinctOrdered) AggregateOrder() []OrderExpression {
	return d.AggregateFunc.(OrderedAggregateFunc).AggregateOrder()
}

func TestOrderedAggregatePlanning(t *testing.T) {
	invoice := NewRootCollectionRef("Invoice", "")
	byDate := orderedBy(AscendingField("InvoiceDate"))
	orderedFirst := From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(FIRST, byDate, Field("Total"))})
	orderedLast := From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(LAST, byDate, Field("Total"))})
	unorderedFirst := From(invoice).NewQuery().SelectColumns(FirstAs(Field("Total"), "v"))

	both := func(first, last, orderBy, stable bool) QueryCapabilities {
		return QueryCapabilities{StableRowOrder: stable, Aggregate: AggregateCapabilities{First: first, Last: last, OrderBy: orderBy}}
	}
	for name, tc := range map[string]struct {
		q    StructuredQuery
		caps QueryCapabilities
		want AggregationStrategy
	}{
		"first without Aggregate.OrderBy is planned in memory": {orderedFirst, both(true, true, false, false), AggregationHash},
		"last without Aggregate.OrderBy is planned in memory":  {orderedLast, both(true, true, false, false), AggregationHash},
		"first without First is planned in memory":             {orderedFirst, both(false, true, true, false), AggregationHash},
		"last without Last is planned in memory":               {orderedLast, both(true, false, true, false), AggregationHash},
		"first with First and Aggregate.OrderBy is native":     {orderedFirst, both(true, false, true, false), AggregationNative},
		"last with Last and Aggregate.OrderBy is native":       {orderedLast, both(false, true, true, false), AggregationNative},
		"an unordered first still needs the stable order":      {unorderedFirst, both(true, false, true, true), AggregationNative},
		"an ordered first does not need the stable order":      {orderedFirst, both(false, false, false, false), AggregationHash},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := PlanAggregation(tc.q, tc.caps)
			if err != nil || plan.Strategy != tc.want {
				t.Fatalf("plan = %#v, err = %v, want %s", plan, err, tc.want)
			}
		})
	}

	// An unordered first or last keeps its refusal, and the refusal keeps its opening words.
	_, err := PlanAggregation(unorderedFirst, both(true, true, true, false))
	if err == nil || !strings.Contains(err.Error(), "FIRST/LAST require a provider-declared stable input order") ||
		!strings.Contains(err.Error(), "or an order of their own (aggregate orderBy)") {
		t.Fatalf("error = %v", err)
	}
	// An ordered aggregate beside an unordered one still needs the stable order for the unordered one.
	mixed := From(invoice).NewQuery().SelectColumns(
		Column{Alias: "a", Expression: NewOrderedAggregate(FIRST, byDate, Field("Total"))}, FirstAs(Field("Total"), "b"))
	if _, err := PlanAggregation(mixed, both(true, true, true, false)); err == nil {
		t.Fatal("an unordered first beside an ordered one was planned without a stable order")
	}
	// An ordered aggregate that is not native over a provider with every capability.
	if plan, err := PlanAggregation(mixed, both(true, true, true, true)); err != nil || plan.Strategy != AggregationNative {
		t.Fatalf("plan = %#v, err = %v", plan, err)
	}
	// An invalid ordered aggregate is refused by the planner.
	bad := From(invoice).NewQuery().SelectColumns(Column{Alias: "v", Expression: NewOrderedAggregate(SUM, byDate, Field("Total"))})
	if _, err := PlanAggregation(bad, QueryCapabilities{}); err == nil {
		t.Fatal("an order on SUM was planned")
	}
}

func TestOrderedAggregateNativeCapabilityRule(t *testing.T) {
	byDate := orderedBy(AscendingField("InvoiceDate"))
	for _, tc := range []struct {
		function string
		caps     AggregateCapabilities
		want     bool
	}{
		{FIRST, AggregateCapabilities{First: true, OrderBy: true}, true},
		{FIRST, AggregateCapabilities{First: true}, false},
		{FIRST, AggregateCapabilities{OrderBy: true}, false},
		{FIRST, AggregateCapabilities{Last: true, OrderBy: true}, false},
		{LAST, AggregateCapabilities{Last: true, OrderBy: true}, true},
		{LAST, AggregateCapabilities{Last: true}, false},
		{LAST, AggregateCapabilities{First: true, OrderBy: true}, false},
		// A function that takes no order is never run natively with one; validation refuses it.
		{SUM, AggregateCapabilities{Sum: true, First: true, Last: true, OrderBy: true}, false},
	} {
		aggregate := NewOrderedAggregate(tc.function, byDate, Field("Total"))
		if got := orderedAggregateNative(aggregate, tc.caps); got != tc.want {
			t.Fatalf("%s with %+v: got %v, want %v", tc.function, tc.caps, got, tc.want)
		}
	}

	// A provider runs a query's ordered aggregates when it runs each of them; a query with none is always run.
	q := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "a", Expression: NewOrderedAggregate(FIRST, byDate, Field("Total"))},
		Column{Alias: "b", Expression: NewOrderedAggregate(LAST, byDate, Field("Total"))},
		FirstAs(Field("Total"), "c"))
	for caps, want := range map[string]bool{"both": true, "only first": false, "only last": false, "neither": false} {
		c := QueryCapabilities{Aggregate: AggregateCapabilities{OrderBy: true, First: caps == "both" || caps == "only first", Last: caps == "both" || caps == "only last"}}
		if got := providerRunsOrderedAggregates(q, c); got != want {
			t.Fatalf("%s: got %v, want %v", caps, got, want)
		}
	}
	if !providerRunsOrderedAggregates(From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(FirstAs(Field("Total"), "c")), QueryCapabilities{}) {
		t.Fatal("a query with no ordered aggregate is not run by the provider")
	}
}
