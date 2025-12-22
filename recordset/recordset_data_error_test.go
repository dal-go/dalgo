package recordset

import (
	"fmt"
	"reflect"
	"testing"
)

type mockColumn struct {
	name string
}

func (m mockColumn) Name() string                { return m.name }
func (m mockColumn) DefaultValue() any           { return nil }
func (m mockColumn) Add(_ any) error             { return nil }
func (m mockColumn) GetValue(_ int) (any, error) { return nil, fmt.Errorf("error") }
func (m mockColumn) SetValue(_ int, _ any) error { return nil }
func (m mockColumn) ValueType() reflect.Type     { return nil }
func (m mockColumn) IsBitmap() bool              { return false }

type mockRecordset struct {
	ColumnAccessor
	name string
}

func (m mockRecordset) Name() string     { return m.name }
func (m mockRecordset) NewRow() Row      { return nil }
func (m mockRecordset) GetRow(_ int) Row { return nil }
func (m mockRecordset) RowsCount() int   { return 0 }

func TestDataError(t *testing.T) {
	col := mockColumn{name: "err_col"}
	rs := mockRecordset{
		ColumnAccessor: &columns{cols: []Column[any]{col}},
		name:           "mock_rs",
	}
	row := &columnarRow{i: 0}
	_, err := row.Data(rs)
	if err == nil {
		t.Error("expected error from Data()")
	}
}
