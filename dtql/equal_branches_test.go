package dtql

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestEqual_nilCases(t *testing.T) {
	q := fakeQuery{from: rootFrom()}
	cases := []struct {
		a, b dal.StructuredQuery
		want bool
	}{
		{nil, nil, true},
		{nil, q, false},
		{q, nil, false},
	}
	for i, c := range cases {
		if got := Equal(c.a, c.b); got != c.want {
			t.Errorf("case #%d: Equal = %v, want %v", i, got, c.want)
		}
	}
}

func TestEqual_topLevelDifferences(t *testing.T) {
	base := fakeQuery{
		from:    rootFrom(),
		columns: []dal.Column{{Expression: dal.Field("name")}},
		where:   dal.WhereField("age", dal.Equal, 18),
		orderBy: []dal.OrderExpression{dal.AscendingField("name")},
		limit:   10,
		offset:  20,
	}
	diffs := map[string]fakeQuery{
		"offset":  {from: rootFrom(), columns: base.columns, where: base.where, orderBy: base.orderBy, limit: 10, offset: 99},
		"columns": {from: rootFrom(), columns: []dal.Column{{Expression: dal.Field("other")}}, where: base.where, orderBy: base.orderBy, limit: 10, offset: 20},
		"order":   {from: rootFrom(), columns: base.columns, where: base.where, orderBy: []dal.OrderExpression{dal.DescendingField("name")}, limit: 10, offset: 20},
		"where":   {from: rootFrom(), columns: base.columns, where: dal.WhereField("age", dal.Equal, 99), orderBy: base.orderBy, limit: 10, offset: 20},
	}
	for name, d := range diffs {
		if Equal(base, d) {
			t.Errorf("%s difference: expected Equal to be false", name)
		}
	}
	if !Equal(base, base) {
		t.Error("identical queries should be Equal")
	}
}

func TestFromEqual(t *testing.T) {
	root := rootFrom()
	other := dal.From(dal.NewRootCollectionRef("orders", ""))
	groupRef := dal.From(dal.NewCollectionGroupRef("users", ""))
	cases := []struct {
		name string
		a, b dal.FromSource
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, root, false},
		{"b nil", root, nil, false},
		{"a not root collection", groupRef, root, false},
		{"b not root collection", root, groupRef, false},
		{"different name", root, other, false},
		{"equal", root, rootFrom(), true},
	}
	for _, c := range cases {
		if got := fromEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: fromEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestColumnsEqual(t *testing.T) {
	col := func(name, alias string) dal.Column { return dal.Column{Expression: dal.Field(name), Alias: alias} }
	cases := []struct {
		name string
		a, b []dal.Column
		want bool
	}{
		{"len differ", []dal.Column{col("a", "")}, []dal.Column{col("a", ""), col("b", "")}, false},
		{"alias differ", []dal.Column{col("a", "x")}, []dal.Column{col("a", "y")}, false},
		{"expr differ", []dal.Column{col("a", "")}, []dal.Column{col("b", "")}, false},
		{"equal", []dal.Column{col("a", "x")}, []dal.Column{col("a", "x")}, true},
	}
	for _, c := range cases {
		if got := columnsEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: columnsEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOrderEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b []dal.OrderExpression
		want bool
	}{
		{"len differ", []dal.OrderExpression{dal.AscendingField("a")}, nil, false},
		{"desc differ", []dal.OrderExpression{dal.AscendingField("a")}, []dal.OrderExpression{dal.DescendingField("a")}, false},
		{"expr differ", []dal.OrderExpression{dal.AscendingField("a")}, []dal.OrderExpression{dal.AscendingField("b")}, false},
		{"equal", []dal.OrderExpression{dal.AscendingField("a")}, []dal.OrderExpression{dal.AscendingField("a")}, true},
	}
	for _, c := range cases {
		if got := orderEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: orderEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCondEqual(t *testing.T) {
	cmp := dal.NewComparison(dal.Field("a"), dal.Equal, dal.Constant{Value: 1})
	grp := dal.NewGroupCondition(dal.And, cmp)
	cases := []struct {
		name string
		a, b dal.Condition
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil b set", nil, cmp, false},
		{"comparison equal", cmp, cmp, true},
		{"group equal", grp, grp, true},
		{"unsupported condition", unsupportedCond{}, nil, false},
	}
	for _, c := range cases {
		if got := condEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: condEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestComparisonAndGroupEqual(t *testing.T) {
	cmp := dal.NewComparison(dal.Field("a"), dal.Equal, dal.Constant{Value: 1})
	cmp2 := dal.NewComparison(dal.Field("a"), dal.GreaterThen, dal.Constant{Value: 1})
	grpAnd := dal.NewGroupCondition(dal.And, cmp)
	grpOr := dal.NewGroupCondition(dal.Or, cmp)
	grpTwo := dal.NewGroupCondition(dal.And, cmp, cmp)
	grpBadChild := dal.NewGroupCondition(dal.And, cmp2)

	if comparisonEqual(cmp, grpAnd) {
		t.Error("comparisonEqual against a non-comparison should be false")
	}
	if groupEqual(grpAnd, cmp) {
		t.Error("groupEqual against a non-group should be false")
	}
	if groupEqual(grpAnd, grpOr) {
		t.Error("groupEqual with different operators should be false")
	}
	if groupEqual(grpAnd, grpTwo) {
		t.Error("groupEqual with different lengths should be false")
	}
	if groupEqual(grpAnd, grpBadChild) {
		t.Error("groupEqual with a differing child should be false")
	}
	if !groupEqual(grpAnd, grpAnd) {
		t.Error("identical groups should be equal")
	}
}

func TestExprEqual(t *testing.T) {
	field := dal.Field("a")
	constant := dal.Constant{Value: 1}
	array := dal.NewArray([]int{1, 2})
	cases := []struct {
		name string
		a, b dal.Expression
		want bool
	}{
		{"field vs constant", field, constant, false},
		{"constant vs field", constant, field, false},
		{"array vs field", array, field, false},
		{"unsupported", unsupportedExpr{}, unsupportedExpr{}, false},
		{"field equal", field, dal.Field("a"), true},
		{"constant equal", constant, dal.Constant{Value: 1}, true},
		{"array equal", array, dal.NewArray([]int{1, 2}), true},
	}
	for _, c := range cases {
		if got := exprEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: exprEqual = %v, want %v", c.name, got, c.want)
		}
	}
}
