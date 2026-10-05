package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

func TestDocumentWalkRefusesRevisitAndExcessDepth(t *testing.T) {
	expression := dal.Binary(dal.Field("x"), dal.Add, dal.NewConstant(1))
	walk := &documentWalk{depth: maxDocumentWalkDepth}
	if _, exceeded := walk.enter(expression); !exceeded {
		t.Fatal("document depth bound was not enforced")
	}
	if _, err := exprToYAMLAt(expression, walk); err == nil || !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("expression error = %v", err)
	}
	walk = &documentWalk{}
	leave, exceeded := walk.enter(&expression)
	if leave == nil || exceeded {
		t.Fatal("first visit refused")
	}
	if _, err := exprToYAMLAt(&expression, walk); err == nil || !strings.Contains(err.Error(), "holds itself") {
		t.Fatalf("repeat error = %v", err)
	}
	leave()
}

func TestAggregateShapeRefusesAmbiguousAndUnknownKeys(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"[]", "mapping"},
		{"{orderBy: [], orderBy: []}", "appear once"},
		{"{function: first, typo: true}", "not found"},
	} {
		var aggregate aggregateYAML
		if err := yaml.Unmarshal([]byte(tc.source), &aggregate); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v", tc.source, err)
		}
	}
}

func TestAggregateDocumentHelpersRefuseMalformedOrderKeys(t *testing.T) {
	for _, order := range [][]dal.OrderExpression{{nil}, {dal.Ascending(dal.NewConstant(1))}} {
		aggregate := dal.NewOrderedAggregate(dal.FIRST, order, dal.Field("x"))
		if _, err := exprToYAML(aggregate); err == nil || !strings.Contains(err.Error(), "orderBy") {
			t.Fatalf("bad order %v: %v", order, err)
		}
	}
	encoded := exprYAML{Aggregate: &aggregateYAML{Function: "first", Args: []exprYAML{{Field: "x"}}, OrderBy: []orderYAML{{exprYAML: exprYAML{Source: "i"}}}}}
	if _, err := exprFromYAMLAt(encoded, "columns[0]"); err == nil || !strings.Contains(err.Error(), "orderBy") {
		t.Fatalf("bad decoded order: %v", err)
	}
}

func TestJoinFieldWalkRefusesCyclicExpressionsAndConditions(t *testing.T) {
	from := dal.From(dal.NewRootCollectionRef("Invoice", "i")).Join(dal.NewJoinedSource(
		dal.NewRootCollectionRef("Customer", "c"), dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("i", "customer"), dal.Equal, dal.NewFieldRef("c", "id"))))
	binary := &dal.BinaryExpression{}
	binary.Left = binary
	binary.Right = dal.NewFieldRef("i", "value")
	q := fakeQuery{from: from, columns: []dal.Column{{Expression: binary}}}
	if err := validateJoinClauseFields(q); err == nil || !strings.Contains(err.Error(), "holds itself") {
		t.Fatalf("cyclic JOIN expression = %v", err)
	}
	children := make([]dal.Condition, 1)
	group := dal.NewGroupCondition(dal.And, children...)
	children[0] = &group
	q = fakeQuery{from: from, where: &group, columns: []dal.Column{{Expression: dal.NewFieldRef("i", "value")}}}
	if err := validateJoinClauseFields(q); err == nil || !strings.Contains(err.Error(), "holds itself") {
		t.Fatalf("cyclic JOIN condition = %v", err)
	}
}

func TestJoinFieldWalkChecksPointerConditions(t *testing.T) {
	from := dal.From(dal.NewRootCollectionRef("Invoice", "i")).Join(dal.NewJoinedSource(
		dal.NewRootCollectionRef("Customer", "c"), dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("i", "customer"), dal.Equal, dal.NewFieldRef("c", "id"))))
	field := dal.NewFieldRef("i", "value")
	comparison := dal.NewComparison(field, dal.Equal, dal.NewConstant(1))
	nullTest := dal.NewIsNullCondition(field)
	group := dal.NewGroupCondition(dal.And, &comparison, &nullTest)
	binary := dal.Binary(field, dal.Add, dal.NewConstant(1))
	q := fakeQuery{from: from, where: &group, columns: []dal.Column{{Expression: &binary}}}
	if err := validateJoinClauseFields(q); err != nil {
		t.Fatal(err)
	}
}

func TestJoinFieldWalkRefusesExcessExpressionAndConditionDepth(t *testing.T) {
	from := dal.From(dal.NewRootCollectionRef("Invoice", "i")).Join(dal.NewJoinedSource(
		dal.NewRootCollectionRef("Customer", "c"), dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("i", "customer"), dal.Equal, dal.NewFieldRef("c", "id"))))
	var expression dal.Expression = dal.NewFieldRef("i", "value")
	for range maxDocumentWalkDepth + 1 {
		expression = dal.Binary(expression, dal.Add, dal.NewConstant(1))
	}
	if err := validateJoinClauseFields(fakeQuery{from: from, columns: []dal.Column{{Expression: expression}}}); err == nil || !strings.Contains(err.Error(), "expression is too deep") {
		t.Fatalf("deep JOIN expression = %v", err)
	}
	var condition dal.Condition = dal.NewComparison(dal.NewFieldRef("i", "value"), dal.Equal, dal.NewConstant(1))
	for range maxDocumentWalkDepth + 1 {
		condition = dal.NewGroupCondition(dal.And, condition)
	}
	if err := validateJoinClauseFields(fakeQuery{from: from, where: condition, columns: []dal.Column{{Expression: dal.NewFieldRef("i", "value")}}}); err == nil || !strings.Contains(err.Error(), "condition is too deep") {
		t.Fatalf("deep JOIN condition = %v", err)
	}
}

type lateChangingDocumentQuery struct {
	fakeQuery
	flipGroupAt, flipHavingAt int
	groupCalls, havingCalls   int
}

func (q *lateChangingDocumentQuery) GroupBy() []dal.Expression {
	q.groupCalls++
	if q.flipGroupAt > 0 && q.groupCalls >= q.flipGroupAt {
		return []dal.Expression{unsupportedExpr{}}
	}
	return q.groupBy
}

func (q *lateChangingDocumentQuery) Having() dal.Condition {
	q.havingCalls++
	if q.flipHavingAt > 0 && q.havingCalls >= q.flipHavingAt {
		return unsupportedCond{}
	}
	return q.having
}

func TestDocumentWriterPropagatesLateInvalidGroupingAndHaving(t *testing.T) {
	count := dal.Count()
	for _, tc := range []struct {
		name  string
		base  fakeQuery
		want  string
		group bool
	}{
		{"group", fakeQuery{from: rootFrom(), groupBy: []dal.Expression{dal.Field("category")}, columns: []dal.Column{{Expression: dal.Field("category")}}}, "groupBy #0", true},
		{"having", fakeQuery{from: rootFrom(), columns: []dal.Column{count}, having: dal.NewComparison(count.Expression, dal.GreaterThen, dal.NewConstant(0))}, "having: unsupported condition", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for threshold := 2; threshold <= 30; threshold++ {
				q := &lateChangingDocumentQuery{fakeQuery: tc.base}
				if tc.group {
					q.flipGroupAt = threshold
				} else {
					q.flipHavingAt = threshold
				}
				_, err := Serialize(q)
				if err != nil && strings.Contains(err.Error(), tc.want) && !strings.Contains(err.Error(), "invalid aggregation") {
					return
				}
			}
			t.Fatalf("no validation-to-serialization transition propagated %s error", tc.name)
		})
	}
}
