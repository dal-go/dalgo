package dal

import (
	"fmt"
	"strings"
	"testing"
)

type subqueryTestQuery struct {
	StructuredQuery
	from    FromSource
	where   Condition
	columns []Column
}

func (q *subqueryTestQuery) From() FromSource           { return q.from }
func (q *subqueryTestQuery) Where() Condition           { return q.where }
func (q *subqueryTestQuery) Having() Condition          { return nil }
func (q *subqueryTestQuery) GroupBy() []Expression      { return nil }
func (q *subqueryTestQuery) OrderBy() []OrderExpression { return nil }
func (q *subqueryTestQuery) Columns() []Column          { return q.columns }

func TestQueryNodesAndInspection(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	source := NewQuerySource(leaf, "invoices")
	source.recordsetSource()
	if source.Name() != "invoices" || source.Alias() != "invoices" || source.Query().From() == nil {
		t.Fatalf("unexpected query source: %#v", source)
	}
	expression := NewQueryExpression(leaf, "count")
	if expression.As() != "count" || expression.Query().From() == nil || !strings.Contains(expression.String(), "count") {
		t.Fatalf("unexpected query expression: %s", expression)
	}
	if NewQueryExpression(leaf, "").String() != "(SUBQUERY)" {
		t.Fatal("empty scalar query name")
	}
	exists, notExists := NewExistsCondition(leaf), NewNotExistsCondition(leaf)
	if exists.Negated() || !notExists.Negated() || !strings.HasPrefix(notExists.String(), "NOT EXISTS") {
		t.Fatalf("unexpected EXISTS nodes")
	}
	if exists.String() != "EXISTS (SUBQUERY)" {
		t.Fatal("EXISTS string")
	}
	if (&QueryValidationError{Category: "shape", Message: "bad"}).Error() != "shape: bad" {
		t.Fatal("unpathed validation error")
	}
	outer := From(source).NewQuery().Where(exists).SelectColumns(Column{Expression: expression})
	if !HasSubquery(outer) {
		t.Fatal("HasSubquery did not find recursive nodes")
	}
	plain := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("c", "id")})
	if HasSubquery(plain) {
		t.Fatal("HasSubquery reported a flat query")
	}
}

func TestQueryTreeInspectionForms(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	child := From(NewQuerySource(leaf, "d")).Join(NewJoinedSource(NewRootCollectionRef("Payment", "p"), JoinInner, NewComparison(NewFieldRef("d", "id"), Equal, NewFieldRef("p", "invoice")))).NewQuery().Where(NewGroupCondition(And,
		NewComparison(Binary(NewQueryExpression(leaf, ""), Add, Constant{Value: 1}), GreaterThen, NewAggregate(COUNT, false, Star())),
		NewExistsCondition(leaf),
	)).GroupBy(NewQueryExpression(leaf, "g")).OrderBy(Ascending(NewQueryExpression(leaf, "o"))).SelectColumns(Column{Expression: NewQueryExpression(leaf, "v")})
	if !HasSubquery(child) {
		t.Fatal("nested forms were not inspected")
	}
}

func TestQueryTreeInspectionTraversesEveryRecursivePosition(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	derived := NewQuerySource(leaf, "d")
	from := From(derived).Join(NewJoinedSource(NewRootCollectionRef("Payment", "p"), JoinInner,
		NewComparison(NewFieldRef("d", "id"), Equal, NewFieldRef("p", "invoice"))))
	query := from.NewQuery().Where(NewGroupCondition(And,
		NewExistsCondition(leaf),
		NewComparison(NewQueryExpression(leaf, "left"), Equal, Binary(NewQueryExpression(leaf, "right"), Add, Constant{Value: 1})),
	)).Having(NewExistsCondition(leaf)).GroupBy(NewQueryExpression(leaf, "group")).OrderBy(Ascending(NewQueryExpression(leaf, "order"))).SelectColumns(
		Column{Expression: NewQueryExpression(leaf, "column")},
		Column{Expression: NewAggregate(COUNT, false, NewQueryExpression(leaf, "aggregate"))},
	)
	if inspectQueryTree(query, func(StructuredQuery) bool { return false }) {
		t.Fatal("false predicate reported a recursive node")
	}
	if !inspectQueryTree(query, func(candidate StructuredQuery) bool { return candidate.String() == leaf.String() }) {
		t.Fatal("recursive node was not reported to predicate")
	}
}

