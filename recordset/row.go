package recordset

// Row defines an interface for storing recordset row data.
// We intentionally pass cols information to functions of the interface
// so we do not need to allocate memory for cols pointer in each row.
type Row interface {
	// GetValueByName returns the value of the named column from rs.
	GetValueByName(name string, rs Recordset) (any, error)
	// SetValueByName stores value in the named column of rs.
	SetValueByName(name string, value any, rs Recordset) error
	// GetValueByIndex returns the value of column i from rs.
	GetValueByIndex(i int, rs Recordset) (any, error)
	// SetValueByIndex stores value in column i of rs.
	SetValueByIndex(i int, value any, rs Recordset) error
	// Data returns the row values from rs in column order.
	Data(rs Recordset) (data []any, err error)
}
