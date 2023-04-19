package dal

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
)

// Query holds definition of a query
type Query struct {

	// From defines target table/collection
	From *CollectionRef

	// Where defines filter condition
	Where Condition

	// GroupBy defines expressions to group by
	GroupBy []Expression

	// OrderBy defines expressions to order by
	OrderBy []OrderExpression

	// Columns defines what columns to return
	Columns []Column

	Into func() Record

	// Offset specifies number of records to skip
	Offset int

	// Limit specifies maximum number of records to be returned
	Limit int

	IDKind reflect.Kind

	// StartCursor specifies the cursor/point to start from
	StartCursor string
}

func (q Query) String() string {
	writer := bytes.NewBuffer(make([]byte, 0, 1024))
	writer.WriteString("SELECT")
	if q.Limit > 0 {
		writer.WriteString(" TOP " + strconv.Itoa(q.Limit))
	}
	switch len(q.Columns) {
	case 0:
		writer.WriteString(" *")
	case 1:
		_, _ = fmt.Fprint(writer, " ", q.Columns[0].String())
	default:
		for _, col := range q.Columns {
			_, _ = fmt.Fprint(writer, "\n\t", col.String())
		}
	}
	is1liner := len(q.Columns) <= 1 &&
		(q.Where == nil || reflect.TypeOf(q.Where) == reflect.TypeOf(Comparison{}))

	if q.From != nil {
		if is1liner {
			writer.WriteString(" ")
		} else {
			writer.WriteString("\n")
		}
		fmt.Fprintf(writer, "FROM [%v]", q.From.Path())
	}
	if q.Where != nil {
		if is1liner {
			writer.WriteString(" ")
		} else {
			writer.WriteString("\n")
		}
		writer.WriteString("WHERE " + q.Where.String())
	}
	if len(q.OrderBy) > 0 {
		writer.WriteString("\nORDER BY ")
		for i, expr := range q.OrderBy {
			if i > 0 {
				writer.WriteString(", ")
			}
			writer.WriteString(expr.String())
		}
	}
	if len(q.GroupBy) > 0 {
		writer.WriteString("\nGROUP BY ")
		for i, expr := range q.GroupBy {
			if i > 0 {
				writer.WriteString(", ")
			}
			writer.WriteString(expr.String())
		}
	}
	return writer.String()
}

var _ fmt.Stringer = (*Query)(nil)

// And creates a new query by adding a condition to a predefined query
func (q Query) groupWithConditions(operator Operator, conditions ...Condition) Query {
	qry := Query{From: q.From}
	and := GroupCondition{operator: operator, conditions: make([]Condition, len(conditions)+1)}
	and.conditions[0] = q.Where
	for i, condition := range conditions {
		and.conditions[i+1] = condition
	}
	qry.Where = and
	return qry
}

// And creates an inherited query by adding AND conditions
func (q Query) And(conditions ...Condition) Query {
	return q.groupWithConditions(And, conditions...)
}

// Or creates an inherited query by adding OR conditions
func (q Query) Or(conditions ...Condition) Query {
	return q.groupWithConditions(Or, conditions...)
}
