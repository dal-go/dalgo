package dal

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
)

var _ Query = query{}
var _ Query = (*query)(nil)

// query holds definition of a query
type query struct {

	// From defines target table/collection
	from *CollectionRef

	// Where defines filter condition
	where Condition

	// GroupBy defines expressions to group by
	groupBy []Expression

	// OrderBy defines expressions to order by
	orderBy []OrderExpression

	// Columns defines what columns to return
	columns []Column

	into func() Record

	// Offset specifies number of records to skip
	offset int

	// Limit specifies maximum number of records to be returned
	limit int

	idKind reflect.Kind

	// StartCursor specifies the cursor/point to start from
	startCursor string
}

func (q query) From() *CollectionRef {
	return q.from
}

func (q query) Where() Condition {
	return q.where
}

func (q query) GroupBy() []Expression {
	return q.groupBy[:]
}

func (q query) OrderBy() []OrderExpression {
	return q.orderBy[:]
}

func (q query) Columns() []Column {
	return q.columns[:]
}

func (q query) Into() func() Record {
	return q.into
}

func (q query) IDKind() reflect.Kind {
	return q.idKind
}

func (q query) StartCursor() string {
	return q.startCursor
}

func (q query) Offset() int {
	return q.offset
}

func (q query) Limit() int {
	return q.limit
}

func (q query) String() string {
	writer := bytes.NewBuffer(make([]byte, 0, 1024))
	writer.WriteString("SELECT")
	if q.limit > 0 {
		writer.WriteString(" TOP " + strconv.Itoa(q.limit))
	}
	switch len(q.columns) {
	case 0:
		writer.WriteString(" *")
	case 1:
		_, _ = fmt.Fprint(writer, " ", q.columns[0].String())
	default:
		for _, col := range q.columns {
			_, _ = fmt.Fprint(writer, "\n\t", col.String())
		}
	}
	is1liner := len(q.columns) <= 1 &&
		(q.where == nil || reflect.TypeOf(q.where) == reflect.TypeOf(Comparison{}))

	if q.from != nil {
		if is1liner {
			writer.WriteString(" ")
		} else {
			writer.WriteString("\n")
		}
		writer.WriteString(fmt.Sprintf("FROM [%v]", q.from.Path()))
	}
	if q.where != nil {
		if is1liner {
			writer.WriteString(" ")
		} else {
			writer.WriteString("\n")
		}
		writer.WriteString("WHERE " + q.where.String())
	}
	if len(q.orderBy) > 0 {
		writer.WriteString("\nORDER BY ")
		for i, expr := range q.orderBy {
			if i > 0 {
				writer.WriteString(", ")
			}
			writer.WriteString(expr.String())
		}
	}
	if len(q.groupBy) > 0 {
		writer.WriteString("\nGROUP BY ")
		for i, expr := range q.groupBy {
			if i > 0 {
				writer.WriteString(", ")
			}
			writer.WriteString(expr.String())
		}
	}
	return writer.String()
}

var _ fmt.Stringer = (*query)(nil)

// And creates a new query by adding a condition to a predefined query
func (q query) groupWithConditions(operator Operator, conditions ...Condition) query {
	qry := query{from: q.from}
	and := GroupCondition{operator: operator, conditions: make([]Condition, len(conditions)+1)}
	and.conditions[0] = q.where
	for i, condition := range conditions {
		and.conditions[i+1] = condition
	}
	qry.where = and
	return qry
}

// And creates an inherited query by adding AND conditions
func (q query) And(conditions ...Condition) query {
	return q.groupWithConditions(And, conditions...)
}

// Or creates an inherited query by adding OR conditions
func (q query) Or(conditions ...Condition) query {
	return q.groupWithConditions(Or, conditions...)
}
