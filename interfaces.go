package dalgo

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"strings"
)

// TypeOfID represents type of ID: IsComplexID, IsStringID, IsIntID
type TypeOfID int

type Validatable interface {
	Validate() error
}

// RecordRef hold a reference to a single record within a root or nested recordset.
type RecordRef struct {
	Kind string      `json:"kind"`
	ID   interface{} `json:"id"` // Usually string or int
}

// RecordKey represents a full path to a given record (1 item in case of root recordset)
type RecordKey = []RecordRef

func validateRecordKey(key RecordKey) error {
	for i, ref := range key {
		const prefix = "db record key is invalid at record reference #%v: "
		if strings.TrimSpace(ref.Kind) == "" {
			return errors.New(fmt.Sprintf(prefix, i+1) + "kind is a required property")
		}
		if i < len(key)-1 && ref.ID == nil {
			return errors.New(fmt.Sprintf(prefix, i+1) + "ID is a required property")
		}
	}
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

// MultiUpdater is an interface that describe DB provider that can update multiple records at once (batch mode)
type MultiUpdater interface {
	UpdateMulti(c context.Context, records []Record) error
}

// MultiGetter is an interface that describe DB provider that can get multiple records at once (batch mode)
type MultiGetter interface {
	GetMulti(ctx context.Context, records []Record) error
}

// MultiSetter is an interface that describe DB provider that can set multiple records at once (batch mode)
type MultiSetter interface {
	SetMulti(ctx context.Context, records []Record) error
}

// Getter is an interface that describe DB provider that can get a single record by key
type Getter interface {
	Get(ctx context.Context, record Record) error
}

// Setter is an interface that describe DB provider that can set a single record by key
type Setter interface {
	Set(ctx context.Context, record Record) error
}

// Upserter is an interface that describe DB provider that can upsert a single record by key
type Upserter interface {
	Upsert(ctx context.Context, record Record) error
}

// Updater is an interface that describe DB provider that can update a single EXISTING record by a key
type Updater interface {
	Update(ctx context.Context, record Record) error
}

// Deleter is an interface that describe DB provider that can delete a single record by key
type Deleter interface {
	Delete(ctx context.Context, key RecordKey) error
}

type MultiDeleter interface {
	DeleteMulti(ctx context.Context, keys []RecordKey) error
}

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunInTransaction(
		ctx context.Context,
		f func(ctx context.Context, tx Transaction) error,
		options ...TransactionOption,
	) error
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
}

type Transaction interface {
	Session
}

// Database is an interface that define a DB provider
type Database interface {
	TransactionCoordinator
	Session
}
