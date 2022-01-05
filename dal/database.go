package dal

import (
	"context"
)

// Database is an interface that defines a DB provider
type Database interface {
	TransactionCoordinator
	ReadonlySession
}

// ROTxWorker defines a callback to be called to do work within a readonly transaction
type ROTxWorker = func(ctx context.Context, tx ReadonlyTransaction) error

// RWTxWorker defines a callback to be called to do work within a readwrite transaction
type RWTxWorker = func(ctx context.Context, tx ReadwriteTransaction) error

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	ReadonlyTransactionCoordinator
	ReadwriteTransactionCoordinator
}

// ReadonlyTransactionCoordinator creates a readonly transaction
type ReadonlyTransactionCoordinator interface {
	// RunReadonlyTransaction starts readonly transaction
	RunReadonlyTransaction(ctx context.Context, f ROTxWorker, options ...TransactionOption) error
}

// ReadwriteTransactionCoordinator creates a read-write transaction
type ReadwriteTransactionCoordinator interface {
	// RunReadwriteTransaction starts read-write transaction
	RunReadwriteTransaction(ctx context.Context, f RWTxWorker, options ...TransactionOption) error
}

// Transaction defines an instance of DALgo transaction
type Transaction interface {
	Options() TransactionOptions
}

// ReadonlyTransaction defines an interface for a transaction
type ReadonlyTransaction interface {
	Transaction
	ReadonlySession
}

// ReadwriteTransaction defines an interface for a transaction
type ReadwriteTransaction interface {
	Transaction
	ReadwriteSession
}

// ReadonlySession defines methods that do not modify database
type ReadonlySession interface {

	// Get gets a single record from database by key
	Get(ctx context.Context, record Record) error

	// GetMulti gets multiples records from database by keys
	GetMulti(ctx context.Context, records []Record) error

	// Select executes a data retrieval query
	Select(ctx context.Context, query Select) (Reader, error)
}

// ReadwriteSession defines methods that can modify database
type ReadwriteSession interface {
	ReadonlySession
	writeOnlySession
}

type writeOnlySession interface {

	// Insert inserts a single record in database
	Insert(c context.Context, record Record, opts ...InsertOption) error

	// Set sets a single record in database by key
	Set(ctx context.Context, record Record) error

	// SetMulti sets multiples records in database by keys
	SetMulti(ctx context.Context, records []Record) error

	// Update updates a single record in database by key
	Update(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error

	// UpdateMulti updates multiples records in database by keys
	UpdateMulti(c context.Context, keys []*Key, updates []Update, preconditions ...Precondition) error

	// Delete deletes a single record from database by key
	Delete(ctx context.Context, key *Key) error

	// DeleteMulti deletes multiple records from database by keys
	DeleteMulti(ctx context.Context, keys []*Key) error
}

// Reader reads records one by one
type Reader interface {

	// Next returns the next record for a query.
	// If no more records a nil record and ErrNoMoreRecords are returned.
	Next() (Record, error)
}
