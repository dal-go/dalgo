package dal

import (
	"context"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

var _ TextQuery = (*textQuery)(nil)

type textQuery struct {
	text             string
	args             []QueryArg
	offset           int
	limit            int
	getKey           func(data any, args []QueryArg) *record.Key
	recordsetOptions []recordset.Option
}

// GetRecordsetReader delegates to qe.ExecuteQueryToRecordsetReader and passes q's recordset options.
func (q textQuery) GetRecordsetReader(ctx context.Context, qe QueryExecutor) (reader RecordsetReader, err error) {
	return qe.ExecuteQueryToRecordsetReader(ctx, q, q.recordsetOptions...)
}

// Offset returns the number of records this query skips.
func (q textQuery) Offset() int {
	return q.offset
}

// Limit returns the maximum number of records this query returns.
func (q textQuery) Limit() int {
	return q.limit
}

// GetRecordsReader delegates to qe.ExecuteQueryToRecordsReader.
func (q textQuery) GetRecordsReader(ctx context.Context, qe QueryExecutor) (reader RecordsReader, err error) {
	return qe.ExecuteQueryToRecordsReader(ctx, q)
}

// Text returns the query text.
func (q textQuery) Text() string {
	return q.text
}

// Args returns the query arguments.
func (q textQuery) Args() []QueryArg {
	return q.args
}

func (q textQuery) String() string {
	return q.text
}

// QueryArg is a named value supplied to a text query.
type QueryArg struct {
	Name  string
	Value any
}

// NewTextQuery returns a text query that stores text, getKey, and args.
func NewTextQuery(text string, getKey func(data any, args []QueryArg) *record.Key, args ...QueryArg) TextQuery {
	return &textQuery{
		text:   text,
		args:   args,
		getKey: getKey,
	}
}
