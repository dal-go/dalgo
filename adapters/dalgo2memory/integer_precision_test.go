package dalgo2memory

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

type walletData struct {
	Balance int64 `json:"balance"`
	Seq     int64 `json:"seq"`
}

type userDbo struct {
	ID     string     `json:"id,omitempty"`
	Name   string     `json:"name,omitempty"`
	Wallet walletData `json:"wallet"`
}

type seqDbo struct {
	ID    string  `json:"id,omitempty"`
	Seq   int64   `json:"seq"`
	Tags  []int64 `json:"tags,omitempty"`
	Title string  `json:"title,omitempty"`
}

type orderDbo struct {
	ID      string `json:"id,omitempty"`
	UserSeq int64  `json:"user_seq"`
	Amount  int64  `json:"amount"`
}

func userKey(id string) *record.Key {
	return record.NewKeyWithID("users", id)
}

func seqKey(id string) *record.Key {
	return record.NewKeyWithID("seqs", id)
}

func orderKey(id string) *record.Key {
	return record.NewKeyWithID("orders", id)
}

// TestIntegerPrecision_TransactionGetInsertSetUpdate verifies that MaxInt64, MinInt64,
// and adjacent integer values survive Insert, Set, Get, unrelated Update, clone,
// and re-read within transactions across both serialized and columnar engines,
// respecting read-before-write semantics in standard mode and supporting interleaved reads.
func TestIntegerPrecision_TransactionGetInsertSetUpdate(t *testing.T) {
	t.Parallel()

	testEngines := []struct {
		name string
		opts []CollectionOption
	}{
		{"Serialized", []CollectionOption{WithSerializedStorage()}},
		{"Columnar", []CollectionOption{WithColumnarStorage()}},
	}

	testModes := []struct {
		name   string
		dbOpts []Option
	}{
		{"LockedMode", nil},
		{"OptimisticMode", []Option{WithOptimisticConcurrency()}},
	}

	for _, eng := range testEngines {
		for _, mode := range testModes {
			t.Run("ReadBeforeWrite/"+eng.name+"/"+mode.name, func(t *testing.T) {
				ctx := context.Background()
				dbOpts := append([]Option{}, mode.dbOpts...)
				dbOpts = append(dbOpts, WithSchema(false,
					WithCollection[userDbo]("users", nil, eng.opts...),
				))
				db := newDatabase(dbOpts...)

				maxUser := userDbo{
					ID:   "u-max",
					Name: "MaxUser",
					Wallet: walletData{
						Balance: math.MaxInt64,
						Seq:     math.MaxInt64,
					},
				}
				adjUser := userDbo{
					ID:   "u-adj",
					Name: "AdjUser",
					Wallet: walletData{
						Balance: math.MaxInt64 - 1,
						Seq:     math.MaxInt64 - 1,
					},
				}
				minUser := userDbo{
					ID:   "u-min",
					Name: "MinUser",
					Wallet: walletData{
						Balance: math.MinInt64,
						Seq:     math.MinInt64,
					},
				}

				require.NoError(t, db.Set(ctx, record.NewRecordWithData(userKey("u-max"), &maxUser)))
				require.NoError(t, db.Set(ctx, record.NewRecordWithData(userKey("u-adj"), &adjUser)))
				require.NoError(t, db.Set(ctx, record.NewRecordWithData(userKey("u-min"), &minUser)))

				// Standard read-before-write transaction:
				// 1. Reads occur before writes.
				// 2. Unrelated Update occurs on "name".
				// 3. Commit persists exact numbers.
				err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					var readMax userDbo
					err := tx.Get(ctx, record.NewRecordWithData(userKey("u-max"), &readMax))
					require.NoError(t, err)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Seq)

					var readAdj userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-adj"), &readAdj))
					require.NoError(t, err)
					require.Equal(t, int64(math.MaxInt64-1), readAdj.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64-1), readAdj.Wallet.Seq)

					var readMin userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-min"), &readMin))
					require.NoError(t, err)
					require.Equal(t, int64(math.MinInt64), readMin.Wallet.Balance)
					require.Equal(t, int64(math.MinInt64), readMin.Wallet.Seq)

					// Unrelated update
					err = tx.Update(ctx, userKey("u-max"), []update.Update{
						update.ByFieldName("name", "MaxUserUpdated"),
					})
					require.NoError(t, err)

					// Nested wallet field update via field path
					err = tx.Update(ctx, userKey("u-adj"), []update.Update{
						update.ByFieldPath(update.FieldPath{"wallet", "seq"}, int64(math.MaxInt64)),
					})
					require.NoError(t, err)

					return nil
				})
				require.NoError(t, err)

				// Verify in fresh transaction
				err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					var readMax userDbo
					err := tx.Get(ctx, record.NewRecordWithData(userKey("u-max"), &readMax))
					require.NoError(t, err)
					require.Equal(t, "MaxUserUpdated", readMax.Name)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Seq)

					var readAdj userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-adj"), &readAdj))
					require.NoError(t, err)
					require.Equal(t, int64(math.MaxInt64-1), readAdj.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64), readAdj.Wallet.Seq)

					var readMin userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-min"), &readMin))
					require.NoError(t, err)
					require.Equal(t, int64(math.MinInt64), readMin.Wallet.Balance)
					require.Equal(t, int64(math.MinInt64), readMin.Wallet.Seq)
					return nil
				})
				require.NoError(t, err)
			})

			t.Run("InterleavedReadWrite/"+eng.name+"/"+mode.name, func(t *testing.T) {
				ctx := context.Background()
				dbOpts := append([]Option{}, mode.dbOpts...)
				dbOpts = append(dbOpts,
					WithInterleavedReadsAndWritesInTransaction(),
					WithSchema(false, WithCollection[userDbo]("users", nil, eng.opts...)),
				)
				db := newDatabase(dbOpts...)

				maxUser := userDbo{
					ID:   "u-max-i",
					Name: "MaxInterleaved",
					Wallet: walletData{
						Balance: math.MaxInt64,
						Seq:     math.MaxInt64,
					},
				}

				err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					err := tx.Insert(ctx, record.NewRecordWithData(userKey("u-max-i"), &maxUser))
					require.NoError(t, err)

					// Read back immediately after insert within the same transaction
					var readMax userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-max-i"), &readMax))
					require.NoError(t, err)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64), readMax.Wallet.Seq)

					// Unrelated update
					err = tx.Update(ctx, userKey("u-max-i"), []update.Update{
						update.ByFieldName("name", "MaxInterleavedUpdated"),
					})
					require.NoError(t, err)

					// Re-read immediately after update
					var readAfterUpdate userDbo
					err = tx.Get(ctx, record.NewRecordWithData(userKey("u-max-i"), &readAfterUpdate))
					require.NoError(t, err)
					require.Equal(t, "MaxInterleavedUpdated", readAfterUpdate.Name)
					require.Equal(t, int64(math.MaxInt64), readAfterUpdate.Wallet.Balance)
					require.Equal(t, int64(math.MaxInt64), readAfterUpdate.Wallet.Seq)

					return nil
				})
				require.NoError(t, err)

				// Verify in fresh transaction
				var finalUser userDbo
				require.NoError(t, db.Get(ctx, record.NewRecordWithData(userKey("u-max-i"), &finalUser)))
				require.Equal(t, "MaxInterleavedUpdated", finalUser.Name)
				require.Equal(t, int64(math.MaxInt64), finalUser.Wallet.Balance)
				require.Equal(t, int64(math.MaxInt64), finalUser.Wallet.Seq)
			})
		}
	}
}

