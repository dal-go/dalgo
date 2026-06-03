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
	for _, c := range rs.cols {
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

func NewColumnarRecordset(name string, cols ...Column[any]) *ColumnarRecordset {
	return &ColumnarRecordset{
		name:    name,
		columns: columns{cols: cols},
	}
}

type computedResult struct {
	v   any
	err error
}

type columnarRow struct {
	i    int
	memo map[string]computedResult
}

// resolveComputed evaluates a computed column lazily, memoizing the result
// (including errors) per row instance so the evaluator runs at most once.
func (row *columnarRow) resolveComputed(cc ComputedColumn, rs Recordset) (any, error) {
	name := cc.Name()
	if row.memo == nil {
		row.memo = make(map[string]computedResult)
	} else if cached, ok := row.memo[name]; ok {
		return cached.v, cached.err
	}

	stored := make(map[string]any)
	for _, col := range rs.Columns() {
		if _, isComputed := col.(ComputedColumn); isComputed {
			continue
		}
		v, err := col.GetValue(row.i)
		if err != nil {
			row.memo[name] = computedResult{nil, err}
			return nil, err
		}
		stored[col.Name()] = v
	}

	v, err := cc.Evaluator().Eval(stored)
	row.memo[name] = computedResult{v, err}
	return v, err
}

func (row *columnarRow) Data(rs Recordset) (data []any, err error) {
	data = make([]any, rs.ColumnsCount())
	for i := 0; i < rs.ColumnsCount(); i++ {
		c := rs.GetColumnByIndex(i)
		if cc, ok := c.(ComputedColumn); ok {
			if data[i], err = row.resolveComputed(cc, rs); err != nil {
				return
			}
			continue
		}
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
	if cc, ok := col.(ComputedColumn); ok {
		return row.resolveComputed(cc, rs)
	}
	return col.GetValue(row.i)
}

func (row *columnarRow) GetValueByName(name string, rs Recordset) (value any, err error) {
	col := rs.GetColumnByName(name)
	if col == nil {
		return nil, fmt.Errorf("unexpected column name: %s", name)
	}
	if cc, ok := col.(ComputedColumn); ok {
		return row.resolveComputed(cc, rs)
	}
	return col.GetValue(row.i)
}

func (row *columnarRow) SetValueByName(name string, value any, rs Recordset) error {
	col := rs.GetColumnByName(name)
	if _, ok := col.(ComputedColumn); ok {
		return fmt.Errorf("cannot set value on computed column %q", name)
	}
	return col.SetValue(row.i, value)
}

func (row *columnarRow) SetValueByIndex(i int, value any, rs Recordset) error {
	col := rs.GetColumnByIndex(i)
	if _, ok := col.(ComputedColumn); ok {
		return fmt.Errorf("cannot set value on computed column %q", col.Name())
	}
	return col.SetValue(row.i, value)
}
