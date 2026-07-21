package dal

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// ErrInsertOptionNotHonored is returned by Collection[K, T].Insert when the
// underlying WriteSession reports success but leaves the record's key without an
// id — i.e. the adapter ignored the InsertOption generator. It makes a generated
// Insert fail LOUDLY on a non-honoring adapter instead of reporting a false
// success under a <nil> id.
var ErrInsertOptionNotHonored = errors.New("dal: insert option not honored: record key has no id after a successful insert")

// CollectionNamer is implemented by a record type that knows its own collection
// name. The CollectionName method MUST be declared on a value receiver so that
// CollectionOf[K, T]() can resolve the name from the zero value of T.
type CollectionNamer interface {
	CollectionName() string
}

// Collection is a session-less, generic, reusable handle to a collection of
// records of type T keyed by id type K. It carries only path identity (a
// composed CollectionRef) and the phantom types K, T — it holds no session or
// connection, so a single value can be declared once (e.g. a package-level var)
// and reused across calls.
//
// K is the (scalar) id type — id arguments are strongly typed as K rather than
// any. Composite / multi-field keys are addressed through the *ByKey terminals
// (build a *record.Key with record.NewKeyWithFields).
//
// Read terminals take a ReadSession; write terminals take a WriteSession.
// Because dal.DB satisfies ReadSession but not WriteSession, calling a write
// terminal with a plain DB is a compile error — writes go through a
// read-write transaction handle (see RunReadwriteTransaction).
type Collection[K comparable, T any] interface {
	// GetData returns the record stored at id decoded as a T. On not-found it
	// returns the zero T and the not-found error from the session Get call (use
	// record.IsNotFound to test it).
	GetData(ctx context.Context, s ReadSession, id K) (T, error)

	// Get is a deprecated alias for GetData.
	//
	// Deprecated: use GetData.
	Get(ctx context.Context, s ReadSession, id K) (T, error)

	// GetRecord returns the underlying record.Record stored at id, with its Data set to
	// a *T. On not-found it returns the record (Exists() == false) together with
	// the not-found error from the session Get call.
	GetRecord(ctx context.Context, s ReadSession, id K) (record.Record, error)

	// GetRecordWithID reads id and returns a typed record.WithID[K] (id + key +
	// record, no typed data). On not-found it returns the zero value and the
	// session's not-found error.
	GetRecordWithID(ctx context.Context, s ReadSession, id K) (record.WithID[K], error)

	// GetRecordWithDataAndID reads id and returns a record.DataWithID[K, *T]
	// whose Data is the decoded *T (the same pointer held by the record.Record). On
	// not-found it returns the zero value and the session's not-found error.
	GetRecordWithDataAndID(ctx context.Context, s ReadSession, id K) (record.DataWithID[K, *T], error)

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
	Exists(ctx context.Context, s ReadSession, id K) (bool, error)

	// First returns the first record in the collection (an underlying limit-1
	// query). An empty collection yields (zero T, false, nil); an incapable
	// backend surfaces ErrNotSupported.
	First(ctx context.Context, s ReadSession) (value T, found bool, err error)

	// Insert inserts value under a GENERATED id and returns the assigned key.
	// When opts is empty a default generator (WithRandomStringKey) is injected.
	// Only this terminal accepts InsertOption — generators cannot reach the
	// id-taking terminals.
	Insert(ctx context.Context, s WriteSession, value T, opts ...InsertOption) (*record.Key, error)

	// InsertWithID inserts value at a known id and returns the record's key.
	InsertWithID(ctx context.Context, s WriteSession, id K, value T) (*record.Key, error)

	// InsertRecord inserts a caller-built record. It is the shared primitive the
	// other inserts delegate to; opts carry an id generator for generated inserts.
	InsertRecord(ctx context.Context, s WriteSession, r record.Record, opts ...InsertOption) error

	// SetByID stores (upserts) value at id.
	SetByID(ctx context.Context, s WriteSession, id K, value T) error

	// Set is a deprecated alias for SetByID.
	//
	// Deprecated: use SetByID.
	Set(ctx context.Context, s WriteSession, id K, value T) error

	// SetRecord stores (upserts) a caller-built record.
	SetRecord(ctx context.Context, s WriteSession, r record.Record) error

	// UpdateByID applies field-level updates to the record at id.
	UpdateByID(ctx context.Context, s WriteSession, id K, updates []update.Update, preconditions ...Precondition) error

	// Update is a deprecated alias for UpdateByID.
	//
	// Deprecated: use UpdateByID.
	Update(ctx context.Context, s WriteSession, id K, updates []update.Update, preconditions ...Precondition) error

	// UpdateByKey applies field-level updates to the record at an explicit key.
	UpdateByKey(ctx context.Context, s WriteSession, k *record.Key, updates []update.Update, preconditions ...Precondition) error

	// DeleteByID deletes the record at id.
	DeleteByID(ctx context.Context, s WriteSession, id K) error

	// Delete is a deprecated alias for DeleteByID.
	//
	// Deprecated: use DeleteByID.
	Delete(ctx context.Context, s WriteSession, id K) error

	// DeleteByKey deletes the record at an explicit key.
	DeleteByKey(ctx context.Context, s WriteSession, k *record.Key) error

	// In returns a handle scoped under parent (one level of nesting).
	In(parent *record.Key) Collection[K, T]
}