// TestIntegerPrecision_TransactionRollback verifies that rolled back transactions
// discard pending writes without corrupting stored exact integer state.
func TestIntegerPrecision_TransactionRollback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, mode := range [][]Option{nil, {WithOptimisticConcurrency()}} {
		db := newDatabase(append(mode, WithSchema(false, WithCollection[userDbo]("users", nil)))...)

		initial := userDbo{
			ID:   "u1",
			Name: "Initial",
			Wallet: walletData{
				Balance: math.MaxInt64,
				Seq:     math.MaxInt64,
			},
		}
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(userKey("u1"), &initial)))

		// Rollback attempt
		injectedErr := errors.New("rollback this tx")
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			err := tx.Update(ctx, userKey("u1"), []update.Update{
				update.ByFieldName("name", "ShouldNotPersist"),
				update.ByFieldPath(update.FieldPath{"wallet", "seq"}, int64(1)),
			})
			require.NoError(t, err)
			return injectedErr
		})
		require.ErrorIs(t, err, injectedErr)

		// Verify prior state remains intact
		var current userDbo
		require.NoError(t, db.Get(ctx, record.NewRecordWithData(userKey("u1"), &current)))
		require.Equal(t, "Initial", current.Name)
		require.Equal(t, int64(math.MaxInt64), current.Wallet.Balance)
		require.Equal(t, int64(math.MaxInt64), current.Wallet.Seq)
	}
}

