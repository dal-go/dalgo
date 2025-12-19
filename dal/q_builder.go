package dal

import "reflect"

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
	OrderBy(expressions ...OrderExpression) IQueryBuilder
	SelectInto(func() Record) StructuredQuery
	SelectKeysOnly(idKind reflect.Kind) StructuredQuery
	StartFrom(cursor Cursor) IQueryBuilder
}

var _ IQueryBuilder = (*QueryBuilder)(nil)

// NewQueryBuilder creates a new IQueryBuilder - it's an entry point to build a query.
// We can use From() directly but this is easier to remember.
func NewQueryBuilder(from FromSource) *QueryBuilder {
	return &QueryBuilder{q: structuredQuery{from: from}}
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
	conditions []Condition
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

func (s *QueryBuilder) WhereField(name string, operator Operator, v any) IQueryBuilder {
	s.conditions = append(s.conditions, WhereField(name, operator, v))
	return s
}

func (s *QueryBuilder) WhereInArrayField(name string, v any) IQueryBuilder {
	s.conditions = append(s.conditions, Comparison{Left: Constant{Value: v}, Operator: In, Right: FieldRef{name: name}})
	return s
}

func (s *QueryBuilder) SelectInto(into func() Record) StructuredQuery {
	q := s.newQuery()
	q.into = into
	return q
}

func (s *QueryBuilder) SelectKeysOnly(idKind reflect.Kind) StructuredQuery {
	q := s.newQuery()
	q.idKind = idKind
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
	return s.q
}
