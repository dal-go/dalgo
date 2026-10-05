package dal

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/recordset"
)

func hasSubqueryLeaf() StructuredQuery {
	return From(NewRootCollectionRef("Secret", "s")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("s", "id")})
}

// countingAggregate is an aggregate whose arguments can hold itself. It counts
// each visit of its arguments and panics past a bound, so a walk that does not
// stop at a cycle is reported as a failure of the test.
type countingAggregate struct {
	args   []Expression
	visits *int
}

func (a *countingAggregate) String() string   { return "counting" }
func (a *countingAggregate) FuncName() string { return COUNT }
func (a *countingAggregate) FuncArgs() []Expression {
	*a.visits++
	if *a.visits > 2*maxQueryTreeDepth {
		panic("the walk does not stop at a cycle")
	}
	return a.args
}

// valueAggregate is an aggregate held by value whose arguments can hold a copy of
// itself, through the slice of arguments they share. It counts each visit and
// panics past a bound, as countingAggregate does.
type valueAggregate struct {
	args   []Expression
	visits *int
}

func (a valueAggregate) String() string   { return "counting" }
func (a valueAggregate) FuncName() string { return COUNT }
func (a valueAggregate) FuncArgs() []Expression {
	*a.visits++
	if *a.visits > 2*maxQueryTreeDepth {
		panic("the walk does not stop at a cycle")
	}
	return a.args
}

// nestedGroups is a query whose WHERE is levels condition groups held by value,
// one inside the other, around one comparison. Its longest path has levels+3
// nodes: the query, each group, the comparison and the field it names.
func nestedGroups(levels int) StructuredQuery {
	var condition Condition = NewComparison(Field("a"), Equal, Field("b"))
	for i := 0; i < levels; i++ {
		condition = NewGroupCondition(And, condition)
	}
	return From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(condition).SelectKeysOnly(0)
}

// countingFrom is a source tree that can join itself. It counts each visit of
// its joins and panics past a bound, as countingAggregate does.
type countingFrom struct {
	base   RecordsetSource
	joins  []JoinedSource
	visits *int
}

func (f *countingFrom) Base() RecordsetSource        { return f.base }
func (f *countingFrom) Join(JoinedSource) FromSource { return f }
func (f *countingFrom) NewQuery() *QueryBuilder      { return NewQueryBuilder(f) }
func (f *countingFrom) Joins() []JoinedSource {
	*f.visits++
	if *f.visits > 2*maxQueryTreeDepth {
		panic("the walk does not stop at a cycle")
	}
	return f.joins
}

