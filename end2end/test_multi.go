package end2end

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

func deleteAllRecords(ctx context.Context, t *testing.T, db dal.DB, keys []*record.Key) {
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.DeleteMulti(ctx, keys)
	}, dal.TxWithMessage("deleteAllRecords"))
	require.NoErrorf(t, err, "DeleteMulti(ctx, keys) for %v records", len(keys))
}

func multiOperationsTest(ctx context.Context, t *testing.T, db dal.DB) {

	var k1r1Key = record.NewKeyWithID(E2ETestKind1, "k1r1")
	var k1r2Key = record.NewKeyWithID(E2ETestKind1, "k1r2")
	var k2r1Key = record.NewKeyWithID(E2ETestKind2, "k2r1")

	var allKeys = []*record.Key{
		k1r1Key,
		k1r2Key,
		k2r1Key,
	}

	t.Run("1st_initial_delete", func(t *testing.T) {
		deleteAllRecords(ctx, t, db, allKeys)
	})
	t.Run("2nd_initial_delete", func(t *testing.T) {
		deleteAllRecords(ctx, t, db, allKeys)
	})
	t.Run("get_3_non_existing_records", func(t *testing.T) {
		get3NonExistingRecords(t, db)
	})
	t.Run("SetMulti", func(t *testing.T) {
		setMulti(t, db, k1r1Key, k1r2Key, k2r1Key)
	})
	t.Run("GetMulti", func(t *testing.T) {
		t.Run("3_existing_records", func(t *testing.T) {
			getMulti3existingRecords(t, allKeys, db)
		})
		t.Run("2_existing_2_missing_records", func(t *testing.T) {
			getMulti2existing2missingRecords(t, db, k1r1Key, k1r2Key)
		})
	})
	t.Run("update_2_records", func(t *testing.T) {
		update2records(t, db, k1r1Key, k1r2Key, k2r1Key)
	})
	t.Run("cleanup_delete", func(t *testing.T) {
		cleanupDelete(t, db, allKeys)
	})
}

func getMulti2existing2missingRecords(t *testing.T, db dal.DB, k1r1Key, k1r2Key *record.Key) {
	keys := []*record.Key{
		k1r1Key,
		k1r2Key,
		record.NewKeyWithID(E2ETestKind1, "k1r9"),
		record.NewKeyWithID(E2ETestKind2, "k2r9"),
	}
	data := make([]TestData, len(keys))
	records := make([]record.Record, len(keys))
	for i, key := range keys {
		records[i] = record.NewRecordWithData(key, &data[i])
	}
	ctx := context.Background()
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.GetMulti(ctx, records)
	}, dal.TxWithMessage("getMulti2existing2missingRecords")))
	recordsMustExist(t, records[:2])
	recordsMustNotExist(t, records[2:])
	require.Equal(t, "k1r1str", data[0].StringProp)
	require.Equal(t, "k1r2str", data[1].StringProp)
	for i := 2; i < 4; i++ {
		require.False(t, records[i].Exists(), "record unexpectedly showing as existing, key: %v", records[i].Key())
	}
}

func get3NonExistingRecords(t *testing.T, db dal.DB) {
	records := make([]record.Record, 3)
	for i := 0; i < 3; i++ {
		records[i] = record.NewRecordWithData(
			record.NewKeyWithID("NonExistingKind", fmt.Sprintf("non_existing_id_%v", i)),
			&TestData{},
		)
	}
	ctx := context.Background()

	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.GetMulti(ctx, records)
	}, dal.TxWithMessage("get3NonExistingRecords")))
	recordsMustNotExist(t, records)

}
func setMulti(t *testing.T, db dal.DB, k1r1Key, k1r2Key, k2r1Key *record.Key) {
	newRecord := func(key *record.Key) record.Record {
		return record.NewRecordWithData(key, &TestData{
			StringProp: fmt.Sprintf("%vstr", key.ID),
		})
	}
	records := []record.Record{
		newRecord(k1r1Key),
		newRecord(k1r2Key),
		newRecord(k2r1Key),
	}
	ctx := context.Background()
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.SetMulti(ctx, records)
	}, dal.TxWithMessage("setMulti"))
	require.NoError(t, err)
}

