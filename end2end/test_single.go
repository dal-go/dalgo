package end2end

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func singleOperationsTest(ctx context.Context, t *testing.T, db dal.DB) {
	t.Run("single", func(t *testing.T) {
		const id = "r0"
		key := dal.NewKeyWithID(E2ETestKind1, id)
		t.Run("delete1", func(t *testing.T) {
			singleDeleteTest(t, db, key)
		})
		t.Run("get1", func(t *testing.T) {
			singleGetTest(ctx, t, db, key, false)
		})
		t.Run("exists1", func(t *testing.T) {
			singleExistsTest(ctx, t, db, key, false)
		})
		t.Run("create", func(t *testing.T) {
			t.Run("with_predefined_id", func(t *testing.T) {
				singleCreateWithPredefinedIDTest(ctx, t, db, key)
			})
		})
		t.Run("exists2", func(t *testing.T) {
			singleExistsTest(ctx, t, db, key, true)
		})
		t.Run("get2", func(t *testing.T) {
			singleGetTest(ctx, t, db, key, true)
		})
		t.Run("delete2", func(t *testing.T) {
			singleDeleteTest(t, db, key)
		})
		t.Run("exists3", func(t *testing.T) {
			singleExistsTest(ctx, t, db, key, false)
		})
	})
}

func singleDeleteTest(t *testing.T, db dal.DB, key *dal.Key) {
	ctx := context.Background()
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Delete(ctx, key)
	}, dal.TxWithName("singleDeleteTest"))
	require.NoError(t, err)
}

func singleExistsTest(ctx context.Context, t *testing.T, db dal.DB, key *dal.Key, expectedToExist bool) {
	exists, err := db.Exists(ctx, key)
	require.NoError(t, err)
	require.Equal(t, expectedToExist, exists)
}

func singleGetTest(ctx context.Context, t *testing.T, db dal.DB, key *dal.Key, mustExists bool) {
	var data = new(TestData)
	record := dal.NewRecordWithData(key, data)
	err := db.Get(ctx, record)
	if dal.IsNotFound(err) {
		require.False(t, mustExists, "record expected to exist but received error: %v", err)
		return
	}
	require.NoError(t, err)
	require.NotEmpty(t, data.StringProp)
	require.NotZero(t, data.IntegerProp)
	require.True(t, mustExists, "record unexpectedly found")
}

func singleCreateWithPredefinedIDTest(ctx context.Context, t *testing.T, db dal.DB, key *dal.Key) {
	data := TestData{
		StringProp:  "str1",
		IntegerProp: 1,
	}
	record := dal.NewRecordWithData(key, &data)
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record)
	}, dal.TxWithName("singleCreateWithPredefinedIDTest"))
	require.NoError(t, err)
}
