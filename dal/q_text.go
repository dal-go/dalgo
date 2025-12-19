package dal

import "context"

var _ TextQuery = (*textQuery)(nil)

type textQuery struct {
	text   string
	args   []QueryArg
	offset int
	limit  int
	getKey func(data any, args []QueryArg) *Key
}

func (q textQuery) Offset() int {
	return q.offset
}

func (q textQuery) Limit() int {
	return q.limit
}

func (q textQuery) GetReader(ctx context.Context, db DB) (reader Reader, err error) {
	return db.GetReader(ctx, q)
}

func (q textQuery) ReadRecords(ctx context.Context, db DB, o ...ReaderOption) (records []Record, err error) {
	return db.ReadAllRecords(ctx, q, o...)
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
