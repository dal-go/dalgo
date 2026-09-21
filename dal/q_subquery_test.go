package dal

import (
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
