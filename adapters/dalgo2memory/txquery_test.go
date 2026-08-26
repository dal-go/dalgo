package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin transactional query support in optimistic mode: a query
// inside a read-write transaction participates in the transaction's snapshot
// and conflict detection at COLLECTION granularity (see
// optimisticState.observeCollectionAtSnapshot). Before this, every query in
// an optimistic transaction was refused outright with ErrNotSupported — the
// gap that blocked optimistic mode from ever becoming the default, since the
// default locked mode does support queries.
//
// As in snapshot_test.go, external commits are simulated with top-level
// writes on the same database: each is its own single-key commit and cannot
// deadlock against the transaction under test.

// queryThingIDs runs a keys-only query over "things" inside the transaction
// and returns how many records it saw.
func queryThingIDs(ctx context.Context, tx dal.ReadTransaction) (int, error) {
	q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := tx.ExecuteQueryToRecordsReader(ctx, q)
	if err != nil {
		return 0, err
	}
	records, err := dal.ReadAllToRecords(ctx, reader)
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

// TestTxQuery_WorksAndCommits: the basic capability — a query inside an
// optimistic read-write transaction returns the committed rows, and the
// transaction commits when nothing conflicted.
func TestTxQuery_WorksAndCommits(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		n, err := queryThingIDs(ctx, tx)
		if err != nil {
			return err
		}
		assert.Equal(t, 2, n, "the query must see both committed records")
		return tx.Set(ctx, thingRecord("C", 1))
	})
	require.NoError(t, err)
}

// TestTxQuery_PhantomInsertConflictsAtCommit is the reason collection-grained
// registration exists: after this transaction queried "things", an external
// commit INSERTS a new record there. No key in the per-key read set names the
// new record — it did not exist — yet the query's result is now stale, so the
// commit must conflict.
func TestTxQuery_PhantomInsertConflictsAtCommit(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := queryThingIDs(ctx, tx); err != nil {
			return err
		}
		// The phantom: a record the query could not have named as a key.
		require.NoError(t, db.Set(ctx, thingRecord("B", 1)))
		return nil
	})
	assert.True(t, IsTransactionConflict(err),
		"an insert into a queried collection must conflict the querying transaction: %v", err)
}

// TestTxQuery_UnrelatedCollectionWriteDoesNotConflict documents the
// granularity: only writes to the QUERIED collection conflict. A commit to a
// different collection leaves the querying transaction untouched.
func TestTxQuery_UnrelatedCollectionWriteDoesNotConflict(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := queryThingIDs(ctx, tx); err != nil {
			return err
		}
		other := record.NewRecordWithData(record.NewKeyWithID("others", "x"), map[string]any{"n": 1})
		require.NoError(t, db.Set(ctx, other))
		return nil
	})
	require.NoError(t, err, "a write to an unrelated collection must not conflict a query on things")
}

// TestTxQuery_QueryAfterSnapshotFractureAborts: the transaction pinned its
// snapshot with a point read; an external commit then wrote into "things";
// querying "things" now cannot return rows consistent with what the
// transaction already observed, so the query itself aborts — and a second
// query attempt hits the poisoned fast-path, and the swallowed conflict still
// refuses the commit.
func TestTxQuery_QueryAfterSnapshotFractureAborts(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	other := record.NewRecordWithData(record.NewKeyWithID("others", "x"), map[string]any{"n": 1})
	require.NoError(t, db.Set(ctx, other))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		// Pin the snapshot with a point read of an unrelated collection.
		var got map[string]any
		rec := record.NewRecordWithData(record.NewKeyWithID("others", "x"), &got)
		if err := tx.Get(ctx, rec); err != nil {
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("B", 2)))

		_, qErr := queryThingIDs(ctx, tx)
		require.True(t, IsTransactionConflict(qErr),
			"querying a collection written after the snapshot must abort at the query: %v", qErr)
		_, again := queryThingIDs(ctx, tx)
		require.True(t, IsTransactionConflict(again), "the poisoned transaction fails every later query too")
		return nil // swallow, as buggy caller code might
	})
	assert.True(t, IsTransactionConflict(err), "the swallowed query conflict must still refuse the commit")
}

// TestTxQuery_QueryPinsSnapshot: a query can be the transaction's FIRST
// observation, pinning the snapshot exactly as a point read would — proven by
// the point read AFTER a later external commit aborting.
func TestTxQuery_QueryPinsSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := queryThingIDs(ctx, tx); err != nil { // first observation: pins
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
		var got map[string]any
		rec := record.NewRecordWithData(thingKey("A"), &got)
		readErr := tx.Get(ctx, rec)
		require.True(t, IsTransactionConflict(readErr),
			"a point read of a key committed after the query's snapshot must abort: %v", readErr)
		return readErr
	})
	assert.True(t, IsTransactionConflict(err))
}

// TestTxQuery_SameCollectionQueriedTwice: repeat queries of one collection in
// one transaction register a single baseline and both succeed while nothing
// external has committed to it.
func TestTxQuery_SameCollectionQueriedTwice(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for i := 0; i < 2; i++ {
			n, err := queryThingIDs(ctx, tx)
			if err != nil {
				return err
			}
			assert.Equal(t, 1, n)
		}
		return nil
	})
	require.NoError(t, err)
}

// TestTxQuery_RecordsetReaderInsideTransaction: the recordset form delegates
// to the records form, so it inherits the same snapshot participation.
func TestTxQuery_RecordsetReaderInsideTransaction(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
		reader, err := tx.ExecuteQueryToRecordsetReader(ctx, q)
		if err != nil {
			return err
		}
		require.NotNil(t, reader)
		return nil
	})
	require.NoError(t, err)
}
