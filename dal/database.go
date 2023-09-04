package dal

// Database is an interface that defines a DB provider
type Database interface {

	// ID is an identifier provided at time of Database creation
	ID() string

	// Adapter provides information about underlying name to access data
	Adapter() Adapter

	// TransactionCoordinator provides shortcut methods to work with transactions
	// without opening connection explicitly.
	TransactionCoordinator

	// Removed members:
	// ===================================================================================
	// Close() error - is part of a connection.
	// Connect(ctx context.Context) (connection, error) - considered unneeded
	// ReadSession - decided to sacrifice some simplicity for the sake of interoperability
}
