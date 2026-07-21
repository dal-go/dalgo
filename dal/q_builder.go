package dal

import (
	"reflect"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type SingleSource interface {
	Where(conditions ...Condition) IQueryBuilder
}

type Cursor string

type IQueryBuilder interface {
	Clone() IQueryBuilder
	Offset(int) IQueryBuilder
	Limit(int) IQueryBuilder
	Where(conditions ...Condition) IQueryBuilder
	WhereField(name string, operator Operator, v any) IQueryBuilder
	WhereInArrayField(name string, v any) IQueryBuilder
	WhereArrayContains(name string, v any) IQueryBuilder
	WhereArrayContainsAny(name string, values any) IQueryBuilder
	GroupBy(expressions ...Expression) IQueryBuilder
	Having(conditions ...Condition) IQueryBuilder
	OrderBy(expressions ...OrderExpression) IQueryBuilder
	SelectIntoRecord(func() record.Record) StructuredQuery
	SelectIntoRecordset(options ...recordset.Option) StructuredQuery
	SelectKeysOnly(idKind reflect.Kind) StructuredQuery
	SelectColumns(columns ...Column) StructuredQuery
	StartFrom(cursor Cursor) IQueryBuilder
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

func (s *QueryBuilder) Clone() IQueryBuilder {
	s2 := *s
	return &s2
}

func (s *QueryBuilder) StartFrom(cursor Cursor) IQueryBuilder {
	s.q.startCursor = cursor
	return s
}

func (s *QueryBuilder) Offset(i int) IQueryBuilder {
	s.q.offset = i
	return s
}

func (s *QueryBuilder) Limit(i int) IQueryBuilder {
	s.q.limit = i
	return s
}

func (s *QueryBuilder) OrderBy(expressions ...OrderExpression) IQueryBuilder {
	s.q.orderBy = append(s.q.orderBy, expressions...)
	return s
}

func (s *QueryBuilder) Where(conditions ...Condition) IQueryBuilder {
	s.conditions = append(s.conditions, conditions...)
	return s
}

func (s *QueryBuilder) GroupBy(expressions ...Expression) IQueryBuilder {
	s.q.groupBy = append(s.q.groupBy, expressions...)
	return s
}

func (s *QueryBuilder) Having(conditions ...Condition) IQueryBuilder {
	s.havingConditions = append(s.havingConditions, conditions...)
	return s
}

func (s *QueryBuilder) WhereField(name string, operator Operator, v any) IQueryBuilder {
	s.conditions = append(s.conditions, WhereField(name, operator, v))
	return s
}

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

func (s *QueryBuilder) SelectIntoRecord(into func() record.Record) StructuredQuery {
	q := s.newQuery()
	q.intoRecord = into
	return q
}

func (s *QueryBuilder) SelectIntoRecordset(options ...recordset.Option) StructuredQuery {
	q := s.newQuery()
	q.recordsetOptions = options
	return q
}

func (s *QueryBuilder) SelectKeysOnly(idKind reflect.Kind) StructuredQuery {
	q := s.newQuery()
	q.idKind = idKind
	return q
}

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
