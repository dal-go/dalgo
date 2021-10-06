package dalgo

import (
	"context"
	"errors"
)

// Database is an interface that define a DB provider
type Database interface {
	TransactionCoordinator
	Session
}

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunInTransaction(
		ctx context.Context,
		f func(ctx context.Context, tx Transaction) error,
		options ...TransactionOption,
	) error
}

// Transaction defines an interface for a transaction
type Transaction interface {
	Session
}


// Session defines interface
type Session interface {
	Insert(c context.Context, record Record, opts ...InsertOption) error
	Upsert(ctx context.Context, record Record) error
	Get(ctx context.Context, record Record) error
	Set(ctx context.Context, record Record) error
	Update(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error
	Delete(ctx context.Context, key *Key) error
	GetMulti(ctx context.Context, records []Record) error
	SetMulti(ctx context.Context, records []Record) error
	UpdateMulti(c context.Context, keys []*Key, updates []Update, preconditions ...Precondition) error
	DeleteMulti(ctx context.Context, keys []*Key) error
	Select(ctx context.Context, query Query) (Reader, error)
}

// TypeOfID represents type of Value: IsComplexID, IsStringID, IsIntID
type TypeOfID int

// Validatable defines an object that can be validated
type Validatable interface {
	Validate() error
}

type RecordConstructor = func() Record

var ErrNoMoreRecords = errors.New("no more errors")

// Reader reads records one by one
type Reader interface {
	Next() (Record, error)
}




