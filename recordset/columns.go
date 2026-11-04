package recordset

// ColumnAccessor reads the columns of a recordset.
type ColumnAccessor interface {
	// Columns returns the columns.
	Columns() []Column[any]
	// ColumnsCount returns the number of columns.
	ColumnsCount() int
	// GetColumnByIndex returns the column at i, or nil when i is past the last column.
	GetColumnByIndex(i int) Column[any]
	// GetColumnByName returns the column named name, or nil when no column has that name.
	GetColumnByName(name string) Column[any]
	// GetColumnIndex returns the index of the column named name, or -1 when it is absent.
	GetColumnIndex(name string) int
}

type columns struct {
	cols []Column[any]
}

// Columns returns a copy of the column slice.
func (c *columns) Columns() (columns []Column[any]) {
	columns = make([]Column[any], len(c.cols))
	copy(columns, c.cols)
	return
}

// ColumnsCount returns the number of columns.
func (c *columns) ColumnsCount() int {
	return len(c.cols)
}

// GetColumnByIndex returns the column at i, or nil when i is greater than or equal to the column count.
func (c *columns) GetColumnByIndex(i int) Column[any] {
	if i >= len(c.cols) {
		return nil
	}
	return c.cols[i]
}

// GetColumnByName returns the first column whose name equals name, or nil when none match.
func (c *columns) GetColumnByName(name string) Column[any] {
	for _, col := range c.cols {
		if col.Name() == name {
			return col
		}
	}
	return nil
}

// GetColumnIndex returns the index of the first column named name, or -1 when none match.
func (c *columns) GetColumnIndex(name string) int {
	for i, column := range c.cols {
		if column.Name() == name {
			return i
		}
	}
	return -1
}