// TestIntegerPrecision_TransactionRetry verifies that optimistic conflict retries
// retain exact integer precision across retry attempts.
func TestIntegerPrecision_TransactionRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase(WithOptimisticConcurrency(), WithSchema(false, WithCollection[userDbo]("users", nil)))

	initial := userDbo{
		ID:   "u1",
		Name: "Initial",
		Wallet: walletData{
			Balance: math.MaxInt64,
			Seq:     math.MaxInt64,
		},
	}
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(userKey("u1"), &initial)))

	attempts := 0
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		attempts++
		var u userDbo
		err := tx.Get(ctx, record.NewRecordWithData(userKey("u1"), &u))
		require.NoError(t, err)

		if attempts == 1 {
			// Concurrently modify the same key from outside to induce ErrTransactionConflict
			concurrent := userDbo{
				ID:   "u1",
				Name: "Concurrent",
				Wallet: walletData{
					Balance: math.MaxInt64 - 1,
					Seq:     math.MaxInt64 - 1,
				},
			}
			require.NoError(t, db.Set(context.Background(), record.NewRecordWithData(userKey("u1"), &concurrent)))
		}

		return tx.Update(ctx, userKey("u1"), []update.Update{
			update.ByFieldName("name", "AfterRetry"),
			update.ByFieldPath(update.FieldPath{"wallet", "balance"}, int64(math.MaxInt64)),
		})
	}, dal.TxWithAttempts(3))

	require.NoError(t, err)
	require.Equal(t, 2, attempts)

	var finalUser userDbo
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(userKey("u1"), &finalUser)))
	require.Equal(t, "AfterRetry", finalUser.Name)
	require.Equal(t, int64(math.MaxInt64), finalUser.Wallet.Balance)
	require.Equal(t, int64(math.MaxInt64-1), finalUser.Wallet.Seq)
}

