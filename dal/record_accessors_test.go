package dal_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollection_GetRecordWithID(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[string, User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.SetByID(ctx, tx, "u1", User{Name: "Alice"})
	})

	got, err := users.GetRecordWithID(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "u1", got.ID)
	require.NotNil(t, got.Key)
	assert.Equal(t, "users/u1", got.Key.String())
	require.NotNil(t, got.Record)
	assert.Equal(t, "Alice", got.Record.Data().(*User).Name)

	// not-found returns the zero value + the session not-found error.
	missing, err := users.GetRecordWithID(ctx, db, "missing")
	require.Error(t, err)
	assert.True(t, record.IsNotFound(err))
	assert.Equal(t, record.WithID[string]{}, missing)
}

func TestCollection_GetRecordWithDataAndID(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[string, User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.SetByID(ctx, tx, "u1", User{Name: "Alice"})
	})

	got, err := users.GetRecordWithDataAndID(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "u1", got.ID)
	assert.Equal(t, "users/u1", got.Key.String())
	require.NotNil(t, got.Data)
	assert.Equal(t, "Alice", got.Data.Name)
	// Data is the same *T the record.Record holds (no copy).
	assert.Same(t, got.Data, got.Record.Data().(*User))

	// not-found returns the zero value + the session not-found error.
	missing, err := users.GetRecordWithDataAndID(ctx, db, "missing")
	require.Error(t, err)
	assert.True(t, record.IsNotFound(err))
	assert.Equal(t, record.DataWithID[string, *User]{}, missing)
}

func TestGetRecordWithIDIntoData(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[string, User]()

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.SetByID(ctx, tx, "u1", User{Name: "Alice"})
	})

	key := record.NewKeyWithID("users", "u1")

	// Concrete pointer data: decoded INTO the provided value.
	into := &User{}
	got, err := dal.GetRecordWithIDIntoData(ctx, db, key, "u1", into)
	require.NoError(t, err)
	assert.Equal(t, "u1", got.ID)
	assert.Same(t, into, got.Data, "must decode into the caller-supplied value")
	assert.Equal(t, "Alice", into.Name)

	// Interface data (factory pattern): D is an interface holding a concrete
	// pointer — exactly the case Collection.GetRecordWithDataAndID cannot serve.
	var iface any = &User{}
	gotIface, err := dal.GetRecordWithIDIntoData(ctx, db, key, "u1", iface)
	require.NoError(t, err)
	assert.Equal(t, "Alice", gotIface.Data.(*User).Name)

	// not-found returns the built value + the session not-found error.
	_, err = dal.GetRecordWithIDIntoData(ctx, db, record.NewKeyWithID("users", "missing"), "missing", &User{})
	require.Error(t, err)
	assert.True(t, record.IsNotFound(err))

	// invalid data (not a pointer/interface to struct/map) panics via
	// record.NewDataWithID.
	assert.Panics(t, func() {
		_, _ = dal.GetRecordWithIDIntoData(ctx, db, key, "u1", User{})
	})
}
