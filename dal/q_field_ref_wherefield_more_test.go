package dal

import (
	"testing"
	"time"
)

func TestWhereField_moreBranches(t *testing.T) {
	// time.Time
	_ = WhereField("ts", Equal, time.Unix(0, 0))

	// slice with In
	_ = WhereField("arr", In, []int{1, 2, 3})

	// Array value with In
	_ = WhereField("arr", In, Array{Value: []string{"a", "b"}})
}

func TestWhereField_panics_on_wrong_array_operator(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for array with non-In operator")
		}
	}()
	_ = WhereField("arr", Equal, []int{1, 2})
}

func TestWhereField_panics_on_wrong_array_expr_operator(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for Array expr with non-In operator")
		}
	}()
	_ = WhereField("arr", Equal, Array{Value: []int{1}})
}

func TestWhereField_panics_on_unsupported_type(t *testing.T) {
	type unsupported struct{}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for unsupported type")
		}
	}()
	_ = WhereField("x", Equal, unsupported{})
}
