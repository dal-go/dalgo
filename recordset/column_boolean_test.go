package recordset

import (
	"reflect"
	"testing"
)

func TestNewBoolColumn(t *testing.T) {
	col := NewBoolColumn("test")
	if col.Name() != "test" {
		t.Errorf("expected name test, got %s", col.Name())
	}
	if col.DefaultValue() != false {
		t.Errorf("expected default value false, got %v", col.DefaultValue())
	}
	if col.ValueType() != reflect.TypeOf(true) {
		t.Errorf("expected value type bool, got %v", col.ValueType())
	}
	if !col.IsBitmap() {
		t.Error("expected IsBitmap() to be true")
	}
}

func TestColumnBool_GetValue(t *testing.T) {
	col := NewBoolColumn("test")
	val, err := col.GetValue(0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != false {
		t.Errorf("expected false, got %v", val)
	}

	_ = col.SetValue(0, true)
	val, err = col.GetValue(0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != true {
		t.Errorf("expected true, got %v", val)
	}
}

func TestColumnBool_SetValue(t *testing.T) {
	col := NewBoolColumn("test")
	err := col.SetValue(1, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ := col.GetValue(1)
	if val != true {
		t.Errorf("expected true, got %v", val)
	}

	err = col.SetValue(1, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ = col.GetValue(1)
	if val != false {
		t.Errorf("expected false, got %v", val)
	}
}

func TestColumnBool_Values(t *testing.T) {
	col := NewBoolColumn("test")
	_ = col.Add(true)
	_ = col.Add(false)
	values := col.Values()
	if len(values) != 2 {
		t.Errorf("expected 2 values, got %d", len(values))
	}
	if values[0] != true || values[1] != false {
		t.Errorf("unexpected values: %v", values)
	}
}

func TestColumnBool_Add(t *testing.T) {
	col := NewBoolColumn("test")
	err := col.Add(true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ := col.GetValue(0)
	if val != true {
		t.Errorf("expected true, got %v", val)
	}

	err = col.Add(false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	val, _ = col.GetValue(1)
	if val != false {
		t.Errorf("expected false, got %v", val)
	}
}
