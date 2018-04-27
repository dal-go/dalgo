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

func newMockKey(holder db.EntityHolder) MockKey {
	return MockKey{
		Kind:  holder.Kind(),
		IntID: holder.IntID(),
		StrID: holder.StrID(),
	}
}

// EntitiesStorage emulates datastore persistent storage
type EntitiesStorage map[string]map[MockKey]db.EntityHolder

// MockDB emulates gae DAL
type MockDB struct {
	UpdatesCount   int
	GetsCount      int
	DeletesCount   int
	EntitiesByKind EntitiesStorage
	onSave         trigger
	onLoad         trigger
}

type trigger func(holder db.EntityHolder) (db.EntityHolder, error)

// NewMockDB creates new mock DB
func NewMockDB(onSave, onLoad trigger) *MockDB {
	return &MockDB{
		onSave:         onSave,
		onLoad:         onLoad,
		EntitiesByKind: make(EntitiesStorage),
	}
}

// RunInTransaction starts transaction
func (mdb *MockDB) RunInTransaction(c context.Context, f func(c context.Context) error, options db.RunOptions) (err error) {
	return f(c)
}

// IsInTransaction checks if we are in transaction
func (mdb *MockDB) IsInTransaction(c context.Context) bool {
	panic("not implemented")
}

// NonTransactionalContext not implemented
func (mdb *MockDB) NonTransactionalContext(tc context.Context) (c context.Context) {
	panic("not implemented")
}

// InsertWithRandomIntID not implemented
func (mdb *MockDB) InsertWithRandomIntID(c context.Context, entityHolder db.EntityHolder) error {
	panic("not implemented")
}

// InsertWithRandomStrID inserts with random string ID
func (mdb *MockDB) InsertWithRandomStrID(c context.Context, entityHolder db.EntityHolder, idLength uint8, attempts int) error {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	entity := entityHolder.Entity()
	if entity == nil {
		panic("entity == nil")
	}

	if entityHolder.StrID() != "" {
		panic("entityHolder.StrID(): " + entityHolder.StrID())
	}

	if entityHolder.IntID() != 0 {
		panic(fmt.Sprintf("entityHolder.IntID(): %v", entityHolder.IntID()))
	}

	entities, ok := mdb.EntitiesByKind[entityHolder.Kind()]
	if !ok {
		entities = make(map[MockKey]db.EntityHolder, 1)
		mdb.EntitiesByKind[entityHolder.Kind()] = entities
	}
	for i := 0; i < attempts; i++ {
		entityHolder.SetStrID(db.RandomStringID(idLength))
		key := newMockKey(entityHolder)
		if _, ok = entities[key]; !ok {
			entities[key] = entityHolder
			return nil
		}
	}

	return errors.Errorf("too many attempts to create a new %v record with unique ID of length %v", entityHolder.Kind(), idLength)
}

// UpdateMulti updates multiple entities
func (mdb *MockDB) UpdateMulti(c context.Context, entityHolders []db.EntityHolder) error {
	for _, eh := range entityHolders {
		if err := mdb.Update(c, eh); err != nil {
			return err
		}
	}
	return nil
}

// GetMulti gets multiple entities
func (mdb *MockDB) GetMulti(c context.Context, entityHolders []db.EntityHolder) error {
	for _, eh := range entityHolders {
		if err := mdb.Get(c, eh); err != nil {
			return err
		}
	}
	return nil
}

// Get get entity
func (mdb *MockDB) Get(c context.Context, entityHolder db.EntityHolder) error {
	mdb.GetsCount++
	kind := entityHolder.Kind()
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		return db.NewErrNotFoundID(entityHolder, fmt.Errorf("kind %v has no entities", kind))
	}
	var entityHolder2 db.EntityHolder
	if entityHolder2, ok = entities[newMockKey(entityHolder)]; !ok {
		return db.NewErrNotFoundID(entityHolder, nil)
	}
	entityHolder.SetEntity(entityHolder2.Entity())
	return nil
}

// Update entity
func (mdb *MockDB) Update(c context.Context, entityHolder db.EntityHolder) error {
	kind := entityHolder.Kind()
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		entities = make(map[MockKey]db.EntityHolder)
		mdb.EntitiesByKind[kind] = entities
	}
	entities[newMockKey(entityHolder)] = entityHolder
	mdb.UpdatesCount++
	return nil
}

// Delete entity
func (mdb *MockDB) Delete(c context.Context, entityHolder db.EntityHolder) error {
	mdb.DeletesCount++
	kind := entityHolder.Kind()
	entities, ok := mdb.EntitiesByKind[kind]
	if !ok {
		return nil
	}
	delete(entities, newMockKey(entityHolder))
	return nil
}