// TestIntegerPrecision_QueriesAndComparisons checks WHERE equality, inequality,
// and ORDER BY on adjacent large integers across Serialized and Columnar storage.
func TestIntegerPrecision_QueriesAndComparisons(t *testing.T) {
	t.Parallel()

	engines := []struct {
		name string
		opts []CollectionOption
	}{
		{"Serialized", []CollectionOption{WithSerializedStorage()}},
		{"Columnar", []CollectionOption{WithColumnarStorage()}},
	}

	for _, eng := range engines {
		t.Run(eng.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(WithSchema(false, WithCollection[seqDbo]("seqs", nil, eng.opts...)))

			items := []seqDbo{
				{ID: "r1", Seq: math.MaxInt64, Tags: []int64{math.MaxInt64, 42}, Title: "Max"},
				{ID: "r2", Seq: math.MaxInt64 - 1, Tags: []int64{math.MaxInt64 - 1, 42}, Title: "MaxMinus1"},
				{ID: "r3", Seq: math.MinInt64, Tags: []int64{math.MinInt64, 42}, Title: "Min"},
				{ID: "r4", Seq: math.MinInt64 + 1, Tags: []int64{math.MinInt64 + 1, 42}, Title: "MinPlus1"},
			}
			for _, item := range items {
				require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey(item.ID), item)))
			}

			intoSeq := func() record.Record {
				return record.NewRecordWithIncompleteKey("seqs", reflect.String, &seqDbo{})
			}

			// Equality query for exact MaxInt64: must match ONLY r1, never r2!
			qMax := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				WhereField("seq", dal.Equal, int64(math.MaxInt64)).
				SelectIntoRecord(intoSeq)
			reader, err := db.ExecuteQueryToRecordsReader(ctx, qMax)
			require.NoError(t, err)
			recs, err := dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r1", recs[0].Key().ID)
			require.Equal(t, int64(math.MaxInt64), recs[0].Data().(*seqDbo).Seq)

			// Equality query for MaxInt64 - 1: must match ONLY r2, never r1!
			qAdj := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				WhereField("seq", dal.Equal, int64(math.MaxInt64-1)).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qAdj)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r2", recs[0].Key().ID)
			require.Equal(t, int64(math.MaxInt64-1), recs[0].Data().(*seqDbo).Seq)

			// Inequality query: seq > MaxInt64 - 1: must match ONLY r1
			qGt := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				WhereField("seq", dal.GreaterThen, int64(math.MaxInt64-1)).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qGt)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r1", recs[0].Key().ID)

			// Inequality query: seq < MinInt64 + 1: must match ONLY r3
			qLt := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				WhereField("seq", dal.LessThen, int64(math.MinInt64+1)).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qLt)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r3", recs[0].Key().ID)

			// Array contains query with exact MaxInt64
			qArray := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				WhereField("tags", dal.In, dal.Array{Value: []any{int64(math.MaxInt64)}}).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qArray)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r1", recs[0].Key().ID)

			// Constant In FieldRef with exact MaxInt64
			qConstIn := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				Where(dal.NewComparison(dal.NewConstant(int64(math.MaxInt64)), dal.In, dal.NewFieldRef("", "tags"))).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qConstIn)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "r1", recs[0].Key().ID)

			// ORDER BY ASC
			qAsc := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				OrderBy(dal.AscendingField("seq")).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qAsc)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 4)
			require.Equal(t, "r3", recs[0].Key().ID) // MinInt64
			require.Equal(t, "r4", recs[1].Key().ID) // MinInt64 + 1
			require.Equal(t, "r2", recs[2].Key().ID) // MaxInt64 - 1
			require.Equal(t, "r1", recs[3].Key().ID) // MaxInt64

			// ORDER BY DESC
			qDesc := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
				OrderBy(dal.DescendingField("seq")).
				SelectIntoRecord(intoSeq)
			reader, err = db.ExecuteQueryToRecordsReader(ctx, qDesc)
			require.NoError(t, err)
			recs, err = dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			require.Len(t, recs, 4)
			require.Equal(t, "r1", recs[0].Key().ID) // MaxInt64
			require.Equal(t, "r2", recs[1].Key().ID) // MaxInt64 - 1
			require.Equal(t, "r4", recs[2].Key().ID) // MinInt64 + 1
			require.Equal(t, "r3", recs[3].Key().ID) // MinInt64
		})
	}
}

// TestIntegerPrecision_Aggregates verifies that MIN, MAX, COUNT, SUM
// correctly handle retained numeric representations.
func TestIntegerPrecision_Aggregates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase(WithSchema(false, WithCollection[seqDbo]("seqs", nil)))

	items := []seqDbo{
		{ID: "1", Seq: math.MaxInt64 - 1, Title: "A"},
		{ID: "2", Seq: math.MaxInt64, Title: "A"},
		{ID: "3", Seq: 10, Title: "B"},
		{ID: "4", Seq: 20, Title: "B"},
	}
	for _, item := range items {
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey(item.ID), item)))
	}

	count := dal.Count()
	count.Alias = "count"

	// GROUP BY Title with MAX and MIN on Seq
	q := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
		GroupBy(dal.Field("title")).
		OrderBy(dal.AscendingField("title")).
		SelectColumns(
			dal.Column{Expression: dal.Field("title")},
			dal.MaxAs(dal.Field("seq"), "max_seq"),
			dal.MinAs(dal.Field("seq"), "min_seq"),
			count,
			dal.SumAs(dal.Field("seq"), "sum_seq"),
		)

	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	recs, err := dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 2)

	// Group "A"
	rowA := recs[0].Data().(map[string]any)
	require.Equal(t, "A", rowA["title"])
	require.Equal(t, float64(math.MaxInt64), rowA["max_seq"])
	require.Equal(t, float64(math.MaxInt64-1), rowA["min_seq"])
	require.EqualValues(t, 2, rowA["count"])

	// Group "B"
	rowB := recs[1].Data().(map[string]any)
	require.Equal(t, "B", rowB["title"])
	require.Equal(t, float64(20), rowB["max_seq"])
	require.Equal(t, float64(10), rowB["min_seq"])
	require.EqualValues(t, 2, rowB["count"])
	require.Equal(t, float64(30), rowB["sum_seq"])
}

