package dal

import (
	"fmt"
	"reflect"
)

// Query represents a query to a recordsetSource
type Query interface {
	fmt.Stringer

	// From defines target table/recordsetSource
	From() RecordsetSource

	// Where defines filter condition
	Where() Condition

	// GroupBy defines expressions to group by
	GroupBy() []Expression

	// OrderBy defines expressions to order by
	OrderBy() []OrderExpression

	// Columns defines what columns to return
	Columns() []Column

	// Into defines the type of the result
	Into() func() Record

	// IDKind defines the type of the ID
	IDKind() reflect.Kind // TODO: what about composite keys?

	// Offset specifies number of records to skip
	Offset() int

	// Limit specifies maximum number of records to be returned
	Limit() int

	// StartFrom specifies the startCursor/point to start from
	StartFrom() Cursor
}
