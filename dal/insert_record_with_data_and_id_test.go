package dal_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertRecordWithDataAndID(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	users := dal.CollectionOf[string, User]()

	// Concrete pointer data.
	var got dal.RecordWithDataAndID[string, *User]
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		got, err = dal.InsertRecordWithDataAndID(ctx, tx, dal.NewKeyWithID("users", "u1"), "u1", &User{Name: "Alice"})
		return err
	})
	assert.Equal(t, "u1", got.ID)
	require.NotNil(t, got.Data)
	assert.Equal(t, "Alice", got.Data.Name)
	stored, err := users.GetData(ctx, db, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Alice", stored.Name)

	// Interface data (factory pattern): D is an interface holding a concrete
	// pointer — the case Collection.InsertWithID can't serve cleanly.
	var iface any = &User{Name: "Bob"}
	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, err := dal.InsertRecordWithDataAndID(ctx, tx, dal.NewKeyWithID("users", "u2"), "u2", iface)
		return err
	})
	stored2, err := users.GetData(ctx, db, "u2")
	require.NoError(t, err)
	assert.Equal(t, "Bob", stored2.Name)

	// Insert error path: inserting again at an existing id surfaces the session
	// Insert error.
	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, e := dal.InsertRecordWithDataAndID(ctx, tx, dal.NewKeyWithID("users", "u1"), "u1", &User{Name: "Dup"})
		return e
	})
	require.Error(t, err)

	// Invalid data (not a pointer/interface to struct/map) panics via
	// NewRecordWithDataAndID.
	assert.Panics(t, func() {
		_ = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			_, e := dal.InsertRecordWithDataAndID(ctx, tx, dal.NewKeyWithID("users", "u3"), "u3", User{})
			return e
		})
	})
}
