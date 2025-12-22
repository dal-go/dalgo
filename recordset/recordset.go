package recordset

// Recordset provides interface for working with a set of row/column based data
type Recordset interface {
	ColumnAccessor
	Name() string // Can be useful for debugging purpose (or for joining?)
	NewRow() Row

	// GetRow returns nil if out of range
	GetRow(i int) Row

	RowsCount() int
}
