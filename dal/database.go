package dal

import (
	"context"
)

// Database is an interface that defines a DB provider
type Database interface {
	TransactionCoordinator
	ReadSession
}

// ROTxWorker defines a callback to be called to do work within a readonly transaction
type ROTxWorker = func(ctx context.Context, tx ReadTransaction) error

// RWTxWorker defines a callback to be called to do work within a readwrite transaction
type RWTxWorker = func(ctx context.Context, tx ReadwriteTransaction) error

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	ReadTransactionCoordinator
	ReadwriteTransactionCoordinator
}

// ReadTransactionCoordinator creates a readonly transaction
type ReadTransactionCoordinator interface {
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
	// Options indicates parameters that were requested at time of transaction creation.
	Options() TransactionOptions
}

// ReadTransaction defines an interface for a transaction
type ReadTransaction interface {
	Transaction
	ReadSession
}

// ReadwriteTransaction defines an interface for a transaction
type ReadwriteTransaction interface {
	Transaction
	ReadwriteSession
}

// ReadSession defines methods that do not modify database
type ReadSession interface {

	// Get gets a single record from database by key
	Get(ctx context.Context, record Record) error

	// GetMulti gets multiples records from database by keys
	GetMulti(ctx context.Context, records []Record) error

	// Select executes a data retrieval query
	Select(ctx context.Context, query Query) (Reader, error)
}

// ReadwriteSession defines methods that can read & modify database
type ReadwriteSession interface {
	ReadSession
	WriteSession
}

// WriteSession defines methods that can modify database
type WriteSession interface {

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
