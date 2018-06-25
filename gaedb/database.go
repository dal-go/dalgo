package gaedb

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/db"
	"github.com/strongo/log"
	"google.golang.org/appengine/datastore"
	"strconv"
)

type gaeDatabase struct {
}

// NewDatabase create database provider to Google Datastore
func NewDatabase() db.Database {
	return gaeDatabase{}
}

func newErrNotFound(err error, key *datastore.Key) error {
	if intID := key.IntID(); intID != 0 {
		return db.NewErrNotFoundByIntID(key.Kind(), intID, err)
	} else if strID := key.StringID(); strID != "" {
		return db.NewErrNotFoundByStrID(key.Kind(), strID, err)
	} else {
		panic("Wrong key")
	}
}

func (gaeDatabase) Get(c context.Context, entityHolder db.EntityHolder) (err error) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	key, isIncomplete, err := getEntityHolderKey(c, entityHolder)
	if err != nil {
		return
	}
	if isIncomplete {
		panic("can't get entity by incomplete key")
	}
	entity := entityHolder.NewEntity()
	if err = Get(c, key, entity); err != nil {
		if err == datastore.ErrNoSuchEntity {
			err = newErrNotFound(err, key)
		}
		return
	}
	entityHolder.SetEntity(entity)
	if entityHolder.Entity() != entity {
		panic("entityHolder.Entity() != entity")
	}
	return
}

func (gaeDatabase) Delete(c context.Context, entityHolder db.EntityHolder) (err error) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	key, isIncomplete, err := getEntityHolderKey(c, entityHolder)
	if err != nil {
		return
	}
	if isIncomplete {
		panic("can't delete entity by incomplete key")
	}
	if err = Delete(c, key); err != nil {
		return
	}
	return
}

func (gaeDatabase) DeleteMulti(c context.Context, entityHolders []db.EntityHolder) (err error) {
	if len(entityHolders) == 0 {
		return
	}
	keys := make([]*datastore.Key, len(entityHolders))
	for i, entityHolder := range entityHolders {
		key, isIncomplete, err := getEntityHolderKey(c, entityHolder)
		if err != nil {
			return errors.WithMessage(err, "i="+strconv.Itoa(i))
		}
		if isIncomplete {
			panic("can't delete entity by incomplete key, i=" + strconv.Itoa(i))
		}
		keys[i] = key
	}
	if err = DeleteMulti(c, keys); err != nil {
		return
	}
	return
}

func (gaeDatabase) InsertWithRandomIntID(c context.Context, entityHolder db.EntityHolder) (err error) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	log.Debugf(c, "InsertWithRandomIntID(kind=%v)", entityHolder.Kind())
	entity := entityHolder.Entity()
	if entity == nil {
		panic("entity == nil")
	}

	wrapErr := func(err error) error {
		return errors.WithMessage(err, "failed to create record with random int ID for: "+entityHolder.Kind())
	}

	key, isIncomplete, err := getEntityHolderKey(c, entityHolder)
	if err != nil {
		return wrapErr(err)
	} else if !isIncomplete {
		panic(fmt.Sprintf("gaeDatabase.InsertWithRandomIntID() called for key with ID: %v", key))
	}

	if key, err = Put(c, key, entity); err != nil {
		return wrapErr(err)
	}
	setEntityHolderID(key, entityHolder)
	return
}

func (gaeDb gaeDatabase) InsertWithRandomStrID(c context.Context, entityHolder db.EntityHolder, idLength uint8, attempts int, prefix string) (err error) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	log.Debugf(c, "InsertWithRandomIntID(kind=%v)", entityHolder.Kind())
	entity := entityHolder.Entity()
	if entity == nil {
		panic("entity == nil")
	}

	wrapErr := func(err error) error {
		return errors.WithMessage(err, "failed to create record with random str ID for: "+entityHolder.Kind())
	}

	if key, isIncomplete, err := getEntityHolderKey(c, entityHolder); err != nil {
		return wrapErr(err)
	} else if !isIncomplete {
		panic(fmt.Sprintf("gaeDatabase.InsertWithRandomStrID() called for key with ID: %v", key))
	}

	for i := 0; i < attempts; i++ {
		entityHolder.SetStrID(prefix + db.RandomStringID(idLength))
		var key *datastore.Key
		if key, _, err = getEntityHolderKey(c, entityHolder); err != nil {
			return wrapErr(err)
		} else if err = gaeDb.Get(c, entityHolder); err != nil {
			if db.IsNotFound(err) {
				if key, err = Put(c, key, entity); err != nil {
					return wrapErr(err)
				}
				setEntityHolderID(key, entityHolder)
				return
			}
			return
		}
	}
	return errors.Errorf("too many attempts to create a new %v record with unique ID of length %v", entityHolder.Kind(), idLength)
}