// TestIntegerPrecision_Join verifies join predicate matching on exact large integer fields.
func TestIntegerPrecision_Join(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase(WithSchema(false,
		WithCollection[seqDbo]("seqs", nil),
		WithCollection[orderDbo]("orders", nil),
	))

	require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey("s1"), seqDbo{ID: "s1", Seq: math.MaxInt64, Title: "ExactMax"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey("s2"), seqDbo{ID: "s2", Seq: math.MaxInt64 - 1, Title: "AdjMax"})))

	require.NoError(t, db.Set(ctx, record.NewRecordWithData(orderKey("o1"), orderDbo{ID: "o1", UserSeq: math.MaxInt64, Amount: 500})))

	join := dal.NewJoinedSource(
		dal.NewRootCollectionRef("orders", "o"),
		dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("s", "seq"), dal.Equal, dal.NewFieldRef("o", "user_seq")),
	)
	q := dal.From(dal.NewRootCollectionRef("seqs", "s")).Join(join).NewQuery().
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithData(record.NewKeyWithID("seqs", ""), &map[string]any{})
		})

	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	recs, err := dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 1, "INNER join must match only exact seq == user_seq, not adjacent integer")

	data := recs[0].Data().(map[string]any)
	require.Equal(t, "ExactMax", data["title"])
	require.Equal(t, float64(500), data["amount"])
}

// TestIntegerPrecision_ExternalMapCompatibility verifies that reading records
// into map[string]any decodes numbers as float64, preserving existing public caller expectations.
func TestIntegerPrecision_ExternalMapCompatibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase()
	key := record.NewKeyWithID("items", "item1")
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, map[string]any{
		"age":  30,
		"name": "Alice",
	})))

	// Non-transactional Get into map[string]any
	var got map[string]any
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &got)))
	require.Equal(t, float64(30), got["age"])
	require.Equal(t, "Alice", got["name"])

	// Transactional Get into map[string]any
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var txGot map[string]any
		err := tx.Get(ctx, record.NewRecordWithData(key, &txGot))
		require.NoError(t, err)
		require.Equal(t, float64(30), txGot["age"])
		require.Equal(t, "Alice", txGot["name"])
		return nil
	})
	require.NoError(t, err)
}

// TestIntegerPrecision_NaNQueries verifies that NaN constants and comparisons
// never match ordinary numeric rows across =, >, <, >=, <= operators.
func TestIntegerPrecision_NaNQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase(WithSchema(false, WithCollection[seqDbo]("seqs", nil)))

	items := []seqDbo{
		{ID: "s1", Seq: 1, Title: "One"},
		{ID: "s2", Seq: 2, Title: "Two"},
		{ID: "s3", Seq: math.MaxInt64, Title: "Max"},
	}
	for _, it := range items {
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey(it.ID), it)))
	}

	for _, op := range []dal.Operator{
		dal.Equal,
		dal.GreaterThen,
		dal.LessThen,
		dal.GreaterOrEqual,
		dal.LessOrEqual,
	} {
		q := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
			Where(dal.WhereField("seq", op, math.NaN())).
			SelectColumns(dal.Column{Expression: dal.Field("id")})

		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.NoError(t, err)
		recs, err := dal.ReadAllToRecords(ctx, reader)
		require.NoError(t, err)
		require.Empty(t, recs, "Comparison %v with NaN must match 0 rows", op)
	}
}

