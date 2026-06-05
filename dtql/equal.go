package dtql

import (
	"reflect"

	"github.com/dal-go/dalgo/dal"
)

// Equal reports whether two in-scope dal.StructuredQuery values are structurally
// equal across the DTQL-covered surface: From, Columns, the Where condition tree
// (including inline Constant/Array values), OrderBy, Limit and Offset. It compares
// via the dal accessor interface, so it is agnostic to the concrete query type
// (e.g. a builder query vs. a deserialized one). It is the equality mechanism
// behind the structural round-trip guarantee. The in-scope dal nodes are value
// types (Comparison, GroupCondition, FieldRef, Constant, Array), so the type
// switches handle the value forms.
func Equal(a, b dal.StructuredQuery) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if !fromEqual(a.From(), b.From()) {
		return false
	}
	if a.Limit() != b.Limit() || a.Offset() != b.Offset() {
		return false
	}
	if !columnsEqual(a.Columns(), b.Columns()) {
		return false
	}
	if !condEqual(a.Where(), b.Where()) {
		return false
	}
	return orderEqual(a.OrderBy(), b.OrderBy())
}

func fromEqual(a, b dal.FromSource) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	an, aok := rootCollection(a)
	bn, bok := rootCollection(b)
	if !aok || !bok {
		return false
	}
	return an.Equal(bn, false)
}

func rootCollection(from dal.FromSource) (dal.CollectionRef, bool) {
	base, ok := from.Base().(dal.CollectionRef)
	return base, ok
}

func columnsEqual(a, b []dal.Column) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Alias != b[i].Alias || !exprEqual(a[i].Expression, b[i].Expression) {
			return false
		}
	}
	return true
}

func orderEqual(a, b []dal.OrderExpression) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Descending() != b[i].Descending() || !exprEqual(a[i].Expression(), b[i].Expression()) {
			return false
		}
	}
	return true
}

func condEqual(a, b dal.Condition) bool {
	switch ac := a.(type) {
	case nil:
		return b == nil
	case dal.Comparison:
		return comparisonEqual(ac, b)
	case dal.GroupCondition:
		return groupEqual(ac, b)
	default:
		return false
	}
}

func comparisonEqual(a dal.Comparison, b dal.Condition) bool {
	bc, ok := b.(dal.Comparison)
	if !ok {
		return false
	}
	return a.Operator == bc.Operator && exprEqual(a.Left, bc.Left) && exprEqual(a.Right, bc.Right)
}

func groupEqual(a dal.GroupCondition, b dal.Condition) bool {
	bg, ok := b.(dal.GroupCondition)
	if !ok {
		return false
	}
	if a.Operator() != bg.Operator() {
		return false
	}
	ac, bc := a.Conditions(), bg.Conditions()
	if len(ac) != len(bc) {
		return false
	}
	for i := range ac {
		if !condEqual(ac[i], bc[i]) {
			return false
		}
	}
	return true
}

func exprEqual(a, b dal.Expression) bool {
	switch ae := a.(type) {
	case dal.FieldRef:
		bv, ok := b.(dal.FieldRef)
		return ok && ae.Equal(bv)
	case dal.Constant:
		bv, ok := b.(dal.Constant)
		return ok && reflect.DeepEqual(ae.Value, bv.Value)
	case dal.Array:
		bv, ok := b.(dal.Array)
		return ok && ae.Equal(bv)
	default:
		return false
	}
}
