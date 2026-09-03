package dal

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

var _ StructuredQuery = (*structuredQuery)(nil)

// query holds definition of a query
type structuredQuery struct {

	// From defines target table/recordsetSource
	from FromSource

	// Where defines filter condition
	where Condition

	// GroupBy defines expressions to group by
	groupBy []Expression

	// Having defines a post-aggregation filter condition
	having Condition

	// OrderBy defines expressions to order by
	orderBy []OrderExpression

	// Columns define what columns to return
	columns []Column

	intoRecord       func() record.Record
	recordsetOptions []recordset.Option

	// Offset specifies the number of records to skip
	offset int

	// Limit specifies the maximum number of records to be returned
	limit int

	idKind reflect.Kind

	// StartCursor specifies the startCursor/point to start from
	startCursor Cursor
	startAfter  Cursor
}

func (q structuredQuery) GetRecordsReader(ctx context.Context, qe QueryExecutor) (reader RecordsReader, err error) {
	return qe.ExecuteQueryToRecordsReader(ctx, q)
}

func (q structuredQuery) GetRecordsetReader(ctx context.Context, qe QueryExecutor) (reader RecordsetReader, err error) {
	return qe.ExecuteQueryToRecordsetReader(ctx, q, q.recordsetOptions...)
}

func (q structuredQuery) Text() string {
	return q.String()
}

func (q structuredQuery) From() FromSource {
	return q.from
}

func (q structuredQuery) Where() Condition {
	return q.where
}

func (q structuredQuery) GroupBy() []Expression {
	return q.groupBy[:]
}

func (q structuredQuery) Having() Condition {
	return q.having
}

func (q structuredQuery) OrderBy() []OrderExpression {
	return q.orderBy[:]
}

func (q structuredQuery) Columns() []Column {
	return q.columns[:]
}

func (q structuredQuery) IntoRecord() record.Record {
	if q.intoRecord == nil {
		return nil
	}
	return q.intoRecord()
}

func (q structuredQuery) IDKind() reflect.Kind {
	return q.idKind
}

func (q structuredQuery) StartFrom() Cursor {
	return q.startCursor
}

func (q structuredQuery) StartAfter() Cursor { return q.startAfter }

func (q structuredQuery) Offset() int {
	return q.offset
}

func (q structuredQuery) Limit() int {
	return q.limit
}

func (q structuredQuery) String() string {
	return QueryString(q)
}

// QueryString renders a structured query in the textual form String() uses,
// reading every part through the StructuredQuery interface so a wrapper (see
// WithWhere) renders exactly like a native query.
func QueryString(q StructuredQuery) string {
	writer := bytes.NewBuffer(make([]byte, 0, 1024))
	_, _ = writer.WriteString("SELECT")
	limit := q.Limit()
	if limit > 0 {
		_, _ = writer.WriteString(" TOP " + strconv.Itoa(limit))
	}
	columns := q.Columns()
	where := q.Where()

	is1liner := len(columns) <= 1 &&
		(where == nil || reflect.TypeOf(where) == reflect.TypeOf(Comparison{}))

	switch len(columns) {
	case 0:
		_, _ = writer.WriteString(" *")
	case 1:
		_, _ = fmt.Fprint(writer, " ", columns[0].String())
	default:
		for i, col := range columns {
			_, _ = fmt.Fprint(writer, "\n\t", col.String())
			if i < len(columns)-1 {
				_, _ = writer.WriteString(",")
			}
		}
	}
	if from := q.From(); from != nil {
		if is1liner {
			_, _ = writer.WriteString(" ")
		} else {
			_, _ = writer.WriteString("\n")
		}
		var fromStr string
		switch base := from.Base().(type) {
		case CollectionRef:
			fromStr = base.Path()
		case *CollectionRef:
			fromStr = base.Path()
		case CollectionGroupRef:
			fromStr = base.Name()
		case *CollectionGroupRef:
			fromStr = base.Name()
		}
		_, _ = fmt.Fprintf(writer, "FROM [%v]", fromStr)
	}
	if where != nil {
		if is1liner {
			_, _ = writer.WriteString(" ")
		} else {
			_, _ = writer.WriteString("\n")
		}
		_, _ = writer.WriteString("WHERE " + where.String())
	}
	if groupBy := q.GroupBy(); len(groupBy) > 0 {
		_, _ = writer.WriteString("\nGROUP BY ")
		for i, expr := range groupBy {
			if i > 0 {
				_, _ = writer.WriteString(", ")
			}
			_, _ = writer.WriteString(expr.String())
		}
	}
	if having := q.Having(); having != nil {
		_, _ = writer.WriteString("\nHAVING " + having.String())
	}
	if orderBy := q.OrderBy(); len(orderBy) > 0 {
		_, _ = writer.WriteString("\nORDER BY ")
		for i, expr := range orderBy {
			if i > 0 {
				_, _ = writer.WriteString(", ")
			}
			_, _ = writer.WriteString(expr.String())
		}
	}
	if offset := q.Offset(); offset > 0 {
		_, _ = writer.WriteString("\nOFFSET " + strconv.Itoa(offset))
	}
	return writer.String()
}

