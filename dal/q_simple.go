package dal

type SingleSource interface {
	Where(conditions ...Condition) Selector
}

type Selector interface {
	SelectInto(func() Record) Query
}

var _ Selector = (*selector)(nil)

func From(collection string, conditions ...Condition) Selector {
	return &selector{collection: collection, conditions: conditions}
}

type selector struct {
	collection string
	conditions []Condition
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
