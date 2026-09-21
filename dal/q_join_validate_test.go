package dal

import (
	"errors"
	"strings"
	"testing"
)

func joinFrom(name, alias string) FromSource {
	return From(NewRootCollectionRef(name, alias))
}

func joinOn(leftSource, leftField, rightSource, rightField string) Condition {
	return NewComparison(NewFieldRef(leftSource, leftField), Equal, NewFieldRef(rightSource, rightField))
}

type malformedJoinSource struct{ name, alias string }

func (s malformedJoinSource) Name() string   { return s.name }
func (s malformedJoinSource) Alias() string  { return s.alias }
func (malformedJoinSource) recordsetSource() {}

func TestValidateJoinTree_RecursiveScope(t *testing.T) {
	employees := joinFrom("Employee", "e")
	customers := joinFrom("Customer", "c").Join(NewNestedJoinedSource(employees, JoinLeft, joinOn("c", "SupportRepId", "e", "EmployeeId")))
	invoices := joinFrom("Invoice", "i").Join(NewNestedJoinedSource(customers, JoinInner, joinOn("i", "CustomerId", "c", "CustomerId")))
	if err := ValidateJoinTree(invoices); err != nil {
		t.Fatalf("ValidateJoinTree: %v", err)
	}
	copy := invoices.NewQuery().Clone().(*QueryBuilder).SelectIntoRecordset()
	if err := ValidateJoinTree(copy.From()); err != nil {
		t.Fatalf("ValidateJoinTree cloned tree: %v", err)
	}
}

func TestValidateJoinTree_Diagnostics(t *testing.T) {
	tests := []struct {
		name, category, path string
		from                 FromSource
	}{
		{
			name: "forward sibling", category: "join_scope", path: "from.joins[0].on[0].right.source",
			from: joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "c", "bId"))).Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinInner, joinOn("a", "id", "c", "aId"))),
		},
		{
			name: "unqualified on", category: "join_shape", path: "from.joins[0].on[0]",
			from: joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, NewComparison(Field("id"), Equal, NewFieldRef("b", "aId")))),
		},
		{
			name: "malformed predicate", category: "join_shape", path: "from.joins[0].on[0]",
			from: joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, NewGroupCondition(And, joinOn("a", "id", "b", "aId")))),
		},
		{
			name: "unsupported type", category: "join_type", path: "from.joins[0].type",
			from: joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinRight, joinOn("a", "id", "b", "aId"))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJoinTree(tt.from)
			var diagnostic *JoinValidationError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("ValidateJoinTree error = %v, want JoinValidationError", err)
			}
			if diagnostic.Category != tt.category || diagnostic.Path != tt.path {
				t.Fatalf("diagnostic = %#v, want category=%q path=%q", diagnostic, tt.category, tt.path)
			}
		})
	}
}

func TestValidateJoinTree_RejectsCycle(t *testing.T) {
	from := joinFrom("A", "a")
	from.Join(NewNestedJoinedSource(from, JoinInner, joinOn("a", "id", "a", "parentId")))
	err := ValidateJoinTree(from)
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_cycle" || diagnostic.Path != "from.joins[0].from" {
		t.Fatalf("ValidateJoinTree cycle = %#v, want join_cycle at from.joins[0].from", err)
	}
}

