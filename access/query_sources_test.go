package access

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// shapeQuery is a structured query whose parts are set directly, so a test can
// build shapes the query builder does not: a missing source, a query that
// contains itself.
type shapeQuery struct {
	from    dal.FromSource
	where   dal.Condition
	having  dal.Condition
	columns []dal.Column
	groupBy []dal.Expression
	orderBy []dal.OrderExpression
}

func (*shapeQuery) String() string { return "shape" }
func (*shapeQuery) Offset() int    { return 0 }
func (*shapeQuery) Limit() int     { return 0 }
func (*shapeQuery) GetRecordsReader(context.Context, dal.QueryExecutor) (dal.RecordsReader, error) {
	return nil, nil
}
func (*shapeQuery) GetRecordsetReader(context.Context, dal.QueryExecutor) (dal.RecordsetReader, error) {
	return nil, nil
}
func (q *shapeQuery) From() dal.FromSource           { return q.from }
func (q *shapeQuery) Where() dal.Condition           { return q.where }
func (q *shapeQuery) GroupBy() []dal.Expression      { return q.groupBy }
func (q *shapeQuery) Having() dal.Condition          { return q.having }
func (q *shapeQuery) OrderBy() []dal.OrderExpression { return q.orderBy }
func (q *shapeQuery) Columns() []dal.Column          { return q.columns }
func (*shapeQuery) IntoRecord() record.Record        { return nil }
func (*shapeQuery) IDKind() reflect.Kind             { return reflect.String }
func (*shapeQuery) StartFrom() dal.Cursor            { return "" }
func (*shapeQuery) StartAfter() dal.Cursor           { return "" }

var _ dal.StructuredQuery = (*shapeQuery)(nil)

// strangeNode is an expression and a condition the walk has no case for.
type strangeNode struct{}

func (strangeNode) String() string { return "strange" }

// starAggregate is an aggregate that also marks itself as a star: a type that
// satisfies both dal.AggregateFunc and dal.StarExpression.
type starAggregate struct{ args []dal.Expression }

func (starAggregate) String() string               { return "star aggregate" }
func (starAggregate) IsStar() bool                 { return true }
func (starAggregate) FuncName() string             { return dal.COUNT }
func (a starAggregate) FuncArgs() []dal.Expression { return a.args }

// wrappedSource is a source type the walk has no case for: it satisfies
// dal.RecordsetSource through the collection it embeds.
type wrappedSource struct{ dal.CollectionRef }

// strangeOrder is an order expression whose expression is a strangeNode.
type strangeOrder struct{}

func (strangeOrder) String() string             { return "strange order" }
func (strangeOrder) Expression() dal.Expression { return strangeNode{} }
func (strangeOrder) Descending() bool           { return false }

func resourceNames(resources []Resource) []string {
	names := make([]string, len(resources))
	for i, resource := range resources {
		names[i] = resource.String()
	}
	return names
}

func fromQuery(from dal.FromSource) *shapeQuery { return &shapeQuery{from: from} }

