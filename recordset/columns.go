package recordset

type ColumnAccessor interface {
	Columns() []Column[any]
	ColumnsCount() int
	GetColumnByIndex(i int) Column[any]
	GetColumnByName(name string) Column[any]
	GetColumnIndex(name string) int
}

type columns struct {
	cols []Column[any]
}

func (c *columns) Columns() (columns []Column[any]) {
	columns = make([]Column[any], len(c.cols))
	copy(columns, c.cols)
	return
}

func (c *columns) ColumnsCount() int {
	return len(c.cols)
}

func (c *columns) GetColumnByIndex(i int) Column[any] {
	if i >= len(c.cols) {
		return nil
	}
	return c.cols[i]
}

func (c *columns) GetColumnByName(name string) Column[any] {
	for _, col := range c.cols {
		if col.Name() == name {
			return col
		}
	}
	return nil
}

func (c *columns) GetColumnIndex(name string) int {
	for i, column := range c.cols {
		if column.Name() == name {
			return i
		}
	}
	return -1
}
