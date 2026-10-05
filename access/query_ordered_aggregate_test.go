package access

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// lastBy is LAST(operand ORDER BY keys...).
func lastBy(operand dal.Expression, keys ...dal.Expression) dal.AggregateFunc {
	order := make([]dal.OrderExpression, len(keys))
	for i, key := range keys {
		order[i] = dal.Ascending(key)
	}
	return dal.NewOrderedAggregate(dal.LAST, order, operand)
}

// An order key is read to sort the rows, so it is held to the field list as an
// operand is: a query that sorts by a field the list leaves out is denied, in the
// select list, in HAVING and in ORDER BY.
func TestOrderKeysOfAnAggregateAreHeldToTheFieldList(t *testing.T) {
	sets := fieldList(t, "name", "age")
	name, age, salary := dal.Field("name"), dal.Field("age"), dal.Field("salary")
	selectName := dal.Column{Expression: name}
	having := func(aggregate dal.Expression) dal.IQueryBuilder {
		return usersQuery().GroupBy(name).Having(dal.NewComparison(aggregate, dal.GreaterThen, dal.Constant{Value: 1}))
	}

	allowed := map[string]dal.StructuredQuery{
		"allowed key and operand in the select list": usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, age)}),
		"two allowed keys":                           usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, age, name)}),
		"allowed keys in HAVING":                     having(lastBy(age, name)).SelectColumns(selectName),
		"allowed keys in ORDER BY":                   usersQuery().GroupBy(name).OrderBy(dal.Ascending(lastBy(age, name))).SelectColumns(selectName),
		"a key through a pointer":                    usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, &age)}),
		"an aggregate with no order": usersQuery().SelectColumns(dal.Column{Alias: "v",
			Expression: dal.NewOrderedAggregate(dal.FIRST, nil, name)}),
	}
	for label, query := range allowed {
		t.Run("allowed "+label, func(t *testing.T) {
			if err := validateRequestedQueryFields(query, sets); err != nil {
				t.Fatalf("denied: %v", err)
			}
		})
	}

	denied := []struct {
		name   string
		query  dal.StructuredQuery
		slot   DecisionSlot
		column string
	}{
		{"a hidden key in the select list", usersQuery().SelectColumns(dal.Column{Expression: lastBy(name, salary)}), DecisionSlotFields, "salary"},
		{"a hidden key in the select list, with an alias", usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, salary)}), DecisionSlotFields, "salary"},
		{"a hidden second key", usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, age, salary)}), DecisionSlotFields, "salary"},
		{"a hidden operand with an allowed key", usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(salary, age)}), DecisionSlotFields, "salary"},
		{"a hidden key through a pointer", usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(name, &salary)}), DecisionSlotFields, "salary"},
		{"a hidden key in HAVING", having(lastBy(name, salary)).SelectColumns(selectName), DecisionSlotWhere, "salary"},
		{"a hidden key in HAVING, with the aggregate selected under an alias", having(lastBy(name, salary)).SelectColumns(selectName, dal.Column{Alias: "v", Expression: lastBy(name, salary)}), DecisionSlotWhere, "salary"},
		{"a hidden key in ORDER BY", usersQuery().GroupBy(name).OrderBy(dal.Ascending(lastBy(name, salary))).SelectColumns(selectName), DecisionSlotFields, "salary"},
		{"a hidden key in a null test", usersQuery().GroupBy(name).Having(dal.NewIsNotNullCondition(lastBy(name, salary))).SelectColumns(selectName), DecisionSlotWhere, "salary"},
	}
	for _, c := range denied {
		t.Run("denied "+c.name, func(t *testing.T) {
			var denial *DeniedError
			err := validateRequestedQueryFields(c.query, sets)
			if !errors.As(err, &denial) {
				t.Fatalf("error = %v, want a denial", err)
			}
			if denial.Decision.Code != CodeColumnDenied || denial.Decision.Slot != c.slot || !reflect.DeepEqual(denial.Decision.Columns, [][]string{{c.column}}) {
				t.Fatalf("decision = code %s slot %s columns %v", denial.Decision.Code, denial.Decision.Slot, denial.Decision.Columns)
			}
		})
	}

	// Through an alias: HAVING and ORDER BY may name the alias of an aggregate the list allows, and of
	// no other, so an aggregate that sorts by a hidden field is not a name they may use.
	grouped := func() dal.IQueryBuilder { return usersQuery().GroupBy(name) }
	columns := func(aggregate dal.Expression) []dal.Column {
		return []dal.Column{selectName, {Alias: "v", Expression: aggregate}}
	}
	over := dal.NewComparison(dal.Field("v"), dal.GreaterThen, dal.Constant{Value: 1})
	if err := validateRequestedQueryFieldsWithAliases(grouped().Having(over).OrderBy(dal.Ascending(dal.Field("v"))).SelectColumns(columns(lastBy(age, name))...), sets); err != nil {
		t.Fatalf("the alias of an allowed ordered aggregate was denied: %v", err)
	}
	for label, query := range map[string]dal.StructuredQuery{
		"HAVING":   grouped().Having(over).SelectColumns(columns(lastBy(age, salary))...),
		"ORDER BY": grouped().OrderBy(dal.Ascending(dal.Field("v"))).SelectColumns(columns(lastBy(age, salary))...),
	} {
		if err := validateRequestedQueryFieldsWithAliases(query, sets); err == nil {
			t.Fatalf("%s: the alias of an ordered aggregate that sorts by a hidden field was allowed", label)
		}
	}
	if aliases := allowedAggregateAliases(columns(lastBy(age, salary)), sets); len(aliases) != 0 {
		t.Fatalf("aliases = %v", aliases)
	}
	if aliases := allowedAggregateAliases(columns(lastBy(age, name)), sets); !reflect.DeepEqual(aliases, map[string]bool{"v": true}) {
		t.Fatalf("aliases = %v", aliases)
	}

	// A key that is not a field cannot be checked against a list, so the aggregate is refused as unsupported.
	var nilField *dal.FieldRef
	notChecked := map[string]dal.AggregateFunc{
		"a constant":                    lastBy(name, dal.Constant{Value: 1}),
		"a param":                       lastBy(name, dal.NewParam("p")),
		"arithmetic":                    lastBy(name, dal.Binary(age, dal.Add, age)),
		"an aggregate":                  lastBy(name, dal.NewAggregate(dal.MAX, false, age)),
		"a subquery":                    lastBy(name, dal.NewQueryExpression(secretRows(), "x")),
		"a nil pointer":                 lastBy(name, nilField),
		"a key that is nothing":         dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{dal.Ascending(nil)}, name),
		"a missing key":                 dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{nil}, name),
		"an allowed key after a bad":    lastBy(name, dal.Constant{Value: 1}, age),
		"a function DALgo does not def": dal.NewOrderedAggregate("MEDIAN", []dal.OrderExpression{dal.AscendingField("age")}, age),
	}
	for label, aggregate := range notChecked {
		t.Run("unsupported "+label, func(t *testing.T) {
			var denial *DeniedError
			err := validateRequestedQueryFields(usersQuery().SelectColumns(dal.Column{Alias: "v", Expression: aggregate}), sets)
			if !errors.As(err, &denial) || denial.Decision.Code != CodeEnforcementUnsupported {
				t.Fatalf("error = %v, want an unsupported-expression denial", err)
			}
		})
	}
}