func TestRecursiveScopeValidatorsReportEveryNestedPosition(t *testing.T) {
	visible := map[string]bool{"c": true}
	visiting := map[uintptr]bool{}
	badField := NewFieldRef("missing", "id")
	conditions := []Condition{
		NewExistsCondition(nil),
		NewComparison(badField, Equal, Constant{Value: 1}),
		NewComparison(Constant{Value: 1}, Equal, badField),
		NewGroupCondition(And, NewComparison(badField, Equal, Constant{Value: 1})),
	}
	for i, condition := range conditions {
		if err := validateConditionScope(condition, visible, fmt.Sprintf("where.conditions[%d]", i), visiting); err == nil {
			t.Fatalf("condition %d unexpectedly valid", i)
		}
	}
	expressions := []Expression{
		badField,
		NewQueryExpression(nil, "q"),
		Binary(badField, Add, Constant{Value: 1}),
		Binary(Constant{Value: 1}, Add, badField),
		NewAggregate(COUNT, false, badField),
	}
	for i, expression := range expressions {
		if err := validateExpressionScope(expression, visible, fmt.Sprintf("columns[%d]", i), visiting); err == nil {
			t.Fatalf("expression %d unexpectedly valid", i)
		}
	}
	if err := validateConditionScope(nil, visible, "where", visiting); err != nil {
		t.Fatalf("nil condition: %v", err)
	}
	if err := validateExpressionScope(nil, visible, "columns[0]", visiting); err != nil {
		t.Fatalf("nil expression: %v", err)
	}
}

func TestRecursiveScopeRejectsMalformedJoinTreesBeforeExecution(t *testing.T) {
	root := NewRootCollectionRef("Customer", "c")
	cases := []StructuredQuery{
		From(root).Join(NewNestedJoinedSource(nil, JoinInner)).NewQuery().SelectIntoRecord(nil),
		From(root).Join(NewJoinedSource(NewRootCollectionRef("Invoice", "c"), JoinInner)).NewQuery().SelectIntoRecord(nil),
		From(root).Join(NewJoinedSource(NewRootCollectionRef("Invoice", "i"), JoinInner,
			NewComparison(NewFieldRef("later", "id"), Equal, NewFieldRef("i", "id")))).NewQuery().SelectIntoRecord(nil),
	}
	for i, query := range cases {
		if err := ValidateQueryScope(query); err == nil {
			t.Fatalf("malformed query %d was accepted", i)
		}
	}
}

func TestValidateQueryScope(t *testing.T) {
	inner := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "customer"), Equal, NewFieldRef("c", "id"))).SelectIntoRecordset()
	outer := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(inner)).SelectIntoRecordset()
	if err := ValidateQueryScope(outer); err != nil {
		t.Fatalf("outer correlation rejected: %v", err)
	}
	bad := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewComparison(NewFieldRef("missing", "id"), Equal, Constant{Value: 1})).SelectIntoRecordset()
	if err := ValidateQueryScope(bad); err == nil || !strings.Contains(err.Error(), "query_scope at where.left.source") {
		t.Fatalf("unknown qualifier error = %v", err)
	}
	loop := &subqueryTestQuery{}
	loop.from = From(NewQuerySource(loop, "loop"))
	if err := ValidateQueryScope(loop); err == nil || !strings.Contains(err.Error(), "query_cycle") {
		t.Fatalf("cycle error = %v", err)
	}
	if !HasSubquery(loop) {
		t.Fatal("cycle-safe inspection missed query source")
	}
	if err := ValidateQueryScope(nil); err == nil || !strings.Contains(err.Error(), "query_shape") {
		t.Fatalf("nil query error = %v", err)
	}
	noFrom := &subqueryTestQuery{}
	if err := ValidateQueryScope(noFrom); err == nil || !strings.Contains(err.Error(), "from is required") {
		t.Fatalf("no from error = %v", err)
	}
}

func TestValidateQueryScopeSourceAndExpressionErrors(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecordset()
	for name, query := range map[string]StructuredQuery{
		"derived missing query": (&subqueryTestQuery{from: From(NewQuerySource(nil, "d"))}),
		"duplicate aliases":     From(NewRootCollectionRef("Customer", "c")).Join(NewJoinedSource(NewRootCollectionRef("Invoice", "c"), JoinInner, NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("c", "id")))).NewQuery().SelectIntoRecordset(),
		"wildcard qualifier":    From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(AllColumnsExceptFrom("missing", "id")),
		"scalar qualifier":      From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: Binary(NewFieldRef("missing", "id"), Add, Constant{Value: 1})}),
		"nested scalar":         From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: NewQueryExpression(leaf, "x")}),
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateQueryScope(query)
			if name == "nested scalar" {
				if err != nil {
					t.Fatalf("nested scalar: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected scope validation error")
			}
		})
	}
}
