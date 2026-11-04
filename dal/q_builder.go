package dal

import (
	"reflect"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// SingleSource adds WHERE conditions to a query builder.
type SingleSource interface {
	// Where adds filter conditions and returns the builder.
	Where(conditions ...Condition) IQueryBuilder
}

// Cursor is a position in a result set used to resume a query.
type Cursor string

// IQueryBuilder builds a structured query.
type IQueryBuilder interface {
	// Clone returns a copy of the builder.
	Clone() IQueryBuilder
	// Offset sets the number of records to skip.
	Offset(int) IQueryBuilder
	// Limit sets the maximum number of records to return.
	Limit(int) IQueryBuilder
	// Where adds filter conditions.
	Where(conditions ...Condition) IQueryBuilder
	// WhereField adds a comparison of the named field to v.
	WhereField(name string, operator Operator, v any) IQueryBuilder
	// WhereInArrayField adds a condition that v is in the named array field.
	WhereInArrayField(name string, v any) IQueryBuilder
	// WhereArrayContains adds a condition that the named array field contains v.
	WhereArrayContains(name string, v any) IQueryBuilder
	// WhereArrayContainsAny adds a condition that the named array field contains any of values.
	WhereArrayContainsAny(name string, values any) IQueryBuilder
	// GroupBy adds expressions to group by.
	GroupBy(expressions ...Expression) IQueryBuilder
	// Having adds post-aggregation filter conditions.
	Having(conditions ...Condition) IQueryBuilder
	// OrderBy adds expressions to order by.
	OrderBy(expressions ...OrderExpression) IQueryBuilder
	// SelectIntoRecord finishes the query, reading each row into a record created by the function.
	SelectIntoRecord(func() record.Record) StructuredQuery
	// SelectIntoRecordset finishes the query with the given recordset options.
	SelectIntoRecordset(options ...recordset.Option) StructuredQuery
	// SelectKeysOnly finishes the query returning keys of idKind.
	SelectKeysOnly(idKind reflect.Kind) StructuredQuery
	// SelectColumns finishes the query returning the given columns.
	SelectColumns(columns ...Column) StructuredQuery
	// StartFrom sets the cursor to start from and clears any start-after cursor.
	StartFrom(cursor Cursor) IQueryBuilder
	// StartAfter sets the cursor to start after and clears any start-from cursor.
	StartAfter(cursor Cursor) IQueryBuilder
}

var _ IQueryBuilder = (*QueryBuilder)(nil)

// NewQueryBuilder creates a new IQueryBuilder - it's an entry point to build a query.
// We can use From() directly but this is easier to remember.
func NewQueryBuilder(from FromSource) *QueryBuilder {
	return &QueryBuilder{
		q: structuredQuery{
			from: from,
		},
	}
}

// From creates a new IQueryBuilder with optional conditions.
// We can use NewQueryBuilder() directly but this is shorter.
func From(source RecordsetSource) FromSource {
	return &from{RecordsetSource: source}
}

// QueryBuilder accumulates a structured query and its pending conditions.
type QueryBuilder struct {
	q structuredQuery
	//recordsetSource RecordsetSource
	//offset          int
	//limit           int
	conditions       []Condition
	havingConditions []Condition
	//orderBy         []OrderExpression
	//startCursor     Cursor
}

// Clone returns a shallow copy of the builder.
func (s *QueryBuilder) Clone() IQueryBuilder {
	s2 := *s
	return &s2
}

// StartFrom sets the start cursor and clears the start-after cursor.
func (s *QueryBuilder) StartFrom(cursor Cursor) IQueryBuilder {
	s.q.startCursor = cursor
	s.q.startAfter = ""
	return s
}

// StartAfter sets the start-after cursor and clears the start cursor.
func (s *QueryBuilder) StartAfter(cursor Cursor) IQueryBuilder {
	s.q.startAfter = cursor
	s.q.startCursor = ""
	return s
}

// Offset sets the number of records to skip.
func (s *QueryBuilder) Offset(i int) IQueryBuilder {
	s.q.offset = i
	return s
}

// Limit sets the maximum number of records to return.
func (s *QueryBuilder) Limit(i int) IQueryBuilder {
	s.q.limit = i
	return s
}

// OrderBy appends order expressions to the query.
func (s *QueryBuilder) OrderBy(expressions ...OrderExpression) IQueryBuilder {
	s.q.orderBy = append(s.q.orderBy, expressions...)
	return s
}

// Where appends filter conditions. They are combined when the query is finished.
func (s *QueryBuilder) Where(conditions ...Condition) IQueryBuilder {
	s.conditions = append(s.conditions, conditions...)
	return s
}

// GroupBy appends group-by expressions to the query.
func (s *QueryBuilder) GroupBy(expressions ...Expression) IQueryBuilder {
	s.q.groupBy = append(s.q.groupBy, expressions...)
	return s
}

// Having appends post-aggregation conditions. They are combined when the query is finished.
func (s *QueryBuilder) Having(conditions ...Condition) IQueryBuilder {
	s.havingConditions = append(s.havingConditions, conditions...)
	return s
}

// WhereField appends the condition returned by the WhereField function.
func (s *QueryBuilder) WhereField(name string, operator Operator, v any) IQueryBuilder {
	s.conditions = append(s.conditions, WhereField(name, operator, v))
	return s
}

// WhereInArrayField appends a comparison of v in the named field.
func (s *QueryBuilder) WhereInArrayField(name string, v any) IQueryBuilder {
	s.conditions = append(s.conditions, Comparison{Left: Constant{Value: v}, Operator: In, Right: FieldRef{name: name}})
	return s
}

// WhereArrayContains adds a condition that an array field contains the given value.
// Adapters translate it to the platform's array membership operator,
// e.g. Firestore's "array-contains". It is an alias for WhereInArrayField.
func (s *QueryBuilder) WhereArrayContains(name string, v any) IQueryBuilder {
	return s.WhereInArrayField(name, v)
}

// WhereArrayContainsAny adds a condition that an array field contains at least one
// element of the given values, e.g. Firestore's "array-contains-any".
// The values must be a dal.Array or a slice type supported by NewArray.
func (s *QueryBuilder) WhereArrayContainsAny(name string, values any) IQueryBuilder {
	arr, ok := values.(Array)
	if !ok {
		arr = NewArray(values)
	}
	s.conditions = append(s.conditions, Comparison{Left: FieldRef{name: name}, Operator: In, Right: arr})
	return s
}

// SelectIntoRecord returns the built query, combining accumulated WHERE and HAVING conditions, with its row factory set to into.
func (s *QueryBuilder) SelectIntoRecord(into func() record.Record) StructuredQuery {
	q := s.newQuery()
	q.intoRecord = into
	return q
}

// SelectIntoRecordset returns the built query, combining accumulated WHERE and HAVING conditions, with the given recordset options.
func (s *QueryBuilder) SelectIntoRecordset(options ...recordset.Option) StructuredQuery {
	q := s.newQuery()
	q.recordsetOptions = options
	return q
}

// SelectKeysOnly returns the built query, combining accumulated WHERE and HAVING conditions, with its ID kind set to idKind.
func (s *QueryBuilder) SelectKeysOnly(idKind reflect.Kind) StructuredQuery {
	q := s.newQuery()
	q.idKind = idKind
	return q
}

// SelectColumns returns the built query, combining accumulated WHERE and HAVING conditions, with the given columns.
func (s *QueryBuilder) SelectColumns(columns ...Column) StructuredQuery {
	q := s.newQuery()
	q.columns = columns
	return q
}

func (s *QueryBuilder) newQuery() structuredQuery {
	switch len(s.conditions) {
	case 0: // no conditions
	case 1:
		s.q.where = s.conditions[0]
	default:
		s.q.where = GroupCondition{conditions: s.conditions, operator: And}
	}
	switch len(s.havingConditions) {
	case 0: // no having conditions
	case 1:
		s.q.having = s.havingConditions[0]
	default:
		s.q.having = GroupCondition{conditions: s.havingConditions, operator: And}
	}
	return s.q
}
