package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func orderedAggregateQuery(aggregate dal.AggregateFunc) fakeQuery {
	return fakeQuery{
		from:    rootFrom(),
		columns: []dal.Column{{Alias: "v", Expression: aggregate}},
	}
}

// This release can read no document with an order on an aggregate, so it must not
// write one: an aggregate with an order would come out as one without.
func TestSerializeRefusesAnAggregateWithAnOrder(t *testing.T) {
	order := []dal.OrderExpression{dal.AscendingField("created")}
	ordered := dal.NewOrderedAggregate(dal.LAST, order, dal.Field("total"))
	const want = "an aggregate with an order cannot be written by this version"

	for name, q := range map[string]dal.StructuredQuery{
		"in columns": orderedAggregateQuery(ordered),
		"inside arithmetic": fakeQuery{from: rootFrom(), columns: []dal.Column{
			{Alias: "v", Expression: dal.Binary(ordered, dal.Subtract, dal.NewConstant(1))}}},
		"in having": fakeQuery{from: rootFrom(), groupBy: []dal.Expression{dal.Field("g")},
			columns: []dal.Column{{Expression: dal.Field("g")}},
			having:  dal.NewComparison(ordered, dal.GreaterThen, dal.NewConstant(1))},
		"in order by": fakeQuery{from: rootFrom(), groupBy: []dal.Expression{dal.Field("g")},
			columns: []dal.Column{{Expression: dal.Field("g")}},
			orderBy: []dal.OrderExpression{dal.Ascending(ordered)}},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Serialize(q)
			if err == nil || !strings.Contains(err.Error(), want) || data != nil {
				t.Fatalf("data = %q, err = %v", data, err)
			}
		})
	}

	// An aggregate whose order is empty is the aggregate without one: the same bytes.
	plain, err := Serialize(orderedAggregateQuery(dal.NewAggregate(dal.FIRST, false, dal.Field("total"))))
	if err != nil {
		t.Fatal(err)
	}
	for _, empty := range [][]dal.OrderExpression{nil, {}} {
		got, err := Serialize(orderedAggregateQuery(dal.NewOrderedAggregate(dal.FIRST, empty, dal.Field("total"))))
		if err != nil || string(got) != string(plain) {
			t.Fatalf("got %q, %v; want %q", got, err, plain)
		}
	}
}

func TestEqualComparesTheOrdersOfAggregates(t *testing.T) {
	total := dal.Field("total")
	byCreated := []dal.OrderExpression{dal.AscendingField("created")}
	q := func(a dal.AggregateFunc) dal.StructuredQuery { return orderedAggregateQuery(a) }

	same := dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.AscendingField("created"), dal.DescendingField("id")}, total)
	for name, tc := range map[string]struct {
		a, b dal.AggregateFunc
		want bool
	}{
		"the same order": {same, dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.AscendingField("created"), dal.DescendingField("id")}, total), true},
		"another key":    {dal.NewOrderedAggregate(dal.LAST, byCreated, total), dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.AscendingField("updated")}, total), false},
		"another source": {dal.NewOrderedAggregate(dal.LAST, byCreated, total), dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.Ascending(dal.NewFieldRef("u", "created"))}, total), false},
		"another direction": {dal.NewOrderedAggregate(dal.LAST, byCreated, total),
			dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.DescendingField("created")}, total), false},
		"another number of keys":                         {same, dal.NewOrderedAggregate(dal.LAST, byCreated, total), false},
		"an order and none":                              {dal.NewOrderedAggregate(dal.LAST, byCreated, total), dal.NewAggregate(dal.LAST, false, total), false},
		"none and an order":                              {dal.NewAggregate(dal.LAST, false, total), dal.NewOrderedAggregate(dal.LAST, byCreated, total), false},
		"no order on either":                             {dal.NewAggregate(dal.LAST, false, total), dal.NewOrderedAggregate(dal.LAST, nil, total), true},
		"an order and an aggregate that cannot hold one": {dal.NewOrderedAggregate(dal.LAST, byCreated, total), plainAggregate{dal.NewAggregate(dal.LAST, false, total)}, false},
		"an aggregate that cannot hold one and none":     {plainAggregate{dal.NewAggregate(dal.LAST, false, total)}, dal.NewAggregate(dal.LAST, false, total), true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Equal(q(tc.a), q(tc.b)); got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}

// plainAggregate is an aggregate that is not DALgo's own type: it implements
// AggregateFunc and nothing more, so it has no order.
type plainAggregate struct{ inner dal.AggregateFunc }

func (a plainAggregate) String() string             { return a.inner.String() }
func (a plainAggregate) FuncName() string           { return a.inner.FuncName() }
func (a plainAggregate) FuncArgs() []dal.Expression { return a.inner.FuncArgs() }

// Every walk of a JOIN document holds the keys of an aggregate's order to the sources of the query.
func TestJoinClauseFieldsHoldTheOrderKeysOfAnAggregate(t *testing.T) {
	invoice := dal.NewRootCollectionRef("Invoice", "i")
	customer := dal.NewRootCollectionRef("Customer", "c")
	from := dal.From(invoice).Join(dal.NewJoinedSource(customer, dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("i", "CustomerId"), dal.Equal, dal.NewFieldRef("c", "Id"))))
	build := func(key dal.Expression) dal.StructuredQuery {
		aggregate := dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.Ascending(key)}, dal.NewFieldRef("i", "Total"))
		return fakeQuery{from: from, columns: []dal.Column{{Alias: "v", Expression: aggregate}}}
	}

	if err := validateJoinClauseFields(build(dal.NewFieldRef("i", "InvoiceDate"))); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		key  dal.Expression
		want string
	}{
		"a source the query does not hold": {dal.NewFieldRef("ghost", "InvoiceDate"), `unknown alias "ghost"`},
		"a field with no source":           {dal.Field("InvoiceDate"), "unqualified JOIN field requires schema metadata"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateJoinClauseFields(build(tc.key))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "orderBy[0]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	// A key that is missing is for validation to refuse; the walk passes over it.
	nilKey := fakeQuery{from: from, columns: []dal.Column{{Alias: "v", Expression: dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{nil}, dal.NewFieldRef("i", "Total"))}}}
	if err := validateJoinClauseFields(nilKey); err != nil {
		t.Fatal(err)
	}
}