func TestHasSubqueryFindsASubqueryInEveryPositionAndForm(t *testing.T) {
	leaf := hasSubqueryLeaf()
	scalar := NewQueryExpression(leaf, "x")
	pointerScalar := &scalar
	var nilScalar *QueryExpression
	exists := NewExistsCondition(leaf)
	isNull := NewIsNullCondition(scalar)
	comparison := NewComparison(Field("a"), Equal, scalar)
	group := NewGroupCondition(Or, WhereField("a", Equal, 1), exists)
	sum := Binary(Field("a"), Add, scalar)
	customers := NewRootCollectionRef("Customer", "c")
	orders := NewRootCollectionRef("Order", "o")
	on := NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("o", "ref"))
	query := func(where Condition) StructuredQuery {
		return From(customers).NewQuery().Where(where).SelectKeysOnly(0)
	}
	column := func(expression Expression) StructuredQuery {
		return From(customers).NewQuery().SelectColumns(Column{Expression: expression})
	}
	scanned := customers.WithScan(5, Ascending(scalar))
	otherJoin := NewJoinedFrom(From(orders), JoinInner, on)
	otherJoin.RecordsetSource = NewQuerySource(leaf, "d")

	cases := []struct {
		name  string
		query StructuredQuery
		want  bool
	}{
		{"a flat query", From(customers).NewQuery().SelectKeysOnly(0), false},
		{"a scan order that holds no query", From(customers.WithScan(5, AscendingField("a"))).NewQuery().SelectKeysOnly(0), false},
		{"a missing scan order", From(customers.WithScan(5, nil)).NewQuery().SelectKeysOnly(0), false},
		{"a scan order that holds a query", From(scanned).NewQuery().SelectKeysOnly(0), true},
		{"a scan order that holds a query, through a pointer", From(&scanned).NewQuery().SelectKeysOnly(0), true},
		{"a scan order of a joined source", From(customers).Join(NewJoinedSource(orders.WithScan(5, Descending(scalar)), JoinInner, on)).NewQuery().SelectKeysOnly(0), true},
		{"a scan order of a joined source that holds no query", From(customers).Join(NewJoinedSource(orders.WithScan(5, AscendingField("a")), JoinInner, on)).NewQuery().SelectKeysOnly(0), false},
		{"a scan order of a source at depth two", From(customers).Join(NewJoinedFrom(
			From(orders).Join(NewJoinedSource(NewRootCollectionRef("Line", "l").WithScan(5, Ascending(scalar)), JoinInner, on)),
			JoinInner, on)).NewQuery().SelectKeysOnly(0), true},
		{"a join whose own source differs from its tree", From(customers).Join(otherJoin).NewQuery().SelectKeysOnly(0), true},
		{"a join whose own source is the base of its tree", From(customers).Join(NewJoinedFrom(From(orders), JoinInner, on)).NewQuery().SelectKeysOnly(0), false},
		{"a join with no source", From(customers).Join(NewNestedJoinedSource(nil, JoinInner, on)).NewQuery().SelectKeysOnly(0), false},
		{"a join whose tree has no own source", From(customers).Join(JoinedSource{from: From(orders), joinType: JoinInner, on: []Condition{on}}).NewQuery().SelectKeysOnly(0), false},
		{"a nil collection pointer as a source", From((*CollectionRef)(nil)).NewQuery().SelectKeysOnly(0), false},
		{"a join ON", From(customers).Join(NewJoinedSource(orders, JoinInner, comparison)).NewQuery().SelectKeysOnly(0), true},
		{"EXISTS", query(exists), true},
		{"EXISTS through a pointer", query(&exists), true},
		{"a null test of a scalar subquery", query(isNull), true},
		{"a null test through a pointer", query(&isNull), true},
		{"a comparison through a pointer", query(&comparison), true},
		{"a condition group through a pointer", query(&group), true},
		{"a scalar subquery through a pointer", column(pointerScalar), true},
		{"an arithmetic operand", column(sum), true},
		{"an arithmetic operand through a pointer", column(&sum), true},
		{"a nil scalar subquery pointer", column(nilScalar), false},
		{"a nil binary expression pointer", column((*BinaryExpression)(nil)), false},
		{"a nil comparison pointer", query((*Comparison)(nil)), false},
		{"a nil null test pointer", query((*IsNullCondition)(nil)), false},
		{"a nil condition group pointer", query((*GroupCondition)(nil)), false},
		{"a nil exists pointer", query((*ExistsCondition)(nil)), false},
		{"a derived source through a pointer", func() StructuredQuery {
			source := NewQuerySource(leaf, "d")
			return From(&source).NewQuery().SelectKeysOnly(0)
		}(), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasSubquery(c.query); got != c.want {
				t.Fatalf("HasSubquery = %v, want %v", got, c.want)
			}
		})
	}
}

