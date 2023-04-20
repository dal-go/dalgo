package dal

import "reflect"

type SingleSource interface {
	Where(conditions ...Condition) QueryBuilder
}

type QueryBuilder interface {
	Offset(int) QueryBuilder
	Limit(int) QueryBuilder
	Where(conditions ...Condition) QueryBuilder
	WhereField(name string, operator Operator, v any) QueryBuilder
	OrderBy(expressions ...OrderExpression) QueryBuilder
	SelectInto(func() Record) query
	SelectKeysOnly(idKind reflect.Kind) query
}

var _ QueryBuilder = (*queryBuilder)(nil)

func From(collection string, conditions ...Condition) QueryBuilder {
	return &queryBuilder{collection: collection, conditions: conditions}
}

type queryBuilder struct {
	collection string
	offset     int
	limit      int
	conditions []Condition
	orderBy    []OrderExpression
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

func (s queryBuilder) SelectInto(into func() Record) query {
	q := s.newQuery()
	q.into = into
	return q
}

func (s queryBuilder) SelectKeysOnly(idKind reflect.Kind) query {
	q := s.newQuery()
	q.idKind = idKind
	return q
}

func (s queryBuilder) newQuery() query {
	q := query{
		from:    &CollectionRef{Name: s.collection},
		limit:   s.limit,
		orderBy: s.orderBy,
		offset:  s.offset,
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
