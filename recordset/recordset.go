package recordset

// Recordset provides interface for working with a set of row/column based data
type Recordset interface {
	ColumnAccessor
	// Name returns the recordset name.
	Name() string // Can be useful for debugging purpose (or for joining?)
	// NewRow returns a new row in the recordset.
	NewRow() Row

	// GetRow returns nil if out of range
	GetRow(i int) Row

	// RowsCount returns the number of rows.
	RowsCount() int
}
