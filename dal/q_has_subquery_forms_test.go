package dal

import (
	"testing"
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
	if *a.visits > 1000 {
		panic("the walk does not stop at a cycle")
	}
	return a.args
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
	if *f.visits > 1000 {
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
