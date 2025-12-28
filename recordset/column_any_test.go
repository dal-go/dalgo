package recordset

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestUntypedColWrapper(t *testing.T) {
	inner := NewBoolColumn("test", ColDbType("BOOL"))
	col := UntypedCol(inner)

	assert.Equal(t, "test", col.Name())
	assert.Equal(t, "BOOL", col.DbType())
	assert.Equal(t, false, col.DefaultValue())
	assert.Equal(t, reflect.TypeOf(true), col.ValueType())
	assert.True(t, col.IsBitmap())

	err := col.Add(true)
	assert.NoError(t, err)

	val, err := col.GetValue(0)
	assert.NoError(t, err)
	assert.Equal(t, true, val)

	err = col.SetValue(0, false)
	assert.NoError(t, err)
	val, _ = col.GetValue(0)
	assert.Equal(t, false, val)

	values := col.Values()
	assert.Equal(t, 1, len(values))
	assert.Equal(t, false, values[0])

	t.Run("Add_wrong_type", func(t *testing.T) {
		err := col.Add("wrong")
		assert.Error(t, err)
	})

	t.Run("TypedColumn", func(t *testing.T) {
		wrapper := col.(UntypedColWrapper[bool])
		assert.Equal(t, inner, wrapper.TypedColumn())
	})
}
