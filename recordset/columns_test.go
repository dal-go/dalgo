package recordset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColumns(t *testing.T) {
	c1 := NewColumn[string]("c1", "")
	c2 := NewColumn[int]("c2", 0)
	cols := columns{cols: []Column[any]{c1, c2}}

	t.Run("ColumnsCount", func(t *testing.T) {
		assert.Equal(t, 2, cols.ColumnsCount())
	})

	t.Run("GetColumnByIndex", func(t *testing.T) {
		assert.Equal(t, c1, cols.GetColumnByIndex(0))
		assert.Equal(t, c2, cols.GetColumnByIndex(1))
		assert.Nil(t, cols.GetColumnByIndex(2))
	})

	t.Run("GetColumnByName", func(t *testing.T) {
		assert.Equal(t, c1, cols.GetColumnByName("c1"))
		assert.Equal(t, c2, cols.GetColumnByName("c2"))
		assert.Nil(t, cols.GetColumnByName("c3"))
	})

	t.Run("GetColumnIndex", func(t *testing.T) {
		assert.Equal(t, 0, cols.GetColumnIndex("c1"))
		assert.Equal(t, 1, cols.GetColumnIndex("c2"))
		assert.Equal(t, -1, cols.GetColumnIndex("c3"))
	})
}
