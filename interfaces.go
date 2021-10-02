package dalgo

import (
	"context"
	"errors"
)

// TypeOfID represents type of Value: IsComplexID, IsStringID, IsIntID
type TypeOfID int

// Validatable defines an object that can be validated
type Validatable interface {
	Validate() error
}

// MultiUpdater is an interface that describe DB provider that can update multiple records at once (batch mode)
type MultiUpdater interface {
	UpdateMulti(c context.Context, keys []*Key, updates []Update, preconditions ...Precondition) error
}

// MultiGetter is an interface that describe DB provider that can get multiple records at once (batch mode)
type MultiGetter interface {
	GetMulti(ctx context.Context, records []Record) error
}

// MultiSetter is an interface that describe DB provider that can set multiple records at once (batch mode)
type MultiSetter interface {
	SetMulti(ctx context.Context, records []Record) error
}

// Getter is an interface that describe DB provider that can get a single record by child
type Getter interface {
	Get(ctx context.Context, record Record) error
}

// Setter is an interface that describe DB provider that can set a single record by child
type Setter interface {
	Set(ctx context.Context, record Record) error
}

// Upserter is an interface that describe DB provider that can upsert a single record by child
type Upserter interface {
	Upsert(ctx context.Context, record Record) error
}

// Updater is an interface that describe DB provider that can update a single EXISTING record by a child
type Updater interface {
	Update(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error
}

// Deleter is an interface that describe DB provider that can delete a single record by child
type Deleter interface {
	Delete(ctx context.Context, key *Key) error
}

type MultiDeleter interface {
	DeleteMulti(ctx context.Context, keys []*Key) error
}

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunInTransaction(
		ctx context.Context,
		f func(ctx context.Context, tx Transaction) error,
		options ...TransactionOption,
	) error
}

type RecordConstructor = func() Record

var ErrNoMoreRecords = errors.New("no more errors")

// Reader reads records one by one
type Reader interface {
	Next() (Record, error)
}

// Selector queries records by condition
type Selector interface {
	Select(ctx context.Context, query Query) (Reader, error)
}

// Session defines interface
type Session interface {
	Inserter
	Upserter
	Getter
	Setter
	Updater
	Deleter
	MultiGetter
	MultiSetter
	MultiUpdater
	MultiDeleter
	Selector
}

// Transaction defines an interface for a transaction
type Transaction interface {
	Session
}

// Database is an interface that define a DB provider
type Database interface {
	TransactionCoordinator
	Session
}
