package dal

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/dal-go/dalgo/update"
)

// ErrInsertOptionNotHonored is returned by Collection[T].Insert when the
// underlying WriteSession reports success but leaves the record's key without an
// id — i.e. the adapter ignored the InsertOption generator. It makes a generated
// Insert fail LOUDLY on a non-honoring adapter instead of reporting a false
// success under a <nil> id.
var ErrInsertOptionNotHonored = errors.New("dal: insert option not honored: record key has no id after a successful insert")

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

	// Count returns the number of records in the collection. It surfaces
	// ErrNotSupported from backends that cannot run the underlying query rather
	// than a silent 0.
	Count(ctx context.Context, s ReadSession) (int, error)

	// Exists reports whether a record exists at id. A not-found result maps to
	// (false, nil); any other failure is returned as (false, err).
	Exists(ctx context.Context, s ReadSession, id any) (bool, error)

	// First returns the first record in the collection (an underlying limit-1
	// query). An empty collection yields (zero T, false, nil); an incapable
	// backend surfaces ErrNotSupported.
	First(ctx context.Context, s ReadSession) (value T, found bool, err error)

	// Insert inserts value under a GENERATED id and returns the assigned key.
	// When opts is empty a default generator (WithRandomStringKey) is injected.
	// Only this terminal accepts InsertOption — generators cannot reach the
	// id-taking terminals.
	Insert(ctx context.Context, s WriteSession, value T, opts ...InsertOption) (*Key, error)

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

// Item is a dal-native id+value pair for batch insert. ID follows the same
// id any = plain value | WithID/WithFields convention as the point terminals.
// Item deliberately does NOT reference the record package, so the batch API adds
// no dal -> record import.
type Item[T any] struct {
	ID    any
	Value T
}

// ManyInserter is the opt-in batch-insert interface, mirroring dalgo's
// Inserter/MultiInserter split. The concrete Collection[T] value satisfies it
// (obtain it via a type assertion: c.(dal.ManyInserter[T])).
type ManyInserter[T any] interface {
	// InsertMany inserts each item at its known id and returns the keys in
	// input order.
	InsertMany(ctx context.Context, s WriteSession, items ...Item[T]) (keys []*Key, err error)
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

func (c collection[T]) Insert(ctx context.Context, s WriteSession, value T, opts ...InsertOption) (*Key, error) {
	key := NewIncompleteKey(c.ref.Name(), reflect.String, c.ref.Parent())
	if err := c.guardParent(key); err != nil {
		return nil, err
	}
	record := NewRecordWithData(key, &value)
	if len(opts) == 0 {
		opts = []InsertOption{WithRandomStringKey(DefaultRandomStringIDLength, 5)}
	}
	if err := s.Insert(ctx, record, opts...); err != nil {
		return nil, err
	}
	if id := record.Key().ID; id == nil || id == "" {
		return nil, fmt.Errorf("%w (collection %q)", ErrInsertOptionNotHonored, c.ref.Name())
	}
	return record.Key(), nil
}

func (c collection[T]) Count(ctx context.Context, s ReadSession) (int, error) {
	query := NewQueryBuilder(From(c.ref)).SelectKeysOnly(reflect.String)
	records, err := ExecuteQueryAndReadAllToRecords(ctx, query, s)
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

func (c collection[T]) Exists(ctx context.Context, s ReadSession, id any) (bool, error) {
	key, err := c.keyForID(id)
	if err != nil {
		return false, err
	}
	record := NewRecordWithData(key, new(T))
	if err = s.Get(ctx, record); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c collection[T]) First(ctx context.Context, s ReadSession) (T, bool, error) {
	var zero T
	query := NewQueryBuilder(From(c.ref)).Limit(1).SelectIntoRecord(func() Record {
		return NewRecordWithIncompleteKey(c.ref.Name(), reflect.String, new(T))
	})
	records, err := ExecuteQueryAndReadAllToRecords(ctx, query, s)
	if err != nil {
		return zero, false, err
	}
	if len(records) == 0 {
		return zero, false, nil
	}
	return *records[0].Data().(*T), true, nil
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

// InsertMany inserts each item at its known id, delegating to the session's
// MultiInserter (every WriteSession provides one), and returns the keys in input
// order.
func (c collection[T]) InsertMany(ctx context.Context, s WriteSession, items ...Item[T]) ([]*Key, error) {
	records := make([]Record, len(items))
	keys := make([]*Key, len(items))
	for i, item := range items {
		key, err := c.keyForID(item.ID)
		if err != nil {
			return nil, err
		}
		value := item.Value
		records[i] = NewRecordWithData(key, &value)
		keys[i] = key
	}
	if err := s.InsertMulti(ctx, records); err != nil {
		return nil, err
	}
	return keys, nil
}
