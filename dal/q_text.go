package dal

import (
	"context"

	"github.com/dal-go/dalgo/recordset"
)

var _ TextQuery = (*textQuery)(nil)

type textQuery struct {
	text             string
	args             []QueryArg
	offset           int
	limit            int
	getKey           func(data any, args []QueryArg) *Key
	recordsetOptions []recordset.Option
}

func (q textQuery) GetRecordsetReader(ctx context.Context, qe QueryExecutor) (reader RecordsetReader, err error) {
	return qe.ExecuteQueryToRecordsetReader(ctx, q, q.recordsetOptions...)
}

func (q textQuery) Offset() int {
	return q.offset
}

func (q textQuery) Limit() int {
	return q.limit
}

func (q textQuery) GetRecordsReader(ctx context.Context, qe QueryExecutor) (reader RecordsReader, err error) {
	return qe.ExecuteQueryToRecordsReader(ctx, q)
}

func (q textQuery) Text() string {
	return q.text
}

func (q textQuery) Args() []QueryArg {
	return q.args
}

func (q textQuery) String() string {
	return q.text
}

type QueryArg struct {
	Name  string
	Value any
}

func NewTextQuery(text string, getKey func(data any, args []QueryArg) *Key, args ...QueryArg) TextQuery {
	return &textQuery{
		text:   text,
		args:   args,
		getKey: getKey,
	}
}
