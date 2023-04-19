package dal

import "reflect"

type SingleSource interface {
	Where(conditions ...Condition) QueryBuilder
}

type QueryBuilder interface {
	Where(conditions ...Condition) QueryBuilder
	WhereField(name string, operator Operator, v any) QueryBuilder
	OrderBy(expressions ...OrderExpression) QueryBuilder
	SelectInto(func() Record) Query
	SelectKeysOnly(idKind reflect.Kind) Query
}

var _ QueryBuilder = (*queryBuilder)(nil)

func From(collection string, conditions ...Condition) QueryBuilder {
	return &queryBuilder{collection: collection, conditions: conditions}
}

type queryBuilder struct {
	collection string
	conditions []Condition
	orderBy    []OrderExpression
}

func (s queryBuilder) OrderBy(expressions ...OrderExpression) QueryBuilder {
	return queryBuilder{
		collection: s.collection,
		conditions: s.conditions,
		orderBy:    append(s.orderBy, expressions...),
	}
}

func (s queryBuilder) Where(conditions ...Condition) QueryBuilder {
	return queryBuilder{
		collection: s.collection,
		conditions: append(s.conditions, conditions...),
		orderBy:    s.orderBy,
	}
}

func (s queryBuilder) WhereField(name string, operator Operator, v any) QueryBuilder {
	return queryBuilder{
		collection: s.collection,
		conditions: append(s.conditions, WhereField(name, operator, v)),
		orderBy:    s.orderBy,
	}
}

func (s queryBuilder) SelectInto(into func() Record) Query {
	q := Query{
		From: &CollectionRef{Name: s.collection},
		Into: into,
	}
	switch len(s.conditions) {
	case 0: // no conditions
	case 1:
		q.Where = s.conditions[0]
	default:
		q.Where = GroupCondition{conditions: s.conditions, operator: And}
	}
	return q
}

func (s queryBuilder) SelectKeysOnly(idKind reflect.Kind) Query {
	q := Query{
		From:   &CollectionRef{Name: s.collection},
		Into:   nil,
		IDKind: idKind,
	}
	switch len(s.conditions) {
	case 0: // no conditions
	case 1:
		q.Where = s.conditions[0]
	default:
		q.Where = GroupCondition{conditions: s.conditions, operator: And}
	}
	return q
}
