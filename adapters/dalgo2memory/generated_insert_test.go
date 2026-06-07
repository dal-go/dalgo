package dalgo2memory_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type genUser struct {
	Name string `json:"name"`
}

// TestCollection_Insert_EndToEnd verifies AC e2e-generated-insert-roundtrip: the
// typed Collection[T].Insert (default generator and an explicit
// WithRandomStringKey) returns a complete assigned key against dalgo2memory and
// the record round-trips via Get. This dalgo2memory-specific test is separate
// from the shared gomock-driven TestDalgoDB suite (which stays green).
func TestCollection_Insert_EndToEnd(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.NewDB()
	users := dal.CollectionAt[genUser]("e2eusers")

	// Default generator (no options).
	var k1 *dal.Key
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		k1, err = users.Insert(ctx, tx, genUser{Name: "Alice"})
		return err
	}))
	require.NotNil(t, k1)
	id1, ok := k1.ID.(string)
	require.True(t, ok)
	require.NotEmpty(t, id1)
	got1, err := users.Get(ctx, db, id1)
	require.NoError(t, err)
	assert.Equal(t, "Alice", got1.Name)

	// Explicit generator with a custom length.
	var k2 *dal.Key
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var err error
		k2, err = users.Insert(ctx, tx, genUser{Name: "Bob"}, dal.WithRandomStringKey(24, 5))
		return err
	}))
	id2, ok := k2.ID.(string)
	require.True(t, ok)
	assert.Len(t, id2, 24)
	got2, err := users.Get(ctx, db, id2)
	require.NoError(t, err)
	assert.Equal(t, "Bob", got2.Name)
}

// TestSession_Insert_GeneratesViaInsertOption verifies AC
// memory-generates-via-insert-option: a raw session.Insert (reached via the
// transaction handle, outside the typed layer) with a generator InsertOption
// persists the record under a generated non-empty id, retrievable by that id.
func TestSession_Insert_GeneratesViaInsertOption(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.NewDB()

	var record dal.Record
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		record = dal.NewRecordWithIncompleteKey("users", reflect.String, &genUser{Name: "Alice"})
		return tx.Insert(ctx, record, dal.WithRandomStringKey(16, 5))
	}))

	id, ok := record.Key().ID.(string)
	require.True(t, ok, "generated id must be a string")
	require.NotEmpty(t, id)
	assert.Len(t, id, 16)

	// Retrievable by the generated id.
	out := &genUser{}
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", id), out)))
	assert.Equal(t, "Alice", out.Name)
}

// TestSession_Insert_GenerationPrecedesStorage_NoNilID verifies (part of AC
// generation-precedes-storage) that a generated insert never stores under a
// "<nil>" id.
func TestSession_Insert_GenerationPrecedesStorage_NoNilID(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.NewDB()

	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		record := dal.NewRecordWithIncompleteKey("users", reflect.String, &genUser{Name: "Alice"})
		return tx.Insert(ctx, record, dal.WithRandomStringKey(16, 5))
	}))

	// No record was ever stored under a "<nil>" id (fmt.Sprint(nil)).
	exists, err := db.Exists(ctx, dal.NewKeyWithID("users", "<nil>"))
	require.NoError(t, err)
	assert.False(t, exists, "no record must be stored under a <nil> id")
}

// TestSession_Insert_GeneratorExhaustion verifies (part of AC
// generation-precedes-storage) that when the generator keeps colliding, the
// call returns the exhaustion error and persists nothing new. A day-accuracy
// timestamp generator deterministically produces the same id within a run.
func TestSession_Insert_GeneratorExhaustion(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.NewDB()

	gen := dal.WithTimeStampStringID(dal.TimeStampAccuracyDay, 10, 5)

	// First insert succeeds and occupies the day's id.
	var firstID any
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		record := dal.NewRecordWithIncompleteKey("days", reflect.String, &genUser{Name: "first"})
		if err := tx.Insert(ctx, record, gen); err != nil {
			return err
		}
		firstID = record.Key().ID
		return nil
	}))
	require.NotNil(t, firstID)

	// Second insert collides on every attempt and must exhaust without persisting.
	var second dal.Record
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		second = dal.NewRecordWithIncompleteKey("days", reflect.String, &genUser{Name: "second"})
		return tx.Insert(ctx, second, gen)
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, dal.ErrExceedsMaxNumberOfAttempts)
	assert.Nil(t, second.Key().ID, "id must be reset to nil on exhaustion")

	// Nothing was overwritten: the first record is intact.
	out := &genUser{}
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("days", firstID.(string)), out)))
	assert.Equal(t, "first", out.Name)
}
