package dal_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User is a record type that knows its own collection name via a value-receiver
// CollectionName method (so dal.CollectionOf[User]() works without *User).
type User struct {
	Name string `json:"name"`
}

func (User) CollectionName() string { return "users" }

// thing is a record type that does NOT implement dal.CollectionNamer, used to
// exercise dal.CollectionAt with an explicit name.
type thing struct {
	Name string `json:"name"`
}

// Contact is a nested record type, stored under a parent user.
type Contact struct {
	Email string `json:"email"`
}

func (Contact) CollectionName() string { return "contacts" }

func newMemoryDB(t *testing.T) dal.DB {
	t.Helper()
	return dalgo2memory.NewDB()
}

// write runs f inside a read-write transaction against db.
func write(t *testing.T, db dal.DB, f func(ctx context.Context, tx dal.ReadwriteTransaction) error) {
	t.Helper()
	require.NoError(t, db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, tx)
	}))
}

func TestCollectionOf_ConstructByConvention(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)

	users := dal.CollectionOf[User]()

	// The handle is a reusable value: insert two records via the same handle.
	var key1, key2 *dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		if key1, err = users.InsertWithID(ctx, tx, "u1", User{Name: "Alice"}); err != nil {
			return err
		}
		key2, err = users.InsertWithID(ctx, tx, "u2", User{Name: "Bob"})
		return err
	})

	// The collection path resolved from User.CollectionName() is "users".
	assert.Equal(t, "users", key1.Collection())
	assert.Equal(t, "users/u1", key1.String())
	assert.Equal(t, "users/u2", key2.String())

	// The same reusable handle round-trips both records.
	got1, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got1.Name)
	got2, err := users.Get(ctx, db, "u2")
	require.NoError(t, err)
	assert.Equal(t, "Bob", got2.Name)
}

func TestCollectionAt_ConstructByExplicitName(t *testing.T) {
	db := newMemoryDB(t)

	things := dal.CollectionAt[thing]("things")

	var key *dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		key, err = things.InsertWithID(ctx, tx, "t1", thing{Name: "widget"})
		return err
	})

	assert.Equal(t, "things", key.Collection())
	assert.Equal(t, "things/t1", key.String())
}

func TestCollection_GetRoundtrip(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	got, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)
}

func TestCollection_GetNotFound(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	got, err := users.Get(ctx, db, "missing")
	require.Error(t, err)
	assert.True(t, dal.IsNotFound(err), "error must be a not-found error from the Get call")
	assert.Equal(t, User{}, got, "must return the zero value on not-found")
}

func TestCollection_IDPlainOrKeyOption(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	// Store once using a plain id value.
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	// The same record is addressable by the plain value AND by dal.WithID.
	byPlain, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	byOption, err := users.Get(ctx, db, dal.WithID("u1"))
	require.NoError(t, err)
	assert.Equal(t, byPlain, byOption)
	assert.Equal(t, "Alice", byPlain.Name)

	// A composite-key record is addressable by dal.WithFields.
	composite := dal.WithFields([]dal.FieldVal{{Name: "tenant", Value: "t1"}, {Name: "id", Value: "u9"}})
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, composite, User{Name: "Carol"})
	})
	got, err := users.Get(ctx, db, dal.WithFields([]dal.FieldVal{{Name: "tenant", Value: "t1"}, {Name: "id", Value: "u9"}}))
	require.NoError(t, err)
	assert.Equal(t, "Carol", got.Name)
}

func TestCollection_AllDistinct(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := users.Set(ctx, tx, "u1", User{Name: "Alice"}); err != nil {
			return err
		}
		return users.Set(ctx, tx, "u2", User{Name: "Bob"})
	})

	all, err := users.All(ctx, db)
	require.NoError(t, err)
	require.Len(t, all, 2)

	names := []string{all[0].Name, all[1].Name}
	assert.ElementsMatch(t, []string{"Alice", "Bob"}, names)

	// Results must not alias: mutating one element does not change the other.
	all[0].Name = "Mutated"
	assert.NotEqual(t, all[0].Name, all[1].Name)
}

