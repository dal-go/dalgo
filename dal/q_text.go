package dal

var _ TextQuery = (*textQuery)(nil)

type textQuery struct {
	text   string
	args   []QueryArg
	getKey func(data any, args []QueryArg) *Key
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