// Item is a dal-native id+value pair for batch insert. Item deliberately does
// NOT reference the record package, so the batch API adds no dal -> record
// import.
type Item[K comparable, T any] struct {
	ID    K
	Value T
}

// ManyInserter is the opt-in batch-insert interface, mirroring dalgo's
// Inserter/MultiInserter split. The concrete Collection[K, T] value satisfies it
// (obtain it via a type assertion: c.(dal.ManyInserter[K, T])).
type ManyInserter[K comparable, T any] interface {
	// InsertMany inserts each item at its known id and returns the keys in
	// input order.
	InsertMany(ctx context.Context, s WriteSession, items ...Item[K, T]) (keys []*record.Key, err error)
}

// CollectionOption configures a Collection at construction time.
type CollectionOption func(*collectionOptions)

type collectionOptions struct {
	keyOptions []record.KeyOption
}

// WithKeyOptions configures KeyOptions applied to every key the collection
// builds from a typed id (e.g. record.WithFields for composite keys, a parent key, or
// a custom IDKind). The id passed to a terminal is set first; these options are
// applied after, so an option may augment or override the resulting key.
func WithKeyOptions(keyOptions ...record.KeyOption) CollectionOption {
	return func(o *collectionOptions) {
		o.keyOptions = append(o.keyOptions, keyOptions...)
	}
}

func newCollectionOptions(opts ...CollectionOption) collectionOptions {
	var co collectionOptions
	for _, opt := range opts {
		opt(&co)
	}
	return co
}

// collection is the unexported implementation of Collection[K, T]. It is a small
// value composing a CollectionRef, the configured key options, and the phantom
// types K, T.
type collection[K comparable, T any] struct {
	ref        CollectionRef
	keyOptions []record.KeyOption
}

// CollectionOf returns a Collection[K, T] whose name is resolved from T's
// value-receiver CollectionName method.
func CollectionOf[K comparable, T CollectionNamer](opts ...CollectionOption) Collection[K, T] {
	var t T
	return collection[K, T]{ref: NewRootCollectionRef(t.CollectionName(), ""), keyOptions: newCollectionOptions(opts...).keyOptions}
}

// CollectionAt returns a Collection[K, T] with an explicit collection name.
func CollectionAt[K comparable, T any](name string, opts ...CollectionOption) Collection[K, T] {
	return collection[K, T]{ref: NewRootCollectionRef(name, ""), keyOptions: newCollectionOptions(opts...).keyOptions}
}

