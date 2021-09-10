package db

import (
	"context"
	"github.com/pkg/errors"
)

// TypeOfID represents type of ID: IsComplexID, IsStringID, IsIntID
type TypeOfID int

const (
	// IsComplexID is not implemented yet
	IsComplexID = iota

	// IsStringID for strings IDs
	IsStringID

	// IsIntID for integer IDs
	IsIntID
)

type Validatable interface {
	Validate() error
}

// RecordRef hold a reference to a single record within a root or nested recordset.
type RecordRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// RecordKey represents a full path to a given record (1 item in case of root recordset)
type RecordKey = []RecordRef

func validateRecordKey(key RecordKey) error {
	return nil
}

// NewRecordKey creates a new record key from a sequence of record's references
func NewRecordKey(refs ...RecordRef) RecordKey {
	return refs
}

// Record is an interface a struct should satisfy to comply with "strongo/db" library
type Record interface {
	Key() RecordKey
	Data() Validatable
	SetData(data Validatable)
	Validate() error
}

type record struct {
	key  RecordKey
	data Validatable
}

func (v record) Key() RecordKey {
	return v.key
}

func (v record) Data() Validatable {
	return v.data
}

func (v record) SetData(data Validatable) {
	v.data = data
}

func (v record) Validate() error {
	if err := validateRecordKey(v.key); err != nil {
		return errors.Wrap(err, "invalid record key")
	}
	if err := v.data.Validate(); err != nil {
		return errors.Wrap(err, "invalid record data")
	}
	return nil
}

func NewRecord(key RecordKey, data Validatable) Record {
	return record{key: key, data: data}
}

type RecordWithIntID interface {
	Record
	GetID() int64
	SetIntID(id int64)
}

type RecordWithStrID interface {
	Record
	GetID() string
	SetStrID(id string)
}

// MultiUpdater is an interface that describe DB provider that can update multiple entities at once (batch mode)
type MultiUpdater interface {
	UpdateMulti(c context.Context, records []Record) error
}

// MultiGetter is an interface that describe DB provider that can get multiple entities at once (batch mode)
type MultiGetter interface {
	GetMulti(c context.Context, records []Record) error
}

// Getter is an interface that describe DB provider that can get a single record by key
type Getter interface {
	Get(c context.Context, record Record) error
}

// Upserter is an interface that describe DB provider that can upsert a single record by key
type Upserter interface {
	Upsert(c context.Context, record Record) error
}

// Updater is an interface that describe DB provider that can update a single EXISTING record by a key
type Updater interface {
	Update(c context.Context, record Record) error
}

// Deleter is an interface that describe DB provider that can delete a single record by key
type Deleter interface {
	Delete(c context.Context, key RecordKey) error
}

type MultiDeleter interface {
	DeleteMulti(c context.Context, keys []RecordKey) error
}

// RunOptions hold arbitrary parameters to be passed throw DAL
type RunOptions map[string]interface{}

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunInTransaction(c context.Context, f func(c context.Context) error, options RunOptions) (err error)
	IsInTransaction(c context.Context) bool
	NonTransactionalContext(tc context.Context) (c context.Context)
}

// Database is an interface that define a DB provider
type Database interface {
	TransactionCoordinator
	Inserter
	Upserter
	Getter
	Updater
	Deleter
	MultiGetter
	MultiUpdater
	MultiDeleter
}

var (
	// CrossGroupTransaction is an options that tells DB that multiple record groups are affected, see Google Datastore
	CrossGroupTransaction = RunOptions{"XG": true}

	// SingleGroupTransaction specifies that only single record group is affected, see Google Datastore
	SingleGroupTransaction = RunOptions{}
)