func TestResourcesForQueryListsEverySource(t *testing.T) {
	secretQuery := secretRows()
	exists := dal.NewExistsCondition(secretQuery)
	scalar := dal.NewQueryExpression(secretQuery, "x")
	pointerScalar := &scalar
	ref := dal.NewRootCollectionRef("Customer", "c")
	group := dal.NewCollectionGroupRef("Notes", "n")
	cases := []struct {
		name  string
		query dal.Query
		want  []string
	}{
		{"one collection", dal.From(ref).NewQuery().SelectKeysOnly(reflect.String), []string{"/Customer"}},
		{"a collection through a pointer", dal.From(&ref).NewQuery().SelectKeysOnly(reflect.String), []string{"/Customer"}},
		{"a collection group", dal.From(group).NewQuery().SelectKeysOnly(reflect.String), []string{"collection-group:Notes"}},
		{"a collection group through a pointer", dal.From(&group).NewQuery().SelectKeysOnly(reflect.String), []string{"collection-group:Notes"}},
		{"a first-level join", dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner, joinOn("c", "o"))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order"}},
		{"a join tree lists its base once", dal.From(customers).Join(dal.NewJoinedFrom(dal.From(orders), dal.JoinInner, joinOn("c", "o"))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order"}},
		{"joins at depth two and three, then a sibling join", dal.From(customers).
			Join(dal.NewJoinedFrom(dal.From(orders).
				Join(dal.NewJoinedFrom(dal.From(lines).Join(dal.NewJoinedSource(hidden, dal.JoinInner, joinOn("l", "s"))), dal.JoinInner, joinOn("o", "l"))), dal.JoinInner, joinOn("c", "o"))).
			Join(dal.NewJoinedSource(dal.NewRootCollectionRef("Tag", "t"), dal.JoinInner, joinOn("c", "t"))).
			NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order", "/Line", "/Secret", "/Tag"}},
		{"a derived source reads what its query reads", dal.From(dal.NewQuerySource(secretQuery, "d")).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Secret"}},
		{"a derived source through a pointer", func() dal.Query {
			source := dal.NewQuerySource(secretQuery, "d")
			return dal.From(&source).NewQuery().SelectKeysOnly(reflect.String)
		}(), []string{"/Secret"}},
		{"derived sources nest", dal.From(dal.NewQuerySource(
			dal.From(dal.NewQuerySource(secretQuery, "inner")).NewQuery().SelectKeysOnly(reflect.String), "outer")).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Secret"}},
		{"a derived source in a join", dal.From(customers).Join(dal.NewJoinedSource(dal.NewQuerySource(secretQuery, "d"), dal.JoinInner, joinOn("c", "d"))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Secret"}},
		{"EXISTS", customerQuery().Where(exists).SelectKeysOnly(reflect.String), []string{"/Customer", "/Secret"}},
		{"NOT EXISTS", customerQuery().Where(dal.NewNotExistsCondition(secretQuery)).SelectKeysOnly(reflect.String), []string{"/Customer", "/Secret"}},
		{"EXISTS through a pointer", customerQuery().Where(&exists).SelectKeysOnly(reflect.String), []string{"/Customer", "/Secret"}},
		{"a scalar subquery through a pointer", customerQuery().SelectColumns(dal.Column{Expression: pointerScalar}), []string{"/Customer", "/Secret"}},
		{"a null test of a scalar subquery", customerQuery().Where(dal.NewIsNotNullCondition(scalar)).SelectKeysOnly(reflect.String), []string{"/Customer", "/Secret"}},
		{"a null test through a pointer", func() dal.Query {
			test := dal.NewIsNullCondition(scalar)
			return customerQuery().Where(&test).SelectKeysOnly(reflect.String)
		}(), []string{"/Customer", "/Secret"}},
		{"a comparison through a pointer", func() dal.Query {
			comparison := dal.NewComparison(dal.Field("a"), dal.Equal, scalar)
			return customerQuery().Where(&comparison).SelectKeysOnly(reflect.String)
		}(), []string{"/Customer", "/Secret"}},
		{"a condition group through a pointer", func() dal.Query {
			group := dal.NewGroupCondition(dal.Or, dal.WhereField("a", dal.Equal, 1), exists)
			return customerQuery().Where(&group).SelectKeysOnly(reflect.String)
		}(), []string{"/Customer", "/Secret"}},
		{"an arithmetic operand through a pointer", func() dal.Query {
			sum := dal.Binary(dal.Field("a"), dal.Add, scalar)
			return customerQuery().SelectColumns(dal.Column{Expression: &sum})
		}(), []string{"/Customer", "/Secret"}},
		{"an aggregate argument", customerQuery().SelectColumns(dal.SumAs(dal.Binary(dal.Field("a"), dal.Multiply, scalar), "t")), []string{"/Customer", "/Secret"}},
		{"an aggregate that is also a star holds its arguments' sources", customerQuery().SelectColumns(dal.Column{Expression: starAggregate{args: []dal.Expression{scalar}}}),
			[]string{"/Customer", "/Secret"}},
		{"a join ON", dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner, dal.NewComparison(dal.Field("a"), dal.Equal, scalar))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order", "/Secret"}},
		{"a scan order", dal.From(customers.WithScan(5, dal.Descending(scalar))).NewQuery().SelectKeysOnly(reflect.String), []string{"/Customer", "/Secret"}},
		{"a scan order of a joined source", dal.From(customers).Join(dal.NewJoinedSource(orders.WithScan(5, dal.Ascending(scalar)), dal.JoinInner, joinOn("c", "o"))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order", "/Secret"}},
		{"a scan order through a pointer", func() dal.Query {
			scanned := customers.WithScan(5, dal.Ascending(scalar))
			return dal.From(&scanned).NewQuery().SelectKeysOnly(reflect.String)
		}(), []string{"/Customer", "/Secret"}},
		{"leaf expressions hold no source", customerQuery().
			Where(dal.WhereField("a", dal.In, []string{"x"})).
			GroupBy(dal.FieldName("b"), &dal.Constant{Value: 1}, dal.NewParam("p"), &dal.Param{Name: "q"}, dal.NewArray([]int{1}), &dal.Array{Value: []int{2}}, &dal.FieldRef{}).
			SelectColumns(dal.Count(), dal.Column{Expression: dal.Star()}),
			[]string{"/Customer"}},
		{"parts are met in the order columns, where, group by, having, order by", func() dal.Query {
			in := func(name string) dal.Expression {
				return dal.NewQueryExpression(dal.From(dal.NewRootCollectionRef(name, "")).NewQuery().SelectKeysOnly(reflect.String), "x")
			}
			return customerQuery().
				OrderBy(dal.Ascending(in("ByOrder"))).
				Having(dal.NewComparison(in("InHaving"), dal.Equal, dal.Constant{Value: 1})).
				GroupBy(in("ByGroup")).
				Where(dal.NewComparison(in("InWhere"), dal.Equal, dal.Constant{Value: 1})).
				SelectColumns(dal.Column{Expression: in("InColumns")})
		}(), []string{"/Customer", "/InColumns", "/InWhere", "/ByGroup", "/InHaving", "/ByOrder"}},
		{"a source shared by two branches is listed for each", customerQuery().
			Where(dal.NewGroupCondition(dal.And, exists, exists)).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Secret", "/Secret"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resourceNames(resourcesForQuery(c.query)); !slices.Equal(got, c.want) {
				t.Fatalf("resources = %v, want %v", got, c.want)
			}
		})
		// The same shapes decide the route a query takes: a query that holds a
		// nested query anywhere the walk finds one is executed as a nested query.
		t.Run(c.name+" is routed as the walk finds it", func(t *testing.T) {
			structured := c.query.(dal.StructuredQuery)
			walk := querySourceWalk{onPath: map[uintptr]bool{}}
			walk.query(structured, 0)
			if got, want := dal.HasSubquery(structured), walk.queries > 1; got != want {
				t.Fatalf("HasSubquery = %v, but the walk entered %d queries", got, walk.queries)
			}
		})
	}
}

func TestResourcesForQueryOfNonStructuredQueryIsOpaque(t *testing.T) {
	if got := resourcesForQuery(opaqueQ{}); len(got) != 1 || got[0].Kind() != OpaqueQueryResource {
		t.Fatalf("resources = %v", got)
	}
}

func TestResourcesForQueryUnknownShapesAreOpaque(t *testing.T) {
	var nilQuery *shapeQuery
	var nilQuerySource *dal.QuerySource
	var nilCollection *dal.CollectionRef
	var nilFrom dal.FromSource
	otherJoin := dal.NewJoinedFrom(dal.From(orders), dal.JoinInner, joinOn("c", "o"))
	otherJoin.RecordsetSource = hidden
	cases := []struct {
		name  string
		query dal.StructuredQuery
		want  []string
	}{
		{"an unrecognised condition", customerQuery().Where(strangeNode{}).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:unrecognised query node access.strangeNode"}},
		{"an unrecognised expression inside a comparison", customerQuery().Where(dal.NewComparison(strangeNode{}, dal.Equal, dal.Constant{Value: 1})).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:unrecognised query node access.strangeNode"}},
		{"an unrecognised order expression", customerQuery().OrderBy(strangeOrder{}).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:unrecognised query node access.strangeNode"}},
		{"a query without a from", &shapeQuery{}, []string{"opaque-query:missing query or source tree"}},
		{"a from without a source", fromQuery(dal.From(nil)), []string{"opaque-query:<nil>"}},
		{"a nil pointer where a source is", fromQuery(dal.From(nilCollection)), []string{"opaque-query:<nil>"}},
		{"a nil derived source", fromQuery(dal.From(nilQuerySource)), []string{"opaque-query:missing derived source"}},
		{"a derived source without a query", fromQuery(dal.From(dal.NewQuerySource(nil, "d"))), []string{"opaque-query:missing query or source tree"}},
		{"EXISTS without a query", customerQuery().Where(dal.NewExistsCondition(nil)).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:missing query or source tree"}},
		{"a scalar subquery with a nil pointer query", customerQuery().Where(dal.NewComparison(dal.Field("a"), dal.Equal, dal.NewQueryExpression(nilQuery, "x"))).SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:missing query or source tree"}},
		{"a join without a source", dal.From(customers).Join(dal.NewNestedJoinedSource(nilFrom, dal.JoinInner, joinOn("c", "o"))).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "opaque-query:<nil>"}},
		{"a join tree that differs from the join's source", dal.From(customers).Join(otherJoin).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"/Customer", "/Order", "/Secret"}},
		{"a source type the walk does not know", fromQuery(dal.From(wrappedSource{customers})),
			[]string{"opaque-query:Customer AS c"}},
		{"a qualified collection", dal.From(dal.NewQualifiedRootCollectionRef("private", "Customer", "")).NewQuery().SelectKeysOnly(reflect.String),
			[]string{"opaque-query:private.Customer"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resourceNames(resourcesForQuery(c.query))
			if !slices.Equal(got, c.want) {
				t.Fatalf("resources = %v, want %v", got, c.want)
			}
		})
	}
}

func TestResourcesForQueryMissingExpressionsHoldNoSource(t *testing.T) {
	var nilComparison *dal.Comparison
	var nilScalar *dal.QueryExpression
	query := customerQuery().
		Where(nilComparison).
		OrderBy(nil).
		GroupBy(nilScalar, nil).
		SelectColumns(dal.Column{}, dal.Column{Expression: nilScalar})
	if got := resourceNames(resourcesForQuery(query)); !slices.Equal(got, []string{"/Customer"}) {
		t.Fatalf("resources = %v", got)
	}
}

func TestResourcesForQueryCycleIsAnUnknownShape(t *testing.T) {
	t.Run("a query that contains itself", func(t *testing.T) {
		query := &shapeQuery{from: dal.From(customers)}
		query.where = dal.NewExistsCondition(query)
		got := resourceNames(resourcesForQuery(query))
		if want := []string{"/Customer", "opaque-query:query refers to itself"}; !slices.Equal(got, want) {
			t.Fatalf("resources = %v, want %v", got, want)
		}
	})
	t.Run("a source tree that joins itself", func(t *testing.T) {
		tree := dal.From(customers)
		tree.Join(dal.NewJoinedFrom(tree, dal.JoinInner, joinOn("c", "c")))
		got := resourceNames(resourcesForQuery(tree.NewQuery().SelectKeysOnly(reflect.String)))
		if !slices.Contains(got, "opaque-query:query refers to itself") {
			t.Fatalf("resources = %v", got)
		}
	})
	t.Run("an expression that contains itself", func(t *testing.T) {
		sum := &dal.BinaryExpression{Operator: dal.Add, Right: dal.Constant{Value: 1}}
		sum.Left = sum
		got := resourceNames(resourcesForQuery(customerQuery().SelectColumns(dal.Column{Expression: sum})))
		if want := []string{"/Customer", "opaque-query:query refers to itself"}; !slices.Equal(got, want) {
			t.Fatalf("resources = %v, want %v", got, want)
		}
	})
	t.Run("a node reached twice by different branches is not a cycle", func(t *testing.T) {
		shared := &dal.Comparison{Left: dal.Field("a"), Operator: dal.Equal, Right: dal.Constant{Value: 1}}
		query := customerQuery().Where(dal.NewGroupCondition(dal.And, shared, shared)).SelectKeysOnly(reflect.String)
		if got := resourceNames(resourcesForQuery(query)); !slices.Equal(got, []string{"/Customer"}) {
			t.Fatalf("resources = %v", got)
		}
	})
}

func TestResourcesForQueryNestingIsBounded(t *testing.T) {
	const tooDeep = "opaque-query:query nested too deeply"
	nest := func(levels int) dal.Query {
		var expression dal.Expression = dal.Field("a")
		for i := 0; i < levels; i++ {
			expression = dal.Binary(expression, dal.Add, dal.Constant{Value: 1})
		}
		return customerQuery().SelectColumns(dal.Column{Expression: expression})
	}
	// The outermost operator is one level below the query and every operator
	// adds one, so the innermost operand sits at depth levels+1.
	if got := resourceNames(resourcesForQuery(nest(maxQueryNesting - 1))); !slices.Equal(got, []string{"/Customer"}) {
		t.Fatalf("an expression at the depth limit: resources = %v", got)
	}
	if got := resourceNames(resourcesForQuery(nest(maxQueryNesting))); got[0] != "/Customer" || !slices.Contains(got, tooDeep) {
		t.Fatalf("an expression past the depth limit: resources = %v", got)
	}
	// Subqueries nested past the limit are not analysed either.
	query := secretRows()
	for i := 0; i < maxQueryNesting; i++ {
		query = customerQuery().Where(dal.NewExistsCondition(query)).SelectKeysOnly(reflect.String)
	}
	if got := resourceNames(resourcesForQuery(query)); got[0] != "/Customer" || !slices.Contains(got, tooDeep) || slices.Contains(got, "/Secret") {
		t.Fatalf("nested subqueries past the depth limit: resources = %v", got)
	}
}

func TestUnanalysableQueryNeedsAnOpaquePolicy(t *testing.T) {
	ctx := context.Background()
	query := customerQuery().Where(strangeNode{}).SelectKeysOnly(reflect.String)

	denied := &countingSession{}
	_, err := SecureReadSession(denied, allowEverything()).ExecuteQueryToRecordsReader(ctx, query)
	var denial *DeniedError
	if !errors.Is(err, ErrAccessDenied) || !errors.As(err, &denial) || denial.Decision.Resource.Kind() != OpaqueQueryResource {
		t.Fatalf("a policy without an opaque rule: error = %v", err)
	}
	if denied.reads != 0 {
		t.Fatalf("%d reads reached the wrapped session", denied.reads)
	}

	opaqueAllowed := MustPolicy("opaque-allowed", Root(Allow(Query, "ordinary collections")), OpaqueQueryScope(Allow(Query, "opaque queries")))
	allowed := &countingSession{}
	if _, err := SecureReadSession(allowed, opaqueAllowed).ExecuteQueryToRecordsReader(ctx, query); err != nil {
		t.Fatalf("a policy that allows opaque queries: error = %v", err)
	}
	if allowed.reads != 1 {
		t.Fatalf("%d reads reached the wrapped session, want 1", allowed.reads)
	}
}
