package recordset

// Row defines an interface for storing recordset row data.
// We intentionally pass cols information to functions of the interface
// so we do not need to allocate memory for cols pointer in each row.
type Row interface {
	GetValueByName(name string, rs Recordset) (any, error)
	SetValueByName(name string, value any, rs Recordset) error
	GetValueByIndex(i int, rs Recordset) (any, error)
	SetValueByIndex(i int, value any, rs Recordset) error
	Data(rs Recordset) (data []any, err error)
}