// unsupportedReadSession is a ReadSession whose query executor reports the query
// is unsupported, used to verify All surfaces dal.ErrNotSupported.
type unsupportedReadSession struct{}

func (unsupportedReadSession) Get(context.Context, dal.Record) error { return dal.ErrNotSupported }
func (unsupportedReadSession) Exists(context.Context, *dal.Key) (bool, error) {
	return false, dal.ErrNotSupported
}
func (unsupportedReadSession) GetMulti(context.Context, []dal.Record) error {
	return dal.ErrNotSupported
}
func (unsupportedReadSession) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (unsupportedReadSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}

var _ dal.ReadSession = unsupportedReadSession{}

func TestCollection_AllUnsupportedSurfacesError(t *testing.T) {
	ctx := context.Background()
	users := dal.CollectionOf[User]()

	all, err := users.All(ctx, unsupportedReadSession{})
	require.ErrorIs(t, err, dal.ErrNotSupported)
	assert.Nil(t, all)
}

func TestCollection_InsertGeneratesKey(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	var key *dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		key, err = users.Insert(ctx, tx, User{Name: "Alice"})
		return err
	})

	require.NotNil(t, key)
	id, ok := key.ID.(string)
	require.True(t, ok)
	require.NotEmpty(t, id, "id must be a non-empty generated string")

	got, err := users.Get(ctx, db, id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)
}

func TestCollection_InsertWithExplicitOption(t *testing.T) {
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	var key *dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		key, err = users.Insert(ctx, tx, User{Name: "Alice"}, dal.WithRandomStringKey(20, 5))
		return err
	})

	id, ok := key.ID.(string)
	require.True(t, ok)
	assert.Len(t, id, 20, "explicit WithRandomStringKey(20,...) must yield a 20-char id")
}

// stubWriteSession is a WriteSession whose Insert behavior is supplied per test;
// every other method is an unused no-op. It lets us drive Collection[T].Insert's
// loud-failure guard and the underlying-insert error path without a real backend.
type stubWriteSession struct {
	insert func(ctx context.Context, record dal.Record, opts ...dal.InsertOption) error
}

func (s stubWriteSession) Insert(ctx context.Context, record dal.Record, opts ...dal.InsertOption) error {
	return s.insert(ctx, record, opts...)
}
func (stubWriteSession) InsertMulti(context.Context, []dal.Record, ...dal.InsertOption) error {
	return nil
}
func (stubWriteSession) Set(context.Context, dal.Record) error        { return nil }
func (stubWriteSession) SetMulti(context.Context, []dal.Record) error { return nil }
func (stubWriteSession) Delete(context.Context, *dal.Key) error       { return nil }
func (stubWriteSession) DeleteMulti(context.Context, []*dal.Key) error {
	return nil
}
func (stubWriteSession) Update(context.Context, *dal.Key, []update.Update, ...dal.Precondition) error {
	return nil
}
func (stubWriteSession) UpdateRecord(context.Context, dal.Record, []update.Update, ...dal.Precondition) error {
	return nil
}
func (stubWriteSession) UpdateMulti(context.Context, []*dal.Key, []update.Update, ...dal.Precondition) error {
	return nil
}

var _ dal.WriteSession = stubWriteSession{}

func TestCollection_InsertLoudFailureOnNonHonoringAdapter(t *testing.T) {
	ctx := context.Background()
	users := dal.CollectionOf[User]()

	// A WriteSession that ignores InsertOption leaves the key incomplete.
	dropping := stubWriteSession{insert: func(context.Context, dal.Record, ...dal.InsertOption) error {
		return nil // success, but never assigns an id
	}}

	key, err := users.Insert(ctx, dropping, User{Name: "Alice"})
	require.Error(t, err)
	assert.ErrorIs(t, err, dal.ErrInsertOptionNotHonored)
	assert.Nil(t, key, "no key may be returned when the option was not honored")
}

