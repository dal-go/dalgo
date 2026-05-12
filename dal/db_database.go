package dal

// DB is an interface that defines a database provider
type DB interface {

	// ID is an identifier provided at time of DB creation
	ID() string

	// Adapter provides information about underlying name to access data
	Adapter() Adapter

	// Schema provides schema for the DB - for example, how keys are mapped to columns
	Schema() Schema

	// TransactionCoordinator provides shortcut methods to work with transactions
	// without opening a connection explicitly.
	TransactionCoordinator

	// ReadSession implements a virtual read session that opens connection/session for each read call on DB level
	// TODO: consider to sacrifice some simplicity for the sake of interoperability?
	ReadSession

	// ConcurrencyAware reports whether this backend supports concurrent
	// open connections. Drivers should embed NoConcurrency or
	// ConcurrencyAvailable in their concrete type to satisfy this.
	ConcurrencyAware

	// Removed members:
	// ===================================================================================
	// Close() error - is part of a connection.
	// Connect(ctx context.Context) (connection, error) - considered unneeded
}
