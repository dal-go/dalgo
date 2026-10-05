package dal

import (
	"context"
	"strings"
	"testing"
)

// The scope checks are also called by recursive and federated routes. A tree
// stopped by either guard must be refused before those routes can read it.
func TestScopeWalkGuardsRefuseCyclesAndExcessDepth(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	queryValue := From(base).NewQuery().SelectColumns(Column{Expression: Field("Total")}).(structuredQuery)
	query := &queryValue
	fromValue := From(base)
	condition := NewComparison(Field("Total"), Equal, NewConstant(1))
	expression := Binary(Field("Total"), Add, NewConstant(1))
	checks := []struct {
		name string
		id   uintptr
		call func(*queryTreePath) error
	}{
		{"query", queryPointerID(query), func(w *queryTreePath) error { return validateQueryScopeAt(query, nil, "query", map[uintptr]bool{}, w) }},
		{"source", nodePointerID(fromValue), func(w *queryTreePath) error {
			return validateFromScopeAt(fromValue, nil, map[string]bool{}, "from", map[uintptr]bool{}, w)
		}},
		{"condition", nodePointerID(&condition), func(w *queryTreePath) error {
			return validateConditionScopeAt(&condition, nil, "where", map[uintptr]bool{}, w)
		}},
		{"expression", nodePointerID(&expression), func(w *queryTreePath) error {
			return validateExpressionScopeAt(&expression, nil, "column", map[uintptr]bool{}, w)
		}},
	}
	for _, check := range checks {
		t.Run(check.name+" depth", func(t *testing.T) {
			if err := check.call(&queryTreePath{depth: maxQueryTreeDepth}); err == nil || !strings.Contains(err.Error(), "too deep") {
				t.Fatalf("error = %v", err)
			}
		})
		if check.id != 0 {
			t.Run(check.name+" cycle", func(t *testing.T) {
				if err := check.call(&queryTreePath{seen: map[uintptr]bool{check.id: true}}); err == nil || !strings.Contains(err.Error(), "cycle") {
					t.Fatalf("error = %v", err)
				}
			})
		}
	}
	if free := queryFreeReferencesAt(query, map[uintptr]bool{queryPointerID(query): true}, &queryTreePath{}); !free[""] {
		t.Fatalf("previsited query was treated as uncorrelated: %v", free)
	}
	if err := validateQueryScopeAt(query, nil, "query", map[uintptr]bool{queryPointerID(query): true}, &queryTreePath{}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("previsited query scope = %v", err)
	}
}

func TestFreeReferenceWalkKeepsPointerFormsAndStoppedGraphsCorrelated(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	child := From(base).NewQuery().SelectColumns(Column{Expression: NewFieldRef("outside", "x")})
	binary := Binary(NewFieldRef("i", "x"), Add, NewConstant(1))
	subquery := NewQueryExpression(child, "x")
	nullTest := NewIsNullCondition(&binary)
	exists := NewExistsCondition(child)
	comparison := NewComparison(&subquery, Equal, NewConstant(1))
	group := NewGroupCondition(And, &nullTest, &exists, &comparison)
	q := From(base).NewQuery().Where(&group).SelectColumns(Column{Expression: &binary})
	if free := queryFreeReferences(q, map[uintptr]bool{}); !free["outside"] {
		t.Fatalf("pointer subtree lost outer reference: %v", free)
	}
	children := make([]Condition, 1)
	cycle := NewGroupCondition(And, children...)
	children[0] = &cycle
	q = From(base).NewQuery().Where(&cycle).SelectColumns(Column{Expression: Field("x")})
	if free := queryFreeReferences(q, map[uintptr]bool{}); !free[""] {
		t.Fatalf("condition cycle was treated as uncorrelated: %v", free)
	}
	visits := 0
	from := &countingFrom{base: base, visits: &visits}
	from.joins = []JoinedSource{NewNestedJoinedSource(from, JoinInner, NewComparison(NewFieldRef("i", "x"), Equal, NewConstant(1)))}
	q = from.NewQuery().SelectColumns(Column{Expression: Field("x")})
	if free := queryFreeReferences(q, map[uintptr]bool{}); !free[""] {
		t.Fatalf("source cycle was treated as uncorrelated: %v", free)
	}
}