// idToKey turns the typed id into a *record.Key built from the handle's CollectionRef
// and the collection's configured key options (record.KeyOption-sniffing logic). It
// returns a descriptive error when an option fails or the handle is scoped under
// an incomplete parent key.
func (c collection[K, T]) idToKey(id K) (*record.Key, error) {
	options := make([]record.KeyOption, 0, len(c.keyOptions)+2)
	options = append(options, record.WithKeyID(id))
	if parent := c.ref.Parent(); parent != nil {
		options = append(options, record.WithParentKey(parent))
	}
	options = append(options, c.keyOptions...)
	key, err := record.NewKeyWithOptions(c.ref.Name(), options...)
	if err != nil {
		return nil, err
	}
	if err := c.guardParent(key); err != nil {
		return nil, err
	}
	return key, nil
}

// guardParent returns a descriptive error if any ancestor of key is an
// incomplete key (a parent with no id), rather than letting a terminal proceed
// with a malformed path.
func (c collection[K, T]) guardParent(key *record.Key) error {
	for p := key.Parent(); p != nil; p = p.Parent() {
		if p.ID == nil {
			return fmt.Errorf("dal: collection %q is scoped under an incomplete parent key for collection %q (id is missing)", c.ref.Name(), p.Collection())
		}
	}
	return nil
}

func (c collection[K, T]) GetRecord(ctx context.Context, s ReadSession, id K) (record.Record, error) {
	key, err := c.idToKey(id)
	if err != nil {
		return nil, err
	}
	rec := record.NewRecordWithData(key, new(T))
	if err = s.Get(ctx, rec); err != nil {
		return rec, err
	}
	return rec, nil
}

func (c collection[K, T]) GetData(ctx context.Context, s ReadSession, id K) (T, error) {
	rec, err := c.GetRecord(ctx, s, id)
	if err != nil {
		var zero T
		return zero, err
	}
	return *rec.Data().(*T), nil
}

func (c collection[K, T]) GetRecordWithID(ctx context.Context, s ReadSession, id K) (record.WithID[K], error) {
	rec, err := c.GetRecord(ctx, s, id)
	if err != nil {
		return record.WithID[K]{}, err
	}
	return record.WithID[K]{ID: id, Key: rec.Key(), Record: rec}, nil
}

func (c collection[K, T]) GetRecordWithDataAndID(ctx context.Context, s ReadSession, id K) (record.DataWithID[K, *T], error) {
	rec, err := c.GetRecord(ctx, s, id)
	if err != nil {
		return record.DataWithID[K, *T]{}, err
	}
	return record.DataWithID[K, *T]{
		WithID: record.WithID[K]{ID: id, Key: rec.Key(), Record: rec},
		Data:   rec.Data().(*T),
	}, nil
}

func (c collection[K, T]) Get(ctx context.Context, s ReadSession, id K) (T, error) {
	return c.GetData(ctx, s, id)
}