// TestIntegerPrecision_GroupedAliasHavingAndOrderBy verifies that HAVING and ORDER BY
// evaluate exact integer values on SELECT aliases before final public map normalization,
// accurately distinguishing adjacent MaxInt64 values.
func TestIntegerPrecision_GroupedAliasHavingAndOrderBy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase(WithSchema(false, WithCollection[seqDbo]("seqs", nil)))

	// Group 1 has max MaxInt64; Group 2 has max MaxInt64 - 1
	items := []seqDbo{
		{ID: "g1_1", Title: "G1", Seq: math.MaxInt64},
		{ID: "g1_2", Title: "G1", Seq: math.MaxInt64 - 100},
		{ID: "g2_1", Title: "G2", Seq: math.MaxInt64 - 1},
		{ID: "g2_2", Title: "G2", Seq: math.MaxInt64 - 200},
	}
	for _, it := range items {
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(seqKey(it.ID), it)))
	}

	// 1. HAVING on alias matching G1's max exactly
	q1 := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
		GroupBy(dal.Field("title")).
		Having(dal.WhereField("max_seq", dal.Equal, int64(math.MaxInt64))).
		SelectColumns(
			dal.Column{Expression: dal.Field("title")},
			dal.MaxAs(dal.Field("seq"), "max_seq"),
		)

	reader, err := db.ExecuteQueryToRecordsReader(ctx, q1)
	require.NoError(t, err)
	recs, err := dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 1, "HAVING alias must match only G1 with exact MaxInt64")
	data1 := recs[0].Data().(map[string]any)
	require.Equal(t, "G1", data1["title"])
	require.Equal(t, float64(math.MaxInt64), data1["max_seq"])

	// 2. HAVING on alias matching G2's max exactly
	q2 := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
		GroupBy(dal.Field("title")).
		Having(dal.WhereField("max_seq", dal.Equal, int64(math.MaxInt64-1))).
		SelectColumns(
			dal.Column{Expression: dal.Field("title")},
			dal.MaxAs(dal.Field("seq"), "max_seq"),
		)

	reader, err = db.ExecuteQueryToRecordsReader(ctx, q2)
	require.NoError(t, err)
	recs, err = dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 1, "HAVING alias must match only G2 with exact MaxInt64 - 1")
	data2 := recs[0].Data().(map[string]any)
	require.Equal(t, "G2", data2["title"])
	require.Equal(t, float64(math.MaxInt64-1), data2["max_seq"])

	// 3. ORDER BY max_seq ASC: G2 (MaxInt64 - 1) must sort before G1 (MaxInt64)
	q3 := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
		GroupBy(dal.Field("title")).
		OrderBy(dal.AscendingField("max_seq")).
		SelectColumns(
			dal.Column{Expression: dal.Field("title")},
			dal.MaxAs(dal.Field("seq"), "max_seq"),
		)

	reader, err = db.ExecuteQueryToRecordsReader(ctx, q3)
	require.NoError(t, err)
	recs, err = dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.Equal(t, "G2", recs[0].Data().(map[string]any)["title"])
	require.Equal(t, "G1", recs[1].Data().(map[string]any)["title"])

	// 4. ORDER BY max_seq DESC: G1 (MaxInt64) must sort before G2 (MaxInt64 - 1)
	q4 := dal.From(dal.NewRootCollectionRef("seqs", "")).NewQuery().
		GroupBy(dal.Field("title")).
		OrderBy(dal.DescendingField("max_seq")).
		SelectColumns(
			dal.Column{Expression: dal.Field("title")},
			dal.MaxAs(dal.Field("seq"), "max_seq"),
		)

	reader, err = db.ExecuteQueryToRecordsReader(ctx, q4)
	require.NoError(t, err)
	recs, err = dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.Equal(t, "G1", recs[0].Data().(map[string]any)["title"])
	require.Equal(t, "G2", recs[1].Data().(map[string]any)["title"])
}

