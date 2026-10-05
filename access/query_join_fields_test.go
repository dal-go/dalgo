package access

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func decisionOf(t *testing.T, err error) Decision {
	t.Helper()
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %v, want an access denial", err)
	}
	return denied.Decision
}

func TestCollectJoinClausesListsEveryConditionAndScanOrder(t *testing.T) {
	scalar := scalarSecret()
	baseScan := dal.AscendingField("a")
	joinedScan := dal.Descending(qualified("c", "b"))
	deepScan := dal.AscendingField("d")
	pointerSource := orders.WithScan(3, deepScan)
	tree := dal.From(customers.WithScan(5, baseScan)).
		Join(dal.NewJoinedFrom(
			dal.From(orders.WithScan(5, joinedScan)).
				Join(dal.NewJoinedSource(&pointerSource, dal.JoinInner, joinOn("o", "p"))),
			dal.JoinInner, joinOn("c", "o"), joinOn("c", "o2"))).
		Join(dal.NewJoinedSource(lines, dal.JoinInner, dal.NewComparison(dal.Field("x"), dal.Equal, scalar)))
	clauses, err := collectJoinClauses(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(clauses.conditions), 4; got != want {
		t.Fatalf("%d conditions, want %d", got, want)
	}
	owners := map[dal.OrderExpression]fieldOwner{}
	for _, scan := range clauses.scans {
		owners[scan.order] = scan.owner
	}
	want := map[dal.OrderExpression]fieldOwner{baseScan: ownerBase, joinedScan: ownerJoined, deepScan: ownerJoined}
	if !reflect.DeepEqual(owners, want) || len(clauses.scans) != 3 {
		t.Fatalf("scan orders = %v, want %v", clauses.scans, want)
	}
	for identifier, wantBase := range map[string]bool{"Customer": true, "c": true} {
		if clauses.base[identifier] != wantBase {
			t.Fatalf("base identifiers = %v", clauses.base)
		}
	}
	for _, identifier := range []string{"Order", "o", "Line", "l"} {
		if !clauses.joined[identifier] || clauses.base[identifier] {
			t.Fatalf("joined identifiers = %v, base = %v", clauses.joined, clauses.base)
		}
	}
}

func TestJoinClauseAttribution(t *testing.T) {
	selfJoined := dal.From(customers).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("Order", "c"), dal.JoinInner, joinOn("c", "c")))
	clauses, err := collectJoinClauses(dal.From(customers).
		Join(dal.NewJoinedSource(orders, dal.JoinInner, joinOn("c", "o"))))
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := collectJoinClauses(selfJoined)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		clauses *joinClauses
		field   dal.FieldRef
		own     fieldOwner
		want    fieldOwner
	}{
		{"the base's alias", clauses, qualified("c", "id"), ownerJoined, ownerBase},
		{"the base's name", clauses, qualified("Customer", "id"), ownerJoined, ownerBase},
		{"a joined alias", clauses, qualified("o", "id"), ownerBase, ownerJoined},
		{"a joined name", clauses, qualified("Order", "id"), ownerBase, ownerJoined},
		{"no qualifier in a clause of the base", clauses, dal.Field("id"), ownerBase, ownerBase},
		{"no qualifier in a clause of a joined source", clauses, dal.Field("id"), ownerJoined, ownerJoined},
		{"a qualifier no source has", clauses, qualified("x", "id"), ownerBase, ownerUnknown},
		{"a qualifier of the base and of a joined source", ambiguous, qualified("c", "id"), ownerBase, ownerUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.clauses.attribute(c.field, c.own); got != c.want {
				t.Fatalf("owner = %d, want %d", got, c.want)
			}
		})
	}
}

func TestJoinClausesOfASharedTreeAreGatheredOnce(t *testing.T) {
	shared := dal.From(orders.WithScan(5, dal.AscendingField("a")))
	tree := dal.From(customers).
		Join(dal.NewJoinedFrom(shared, dal.JoinInner, joinOn("c", "o"))).
		Join(dal.NewJoinedFrom(shared, dal.JoinInner, joinOn("c", "o")))
	clauses, err := collectJoinClauses(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses.scans) != 1 || len(clauses.conditions) != 2 {
		t.Fatalf("%d scan orders and %d conditions, want 1 and 2", len(clauses.scans), len(clauses.conditions))
	}
}

