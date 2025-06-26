package dal

type RecordsetSource interface {
	Name() string
	Alias() string
	recordsetSource()
}
