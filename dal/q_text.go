package dal

import "context"

var _ TextQuery = (*textQuery)(nil)

type textQuery struct {
	text   string
	args   []QueryArg
	getKey func(data any, args []QueryArg) *Key
}

func (t textQuery) GetReader(ctx context.Context, db DB) (reader Reader, err error) {
	return db.GetReader(ctx, t)
}

func (t textQuery) ReadRecords(ctx context.Context, db DB, o ...ReaderOption) (records []Record, err error) {
	return db.ReadAllRecords(ctx, t, o...)
}

func (t textQuery) Text() string {
	return t.text
}

func (t textQuery) Args() []QueryArg {
	return t.args
}

func (t textQuery) String() string {
	return t.text
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
