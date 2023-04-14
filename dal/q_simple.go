package dal

import "reflect"

type SingleSource interface {
	Where(conditions ...Condition) Selector
}

type Selector interface {
	Where(conditions ...Condition) Selector
	WhereField(name string, operator Operator, v any) Selector
	OrderBy(expressions ...OrderExpression) Selector
	SelectInto(func() Record) Query
	SelectKeysOnly(idKind reflect.Kind) Query
}

var _ Selector = (*selector)(nil)

func From(collection string, conditions ...Condition) Selector {
	return &selector{collection: collection, conditions: conditions}
}

type selector struct {
	collection string
	conditions []Condition
	orderBy    []OrderExpression
}

func (s selector) OrderBy(expressions ...OrderExpression) Selector {
	return selector{
		collection: s.collection,
		conditions: s.conditions,
		orderBy:    append(s.orderBy, expressions...),
	}
}

func (s selector) Where(conditions ...Condition) Selector {
	return selector{
		collection: s.collection,
		conditions: append(s.conditions, conditions...),
		orderBy:    s.orderBy,
	}
}

func (s selector) WhereField(name string, operator Operator, v any) Selector {
	return selector{
		collection: s.collection,
		conditions: append(s.conditions, WhereField(name, operator, v)),
		orderBy:    s.orderBy,
	}
}

func (s selector) SelectInto(into func() Record) Query {
	q := Query{
		From: &CollectionRef{Name: s.collection},
		Into: into,
	}
	switch len(s.conditions) {
	case 0: // no conditions
	case 1:
		q.Where = s.conditions[0]
	default:
		q.Where = groupCondition{conditions: s.conditions, operator: And}
	}
	return q
}

func (s selector) SelectKeysOnly(idKind reflect.Kind) Query {
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
		q.Where = groupCondition{conditions: s.conditions, operator: And}
	}
	return q
}
