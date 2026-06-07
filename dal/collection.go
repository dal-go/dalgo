package dal

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dal-go/dalgo/update"
)

// CollectionNamer is implemented by a record type that knows its own collection
// name. The CollectionName method MUST be declared on a value receiver so that
// CollectionOf[T]() can resolve the name from the zero value of T.
type CollectionNamer interface {
	CollectionName() string
}

// Collection is a session-less, generic, reusable handle to a collection of
// records of type T. It carries only path identity (a composed CollectionRef)
// and the phantom type T — it holds no session or connection, so a single value
// can be declared once (e.g. a package-level var) and reused across calls.
//
// Read terminals take a ReadSession; write terminals take a WriteSession.
// Because dal.DB satisfies ReadSession but not WriteSession, calling a write
// terminal with a plain DB is a compile error — writes go through a
// read-write transaction handle (see RunReadwriteTransaction).
type Collection[T any] interface {
	// Get returns the record stored at id decoded as a T. On not-found it
	// returns the zero T and the not-found error from the session Get call.
	Get(ctx context.Context, s ReadSession, id any) (T, error)

	// All returns every record in the collection, each decoded into a freshly
	// allocated value so results never alias. It surfaces ErrNotSupported from
	// backends that cannot run the query.
	All(ctx context.Context, s ReadSession) ([]T, error)

	// InsertWithID inserts value at a known id and returns the record's key.
	InsertWithID(ctx context.Context, s WriteSession, id any, value T) (*Key, error)

	// Set stores (upserts) value at id.
	Set(ctx context.Context, s WriteSession, id any, value T) error

	// Update applies field-level updates to the record at id.
	Update(ctx context.Context, s WriteSession, id any, updates []update.Update, preconditions ...Precondition) error

	// Delete deletes the record at id.
	Delete(ctx context.Context, s WriteSession, id any) error

	// In returns a handle scoped under parent (one level of nesting).
	In(parent *Key) Collection[T]
}

// collection is the unexported implementation of Collection[T]. It is a small
// value composing a CollectionRef and the phantom type T.
type collection[T any] struct {
	ref CollectionRef
}

// CollectionOf returns a Collection[T] whose name is resolved from T's
// value-receiver CollectionName method.
func CollectionOf[T CollectionNamer]() Collection[T] {
	var t T
	return collection[T]{ref: NewRootCollectionRef(t.CollectionName(), "")}
}

// CollectionAt returns a Collection[T] with an explicit collection name.
func CollectionAt[T any](name string) Collection[T] {
	return collection[T]{ref: NewRootCollectionRef(name, "")}
}

// keyForID turns the id argument (a plain value OR an eager KeyOption such as
// WithID/WithFields) into a *Key built from the handle's CollectionRef. It
// returns a descriptive error when the handle is scoped under an incomplete
// parent key.
func (c collection[T]) keyForID(id any) (*Key, error) {
	var key *Key
	if opt, ok := id.(KeyOption); ok {
		var err error
		if key, err = NewKeyWithOptions(c.ref.Name(), opt); err != nil {
			return nil, err
		}
	} else {
		key = &Key{collection: c.ref.Name(), ID: id}
	}
	key.parent = c.ref.Parent()
	if err := c.guardParent(key); err != nil {
		return nil, err
	}
	return key, nil
}

// guardParent returns a descriptive error if any ancestor of key is an
// incomplete key (a parent with no id), rather than letting a terminal proceed
// with a malformed path.
func (c collection[T]) guardParent(key *Key) error {
	for p := key.parent; p != nil; p = p.Parent() {
		if p.ID == nil {
			return fmt.Errorf("dal: collection %q is scoped under an incomplete parent key for collection %q (id is missing)", c.ref.Name(), p.Collection())
		}
	}
	return nil
}

func (c collection[T]) Get(ctx context.Context, s ReadSession, id any) (T, error) {
	var value T
	key, err := c.keyForID(id)
	if err != nil {
		return value, err
	}
	record := NewRecordWithData(key, &value)
	if err = s.Get(ctx, record); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

func (c collection[T]) All(ctx context.Context, s ReadSession) ([]T, error) {
	query := NewQueryBuilder(From(c.ref)).SelectIntoRecord(func() Record {
		return NewRecordWithIncompleteKey(c.ref.Name(), reflect.String, new(T))
	})
	records, err := ExecuteQueryAndReadAllToRecords(ctx, query, s)
	if err != nil {
		return nil, err
	}
	values := make([]T, len(records))
	for i, r := range records {
		values[i] = *r.Data().(*T)
	}
	return values, nil
}

func (c collection[T]) InsertWithID(ctx context.Context, s WriteSession, id any, value T) (*Key, error) {
	key, err := c.keyForID(id)
	if err != nil {
		return nil, err
	}
	record := NewRecordWithData(key, &value)
	if err = s.Insert(ctx, record); err != nil {
		return nil, err
	}
	return record.Key(), nil
}

func (c collection[T]) Set(ctx context.Context, s WriteSession, id any, value T) error {
	key, err := c.keyForID(id)
	if err != nil {
		return err
	}
	return s.Set(ctx, NewRecordWithData(key, &value))
}

func (c collection[T]) Update(ctx context.Context, s WriteSession, id any, updates []update.Update, preconditions ...Precondition) error {
	key, err := c.keyForID(id)
	if err != nil {
		return err
	}
	return s.Update(ctx, key, updates, preconditions...)
}

func (c collection[T]) Delete(ctx context.Context, s WriteSession, id any) error {
	key, err := c.keyForID(id)
	if err != nil {
		return err
	}
	return s.Delete(ctx, key)
}

func (c collection[T]) In(parent *Key) Collection[T] {
	return collection[T]{ref: NewCollectionRef(c.ref.Name(), "", parent)}
}
