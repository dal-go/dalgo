package dal

import (
	"reflect"
	"testing"

	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
)

func TestQueryBuilder(t *testing.T) {
	newRecord := func() record.Record {
		return nil
	}
	var qb = From(NewRootCollectionRef("test", "t")).NewQuery().
		Where().
		Limit(10).
		Offset(20).
		StartFrom("cursor_1").
		OrderBy(orderExpression{expression: FieldRef{name: "field_1"}})

	assertQuery := func(t *testing.T, q StructuredQuery) {
		assert.NotNil(t, q)
		assert.Equal(t, "test", q.From().Base().Name())
		assert.NotNil(t, q.Where())
		assert.NotNil(t, q.OrderBy())
		assert.NotNil(t, q.Limit())
		assert.NotNil(t, q.Offset())
		assert.NotNil(t, q.StartFrom())
	}
	t.Run("no_conditions", func(t *testing.T) {
		qbNoConditions := From(NewRootCollectionRef("test", "")).NewQuery()
		q := qbNoConditions.SelectKeysOnly(reflect.String)
		assert.Equal(t, reflect.String, q.IDKind())
		assert.Equal(t, "test", q.From().Base().Name())
	})

	t.Run("with_single_condition", func(t *testing.T) {
		qb2 := qb.Clone().WhereField("field_2", Equal, "value_2")
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)
	})

	t.Run("with_multiple_conditions", func(t *testing.T) {
		qb2 := qb.Clone().
			WhereField("field_2", Equal, "value_2").
			WhereField("field_3", Equal, "value_3")
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)
	})

	t.Run("with_where_in_array_field", func(t *testing.T) {
		qb2 := qb.Clone().WhereInArrayField("tags", "important")
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)

		// Verify the condition was created correctly
		where := q.Where()
		assert.NotNil(t, where)

		// The condition should be a Comparison with the value on the left, In operator, and field on the right
		if comparison, ok := where.(Comparison); ok {
			assert.Equal(t, In, comparison.Operator)
			assert.Equal(t, Constant{Value: "important"}, comparison.Left)
			assert.Equal(t, FieldRef{name: "tags"}, comparison.Right)
		} else {
			t.Errorf("Expected Comparison condition, got %T", where)
		}
	})

	t.Run("with_where_array_contains", func(t *testing.T) {
		qb2 := qb.Clone().WhereArrayContains("tags", "important")
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)

		// Same shape as WhereInArrayField: value In field, what dalgo2firestore
		// translates to "array-contains".
		if comparison, ok := q.Where().(Comparison); ok {
			assert.Equal(t, In, comparison.Operator)
			assert.Equal(t, Constant{Value: "important"}, comparison.Left)
			assert.Equal(t, FieldRef{name: "tags"}, comparison.Right)
		} else {
			t.Errorf("Expected Comparison condition, got %T", q.Where())
		}
	})

	t.Run("with_where_array_contains_any", func(t *testing.T) {
		qb2 := qb.Clone().WhereArrayContainsAny("tags", []string{"a", "b"})
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)

		// Shape: field In array, what dalgo2firestore translates to
		// "array-contains-any".
		if comparison, ok := q.Where().(Comparison); ok {
			assert.Equal(t, In, comparison.Operator)
			assert.Equal(t, FieldRef{name: "tags"}, comparison.Left)
			assert.Equal(t, Array{Value: []string{"a", "b"}}, comparison.Right)
		} else {
			t.Errorf("Expected Comparison condition, got %T", q.Where())
		}
	})

	t.Run("with_where_array_contains_any_passing_array", func(t *testing.T) {
		arr := Array{Value: []int{1, 2}}
		qb2 := qb.Clone().WhereArrayContainsAny("nums", arr)
		q := qb2.SelectIntoRecord(newRecord)
		assertQuery(t, q)

		if comparison, ok := q.Where().(Comparison); ok {
			assert.Equal(t, In, comparison.Operator)
			assert.Equal(t, FieldRef{name: "nums"}, comparison.Left)
			assert.Equal(t, arr, comparison.Right)
		} else {
			t.Errorf("Expected Comparison condition, got %T", q.Where())
		}
	})

	t.Run("SelectIntoRecordset", func(t *testing.T) {
		q := qb.Clone().SelectIntoRecordset()
		assert.NotNil(t, q)
	})
}
