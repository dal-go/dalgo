package dal

// ReadSession defines methods that query data from DB and does not modify it
type ReadSession interface {
	Getter
	MultiGetter
	QueryExecutor
	// Excluded:
	// Connection - as DB interface implements a virtual ReadSession that can't be closed.
}

// WriteSession defines methods that can modify database
type WriteSession interface {
	Setter
	MultiSetter
	Deleter
	MultiDeleter
	Updater
	MultiUpdater
	Inserter
	MultiInserter
	//Connection // TODO
}

// ReadwriteSession defines methods that can read & modify database. Some databases allow to modify data without transaction.
type ReadwriteSession interface {
	ReadSession
	WriteSession
}