// TestIntegerPrecision_CanonicalGrouping verifies that 1, 1.0, json.Number("1"), and json.Number("1.0")
// partition into the same group, while adjacent large integers MaxInt64 and MaxInt64 - 1
// form distinct groups, and unsigned integers 2^63 and 2^63+2048 group with their exactly
// equivalent float tokens while distinct from neighboring unequal integers.
func TestIntegerPrecision_CanonicalGrouping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newDatabase()

	records := []struct {
		id  string
		num any
	}{
		{"r1", int(1)},
		{"r2", float64(1.0)},
		{"r3", json.Number("1")},
		{"r4", json.Number("1.0")},
		{"r5", int64(math.MaxInt64)},
		{"r6", int64(math.MaxInt64 - 1)},
		{"r7", json.Number("9223372036854775808")},    // 2^63 uint token
		{"r8", json.Number("9223372036854775808.0")},  // 2^63 float token
		{"r9", uint64((1 << 63) + 2048)},              // 2^63 + 2048 uint token
		{"r10", json.Number("9223372036854777856.0")}, // 2^63 + 2048 float token
		{"r11", json.Number("9223372036854775809")},   // 2^63 + 1 (neighboring unequal integer)
	}

	for _, r := range records {
		key := record.NewKeyWithID("items", r.id)
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, map[string]any{
			"num": r.num,
		})))
	}

	countCol := dal.Count()
	countCol.Alias = "cnt"

	q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().
		GroupBy(dal.Field("num")).
		OrderBy(dal.AscendingField("num")).
		SelectColumns(
			dal.Column{Expression: dal.Field("num")},
			countCol,
		)

	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	recs, err := dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)

	// We expect 6 distinct groups:
	// Group 1: 1 / 1.0 variants (4 rows)
	// Group 2: MaxInt64 - 1 (1 row)
	// Group 3: MaxInt64 (1 row)
	// Group 4: 2^63 equivalent tokens (2 rows)
	// Group 5: 2^63 + 2048 equivalent tokens (2 rows)
	// Group 6: 2^63 + 1 neighboring unequal integer (1 row)
	require.Len(t, recs, 6, "Expected 6 canonical groups")

	count4Groups := 0
	count2Groups := 0
	count1Groups := 0

	for _, rec := range recs {
		data := rec.Data().(map[string]any)
		cnt := data["cnt"].(int)
		val := data["num"].(float64)
		switch cnt {
		case 4:
			require.Equal(t, 1.0, val, "1 and 1.0 variants must group together into count 4")
			count4Groups++
		case 2:
			count2Groups++
		case 1:
			count1Groups++
		default:
			t.Fatalf("unexpected group count %d for val %v", cnt, val)
		}
	}

	require.Equal(t, 1, count4Groups, "Should find exactly 1 group with count 4 (1 and 1.0 variants)")
	require.Equal(t, 2, count2Groups, "Should find 2 groups with count 2 (2^63 and 2^63+2048 equivalent tokens)")
	require.Equal(t, 3, count1Groups, "Should find 3 groups with count 1 (MaxInt64-1, MaxInt64, and 2^63+1)")
}

// recordingCustomStrategy is a test strategy verifying callback argument types and candidate filtering.
type recordingCustomStrategy struct {
	values     map[int]any
	cleared    map[int]bool
	setCalls   int
	clearCalls int
	equalCalls int
}

func newRecordingCustomStrategy() *recordingCustomStrategy {
	return &recordingCustomStrategy{
		values:  make(map[int]any),
		cleared: make(map[int]bool),
	}
}

func (s *recordingCustomStrategy) SetValue(slot int, value any) {
	s.setCalls++
	s.values[slot] = value
}

func (s *recordingCustomStrategy) ClearValue(slot int) {
	s.clearCalls++
	s.cleared[slot] = true
	delete(s.values, slot)
}

func (s *recordingCustomStrategy) EqualSlots(value any) (SlotSet, bool) {
	s.equalCalls++
	slots := make(SlotSet)
	for slot, v := range s.values {
		if v == value {
			slots[slot] = struct{}{}
		}
	}
	return slots, true
}

