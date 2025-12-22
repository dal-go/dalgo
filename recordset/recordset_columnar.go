package recordset

import (
	"fmt"
)

type ColumnarRecordset struct {
	name string
	columns
	rowsCount int
}

func (rs *ColumnarRecordset) Name() string {
	return rs.name
}

func (rs *ColumnarRecordset) NewRow() Row {
	row := &columnarRow{i: rs.rowsCount}
	for _, c := range rs.columns.cols {
		_ = c.Add(c.DefaultValue())
	}
	rs.rowsCount++
	return row
}

func (rs *ColumnarRecordset) GetRow(i int) Row {
	if i >= rs.rowsCount {
		return nil
	}
	return &columnarRow{i: i}
}

func (rs *ColumnarRecordset) RowsCount() int {
	return rs.rowsCount
}

var _ Recordset = (*ColumnarRecordset)(nil)

func NewColumnarRecordset(cols ...Column[any]) *ColumnarRecordset {
	return &ColumnarRecordset{
		columns: columns{cols: cols},
	}
}

type columnarRow struct {
	i int
}

func (row *columnarRow) Data(rs Recordset) (data []any, err error) {
	data = make([]any, rs.ColumnsCount())
	for i := 0; i < rs.ColumnsCount(); i++ {
		c := rs.GetColumnByIndex(i)
		if data[i], err = c.GetValue(row.i); err != nil {
			return
		}
	}
	return
}

func (row *columnarRow) GetValueByIndex(i int, rs Recordset) (value any, err error) {
	col := rs.GetColumnByIndex(i)

	if col == nil {
		return nil, fmt.Errorf("index out of range for column: %d", i)
	}
	return col.GetValue(row.i)
}

func (row *columnarRow) GetValueByName(name string, rs Recordset) (value any, err error) {
	col := rs.GetColumnByName(name)
	if col == nil {
		return nil, fmt.Errorf("unexpected column name: %s", name)
	}
	return col.GetValue(row.i)
}

func (row *columnarRow) SetValueByName(name string, value any, rs Recordset) error {
	col := rs.GetColumnByName(name)
	return col.SetValue(row.i, value)
}

func (row *columnarRow) SetValueByIndex(i int, value any, rs Recordset) error {
	col := rs.GetColumnByIndex(i)
	return col.SetValue(row.i, value)
}
