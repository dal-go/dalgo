package dal_test

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// TestNewJoinedSource_BuildableFromOutside verifies a typed equi-join can be
// built entirely from outside the dal package via exported constructors, and
// read back (join type + ON conditions) through the public API.
func TestNewJoinedSource_BuildableFromOutside(t *testing.T) {
	users := dal.NewRootCollectionRef("users", "u")
	orders := dal.NewRootCollectionRef("orders", "o")
	on := dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("o", "userId"))

	join := dal.NewJoinedSource(orders, dal.JoinInner, on)
	from := dal.From(users).Join(join)

	joins := from.Joins()
	if len(joins) != 1 {
		t.Fatalf("expected 1 join, got %d", len(joins))
	}
	if got := joins[0].JoinType(); got != dal.JoinInner {
		t.Errorf("join type: expected %q, got %q", dal.JoinInner, got)
	}
	ons := joins[0].On()
	if len(ons) != 1 {
		t.Fatalf("expected 1 ON condition, got %d", len(ons))
	}
	cmp, ok := ons[0].(dal.Comparison)
	if !ok {
		t.Fatalf("expected ON to be a Comparison, got %T", ons[0])
	}
	left, ok := cmp.Left.(dal.FieldRef)
	if !ok || left.Source() != "u" || left.Name() != "id" {
		t.Errorf("ON left: expected u.id, got %v", cmp.Left)
	}
	right, ok := cmp.Right.(dal.FieldRef)
	if !ok || right.Source() != "o" || right.Name() != "userId" {
		t.Errorf("ON right: expected o.userId, got %v", cmp.Right)
	}
}
