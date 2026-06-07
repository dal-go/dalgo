package dal_test

import (
	"context"
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