func (c collection[K, T]) All(ctx context.Context, s ReadSession) ([]T, error) {
	query := NewQueryBuilder(From(c.ref)).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithIncompleteKey(c.ref.Name(), reflect.String, new(T))
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

func (c collection[K, T]) Insert(ctx context.Context, s WriteSession, value T, opts ...InsertOption) (*record.Key, error) {
	key := record.NewIncompleteKey(c.ref.Name(), reflect.String, c.ref.Parent())
	rec := record.NewRecordWithData(key, &value)
	if len(opts) == 0 {
		opts = []InsertOption{WithRandomStringKey(DefaultRandomStringIDLength, 5)}
	}
	if err := c.InsertRecord(ctx, s, rec, opts...); err != nil {
		return nil, err
	}
	if id := rec.Key().ID; id == nil || id == "" {
		return nil, fmt.Errorf("%w (collection %q)", ErrInsertOptionNotHonored, c.ref.Name())
	}
	return rec.Key(), nil
}

func (c collection[K, T]) Count(ctx context.Context, s ReadSession) (int, error) {
	query := NewQueryBuilder(From(c.ref)).SelectKeysOnly(reflect.String)
	records, err := ExecuteQueryAndReadAllToRecords(ctx, query, s)
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

func (c collection[K, T]) Exists(ctx context.Context, s ReadSession, id K) (bool, error) {
	key, err := c.idToKey(id)
	if err != nil {
		return false, err
	}
	rec := record.NewRecordWithData(key, new(T))
	if err = s.Get(ctx, rec); err != nil {
		if record.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c collection[K, T]) First(ctx context.Context, s ReadSession) (T, bool, error) {
	var zero T
	query := NewQueryBuilder(From(c.ref)).Limit(1).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithIncompleteKey(c.ref.Name(), reflect.String, new(T))
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

func (c collection[K, T]) InsertRecord(ctx context.Context, s WriteSession, r record.Record, opts ...InsertOption) error {
	if err := c.guardParent(r.Key()); err != nil {
		return err
	}
	return s.Insert(ctx, r, opts...)
}

func (c collection[K, T]) InsertWithID(ctx context.Context, s WriteSession, id K, value T) (*record.Key, error) {
	key, err := c.idToKey(id)
	if err != nil {
		return nil, err
	}
	rec := record.NewRecordWithData(key, &value)
	if err = c.InsertRecord(ctx, s, rec); err != nil {
		return nil, err
	}
	return rec.Key(), nil
}

func (c collection[K, T]) SetRecord(ctx context.Context, s WriteSession, r record.Record) error {
	if err := c.guardParent(r.Key()); err != nil {
		return err
	}
	return s.Set(ctx, r)
}

func (c collection[K, T]) SetByID(ctx context.Context, s WriteSession, id K, value T) error {
	key, err := c.idToKey(id)
	if err != nil {
		return err
	}
	return c.SetRecord(ctx, s, record.NewRecordWithData(key, &value))
}

func (c collection[K, T]) Set(ctx context.Context, s WriteSession, id K, value T) error {
	return c.SetByID(ctx, s, id, value)
}

func (c collection[K, T]) UpdateByKey(ctx context.Context, s WriteSession, k *record.Key, updates []update.Update, preconditions ...Precondition) error {
	if err := c.guardParent(k); err != nil {
		return err
	}
	return s.Update(ctx, k, updates, preconditions...)
}

func (c collection[K, T]) UpdateByID(ctx context.Context, s WriteSession, id K, updates []update.Update, preconditions ...Precondition) error {
	key, err := c.idToKey(id)
	if err != nil {
		return err
	}
	return c.UpdateByKey(ctx, s, key, updates, preconditions...)
}

func (c collection[K, T]) Update(ctx context.Context, s WriteSession, id K, updates []update.Update, preconditions ...Precondition) error {
	return c.UpdateByID(ctx, s, id, updates, preconditions...)
}

func (c collection[K, T]) DeleteByKey(ctx context.Context, s WriteSession, k *record.Key) error {
	if err := c.guardParent(k); err != nil {
		return err
	}
	return s.Delete(ctx, k)
}

func (c collection[K, T]) DeleteByID(ctx context.Context, s WriteSession, id K) error {
	key, err := c.idToKey(id)
	if err != nil {
		return err
	}
	return c.DeleteByKey(ctx, s, key)
}

func (c collection[K, T]) Delete(ctx context.Context, s WriteSession, id K) error {
	return c.DeleteByID(ctx, s, id)
}

func (c collection[K, T]) In(parent *record.Key) Collection[K, T] {
	return collection[K, T]{ref: NewCollectionRef(c.ref.Name(), "", parent)}
}

// InsertMany inserts each item at its known id, delegating to the session's
// MultiInserter (every WriteSession provides one), and returns the keys in input
// order.
func (c collection[K, T]) InsertMany(ctx context.Context, s WriteSession, items ...Item[K, T]) ([]*record.Key, error) {
	records := make([]record.Record, len(items))
	keys := make([]*record.Key, len(items))
	for i, item := range items {
		key, err := c.idToKey(item.ID)
		if err != nil {
			return nil, err
		}
		value := item.Value
		records[i] = record.NewRecordWithData(key, &value)
		keys[i] = key
	}
	if err := s.InsertMulti(ctx, records); err != nil {
		return nil, err
	}
	return keys, nil
}