// The sources a query reads include those of a subquery in an order key, so a
// policy authorises each of them before any is read.
func TestOrderKeysOfAnAggregateAreWalkedForTheirSources(t *testing.T) {
	scalar := dal.NewQueryExpression(secretRows(), "x")
	for name, tc := range map[string]struct {
		query dal.StructuredQuery
		want  []string
	}{
		"a subquery as a key": {
			customerQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(dal.Field("a"), scalar)}),
			[]string{"/Customer", "/Secret"}},
		"a subquery as the second key": {
			customerQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(dal.Field("a"), dal.Field("b"), scalar)}),
			[]string{"/Customer", "/Secret"}},
		"a key that is a field holds no source": {
			customerQuery().SelectColumns(dal.Column{Alias: "v", Expression: lastBy(dal.Field("a"), dal.Field("b"))}),
			[]string{"/Customer"}},
		"a missing key holds no source": {
			customerQuery().SelectColumns(dal.Column{Alias: "v", Expression: dal.NewOrderedAggregate(dal.LAST, []dal.OrderExpression{nil}, dal.Field("a"))}),
			[]string{"/Customer"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resourceNames(resourcesForQuery(tc.query)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resources = %v, want %v", got, tc.want)
			}
			// The walk and the router agree on whether the query holds a nested query.
			walk := querySourceWalk{onPath: map[uintptr]bool{}}
			walk.query(tc.query, 0)
			if got, want := dal.HasSubquery(tc.query), walk.queries > 1; got != want {
				t.Fatalf("HasSubquery = %v, but the walk entered %d queries", got, walk.queries)
			}
		})
	}
}