func TestFreeReferenceWalkStopsAcrossSiblingBranchesAfterFirstCut(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	visits, siblingVisits := 0, 0
	args := make([]Expression, 2)
	cyclic := valueAggregate{args: args, visits: &visits}
	args[0], args[1] = cyclic, cyclic
	sibling := &countingAggregate{visits: &siblingVisits}
	q := From(base).NewQuery().Where(NewGroupCondition(And,
		NewComparison(cyclic, Equal, NewConstant(1)),
		NewComparison(sibling, Equal, NewConstant(1)),
	)).SelectColumns(Column{Expression: sibling})
	// Only a few levels remain. Without a shared cutoff, the repeated value
	// aggregate explores every path and the later condition/column are visited.
	walk := &queryTreePath{depth: maxQueryTreeDepth - 8}
	free := queryFreeReferencesAt(q, map[uintptr]bool{}, walk)
	if !free[""] || visits > 8 || siblingVisits != 0 {
		t.Fatalf("free = %v, cyclic visits = %d, sibling visits = %d", free, visits, siblingVisits)
	}
}

func TestFreeReferenceWalkPropagatesNestedCuts(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	visits, siblingVisits, sourceVisits := 0, 0, 0
	args := make([]Expression, 2)
	cyclic := valueAggregate{args: args, visits: &visits}
	args[0], args[1] = cyclic, cyclic
	child := From(base).NewQuery().Where(NewComparison(cyclic, Equal, NewConstant(1))).SelectKeysOnly(0)
	sibling := &countingAggregate{visits: &siblingVisits}
	source := &countingFrom{base: NewRootCollectionRef("Other", "o"), visits: &sourceVisits}
	queries := []struct {
		name  string
		query StructuredQuery
	}{
		{"nested condition", From(base).NewQuery().Where(NewGroupCondition(And,
			NewExistsCondition(child), NewComparison(sibling, Equal, NewConstant(1)),
		)).SelectColumns(Column{Expression: sibling})},
		{"derived source", From(NewQuerySource(child, "d")).Join(NewNestedJoinedSource(source, JoinInner,
			NewComparison(sibling, Equal, NewConstant(1)))).NewQuery().SelectColumns(Column{Expression: sibling})},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			visits, siblingVisits, sourceVisits = 0, 0, 0
			free := queryFreeReferencesAt(tc.query, map[uintptr]bool{}, &queryTreePath{depth: maxQueryTreeDepth - 10})
			if !free[""] || visits > 10 || siblingVisits != 0 || sourceVisits != 0 {
				t.Fatalf("free = %v, cyclic visits = %d, sibling visits = %d, source visits = %d", free, visits, siblingVisits, sourceVisits)
			}
		})
	}
}

func TestFreeReferenceWalkStopsAfterCutsInOrderAndJoinClauses(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	other := NewRootCollectionRef("Customer", "c")
	visits, siblingVisits := 0, 0
	args := make([]Expression, 2)
	cyclic := valueAggregate{args: args, visits: &visits}
	args[0], args[1] = cyclic, cyclic
	sibling := &countingAggregate{visits: &siblingVisits}
	compare := func(expr Expression) Condition { return NewComparison(expr, Equal, NewConstant(1)) }
	queries := []struct {
		name  string
		query StructuredQuery
	}{
		{"aggregate order keys", From(base).NewQuery().SelectColumns(Column{Expression: NewOrderedAggregate(LAST, []OrderExpression{Ascending(cyclic), Ascending(sibling)}, Field("amount"))})},
		{"join ON conditions", From(base).Join(NewJoinedSource(other, JoinInner, compare(cyclic), compare(sibling))).
			NewQuery().SelectColumns(Column{Expression: sibling})},
		{"select before group and order", From(base).NewQuery().GroupBy(sibling).OrderBy(Ascending(sibling)).
			SelectColumns(Column{Expression: cyclic})},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			visits, siblingVisits = 0, 0
			free := queryFreeReferencesAt(tc.query, map[uintptr]bool{}, &queryTreePath{depth: maxQueryTreeDepth - 10})
			if !free[""] || visits > 10 || siblingVisits != 0 {
				t.Fatalf("free = %v, cyclic visits = %d, sibling visits = %d", free, visits, siblingVisits)
			}
		})
	}
}