func TestJoinValidationCoverage(t *testing.T) {
	if got := NewJoinedSource(NewRootCollectionRef("B", "b"), "").JoinType(); got != JoinInner {
		t.Fatalf("omitted join type = %s", got)
	}
	if err := ValidateJoinTree(nil); err == nil {
		t.Fatal("nil tree accepted")
	}
	if got := (&JoinValidationError{Category: "join_shape", Path: "from"}).Error(); got != "join_shape at from" {
		t.Fatal(got)
	}
	if NewJoinedFrom(nil, JoinInner).From() != nil {
		t.Fatal("nil nested child")
	}
	base := joinFrom("A", "")
	if err := ValidateJoinTree(base.Join(JoinedSource{joinType: JoinInner})); err == nil {
		t.Fatal("missing source accepted")
	}
	if err := ValidateJoinTree(base.Join(NewJoinedSource(NewRootCollectionRef("B", "a"), JoinInner, joinOn("a", "id", "a", "id")))); err == nil {
		t.Fatal("duplicate alias accepted")
	}
	if err := ValidateJoinTree(joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, NewComparison(NewFieldRef("a", "id"), GreaterThen, NewFieldRef("b", "id"))))); err == nil {
		t.Fatal("operator accepted")
	}
	if err := ValidateJoinTree(joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "a", "other")))); err != nil {
		t.Fatalf("visible same-scope ON rejected: %v", err)
	}
	badChild := joinFrom("B", "b").Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinInner))
	if err := ValidateJoinTree(joinFrom("A", "a").Join(NewNestedJoinedSource(badChild, JoinInner, joinOn("a", "id", "b", "aId")))); err == nil {
		t.Fatal("nested malformed join accepted")
	}
}

func TestValidateJoinTreeMalformedRecursiveStructures(t *testing.T) {
	tests := []struct {
		name string
		from FromSource
		want string
	}{
		{"missing root base", &from{}, "join_shape at from"},
		{"empty root name", From(malformedJoinSource{}), "join_shape at from.name"},
		{"duplicate nested alias", joinFrom("A", "a").Join(NewNestedJoinedSource(joinFrom("B", "b").Join(NewJoinedSource(NewRootCollectionRef("C", "b"), JoinInner, joinOn("b", "id", "b", "id"))), JoinInner, joinOn("a", "id", "b", "aid"))), "join_scope"},
		{"unknown left alias", joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("missing", "id", "b", "aid"))), "join_scope at from.joins[0].on[0].left.source"},
		{"missing nested base", joinFrom("A", "a").Join(NewNestedJoinedSource(&from{}, JoinInner, joinOn("a", "id", "b", "aid"))), "join_shape at from.joins[0].from"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateJoinTree(tt.from); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
		})
	}
	if err := ValidateJoinTree(joinFrom("A", "a").Join(NewJoinedSource(NewRootCollectionRef("B", "a"), JoinInner, joinOn("a", "id", "a", "aid")))); err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Fatalf("parent-child alias collision: %v", err)
	}
	badNested := joinFrom("B", "b").Join(JoinedSource{})
	if _, err := relationAliases(badNested, "from", nil); err == nil || !strings.Contains(err.Error(), "join_shape") {
		t.Fatalf("nested missing source: %v", err)
	}
	cyclic := joinFrom("B", "b")
	cyclic.Join(NewNestedJoinedSource(cyclic, JoinInner))
	if _, err := relationAliases(cyclic, "from", nil); err == nil || !strings.Contains(err.Error(), "join_cycle") {
		t.Fatalf("nested cycle: %v", err)
	}
	badName := joinFrom("B", "b").Join(NewJoinedSource(malformedJoinSource{}, JoinInner))
	if _, err := relationAliases(badName, "from", nil); err == nil || !strings.Contains(err.Error(), "join_shape") {
		t.Fatalf("nested empty source: %v", err)
	}
	if cloned := cloneFrom(cyclic); cloned == nil {
		t.Fatal("cyclic tree clone returned nil")
	}
	root := joinFrom("A", "a")
	if err := validateJoinFrom(root, "from", map[string]bool{"a": true}, map[FromSource]bool{}); err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Fatalf("visible alias collision: %v", err)
	}
	if err := validateJoinFrom(root, "from", nil, map[FromSource]bool{root: true}); err == nil || !strings.Contains(err.Error(), "join_cycle") {
		t.Fatalf("recursive visit: %v", err)
	}
	if _, err := relationAliases(root, "from", map[FromSource]bool{root: true}); err == nil || !strings.Contains(err.Error(), "join_cycle") {
		t.Fatalf("relation ancestor cycle: %v", err)
	}
	if _, err := relationAliases(&from{}, "from", nil); err == nil || !strings.Contains(err.Error(), "join_shape") {
		t.Fatalf("relation without base: %v", err)
	}
}