func (gaeDb gaeDatabase) Update(c context.Context, entityHolder db.EntityHolder) error {
	entity := entityHolder.Entity()
	log.Debugf(c, "entity: %+v", entity)
	if entity == nil {
		panic("entityHolder.Entity() == nil")
	} else if key, isIncomplete, err := getEntityHolderKey(c, entityHolder); err != nil {
		return err
	} else if isIncomplete {
		log.Errorf(c, "gaeDatabase.Update() called for incomplete key, will insert.")
		return gaeDb.InsertWithRandomIntID(c, entityHolder)
	} else if _, err = Put(c, key, entity); err != nil {
		return errors.WithMessage(err, "failed to update "+key2str(key))
	}
	return nil
}

func setEntityHolderID(key *datastore.Key, entityHolder db.EntityHolder) {
	if intID := key.IntID(); intID != 0 {
		entityHolder.SetIntID(key.IntID())
	} else {
		entityHolder.SetStrID(key.StringID())
	}
}

// ErrKeyHasBothIds indicates entity has both string and int ids
var ErrKeyHasBothIds = errors.New("entity has both string and int ids")

// ErrEmptyKind indicates entity holder returned empty kind
var ErrEmptyKind = errors.New("entity holder returned empty kind")

func getEntityHolderKey(c context.Context, entityHolder db.EntityHolder) (key *datastore.Key, isIncomplete bool, err error) {
	if kind := entityHolder.Kind(); kind == "" {
		err = ErrEmptyKind
	} else {
		intID := entityHolder.IntID()
		strID := entityHolder.StrID()
		if isIncomplete = intID == 0 && strID == ""; isIncomplete {
			key = NewIncompleteKey(c, kind, nil)
		} else if intID != 0 || strID != "" {
			key = NewKey(c, kind, strID, intID, nil)
		} else {
			err = errors.WithMessage(ErrKeyHasBothIds, fmt.Sprintf("%v(intID=%d, strID=%v)", kind, intID, strID))
		}
	}
	return
}

func (gaeDatabase) UpdateMulti(c context.Context, entityHolders []db.EntityHolder) (err error) { // TODO: Rename to PutMulti?

	keys := make([]*datastore.Key, len(entityHolders))
	vals := make([]interface{}, len(entityHolders))

	insertedIndexes := make([]int, 0, len(entityHolders))

	for i, entityHolder := range entityHolders {
		if entityHolder == nil {
			panic(fmt.Sprintf("entityHolders[%v] is nil: %v", i, entityHolder))
		}
		isIncomplete := false
		if keys[i], isIncomplete, err = getEntityHolderKey(c, entityHolder); err != nil {
			return
		} else if isIncomplete {
			insertedIndexes = append(insertedIndexes, i)
		}
		if vals[i] = entityHolder.Entity(); vals[i] == nil {
			return fmt.Errorf("entityHolders[%d].Entity() == nil", i)
		}
	}

	// logKeys(c, "gaeDatabase.UpdateMulti", keys)

	if keys, err = PutMulti(c, keys, vals); err != nil {
		return
	}

	for _, i := range insertedIndexes {
		setEntityHolderID(keys[i], entityHolders[i])
		entityHolders[i].SetEntity(vals[i]) // it seems useless but covers case when .Entity() returned newly created object without storing inside entityHolder
	}
	return
}

func (gaeDatabase) GetMulti(c context.Context, entityHolders []db.EntityHolder) error {
	count := len(entityHolders)
	keys := make([]*datastore.Key, count)
	vals := make([]interface{}, count)
	for i := range entityHolders {
		entityHolder := entityHolders[i]
		intID := entityHolder.IntID()
		strID := entityHolder.StrID()
		if intID != 0 && strID != "" {
			return errors.New("intID != 0 && strID is NOT empty string")
		} else if intID == 0 && strID == "" {
			return errors.New("intID == 0 && strID is empty string")
		}
		keys[i] = NewKey(c, entityHolder.Kind(), strID, intID, nil)
		vals[i] = entityHolder.NewEntity()
	}
	if err := GetMulti(c, keys, vals); err != nil {
		return err
	}
	for i := range vals {
		entityHolders[i].SetEntity(vals[i])
	}
	return nil
}

var xgTransaction = &datastore.TransactionOptions{XG: true}

var isInTransactionFlag = "is in transaction"
var nonTransactionalContextKey = "non transactional context"

func (gaeDatabase) RunInTransaction(c context.Context, f func(c context.Context) error, options db.RunOptions) error {
	var to *datastore.TransactionOptions
	if xg, ok := options["XG"]; ok && xg.(bool) == true {
		to = xgTransaction
	}
	tc := context.WithValue(context.WithValue(c, &isInTransactionFlag, true), &nonTransactionalContextKey, c)
	return RunInTransaction(tc, f, to)
}

func (gaeDatabase) IsInTransaction(c context.Context) bool {
	if v := c.Value(&isInTransactionFlag); v != nil && v.(bool) {
		return true
	}
	return false
}

func (gaeDatabase) NonTransactionalContext(tc context.Context) context.Context {
	if c := tc.Value(&nonTransactionalContextKey); c != nil {
		return c.(context.Context)
	}
	return tc
}
