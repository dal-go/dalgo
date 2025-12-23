package dal

import (
	"context"
	"reflect"
)

type Query interface {
	String() string

	// Offset specifies the number of records to skip
	Offset() int

	// Limit specifies the maximum number of records to be returned
	Limit() int

	GetRecordsReader(ctx context.Context, qe QueryExecutor) (reader RecordsReader, err error)
	GetRecordsetReader(ctx context.Context, qe QueryExecutor) (reader RecordsetReader, err error)
}

// TextQuery defines an interface to represent a query with text and associated arguments.
type TextQuery interface {
	Query
	Text() string
	Args() []QueryArg
}

// StructuredQuery represents a query to a recordsetSource
type StructuredQuery interface {
	Query

	// From - defines target table/recordsetSource
	From() FromSource

	// Where defines filter condition
	Where() Condition

	// GroupBy defines expressions to group by
	GroupBy() []Expression

	// OrderBy defines expressions to order by
	OrderBy() []OrderExpression

	// Columns specifies columns to return
	Columns() []Column

	// IntoRecord provides a function that creates a record for a new row
	IntoRecord() Record // TODO: Should this be moved into Query.GetRecordsReader ?

	// IDKind defines the type of the ID
	IDKind() reflect.Kind // TODO: what about composite keys?

	// StartFrom specifies the startCursor/point to start from
	StartFrom() Cursor
}
