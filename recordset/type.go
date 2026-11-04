package recordset

import "strconv"

// Type distinguishes unknown, table, view, and stored-procedure values.
type Type int

const (
	// UnknownType is the zero Type. Its String method returns "Unknown".
	UnknownType Type = iota
	// Table is the table value of Type. Its String method returns "Table".
	Table
	// View is the view value of Type. Its String method returns "View".
	View
	// StoredProcedure is the stored-procedure value of Type. Its String method returns "StoredProcedure".
	StoredProcedure
)

func (t Type) String() string {
	switch t {
	case UnknownType:
		return "Unknown"
	case Table:
		return "Table"
	case View:
		return "View"
	case StoredProcedure:
		return "StoredProcedure"
	default:
		return strconv.Itoa(int(t))
	}
}
