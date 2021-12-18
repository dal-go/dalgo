package end2end

import (
	"context"
	"fmt"
	"github.com/strongo/dalgo/dal"
	"testing"
)

func testMultiOperations(ctx context.Context, t *testing.T, db dal.Database) {
	k1r1Key := dal.NewKeyWithStrID(E2ETestKind1, "k1r1")
	k1r2Key := dal.NewKeyWithStrID(E2ETestKind1, "k1r2")
	k2r1Key := dal.NewKeyWithStrID(E2ETestKind2, "k2r1")
	allKeys := []*dal.Key{k1r1Key, k1r2Key, k2r1Key}

	deleteAllRecords := func(ctx context.Context, t *testing.T, db dal.Database, keys []*dal.Key) {
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.DeleteMulti(ctx, keys)
		})
		if err != nil {
			t.Fatalf("failed at DeleteMulti(ctx, keys) for %v records: %v", len(keys), err)
		}
	}
	t.Run("1st_initial_delete", func(t *testing.T) {
		deleteAllRecords(ctx, t, db, allKeys)
	})
	t.Run("2nd_initial_delete", func(t *testing.T) {
		deleteAllRecords(ctx, t, db, allKeys)
	})
	t.Run("get_3_non_existing_records", func(t *testing.T) {
		records := make([]dal.Record, 3)
		for i := 0; i < 3; i++ {
			records[i] = dal.NewRecordWithData(
				dal.NewKey("NonExistingKind", dal.WithStringID(fmt.Sprintf("non_existing_id_%v", i))),
				&TestData{},
			)
		}
		if err := db.GetMulti(ctx, records); err != nil {
			t.Fatalf("failed to get multiple records at once: %v", err)
		}
		recordsMustNotExist(t, records)
	})
	t.Run("SetMulti", func(t *testing.T) {
		newRecord := func(key *dal.Key) dal.Record {
			return dal.NewRecordWithData(key, TestData{
				StringProp: fmt.Sprintf("%vstr", key.ID),
			})
		}
		records := []dal.Record{
			newRecord(k1r1Key),
			newRecord(k1r2Key),
			newRecord(k2r1Key),
		}
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.SetMulti(ctx, records)
		})
		if err != nil {
			t.Fatalf("failed to set multiple records at once: %v", err)
		}
	})
	t.Run("GetMulti_3_existing_records", func(t *testing.T) {
		var data []TestData
		records := make([]dal.Record, len(allKeys))
		assetProps := func(t *testing.T) {
			recordsMustExist(t, records)
			assertStringProp := func(i int, record dal.Record) {
				id := record.Key().ID.(string)
				if expected, actual := id+"str", data[i].StringProp; actual != expected {
					t.Errorf("field StringProp was expected to have value '%v' got '%v'", expected, actual)
				}
			}
			for i, record := range records {
				assertStringProp(i, record)
			}
		}
		t.Run("using_records_with_data", func(t *testing.T) {
			data = make([]TestData, len(allKeys))
			for i := range records {
				records[i] = dal.NewRecordWithData(allKeys[i], &data[i])
			}
			if err := db.GetMulti(ctx, records); err != nil {
				t.Fatalf("failed to get multiple records at once: %v", err)
			}
			assetProps(t)
		})
		//t.Run("using_DataTo", func(t *testing.T) {
		//	for i := range records {
		//		records[i] = dal.NewRecord(allKeys[i])
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
	})
	t.Run("GetMulti_2_existing_2_missing_records", func(t *testing.T) {
		keys := []*dal.Key{
			k1r1Key,
			k1r2Key,
			dal.NewKeyWithStrID(E2ETestKind1, "k1r9"),
			dal.NewKeyWithStrID(E2ETestKind2, "k2r9"),
		}
		data := make([]TestData, len(keys))
		records := make([]dal.Record, len(keys))
		for i, key := range keys {
			records[i] = dal.NewRecordWithData(key, &data[i])
		}
		if err := db.GetMulti(ctx, records); err != nil {
			t.Fatalf("failed to set multiple records at once: %v", err)
		}
		recordsMustExist(t, records[:2])
		recordsMustNotExist(t, records[2:])
		checkPropValue := func(i int, expected string) error {
			if data[i].StringProp != expected {
				t.Errorf("expected %v got %v, err: %v", expected, data[i].StringProp, records[i].Error())
			}
			return nil
		}
		if err := checkPropValue(0, "k1r1str"); err != nil {
			t.Error(err)
		}
		if err := checkPropValue(1, "k1r2str"); err != nil {
			t.Error(err)
		}
		for i := 2; i < 4; i++ {
			if records[i].Exists() {
				t.Errorf("record unexpectedly showing as existing, key: %v", records[i].Key())
			}
		}
	})
	t.Run("update_2_records", func(t *testing.T) {
		data := make([]TestData, 3)
		const newValue = "UpdateD"
		updates := []dal.Update{
			{Field: "StringProp", Value: newValue},
		}
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.UpdateMulti(ctx, []*dal.Key{k1r1Key, k1r2Key}, updates)
		})
		if err != nil {
			t.Fatalf("failed to update 2 records at once: %v", err)
		}
		records := []dal.Record{
			dal.NewRecordWithData(k1r1Key, &data[0]),
			dal.NewRecordWithData(k1r2Key, &data[1]),
			dal.NewRecordWithData(k2r1Key, &data[2]),
		}
		if err := db.GetMulti(ctx, records); err != nil {
			t.Fatalf("failed to get 3 records at once: %v", err)
		}
		recordsMustExist(t, records)
		if actual := data[0].StringProp; actual != newValue {
			t.Errorf("record expected to have StringProp as '%v' but got '%v', key: %v", newValue, actual, records[0].Key())
		}
		if actual := data[1].StringProp; actual != newValue {
			t.Errorf("record expected to have StringProp as '%v' but got '%v', key: %v", newValue, actual, records[1].Key())
		}
		if actual := data[2].StringProp; actual != "k2r1str" {
			t.Errorf("record expected to have StringProp as '%v' but got '%v', key: %v", newValue, actual, records[2].Key())
		}
	})
	t.Run("cleanup_delete", func(t *testing.T) {
		deleteAllRecords(ctx, t, db, allKeys)
		data := make([]struct{}, len(allKeys))
		records := make([]dal.Record, len(allKeys))
		for i := range records {
			records[i] = dal.NewRecordWithData(allKeys[i], &data[i])
		}
		if err := db.GetMulti(ctx, records); err != nil {
			t.Fatalf("failed to get multiple records at once: %v", err)
		}
		recordsMustNotExist(t, records)
	})
}

func recordsMustExist(t *testing.T, records []dal.Record) {
	t.Helper()
	for _, record := range records {
		if err := record.Error(); err != nil {
			t.Errorf("not able to check record for existence as it has unexpected error: %v", err)
		}
		if !record.Exists() {
			t.Errorf("record was expected to exist, key: %v", record.Key())
		}
	}
}

func recordsMustNotExist(t *testing.T, records []dal.Record) {
	t.Helper()
	for i, record := range records {
		if err := record.Error(); err != nil {
			t.Errorf("record with key=[%v] has unexpected error: %v", record.Key(), err)
		} else if record.Exists() {
			t.Errorf("for record #%v of %v Exists() returned true, but expected false; key: %v",
				i+1, len(records), record.Key())
		}
	}
}
