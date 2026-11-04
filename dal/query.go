package dal

import (
	"context"
	"reflect"

	"github.com/dal-go/record"
)

// Query is a query that can be read as records or as a recordset.
type Query interface {
	// String returns the textual form of the query.
	String() string

	// Offset specifies the number of records to skip
	Offset() int

	// Limit specifies the maximum number of records to be returned
	Limit() int

	// GetRecordsReader returns a records reader for this query from qe.
	GetRecordsReader(ctx context.Context, qe QueryExecutor) (reader RecordsReader, err error)
	// GetRecordsetReader returns a recordset reader for this query from qe.
	GetRecordsetReader(ctx context.Context, qe QueryExecutor) (reader RecordsetReader, err error)
}

// TextQuery defines an interface to represent a query with text and associated arguments.
type TextQuery interface {
	Query
	// Text returns the query text.
	Text() string
	// Args returns the arguments bound to the query text.
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

	// Having defines a post-aggregation filter condition
	Having() Condition

	// OrderBy defines expressions to order by
	OrderBy() []OrderExpression

	// Columns specifies columns to return
	Columns() []Column

	// IntoRecord provides a function that creates a record for a new row
	IntoRecord() record.Record // TODO: Should this be moved into Query.GetRecordsReader ?

	// IDKind defines the type of the ID
	IDKind() reflect.Kind // TODO: what about composite keys?

	// StartFrom specifies the startCursor/point to start from
	StartFrom() Cursor
	// StartAfter returns the cursor after which reading starts.
	StartAfter() Cursor
}
