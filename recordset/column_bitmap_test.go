package recordset

import (
	"reflect"
	"testing"
)

func TestNewBitmapColumn(t *testing.T) {
	col := NewBitmapColumn[string]("test", 0, func() string {
		return ""
	})
	if col.Name() != "test" {
		t.Errorf("expected name test, got %s", col.Name())
	}
	if col.ValueType() != reflect.TypeOf("") {
		t.Errorf("expected value type string, got %v", col.ValueType())
	}
	if !col.IsBitmap() {
		t.Error("expected IsBitmap() to be true")
	}
}

func TestColumnBitmap_GetValue(t *testing.T) {
	col := NewBitmapColumn[string]("test", 0, func() string {
		return ""
	})
	val, err := col.GetValue(0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %v", val)
	}

	_ = col.SetValue(0, "a")
	val, err = col.GetValue(0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "a" {
		t.Errorf("expected a, got %v", val)
	}
}

func TestColumnBitmap_SetValue(t *testing.T) {
	col := NewBitmapColumn[string]("test", 0, func() string {
		return ""
	})
	err := col.SetValue(1, "a")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ := col.GetValue(1)
	if val != "a" {
		t.Errorf("expected a, got %v", val)
	}

	err = col.SetValue(1, "b")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ = col.GetValue(1)
	if val != "b" {
		t.Errorf("expected b, got %v", val)
	}

	// Test overwriting existing value for a row with another existing value
	_ = col.SetValue(2, "a")
	_ = col.SetValue(2, "b")
	val, _ = col.GetValue(2)
	if val != "b" {
		t.Errorf("expected b for row 2, got %v", val)
	}
}

func TestColumnBitmap_Add(t *testing.T) {
	col := NewBitmapColumn[string]("test", 0, func() string {
		return ""
	})
	err := col.Add("a")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ := col.GetValue(0)
	if val != "a" {
		t.Errorf("expected a, got %v", val)
	}

	err = col.Add("b")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ = col.GetValue(1)
	if val != "b" {
		t.Errorf("expected b, got %v", val)
	}
}

func TestColumnBitmap_Values(t *testing.T) {
	col := NewBitmapColumn[string]("test", 0, func() string {
		return ""
	})
	_ = col.Add("a")
	_ = col.Add("b")
	values := col.Values()
	if len(values) != 2 {
		t.Errorf("expected 2 values, got %d", len(values))
	}
	if values[0] != "a" || values[1] != "b" {
		t.Errorf("unexpected values: %v", values)
	}
}
