package gaedb

import (
	"github.com/strongo/db"
	//"github.com/strongo/log"
	"context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"os"
	"github.com/strongo/db/mockdb"
)


func NewMockKeyFromDatastoreKey(key *datastore.Key) mockdb.MockKey {
	return mockdb.MockKey{Kind: key.Kind(), IntID: key.IntID(), StrID: key.StringID()}
}

func SetupNdsMock() {
	if err := os.Setenv("GAE_LONG_APP_ID", "debtstracker"); err != nil {
		panic(err)
	}
	if err := os.Setenv("GAE_PARTITION", "DEVTEST"); err != nil {
		panic(err)
	}
	mockDB = mockdb.NewMockDB(onSave, onLoad)

	Get = func(c context.Context, key *datastore.Key, val interface{}) error {
		panic("not implemented")
		//if c == nil {
		//	panic("c == nil")
		//}
		//if key == nil {
		//	panic("key == nil")
		//}
		//log.Debugf(c, "gaedb.Get(key=%v:%v)", key.Kind(), key.IntID())
		//kind := key.Kind()
		//
		//if entitiesByKey, ok := mockDB.EntitiesByKind[kind]; !ok {
		//	return datastore.ErrNoSuchEntity
		//} else {
		//	mockKey := NewMockKeyFromDatastoreKey(key)
		//	if p, ok := entitiesByKey[mockKey]; !ok {
		//		return datastore.ErrNoSuchEntity
		//	} else {
		//		if e, ok := val.(datastore.PropertyLoadSaver); ok {
		//			return e.Load(p)
		//		} else {
		//			return datastore.LoadStruct(e, p)
		//		}
		//	}
		//}
	}

	Put = func(c context.Context, key *datastore.Key, val interface{}) (*datastore.Key, error) {
		if c == nil {
			panic("c == nil")
		}
		panic("not implemented")
		//kind := key.Kind()
		//entitiesByKey, ok := mockDB.EntitiesByKind[kind]
		//if !ok {
		//	//entitiesByKey = make(map[mockdb.MockKey][]datastore.Property)
		//	//mockDB.EntitiesByKind[kind] = entitiesByKey
		//}
		//mockKey := NewMockKeyFromDatastoreKey(key)
		//if key.StringID() == "" {
		//	for k, _ := range entitiesByKey {
		//		if k.Kind == key.Kind() && k.IntID > mockKey.IntID {
		//			mockKey.IntID = k.IntID + 1
		//		}
		//	}
		//}
		//
		//var p []datastore.Property
		//var err error
		//if e, ok := val.(datastore.PropertyLoadSaver); ok {
		//	if p, err = e.Save(); err != nil {
		//		return key, err
		//	}
		//} else {
		//	if p, err = datastore.SaveStruct(val); err != nil {
		//		return key, err
		//	}
		//}
		//entitiesByKey[mockKey] = p
		//return NewKey(c, mockKey.Kind, mockKey.StrID, mockKey.IntID, nil), nil
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

func onSave(entityHolder db.EntityHolder) (db.EntityHolder, error) {
	return entityHolder, nil
}

func onLoad(entityHolder db.EntityHolder) (db.EntityHolder, error) {
	return entityHolder, nil
}