func TestJoinFieldValidationRefusesStoppedWalksAndAcceptsPointerForms(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	makeExecution := func(q StructuredQuery) *joinExecution {
		return &joinExecution{q: q, aliases: []string{"i"}, fields: map[string][]string{"i": {"x"}}}
	}
	binary := Binary(NewFieldRef("i", "x"), Add, NewConstant(1))
	nullTest := NewIsNullCondition(&binary)
	comparison := NewComparison(NewFieldRef("i", "x"), Equal, NewConstant(1))
	q := From(base).NewQuery().Where(NewGroupCondition(And, &nullTest, &comparison)).SelectColumns(Column{Alias: "x", Expression: &binary})
	if err := makeExecution(q).validateQueryFields(); err != nil {
		t.Fatalf("pointer fields = %v", err)
	}
	deep := Expression(NewFieldRef("i", "x"))
	for range maxQueryTreeDepth + 1 {
		deep = Binary(deep, Add, NewConstant(1))
	}
	q = From(base).NewQuery().SelectColumns(Column{Alias: "x", Expression: deep})
	if err := makeExecution(q).validateQueryFields(); err == nil || !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("deep expression = %v", err)
	}
	children := make([]Condition, 1)
	group := NewGroupCondition(And, children...)
	children[0] = group
	q = From(base).NewQuery().Where(group).SelectColumns(Column{Expression: NewFieldRef("i", "x")})
	if err := makeExecution(q).validateQueryFields(); err == nil || !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("deep condition = %v", err)
	}
}

func TestSimpleRecursiveReadChecksOrderedPlacementBeforeProvider(t *testing.T) {
	q := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(
		NewComparison(NewOrderedAggregate(FIRST, []OrderExpression{AscendingField("at")}, Field("value")), Equal, NewConstant(1))).
		SelectColumns(Column{Expression: Field("value")})
	execution := &joinExecution{ctx: context.Background()}
	if _, err := execution.executeSimpleCapped(q, nil, 1, true); err == nil || !strings.Contains(err.Error(), "cannot stand in where") {
		t.Fatalf("recursive read placement = %v", err)
	}
}

func TestNonMoneyDecimalOrderLeavesStringOperandsAsText(t *testing.T) {
	if _, ok := orderedDecimalValue("10", false); ok {
		t.Fatal("ordinary string became a decimal")
	}
}

func TestScopeWalkAcceptsPointerConditionAndExpressionForms(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: Field("Total")})
	plain := NewComparison(Field("Total"), Equal, NewConstant(1))
	isNull := NewIsNullCondition(Field("Total"))
	group := NewGroupCondition(And, &plain, &isNull)
	exists := NewExistsCondition(query)
	conditions := []Condition{&plain, &isNull, &group, &exists}
	for _, condition := range conditions {
		if err := validateConditionScope(condition, map[string]bool{"i": true}, "where", map[uintptr]bool{}); err != nil {
			t.Fatalf("%T: %v", condition, err)
		}
	}
	binary := Binary(Field("Total"), Add, NewConstant(1))
	subquery := NewQueryExpression(query, "v")
	for _, expression := range []Expression{&binary, &subquery} {
		if err := validateExpressionScope(expression, map[string]bool{"i": true}, "column", map[uintptr]bool{}); err != nil {
			t.Fatalf("%T: %v", expression, err)
		}
	}
}
