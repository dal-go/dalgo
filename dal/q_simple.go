package dal

import "reflect"

type SingleSource interface {
	Where(conditions ...Condition) QueryBuilder
}

type Cursor string

type QueryBuilder interface {
	Offset(int) QueryBuilder
	Limit(int) QueryBuilder
	Where(conditions ...Condition) QueryBuilder
	WhereField(name string, operator Operator, v any) QueryBuilder
	OrderBy(expressions ...OrderExpression) QueryBuilder
	SelectInto(func() Record) Query
	SelectKeysOnly(idKind reflect.Kind) Query
	StartFrom(cursor Cursor) QueryBuilder
}

var _ QueryBuilder = (*queryBuilder)(nil)

func From(collection string, conditions ...Condition) QueryBuilder {
	return &queryBuilder{collection: collection, conditions: conditions}
}

type queryBuilder struct {
	collection  string
	offset      int
	limit       int
	conditions  []Condition
	orderBy     []OrderExpression
	startCursor Cursor
}

func (s queryBuilder) StartFrom(cursor Cursor) QueryBuilder {
	s.startCursor = cursor
	return s
}

func (s queryBuilder) Offset(i int) QueryBuilder {
	s.offset = i
	return s
}

func (s queryBuilder) Limit(i int) QueryBuilder {
	s.limit = i
	return s
}

func (s queryBuilder) OrderBy(expressions ...OrderExpression) QueryBuilder {
	s.orderBy = append(s.orderBy, expressions...)
	return s
}

func (s queryBuilder) Where(conditions ...Condition) QueryBuilder {
	s.conditions = append(s.conditions, conditions...)
	return s
}

func (s queryBuilder) WhereField(name string, operator Operator, v any) QueryBuilder {
	s.conditions = append(s.conditions, WhereField(name, operator, v))
	return s
}

func (s queryBuilder) SelectInto(into func() Record) Query {
	q := s.newQuery()
	q.into = into
	return q
}

func (s queryBuilder) SelectKeysOnly(idKind reflect.Kind) Query {
	q := s.newQuery()
	q.idKind = idKind
	return q
}

func (s queryBuilder) newQuery() theQuery {
	q := theQuery{
		from:        &CollectionRef{Name: s.collection},
		limit:       s.limit,
		orderBy:     s.orderBy,
		offset:      s.offset,
		startCursor: s.startCursor,
	}
	switch len(s.conditions) {
	case 0: // no conditions
	case 1:
		q.where = s.conditions[0]
	default:
		q.where = GroupCondition{conditions: s.conditions, operator: And}
	}
	return q
}
