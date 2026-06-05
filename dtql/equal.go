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
// behind the structural round-trip guarantee.
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
	switch base := from.Base().(type) {
	case dal.CollectionRef:
		return base, true
	case *dal.CollectionRef:
		return *base, true
	default:
		return dal.CollectionRef{}, false
	}
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
	case *dal.Comparison:
		return comparisonEqual(*ac, b)
	case dal.GroupCondition:
		return groupEqual(ac, b)
	case *dal.GroupCondition:
		return groupEqual(*ac, b)
	default:
		return false
	}
}

func comparisonEqual(a dal.Comparison, b dal.Condition) bool {
	var bc dal.Comparison
	switch v := b.(type) {
	case dal.Comparison:
		bc = v
	case *dal.Comparison:
		bc = *v
	default:
		return false
	}
	return a.Operator == bc.Operator && exprEqual(a.Left, bc.Left) && exprEqual(a.Right, bc.Right)
}

func groupEqual(a dal.GroupCondition, b dal.Condition) bool {
	var bg dal.GroupCondition
	switch v := b.(type) {
	case dal.GroupCondition:
		bg = v
	case *dal.GroupCondition:
		bg = *v
	default:
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
		return fieldRefEqual(ae, b)
	case *dal.FieldRef:
		return fieldRefEqual(*ae, b)
	case dal.Constant:
		return constantEqual(ae, b)
	case *dal.Constant:
		return constantEqual(*ae, b)
	case dal.Array:
		return arrayEqual(ae, b)
	case *dal.Array:
		return arrayEqual(*ae, b)
	default:
		return false
	}
}

func fieldRefEqual(a dal.FieldRef, b dal.Expression) bool {
	switch v := b.(type) {
	case dal.FieldRef:
		return a.Equal(v)
	case *dal.FieldRef:
		return a.Equal(*v)
	default:
		return false
	}
}

func constantEqual(a dal.Constant, b dal.Expression) bool {
	switch v := b.(type) {
	case dal.Constant:
		return reflect.DeepEqual(a.Value, v.Value)
	case *dal.Constant:
		return reflect.DeepEqual(a.Value, v.Value)
	default:
		return false
	}
}

func arrayEqual(a dal.Array, b dal.Expression) bool {
	switch v := b.(type) {
	case dal.Array:
		return a.Equal(v)
	case *dal.Array:
		return a.Equal(*v)
	default:
		return false
	}
}