func TestCollection_InsertUnderlyingErrorPassthrough(t *testing.T) {
	ctx := context.Background()
	users := dal.CollectionOf[User]()

	boom := errors.New("insert failed")
	failing := stubWriteSession{insert: func(context.Context, dal.Record, ...dal.InsertOption) error {
		return boom
	}}

	key, err := users.Insert(ctx, failing, User{Name: "Alice"})
	require.ErrorIs(t, err, boom)
	assert.Nil(t, key)
}

func TestCollection_InsertWithIDReturnsKey(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	var key *dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		key, err = users.InsertWithID(ctx, tx, "u1", User{Name: "Alice"})
		return err
	})

	require.NotNil(t, key)
	assert.Equal(t, "u1", key.ID)

	got, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)
}

func TestCollection_SetUpserts(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	// Set with no pre-existing record (insert).
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})
	got, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, User{Name: "Alice"}, got)

	// Set again over the existing record (overwrite).
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Bob"})
	})
	got, err = users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, User{Name: "Bob"}, got)
}

func TestCollection_UpdateAppliesFields(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Update(ctx, tx, "u1", []update.Update{update.ByFieldName("name", "Bob")})
	})

	got, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Bob", got.Name)
}

func TestCollection_DeleteRemoves(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Delete(ctx, tx, "u1")
	})

	_, err := users.Get(ctx, db, "u1")
	assert.True(t, dal.IsNotFound(err), "record must be gone after Delete")
}

func TestCollection_NestedGet(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)

	parentKey := dal.NewKeyWithID("users", "u1")
	contacts := dal.CollectionOf[Contact]().In(parentKey)

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		key, err := contacts.InsertWithID(ctx, tx, "c1", Contact{Email: "a@example.com"})
		if err != nil {
			return err
		}
		assert.Equal(t, "users/u1/contacts/c1", key.String())
		return nil
	})

	got, err := contacts.Get(ctx, db, "c1")
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", got.Email)
}

func TestCollection_NestedIncompleteParentErrors(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)

	incompleteParent := dal.NewIncompleteKey("users", reflect.String, nil)
	contacts := dal.CollectionOf[Contact]().In(incompleteParent)

	assert.NotPanics(t, func() {
		_, err := contacts.Get(ctx, db, "c1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incomplete parent")
	})
}

func TestCollection_KeyOptionError(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	// A KeyOption that fails is surfaced by every terminal's id resolution.
	// (dal.KeyOption is an alias for func(*dal.Key) error, so this value's
	// dynamic type matches the type switch in keyForID.)
	badOpt := func(*dal.Key) error { return errors.New("boom") }
	_, err := users.Get(ctx, db, badOpt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestCollection_InsertWithIDDuplicateErrors(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, err := users.InsertWithID(ctx, tx, "u1", User{Name: "Alice"})
		return err
	})

	// Inserting again at the same id must surface the session Insert error.
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, err := users.InsertWithID(ctx, tx, "u1", User{Name: "Bob"})
		return err
	})
	require.Error(t, err)
}

func TestCollection_WriteTerminalsIncompleteParentError(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	incompleteParent := dal.NewIncompleteKey("users", reflect.String, nil)
	contacts := dal.CollectionOf[Contact]().In(incompleteParent)

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := contacts.Insert(ctx, tx, Contact{}); err == nil {
			return errors.New("Insert must error under incomplete parent")
		}
		if _, err := contacts.InsertWithID(ctx, tx, "c1", Contact{}); err == nil {
			return errors.New("InsertWithID must error under incomplete parent")
		}
		if err := contacts.Set(ctx, tx, "c1", Contact{}); err == nil {
			return errors.New("Set must error under incomplete parent")
		}
		if err := contacts.Update(ctx, tx, "c1", []update.Update{update.ByFieldName("email", "x")}); err == nil {
			return errors.New("Update must error under incomplete parent")
		}
		if err := contacts.Delete(ctx, tx, "c1"); err == nil {
			return errors.New("Delete must error under incomplete parent")
		}
		return nil
	})
	require.NoError(t, err)
}

