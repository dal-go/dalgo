package dal

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRecordWithoutKey_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewRecordWithoutKey(nil)
	})
}

func TestNewRecordWithoutKey_PanicsOnValueTypes(t *testing.T) {
	// int value
	assert.Panics(t, func() {
		_ = NewRecordWithoutKey(123)
	})
	// struct value
	type S struct{ A int }
	assert.Panics(t, func() {
		_ = NewRecordWithoutKey(S{A: 1})
	})
	// string value
	assert.Panics(t, func() {
		_ = NewRecordWithoutKey("str")
	})
}

func TestNewRecordWithoutKey_SucceedsOnPointer(t *testing.T) {
	type S struct{ A int }
	s := &S{A: 42}
	r := NewRecordWithoutKey(s)
	assert.NotNil(t, r)
	// We are in the same package, so we can access concrete type and internal field
	rec := r.(*record)
	assert.Same(t, s, rec.data)
}

func TestNewRecordWithoutKey_AcceptsMapStringAnyByValue(t *testing.T) {
	m := map[string]any{"a": 1, "b": "x"}
	r := NewRecordWithoutKey(m)
	assert.NotNil(t, r)
	rec := r.(*record)
	// compare underlying pointer identity for maps
	assert.Equal(t, reflect.ValueOf(m).Pointer(), reflect.ValueOf(rec.data).Pointer())
}

func TestNewRecordWithoutKey_AcceptsMapStringIntByValue(t *testing.T) {
	m := map[string]int{"a": 1}
	r := NewRecordWithoutKey(m)
	assert.NotNil(t, r)
	rec := r.(*record)
	assert.Equal(t, reflect.ValueOf(m).Pointer(), reflect.ValueOf(rec.data).Pointer())
}

func TestNewRecordWithoutKey_RejectsMapWithNonStringKey(t *testing.T) {
	m := map[int]any{1: "x"}
	assert.Panics(t, func() { _ = NewRecordWithoutKey(m) })
}

func TestNewRecordWithoutKey_RejectsPointerToMap(t *testing.T) {
	m := map[string]any{"a": 1}
	assert.Panics(t, func() { _ = NewRecordWithoutKey(&m) })
}

func TestNewRecordWithoutKey_AcceptsSlices(t *testing.T) {
	ints := []int{1, 2, 3}
	r1 := NewRecordWithoutKey(ints)
	rec1 := r1.(*record)
	assert.Equal(t, reflect.ValueOf(ints).Pointer(), reflect.ValueOf(rec1.data).Pointer())

	anys := []any{"a", 2}
	r2 := NewRecordWithoutKey(anys)
	rec2 := r2.(*record)
	assert.Equal(t, reflect.ValueOf(anys).Pointer(), reflect.ValueOf(rec2.data).Pointer())
}

func TestNewRecordWithoutKey_RejectsPointerToSlice(t *testing.T) {
	ints := []int{1, 2, 3}
	assert.Panics(t, func() { _ = NewRecordWithoutKey(&ints) })
}
