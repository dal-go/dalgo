package mockdb

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/db"
)

// MockKey is mock key
type MockKey struct {
	Kind  string
	IntID int64
	StrID string
}

func newMockKey(key db.RecordKey) MockKey {
	return MockKey{
		Kind:  db.GetRecordKind(key),
		StrID: db.GetRecordKeyPath(key),
	}
}

// EntitiesStorage emulates datastore persistent storage
type EntitiesStorage map[string]map[MockKey]db.Record

// MockDB emulates gae DAL
type MockDB struct {
	UpdatesCount   int
	GetsCount      int
	DeletesCount   int
	EntitiesByKind EntitiesStorage
	onSave         trigger
	onLoad         trigger
}

func (mdb *MockDB) Set(_ context.Context, _ db.Record) error {
	panic("implement me")
}

func (mdb *MockDB) SetMulti(_ context.Context, _ []db.Record) error {
	panic("implement me")
}

func (mdb *MockDB) Insert(ctx context.Context, record db.Record, options db.InsertOptions) error {
	return mdb.insert(ctx, record, options, 5)
}

func (mdb *MockDB) Upsert(_ context.Context, _ db.Record) error {
	panic("implement me")
}

func (mdb *MockDB) Delete(_ context.Context, key db.RecordKey) error {
	mdb.DeletesCount++
	kind := db.GetRecordKind(key)
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		return nil
	}
	delete(entities, newMockKey(key))
	return nil
}

func (mdb *MockDB) DeleteMulti(ctx context.Context, keys []db.RecordKey) error {
	for _, key := range keys {
		if err := mdb.Delete(ctx, key); err != nil {
			return nil
		}
	}
	return nil
}

var _ db.Database = (*MockDB)(nil)

type trigger func(holder db.Record) (db.Record, error)

// NewMockDB creates new mock DB
func NewMockDB(onSave, onLoad trigger) *MockDB {
	return &MockDB{
		onSave:         onSave,
		onLoad:         onLoad,
		EntitiesByKind: make(EntitiesStorage),
	}
}

// RunInTransaction starts transaction
func (mdb *MockDB) RunInTransaction(ctx context.Context, f func(c context.Context, tx db.Transaction) error, options ...db.TransactionOption) (err error) {
	_ = db.NewTransactionOptions(options...)
	ctx = db.NewContextWithTransaction(ctx, nil)
	return f(ctx, mdb)
}

func (mdb *MockDB) insert(c context.Context, record db.Record, options db.InsertOptions, attempts int) error {
	if record == nil {
		panic("record == nil")
	}
	data := record.Data()
	if data == nil {
		panic("data == nil")
	}
	if err := data.Validate(); err != nil {
		return errors.Wrap(err, "invalid record data")
	}

	key := record.Key()
	if key == nil {
		return errors.New("record.Key() returned nil")
	}

	switch len(key) {
	case 0:
		return errors.New("len(record.Key()) == 0")
	case 1:
		break
	default:
		return errors.New("composite keys are not supported by mock yet")
	}

	kind := key[0].Kind

	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		entities = make(map[MockKey]db.Record, 1)
		mdb.EntitiesByKind[kind] = entities
	}
	generateID := options.IDGenerator()

	for i := 0; i < attempts; i++ {
		if err := generateID(c, record); err != nil {
			return errors.Wrap(err, "failed to generate ID")
		}
		key := newMockKey(key)
		if _, ok = entities[key]; !ok {
			if err := beforeSave(record); err != nil {
				return err
			}
			entities[key] = record
			return nil
		}
	}

	return errors.Errorf("too many attempts to create a new %v record with unique ID", kind)
}

func beforeSave(entityHolder db.Record) error {
	if bs, ok := entityHolder.(BeforeSaver); ok {
		if err := bs.BeforeSave(); err != nil {
			return err
		}
	}
	return nil
}

// BeforeSaver defines BeforeSave method that is called to verify entity before saving
type BeforeSaver interface {
	BeforeSave() error
}

// UpdateMulti updates multiple entities
func (mdb *MockDB) UpdateMulti(c context.Context, records []db.Record) error {
	for _, r := range records {
		if err := mdb.Update(c, r); err != nil {
			return err
		}
	}
	return nil
}

// GetMulti gets multiple entities
func (mdb *MockDB) GetMulti(c context.Context, records []db.Record) error {
	for _, r := range records {
		if err := mdb.Get(c, r); err != nil {
			return err
		}
	}
	return nil
}

// Get get entity
func (mdb *MockDB) Get(_ context.Context, record db.Record) error {
	mdb.GetsCount++
	key := record.Key()
	kind := db.GetRecordKind(key)
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		return db.NewErrNotFoundByKey(record, fmt.Errorf("kind %v has no entities", kind))
	}
	var entityHolder2 db.Record
	if entityHolder2, ok = entities[newMockKey(key)]; !ok {
		return db.NewErrNotFoundByKey(record, nil)
	}
	record.SetData(entityHolder2.Data())
	return nil
}

// Update entity
func (mdb *MockDB) Update(_ context.Context, record db.Record) error {
	key := record.Key()
	kind := db.GetRecordKind(key)
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		entities = make(map[MockKey]db.Record)
		mdb.EntitiesByKind[kind] = entities
	}
	if err := beforeSave(record); err != nil {
		return err
	}
	entities[newMockKey(key)] = record
	mdb.UpdatesCount++
	return nil
}
