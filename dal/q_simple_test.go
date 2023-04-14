package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSimpleQuery(t *testing.T) {
	newRecord := func() Record {
		return nil
	}
	var q = From("test",
		Field("field_1").EqualTo("value_1"),
	).SelectInto(newRecord)
	assert.NotNil(t, q)
	assert.NotNil(t, q.From)
	assert.Equal(t, "test", q.From.Name)
	assert.NotNil(t, q.Where)
	assert.NotNil(t, q.Into)
	//assert.Equal(t, newRecord, q.Into)
	t.Log("\n" + q.String())
}
