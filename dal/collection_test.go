package dal_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
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