func cleanupDelete(t *testing.T, db dal.DB, allKeys []*record.Key) {
	ctx := context.Background()
	deleteAllRecords(ctx, t, db, allKeys)
	data := make([]struct{}, len(allKeys))
	records := make([]record.Record, len(allKeys))
	for i := range records {
		records[i] = record.NewRecordWithData(allKeys[i], &data[i])
	}
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.GetMulti(ctx, records)
	}, dal.TxWithMessage("verify_cleanupDelete")))
	recordsMustNotExist(t, records)
}

func update2records(t *testing.T, db dal.DB, k1r1Key, k1r2Key, k2r1Key *record.Key) {
	const newValue = "UpdateD"
	updates := []update.Update{
		update.ByFieldName("StringProp", newValue),
	}
	newRecords := func() []record.Record {
		data := make([]*TestData, 3)
		for i := range data {
			data[i] = new(TestData)
		}
		return []record.Record{
			record.NewRecordWithData(k1r1Key, data[0]),
			record.NewRecordWithData(k1r2Key, data[1]),
			record.NewRecordWithData(k2r1Key, data[2]),
		}
	}
	ctx := context.Background()
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.UpdateMulti(ctx, []*record.Key{k1r1Key, k1r2Key}, updates)
	}, dal.TxWithMessage("update2records"))
	if errors.Is(err, dal.ErrNotSupported) {
		t.Log(err)
		return
	}
	require.NoError(t, err)
	records := newRecords()
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.GetMulti(ctx, records)
	}, dal.TxWithMessage("getMultiNewRecords")))
	recordsMustExist(t, records)
	asserRecord := func(i int, expected string) {
		require.Equal(t, expected, records[i].Data().(*TestData).StringProp, "record #%d key: %v", i+1, records[i].Key())
	}
	asserRecord(0, newValue)
	asserRecord(1, newValue)
	asserRecord(2, "k2r1str")
}

func recordsMustExist(t *testing.T, records []record.Record) (missingCount int) {
	for _, record := range records {
		require.NoError(t, record.Error())
		require.True(t, record.Exists(), "record was expected to exist, key: %v", record.Key())
	}
	return 0
}

func recordsMustNotExist(t *testing.T, records []record.Record) (hasError bool) {
	t.Helper()
	for _, record := range records {
		require.NoError(t, record.Error())
		require.False(t, record.Exists(), "record unexpectedly exists, key: %v", record.Key())
	}
	return false
}

func getMulti3existingRecords(t *testing.T, allKeys []*record.Key, db dal.DB) {
	var data []TestData
	records := make([]record.Record, len(allKeys))
	assetProps := func(t *testing.T) {
		//if recordsMustExist(t, records) > 0 {
		//	return
		//}
		assertStringProp := func(i int, record record.Record) {
			id := record.Key().ID.(string)
			require.Equal(t, id+"str", data[i].StringProp)
		}
		for i, record := range records {
			assertStringProp(i, record)
		}
	}
	t.Run("using_records_with_data", func(t *testing.T) {
		data = make([]TestData, len(allKeys))
		for i := range records {
			records[i] = record.NewRecordWithData(allKeys[i], &data[i])
		}
		ctx := context.Background()
		require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			return tx.GetMulti(ctx, records)
		}, dal.TxWithMessage("using_records_with_data")))
		recordsMustExist(t, records)
		assetProps(t)
	})
	//t.Run("using_DataTo", func(t *testing.T) {
	//	for i := range records {
	//		records[i] = record.NewRecord(allKeys[i])
	//	}
	//	if err := db.GetMulti(ctx, records); err != nil {
	//		t.Fatalf("failed to get multiple records at once: %v", err)
	//	}
	//	recordsMustExist(t, records)
	//	data = make([]TestData, len(allKeys))
	//	for i, record := range records {
	//		if err := record.DataTo(&data[i]); err != nil {
	//			t.Fatalf("failed to record #%v", i+1)
	//		}
	//	}
	//	assetProps(t)
	//})
}
