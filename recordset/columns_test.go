package recordset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColumns(t *testing.T) {
	c1 := NewTypedColumn[string]("c1", "")
	c2 := NewTypedColumn[int]("c2", 0)
	c1untyped := UntypedCol(c1)
	c2untyped := UntypedCol(c2)
	cols := columns{cols: []Column[any]{c1untyped, c2untyped}}

	t.Run("ColumnsCount", func(t *testing.T) {
		assert.Equal(t, 2, cols.ColumnsCount())
	})

	t.Run("GetColumnByIndex", func(t *testing.T) {
		assert.Equal(t, c1untyped, cols.GetColumnByIndex(0))
		assert.Equal(t, c2untyped, cols.GetColumnByIndex(1))
		assert.Nil(t, cols.GetColumnByIndex(2))

		assert.Equal(t, c1, cols.GetColumnByIndex(0).(UntypedColWrapper[string]).TypedColumn())
		assert.Equal(t, c2, cols.GetColumnByIndex(1).(UntypedColWrapper[int]).TypedColumn())
	})

	t.Run("GetColumnByName", func(t *testing.T) {
		assert.Equal(t, c1untyped, cols.GetColumnByName("c1"))
		assert.Equal(t, c2untyped, cols.GetColumnByName("c2"))
		assert.Nil(t, cols.GetColumnByName("c3"))
	})

	t.Run("GetColumnIndex", func(t *testing.T) {
		assert.Equal(t, 0, cols.GetColumnIndex("c1"))
		assert.Equal(t, 1, cols.GetColumnIndex("c2"))
		assert.Equal(t, -1, cols.GetColumnIndex("c3"))
	})
}
