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
	var qb = From("test",
		Field("field_1").EqualTo("value_1"),
	).
		Where().
		WhereField("field_2", Equal, "value_2").
		Limit(10).
		Offset(20).
		StartFrom(Cursor("cursor_1")).
		OrderBy()

	q := qb.SelectInto(newRecord)
	assert.NotNil(t, q)
	assert.NotNil(t, q.From())
	assert.Equal(t, "test", q.From().Name)
	assert.NotNil(t, q.Where())
	assert.NotNil(t, q.Into())
	//assert.Equal(t, newRecordWithOnlyKey, q.Into)

	q = qb.SelectKeysOnly(reflect.String)
	assert.Equal(t, reflect.String, q.IDKind())
}
