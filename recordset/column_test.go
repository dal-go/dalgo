package recordset

import (
	"testing"
)

func TestColumn(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		testCol(t, "string", "abc", "def")
	})
	t.Run("int", func(t *testing.T) {
		testCol[int](t, 0, 1, 2)
	})
}

func testCol[T comparable](t *testing.T, defaultValue T, v1, v2 T) {
	const colName = "test_col_name"
	col := NewTypedColumn[T](colName, defaultValue)
	if col == nil {
		t.Fatal("got nil, want NewTypedColumn[T]")
	}
	if name := col.Name(); name != colName {
		t.Errorf("got name=%q, want %q", name, colName)
	}

	if val := col.DefaultValue(); val != defaultValue {
		t.Errorf("got DefaultValue=%v, want %v", val, defaultValue)
	}

	if err := col.Add(v1); err != nil {
		t.Errorf("Add failed: %v", err)
	}
	if err := col.Add(v2); err != nil {
		t.Errorf("Add failed: %v", err)
	}

	if val, err := col.GetValue(0); err != nil {
		t.Errorf("GetValue(0) failed: %v", err)
	} else if val != any(v1) {
		t.Errorf("GetValue(0) returned %v, want %v", val, v1)
	}

	if err := col.SetValue(0, v2); err != nil {
		t.Errorf("SetValue(0) failed: %v", err)
	}
	if val, err := col.GetValue(0); err != nil {
		t.Errorf("GetValue(0) failed: %v", err)
	} else if val != any(v2) {
		t.Errorf("GetValue(0) returned %v, want %v", val, v2)
	}

	if col.IsBitmap() {
		t.Error("IsBitmap() returned true, want false")
	}

	if col.ValueType() == nil {
		t.Error("ValueType() returned nil")
	}
}
