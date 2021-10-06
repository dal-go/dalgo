package dalgo

import (
	"context"
)

// Database is an interface that defines a DB provider
type Database interface {
	TransactionCoordinator
	Session
}

type TransactionWorker = func(ctx context.Context, tx Transaction) error

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunReadOnlyTransaction(ctx context.Context, f TransactionWorker, options ...TransactionOption) error
	RunReadWriteTransaction(ctx context.Context, f TransactionWorker, options ...TransactionOption) error
}

// Transaction defines an interface for a transaction
type Transaction interface {
	Session
}

// Session defines interface
type Session interface {

	// Insert inserts a single record in database
	Insert(c context.Context, record Record, opts ...InsertOption) error

	// Upsert updates or inserts a single record in database
	Upsert(ctx context.Context, record Record) error

	// Get gets a single record from database by key
	Get(ctx context.Context, record Record) error

	// GetMulti gets multiples records from database by keys
	GetMulti(ctx context.Context, records []Record) error

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

	// Select executes a query on database
	Select(ctx context.Context, query Query) (Reader, error)
}

// Validatable defines an object that can be validated
type Validatable interface {
	Validate() error
}

// Reader reads records one by one
type Reader interface {
	Next() (Record, error)
}
