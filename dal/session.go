package dal

// ReadSession defines methods that query data from DB and does not modify it
type ReadSession interface {
	Connection
	Getter
	MultiGetter
	QueryExecutor
}

// WriteSession defines methods that can modify database
type WriteSession interface {
	Connection
	Setter
	MultiSetter
	Deleter
	MultiDeleter
	Updater
	MultiUpdater
	Inserter
}

// ReadwriteSession defines methods that can read & modify database. Some databases allow to modify data without transaction.
type ReadwriteSession interface {
	ReadSession
	WriteSession
}