// TestIntegerPrecision_CustomColumnStrategy verifies that custom ColumnStrategy callbacks
// preserve declared typed values (including typed json.Number columns) on initial write,
// update, and compaction rebuild, while interface/any columns still expose historical
// recursively normalized float64 values, and candidate filtering safely falls back to
// row evaluation for exact verification.
func TestIntegerPrecision_CustomColumnStrategy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	stgyTyped := newRecordingCustomStrategy()
	stgyJsonNum := newRecordingCustomStrategy()
	stgyAny := newRecordingCustomStrategy()

	db := newDatabase(WithSchema(false,
		WithCollection[map[string]any]("records", nil,
			WithColumnarStorage(
				WithDeclaredColumn[int64]("big_int"),
				WithColumnStrategy("big_int", stgyTyped),
				WithDeclaredColumn[json.Number]("typed_json_num"),
				WithColumnStrategy("typed_json_num", stgyJsonNum),
				WithDeclaredColumn[any]("generic_val"),
				WithColumnStrategy("generic_val", stgyAny),
			),
		),
	))

	// 1. Initial write
	key1 := record.NewKeyWithID("records", "rec1")
	err := db.Set(ctx, record.NewRecordWithData(key1, map[string]any{
		"big_int":        int64(math.MaxInt64),
		"typed_json_num": json.Number("9223372036854775807"),
		"generic_val": map[string]any{
			"amount": json.Number("100"),
			"seq":    json.Number("9223372036854775807"),
			"nested": []any{json.Number("200")},
		},
	}))
	require.NoError(t, err)

	// Typed column strategy received typed int64
	require.Equal(t, int64(math.MaxInt64), stgyTyped.values[0])

	// Declared typed json.Number strategy preserved its exact json.Number type and value
	require.Equal(t, json.Number("9223372036854775807"), stgyJsonNum.values[0])
	require.IsType(t, json.Number(""), stgyJsonNum.values[0], "Declared typed json.Number column must preserve json.Number")

	// Generic column strategy received normalized float64 values, not json.Number
	genVal0, ok := stgyAny.values[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(100), genVal0["amount"])
	require.Equal(t, float64(math.MaxInt64), genVal0["seq"])
	nested0, ok := genVal0["nested"].([]any)
	require.True(t, ok)
	require.Equal(t, float64(200), nested0[0])

	// 2. Update path
	err = db.Update(ctx, key1, []update.Update{
		update.ByFieldName("typed_json_num", json.Number("9223372036854775806")),
		update.ByFieldName("generic_val", map[string]any{
			"amount": json.Number("300"),
		}),
	})
	require.NoError(t, err)

	require.Equal(t, json.Number("9223372036854775806"), stgyJsonNum.values[0])
	require.IsType(t, json.Number(""), stgyJsonNum.values[0], "Updated typed json.Number column must preserve json.Number")

	genValUpdated, ok := stgyAny.values[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(300), genValUpdated["amount"])

	// 3. Compaction rebuild path
	// Insert a second record, delete the first, and trigger compaction
	key2 := record.NewKeyWithID("records", "rec2")
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(key2, map[string]any{
		"big_int":        int64(math.MaxInt64 - 1),
		"typed_json_num": json.Number("9223372036854775805"),
		"generic_val": map[string]any{
			"amount": json.Number("400"),
		},
	})))

	require.NoError(t, db.Delete(ctx, key1))

	// Compacting the collection rebuilds strategies via rebuildStrategies()
	eng := db.collections["records"].(*columnarEngine)
	eng.compact()

	// After compaction, live slot is compacted to 0 for rec2
	require.Equal(t, int64(math.MaxInt64-1), stgyTyped.values[0])
	require.Equal(t, json.Number("9223372036854775805"), stgyJsonNum.values[0])
	require.IsType(t, json.Number(""), stgyJsonNum.values[0], "Compacted live slot must preserve typed json.Number")

	genValCompacted, ok := stgyAny.values[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(400), genValCompacted["amount"])

	// 4. Candidate filtering safety:
	// A query on big_int = MaxInt64 - 1 falls back to row evaluation for exact verification,
	// ensuring exact results even with custom strategy present.
	q := dal.From(dal.NewRootCollectionRef("records", "")).NewQuery().
		Where(dal.WhereField("big_int", dal.Equal, int64(math.MaxInt64-1))).
		SelectColumns(dal.Column{Expression: dal.Field("big_int")})

	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	recs, err := dal.ReadAllToRecords(ctx, reader)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, "rec2", recs[0].Key().ID)
}