func TestHasSubqueryStopsAtACycle(t *testing.T) {
	t.Run("an aggregate that holds itself", func(t *testing.T) {
		visits := 0
		aggregate := &countingAggregate{visits: &visits}
		aggregate.args = []Expression{aggregate}
		query := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: aggregate})
		if HasSubquery(query) {
			t.Fatal("HasSubquery reported a subquery in a cycle that holds none")
		}
	})
	t.Run("an aggregate that holds itself beside a subquery", func(t *testing.T) {
		visits := 0
		aggregate := &countingAggregate{visits: &visits}
		aggregate.args = []Expression{aggregate, NewQueryExpression(hasSubqueryLeaf(), "x")}
		query := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: aggregate})
		if !HasSubquery(query) {
			t.Fatal("HasSubquery missed the subquery beside the cycle")
		}
	})
	t.Run("a condition group that holds itself", func(t *testing.T) {
		conditions := make([]Condition, 2)
		group := NewGroupCondition(Or, conditions...)
		conditions[0] = &group
		conditions[1] = WhereField("a", Equal, 1)
		query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(group).SelectKeysOnly(0)
		if HasSubquery(query) {
			t.Fatal("HasSubquery reported a subquery in a cycle that holds none")
		}
		conditions[1] = NewExistsCondition(hasSubqueryLeaf())
		if !HasSubquery(query) {
			t.Fatal("HasSubquery missed the subquery beside the cycle")
		}
	})
	t.Run("a condition group held by value that holds itself", func(t *testing.T) {
		conditions := make([]Condition, 2)
		group := NewGroupCondition(Or, conditions...)
		conditions[0] = group
		conditions[1] = WhereField("a", Equal, 1)
		query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(group).SelectKeysOnly(0)
		if !HasSubquery(query) {
			t.Fatal("HasSubquery did not report a tree nested past its bound")
		}
	})
	t.Run("an aggregate held by value that holds itself", func(t *testing.T) {
		visits := 0
		args := make([]Expression, 1)
		aggregate := valueAggregate{args: args, visits: &visits}
		args[0] = aggregate
		query := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: aggregate})
		if !HasSubquery(query) {
			t.Fatal("HasSubquery did not report a tree nested past its bound")
		}
		if visits > maxQueryTreeDepth {
			t.Fatalf("the walk visited the arguments %d times, want at most %d", visits, maxQueryTreeDepth)
		}
	})
	t.Run("a source tree that joins itself", func(t *testing.T) {
		visits := 0
		tree := &countingFrom{base: NewRootCollectionRef("Customer", "c"), visits: &visits}
		tree.joins = []JoinedSource{NewNestedJoinedSource(tree, JoinInner, NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("c", "id")))}
		if HasSubquery(tree.NewQuery().SelectKeysOnly(0)) {
			t.Fatal("HasSubquery reported a subquery in a cycle that holds none")
		}
	})
	t.Run("a source tree reached twice is not a cycle", func(t *testing.T) {
		shared := From(NewRootCollectionRef("Order", "o"))
		on := NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("o", "ref"))
		tree := From(NewRootCollectionRef("Customer", "c")).
			Join(NewJoinedFrom(shared, JoinInner, on)).
			Join(NewJoinedFrom(shared, JoinInner, on))
		if HasSubquery(tree.NewQuery().SelectKeysOnly(0)) {
			t.Fatal("HasSubquery reported a subquery in a tree that holds none")
		}
	})
}

// A walk stops at maxQueryTreeDepth nodes along one path and reports a query
// there, so a query the walk cannot follow to its end is handled as one with
// nested queries.
func TestHasSubqueryBoundsTheDepthOfTheTree(t *testing.T) {
	if maxQueryTreeDepth != 10000 {
		t.Fatalf("maxQueryTreeDepth = %d, want 10000: no query written by hand reaches it", maxQueryTreeDepth)
	}
	if HasSubquery(nestedGroups(maxQueryTreeDepth - 3)) {
		t.Fatal("a tree whose longest path is at the bound holds no subquery")
	}
	if !HasSubquery(nestedGroups(maxQueryTreeDepth - 2)) {
		t.Fatal("a tree whose longest path is past the bound is reported as holding a query")
	}
}

// A flat query whose condition nests many groups holds no subquery, as on main.
// Past the access layer's bound of 64 levels it is still reported as none, and a
// database that is not secured sends it to the backend as one query, not to the
// generic engine.
func TestHasSubqueryAnswersNoForADeeplyNestedFlatQuery(t *testing.T) {
	query := nestedGroups(200)
	if HasSubquery(query) {
		t.Fatal("a flat query whose WHERE nests 200 groups holds no subquery")
	}
	ctx := context.Background()
	t.Run("records reader", func(t *testing.T) {
		reader := &aggregationCoverageReader{}
		backend := &aggregationCoverageBackend{records: reader}
		got, err := validatedDB{Backend: backend}.ExecuteQueryToRecordsReader(ctx, query)
		if err != nil || got != reader {
			t.Fatalf("reader = %T, err = %v: the query did not reach the backend", got, err)
		}
		if !reflect.DeepEqual(backend.seen, Query(query)) {
			t.Fatal("the backend was not given the query as it is")
		}
	})
	t.Run("recordset reader", func(t *testing.T) {
		reader := &aggregationRecordsetReader{recordset: recordset.NewColumnarRecordset("direct")}
		backend := &aggregationCoverageBackend{recordset: reader}
		got, err := validatedDB{Backend: backend}.ExecuteQueryToRecordsetReader(ctx, query)
		if err != nil || got != reader {
			t.Fatalf("reader = %T, err = %v: the query did not reach the backend", got, err)
		}
		if !reflect.DeepEqual(backend.seen, Query(query)) {
			t.Fatal("the backend was not given the query as it is")
		}
	})
}
