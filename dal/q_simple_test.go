package dal

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestSimpleQuery(t *testing.T) {
	newRecord := func() Record {
		return nil
	}
	var qb = From(NewRootCollectionRef("test", "t")).
		Where().
		Limit(10).
		Offset(20).
		StartFrom(Cursor("cursor_1")).
		OrderBy(orderExpression{expression: FieldRef{Name: "field_1"}})

	assertQuery := func(t *testing.T, q Query) {
		assert.NotNil(t, q)
		assert.Equal(t, "test", q.From().Name)
		assert.NotNil(t, q.Where())
		assert.NotNil(t, q.OrderBy())
		assert.NotNil(t, q.Limit())
		assert.NotNil(t, q.Offset())
		assert.NotNil(t, q.StartFrom())
	}
	t.Run("no_conditions", func(t *testing.T) {
		qbNoConditions := From(NewRootCollectionRef("test", ""))
		q := qbNoConditions.SelectKeysOnly(reflect.String)
		assert.Equal(t, reflect.String, q.IDKind())
		assert.Equal(t, "test", q.From().Name)
	})

	t.Run("with_single_condition", func(t *testing.T) {
		qb2 := qb.WhereField("field_2", Equal, "value_2")
		q := qb2.SelectInto(newRecord)
		assertQuery(t, q)
	})

	t.Run("with_multiple_conditions", func(t *testing.T) {
		qb2 := qb.
			WhereField("field_2", Equal, "value_2").
			WhereField("field_3", Equal, "value_3")
		q := qb2.SelectInto(newRecord)
		assertQuery(t, q)
	})
}
