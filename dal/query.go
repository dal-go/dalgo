package dal

import (
	"context"
	"reflect"
)

type Query interface {
	Text() string
	String() string
	GetReader(ctx context.Context, db DB) (reader Reader, err error)
	ReadRecords(ctx context.Context, db DB, o ...ReaderOption) (records []Record, err error)
}

// TextQuery defines an interface to represent a query with text and associated arguments.
type TextQuery interface {
	Text() string
	Args() []QueryArg
	String() string
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
