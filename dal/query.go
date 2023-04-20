package dal

import "reflect"

// Query represents a query to a collection
type Query interface {

	// From defines target table/collection
	From() *CollectionRef

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
}