func TestJoinClausesOfAnIncompleteTreeHoldNothingMore(t *testing.T) {
	var nilTree dal.FromSource
	otherJoin := dal.NewJoinedFrom(dal.From(orders), dal.JoinInner, joinOn("c", "o"))
	otherJoin.RecordsetSource = lines.WithScan(5, dal.AscendingField("a"))
	tree := dal.From(customers).
		Join(dal.NewNestedJoinedSource(nilTree, dal.JoinInner, joinOn("c", "o"))).
		Join(otherJoin)
	clauses, err := collectJoinClauses(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses.scans) != 1 || len(clauses.conditions) != 2 || !clauses.joined["Line"] || !clauses.joined["Order"] {
		t.Fatalf("clauses = %+v", clauses)
	}
	empty, err := collectJoinClauses(nilTree)
	if err != nil || len(empty.conditions) != 0 || len(empty.scans) != 0 {
		t.Fatalf("clauses of a missing tree = %+v, error = %v", empty, err)
	}
}

func TestJoinClausesOfATreeThatRefersToItselfCannotBeGathered(t *testing.T) {
	tree := dal.From(customers)
	tree.Join(dal.NewJoinedFrom(tree, dal.JoinInner, joinOn("c", "c")))
	if _, err := collectJoinClauses(tree); !errors.Is(err, errJoinTreeCycle) {
		t.Fatalf("error = %v, want %v", err, errJoinTreeCycle)
	}
}

func TestJoinClausesNestingIsBounded(t *testing.T) {
	nest := func(levels int) dal.FromSource {
		tree := dal.From(customers)
		for i := 0; i < levels; i++ {
			tree = dal.From(orders).Join(dal.NewJoinedFrom(tree, dal.JoinInner, joinOn("c", "o")))
		}
		return tree
	}
	if _, err := collectJoinClauses(nest(maxQueryNesting)); err != nil {
		t.Fatalf("nesting at the limit: %v", err)
	}
	if _, err := collectJoinClauses(nest(maxQueryNesting + 1)); !errors.Is(err, errJoinTreeTooDeep) {
		t.Fatalf("nesting past the limit: error = %v, want %v", err, errJoinTreeTooDeep)
	}
}

func TestFieldListRefusesAJoinTreeItCannotCheck(t *testing.T) {
	sets := fieldList(t, "id", "name")
	cyclic := dal.From(customers)
	cyclic.Join(dal.NewJoinedFrom(cyclic, dal.JoinInner, joinOn("c", "c")))
	tooDeep := dal.From(customers)
	for i := 0; i <= maxQueryNesting; i++ {
		tooDeep = dal.From(orders).Join(dal.NewJoinedFrom(tooDeep, dal.JoinInner, joinOn("c", "o")))
	}
	for name, tree := range map[string]dal.FromSource{"a tree that refers to itself": cyclic, "a tree nested too deeply": tooDeep} {
		t.Run(name, func(t *testing.T) {
			decision := decisionOf(t, validateRequestedQueryFields(selectName(tree), sets))
			if decision.Code != CodeEnforcementUnsupported || !strings.Contains(decision.Explanation, "cannot be checked") {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}

func TestFieldListAttributesJoinFieldsToTheirSource(t *testing.T) {
	sets := fieldList(t, "id", "name")
	collision := dal.NewRootCollectionRef("Order", "c")
	cases := []struct {
		name  string
		query dal.StructuredQuery
		code  ReasonCode
	}{
		{"a qualifier of the base and of a joined source", selectName(dal.From(customers).
			Join(dal.NewJoinedSource(collision, dal.JoinInner, onField(qualified("c", "name"), qualified("c", "ref"))))), CodeEnforcementUnsupported},
		{"a hidden field under a qualifier of the base and of a joined source", selectName(dal.From(customers).
			Join(dal.NewJoinedSource(collision, dal.JoinInner, onField(qualified("c", "secret"), qualified("c", "ref"))))), CodeColumnDenied},
		{"a missing scan order of the base", selectName(dal.From(customers.WithScan(5, nil))), CodeColumnDenied},
		{"a scan order through a pointer", func() dal.StructuredQuery {
			scanned := customers.WithScan(5, dal.AscendingField("secret"))
			return selectName(dal.From(&scanned))
		}(), CodeColumnDenied},
		{"an expression no field check knows in a join ON", selectName(dal.From(customers).
			Join(dal.NewJoinedSource(orders, dal.JoinInner, onField(dal.Binary(dal.Field("id"), dal.Add, dal.Constant{Value: 1}), qualified("o", "ref"))))), CodeEnforcementUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decisionOf(t, validateRequestedQueryFields(c.query, sets)).Code; got != c.code {
				t.Fatalf("code = %s, want %s", got, c.code)
			}
		})
	}
	t.Run("a query with no source has no join clause", func(t *testing.T) {
		if err := validateRequestedQueryFields(&shapeQuery{}, sets); err != nil {
			t.Fatalf("error = %v", err)
		}
	})
}
