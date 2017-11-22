package gaedb

import (
	"google.golang.org/appengine/datastore"
	"github.com/strongo/db"
	"os"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"github.com/strongo/log"
)

type MockKey struct {
	Kind     string
	IntID    int64
	StringID string
}

type EntitiesStorage map[string]map[MockKey][]datastore.Property

type MockDB struct {
	EntitiesByKind EntitiesStorage
}

func NewMockKeyFromDatastoreKey(key *datastore.Key) MockKey {
	return MockKey{Kind: key.Kind(), IntID: key.IntID(), StringID: key.StringID()}
}

func SetupNdsMock() {
	if err := os.Setenv("GAE_LONG_APP_ID", "debtstracker"); err != nil {
		panic(err)
	}
	if err := os.Setenv("GAE_PARTITION", "DEVTEST"); err != nil {
		panic(err)
	}
	mockDB = MockDB{EntitiesByKind: make(EntitiesStorage)}

	Get = func(c context.Context, key *datastore.Key, val interface{}) error {
		if c == nil {
			panic("c == nil")
		}
		if key == nil {
			panic("key == nil")
		}
		log.Debugf(c, "gaedb.Get(key=%v:%v)", key.Kind(), key.IntID())
		kind := key.Kind()

		if entitiesByKey, ok := mockDB.EntitiesByKind[kind]; !ok {
			return datastore.ErrNoSuchEntity
		} else {
			mockKey := NewMockKeyFromDatastoreKey(key)
			if p, ok := entitiesByKey[mockKey]; !ok {
				return datastore.ErrNoSuchEntity
			} else {
				if e, ok := val.(datastore.PropertyLoadSaver); ok {
					return e.Load(p)
				} else {
					return datastore.LoadStruct(e, p)
				}
			}
		}
	}

	Put = func(c context.Context, key *datastore.Key, val interface{}) (*datastore.Key, error) {
		if c == nil {
			panic("c == nil")
		}
		kind := key.Kind()
		entitiesByKey, ok := mockDB.EntitiesByKind[kind]
		if !ok {
			entitiesByKey = make(map[MockKey][]datastore.Property)
			mockDB.EntitiesByKind[kind] = entitiesByKey
		}
		mockKey := NewMockKeyFromDatastoreKey(key)
		if key.StringID() == "" {
			for k, _ := range entitiesByKey {
				if k.Kind == key.Kind() && k.IntID > mockKey.IntID {
					mockKey.IntID = k.IntID + 1
				}
			}
		}

		var p []datastore.Property
		var err error
		if e, ok := val.(datastore.PropertyLoadSaver); ok {
			if p, err = e.Save(); err != nil {
				return key, err
			}
		} else {
			if p, err = datastore.SaveStruct(val); err != nil {
				return key, err
			}
		}
		entitiesByKey[mockKey] = p
		return NewKey(c, mockKey.Kind, mockKey.StringID, mockKey.IntID, nil), nil
	}

	PutMulti = func(c context.Context, keys []*datastore.Key, vals interface{}) ([]*datastore.Key, error) {
		entityHolders := vals.([]db.EntityHolder)
		var err error
		var errs []error
		for i, key := range keys {
			if key, err = Put(c, key, entityHolders[i]); err != nil {
				errs = append(errs, err)
			}
			keys[i] = key
		}
		if len(errs) > 0 {
			return keys, appengine.MultiError(errs)
		}
		return keys, nil
	}
}
