package mockdb

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/dalgo"
	"reflect"
)

// MockKey is mock key
type MockKey struct {
	Kind  string
	IntID int64
	StrID string
}

func newMockKey(key *dalgo.Key) MockKey {
	return MockKey{
		Kind:  dalgo.GetRecordKind(key),
		StrID: dalgo.GetRecordKeyPath(key),
	}
}

// RecordsStorage emulates datastore persistent storage
type RecordsStorage map[string]map[MockKey]dalgo.Record

// MockDB emulates gae DAL
type MockDB struct {
	UpdatesCount  int
	GetsCount     int
	DeletesCount  int
	RecordsByKind RecordsStorage
	onSave        trigger
	onLoad        trigger
}

func (mdb *MockDB) Set(_ context.Context, _ dalgo.Record) error {
	panic("implement me")
}

func (mdb *MockDB) SetMulti(_ context.Context, _ []dalgo.Record) error {
	panic("implement me")
}

func (mdb *MockDB) Insert(ctx context.Context, record dalgo.Record, options ...dalgo.InsertOption) error {
	return mdb.insert(ctx, record, dalgo.NewInsertOptions(options...), 5)
}

func (mdb *MockDB) Upsert(_ context.Context, _ dalgo.Record) error {
	panic("implement me")
}

func (mdb *MockDB) Delete(_ context.Context, key *dalgo.Key) error {
	mdb.DeletesCount++
	kind := dalgo.GetRecordKind(key)
	entities, ok := mdb.RecordsByKind[kind]
	if !ok {
		return nil
	}
	delete(entities, newMockKey(key))
	return nil
}

func (mdb *MockDB) DeleteMulti(ctx context.Context, keys []*dalgo.Key) error {
	for _, key := range keys {
		if err := mdb.Delete(ctx, key); err != nil {
			return nil
		}
	}
	return nil
}

var _ dalgo.Database = (*MockDB)(nil)

type trigger func(holder dalgo.Record) (dalgo.Record, error)

// NewMockDB creates new mock DB
func NewMockDB(onSave, onLoad trigger) *MockDB {
	return &MockDB{
		onSave:        onSave,
		onLoad:        onLoad,
		RecordsByKind: make(RecordsStorage),
	}
}

// RunInTransaction starts transaction
func (mdb *MockDB) RunInTransaction(ctx context.Context, f func(c context.Context, tx dalgo.Transaction) error, options ...dalgo.TransactionOption) (err error) {
	_ = dalgo.NewTransactionOptions(options...)
	ctx = dalgo.NewContextWithTransaction(ctx, nil)
	return f(ctx, mdb)
}

func (mdb *MockDB) insert(c context.Context, record dalgo.Record, options dalgo.InsertOptions, attempts int) error {
	if record == nil {
		panic("record == nil")
	}
	data := record.Data()
	if data == nil {
		panic("data == nil")
	}

	key := record.Key()
	if key == nil {
		return errors.New("record.Key() returned nil")
	}

	if key.Parent() != nil {
		return errors.New("composite keys are not supported by mock yet")
	}

	kind := key.Kind()

	entities, ok := mdb.RecordsByKind[kind]
	if !ok {
		entities = make(map[MockKey]dalgo.Record, 1)
		mdb.RecordsByKind[kind] = entities
	}
	generateID := options.IDGenerator()

	for i := 0; i < attempts; i++ {
		if err := generateID(c, record); err != nil {
			return errors.Wrap(err, "failed to generate Value")
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

	return errors.Errorf("too many attempts to create a new %v record with unique Value", kind)
}

func beforeSave(record dalgo.Record) error {
	if bs, ok := record.(BeforeSaver); ok {
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
func (mdb *MockDB) UpdateMulti(c context.Context, keys []*dalgo.Key, updates []dalgo.Update, preconditions ...dalgo.Precondition) error {
	for _, key := range keys {
		if err := mdb.Update(c, key, updates, preconditions...); err != nil {
			return err
		}
	}
	return nil
}

// GetMulti gets multiple entities
func (mdb *MockDB) GetMulti(c context.Context, records []dalgo.Record) error {
	for _, r := range records {
		if err := mdb.Get(c, r); err != nil {
			return err
		}
	}
	return nil
}

// Get get entity
func (mdb *MockDB) Get(_ context.Context, record dalgo.Record) error {
	mdb.GetsCount++
	key := record.Key()
	kind := dalgo.GetRecordKind(key)
	records, ok := mdb.RecordsByKind[kind]
	if !ok {
		return dalgo.NewErrNotFoundByKey(key, fmt.Errorf("kind %v has no records", kind))
	}
	if r, ok := records[newMockKey(key)]; !ok {
		record.SetError(dalgo.DoesNotExist())
		return dalgo.NewErrNotFoundByKey(key, nil)
	} else {
		rVal := reflect.ValueOf(r).Elem()
		recordVal := reflect.ValueOf(record).Elem()
		for i := 0; i < rVal.NumField(); i++ {
			recordField := recordVal.Field(i)
			recordField.Set(rVal.Field(i))
		}
	}
	return nil
}

// Update entity
func (mdb *MockDB) Update(_ context.Context, key *dalgo.Key, _ []dalgo.Update, preconditions ...dalgo.Precondition) error {
	kind := dalgo.GetRecordKind(key)
	var records, ok = mdb.RecordsByKind[kind]
	if !ok {
		preConditions := dalgo.GetPreconditions(preconditions...)
		if preConditions.Exists() {
			return dalgo.NewErrNotFoundByKey(key, errors.New("update has exists precondition"))
		}
		records = make(map[MockKey]dalgo.Record)
		mdb.RecordsByKind[kind] = records
	}
	//records[newMockKey(key)] = record
	mdb.UpdatesCount++
	return errors.New("func MockDB.Update() is not implemented yet")
}
