package recordset

import "strconv"

type Type int

const (
	UnknownType Type = iota
	Table
	View
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