func TestCollection_CountReturnsTotal(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for _, id := range []string{"u1", "u2", "u3"} {
			if err := users.Set(ctx, tx, id, User{Name: id}); err != nil {
				return err
			}
		}
		return nil
	})

	n, err := users.Count(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestCollection_CountUnsupported(t *testing.T) {
	ctx := context.Background()
	users := dal.CollectionOf[User]()

	n, err := users.Count(ctx, unsupportedReadSession{})
	require.ErrorIs(t, err, dal.ErrNotSupported)
	assert.Equal(t, 0, n)
}

func TestCollection_ExistsTrueFalse(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	exists, err := users.Exists(ctx, db, "u1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = users.Exists(ctx, db, "missing")
	require.NoError(t, err)
	assert.False(t, exists, "not-found must map to (false, nil)")

	// keyForID error path: incomplete parent.
	nested := dal.CollectionOf[Contact]().In(dal.NewIncompleteKey("users", reflect.String, nil))
	_, err = nested.Exists(ctx, db, "c1")
	require.Error(t, err)
}

func TestCollection_ExistsErrorPassthrough(t *testing.T) {
	ctx := context.Background()
	users := dal.CollectionOf[User]()

	// A non-not-found lookup failure must be returned, not swallowed.
	exists, err := users.Exists(ctx, unsupportedReadSession{}, "u1")
	require.ErrorIs(t, err, dal.ErrNotSupported)
	assert.False(t, exists)
}

func TestCollection_ItemTypeShape(t *testing.T) {
	// Item[T] is exactly {ID any; Value T}. (The no-record-import half of the AC
	// is enforced by TestDalDoesNotImportRecord.)
	item := dal.Item[User]{ID: "u1", Value: User{Name: "Alice"}}
	assert.Equal(t, any("u1"), item.ID)
	assert.Equal(t, User{Name: "Alice"}, item.Value)
}

func TestCollection_InsertManyRoundtrips(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	mi, ok := users.(dal.ManyInserter[User])
	require.True(t, ok, "Collection[T] value must satisfy dal.ManyInserter[T]")

	var keys []*dal.Key
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		keys, err = mi.InsertMany(ctx, tx,
			dal.Item[User]{ID: "u1", Value: User{Name: "Alice"}},
			dal.Item[User]{ID: "u2", Value: User{Name: "Bob"}},
		)
		return err
	})

	require.Len(t, keys, 2)
	assert.Equal(t, "u1", keys[0].ID)
	assert.Equal(t, "u2", keys[1].ID)

	got1, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got1.Name)
	got2, err := users.Get(ctx, db, "u2")
	require.NoError(t, err)
	assert.Equal(t, "Bob", got2.Name)
}

func TestCollection_InsertManyErrorPaths(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)

	// keyForID error: incomplete parent.
	incompleteParent := dal.NewIncompleteKey("users", reflect.String, nil)
	nested := dal.CollectionOf[Contact]().In(incompleteParent).(dal.ManyInserter[Contact])
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, e := nested.InsertMany(ctx, tx, dal.Item[Contact]{ID: "c1", Value: Contact{}})
		return e
	})
	require.Error(t, err)

	// InsertMulti error: duplicate id within the same collection.
	users := dal.CollectionOf[User]().(dal.ManyInserter[User])
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, e := users.InsertMany(ctx, tx, dal.Item[User]{ID: "dup", Value: User{Name: "A"}})
		return e
	})
	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, e := users.InsertMany(ctx, tx, dal.Item[User]{ID: "dup", Value: User{Name: "B"}})
		return e
	})
	require.Error(t, err)
}

func TestCollection_WriteNeedsWriteSession(t *testing.T) {
	// Positive half of AC write-needs-write-session: a write terminal compiles
	// and commits when given a transaction handle (which satisfies
	// WriteSession). The negative half — that Set(ctx, db, ...) with a plain DB
	// does NOT compile — is proven by the build-tagged file
	// collection_nocompile_example_test.go.
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.Set(ctx, tx, "u1", User{Name: "Alice"})
	})

	got, err := users.Get(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)
}
