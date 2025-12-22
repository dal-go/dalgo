package recordset

import (
	"testing"
)

func TestNewColumn(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		testNewCol(t, "string")
	})
	t.Run("int", func(t *testing.T) {
		testNewCol[int](t, 0)
	})
}

func testNewCol[T any](t *testing.T, defaultValue T) {
	const colName = "test_col_name"
	col := NewColumn[T](colName, defaultValue)
	if col == nil {
		t.Error("got nil, want NewColumn[T]")
	}
	if name := col.Name(); name != colName {
		t.Errorf("got name=%q, want %q", name, colName)
	}
}