// WithWhere returns a query identical to q except that its Where is
// condition. A query built by QueryBuilder is copied, so adapters and String()
// see an ordinary structured query; any other StructuredQuery implementation
// is wrapped and rendered through QueryString. Callers that narrow a query —
// an access policy conjoining a row condition, for example — use this rather
// than rebuilding the query, so columns, ordering, limits and cursors survive.
func WithWhere(q StructuredQuery, condition Condition) StructuredQuery {
	switch native := q.(type) {
	case structuredQuery:
		native.where = condition
		return native
	case *structuredQuery:
		clone := *native
		clone.where = condition
		return clone
	default:
		return queryOverride{StructuredQuery: q, where: condition, hasWhere: true}
	}
}

// WithColumns returns a query identical to q except that it selects columns.
// Like WithWhere it copies a builder query and wraps any other implementation.
func WithColumns(q StructuredQuery, columns []Column) StructuredQuery {
	switch native := q.(type) {
	case structuredQuery:
		native.columns = columns
		return native
	case *structuredQuery:
		clone := *native
		clone.columns = columns
		return clone
	default:
		return queryOverride{StructuredQuery: q, columns: columns, hasColumns: true}
	}
}

// queryOverride wraps a foreign StructuredQuery implementation with a
// replaced Where and/or Columns. It hands itself, not the wrapped query, to
// executors.
type queryOverride struct {
	StructuredQuery
	where      Condition
	hasWhere   bool
	columns    []Column
	hasColumns bool
}

func (w queryOverride) Where() Condition {
	if w.hasWhere {
		return w.where
	}
	return w.StructuredQuery.Where()
}

func (w queryOverride) Columns() []Column {
	if w.hasColumns {
		return w.columns
	}
	return w.StructuredQuery.Columns()
}

func (w queryOverride) String() string { return QueryString(w) }

func (w queryOverride) GetRecordsReader(ctx context.Context, qe QueryExecutor) (RecordsReader, error) {
	return qe.ExecuteQueryToRecordsReader(ctx, w)
}

func (w queryOverride) GetRecordsetReader(ctx context.Context, qe QueryExecutor) (RecordsetReader, error) {
	return qe.ExecuteQueryToRecordsetReader(ctx, w)
}

var _ fmt.Stringer = (*structuredQuery)(nil)

// And creates a new query by adding a condition to a predefined query
func (q structuredQuery) groupWithConditions(operator Operator, conditions ...Condition) structuredQuery {
	qry := structuredQuery{from: q.from}
	and := GroupCondition{operator: operator, conditions: make([]Condition, len(conditions)+1)}
	and.conditions[0] = q.where
	for i, condition := range conditions {
		and.conditions[i+1] = condition
	}
	qry.where = and
	return qry
}

// And creates an inherited query by adding AND conditions
func (q structuredQuery) And(conditions ...Condition) structuredQuery {
	return q.groupWithConditions(And, conditions...)
}

// Or creates an inherited query by adding OR conditions
func (q structuredQuery) Or(conditions ...Condition) structuredQuery {
	return q.groupWithConditions(Or, conditions...)
}
