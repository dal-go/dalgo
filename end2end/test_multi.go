package end2end

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/strongo/dalgo/dal"
	"sync"
	"testing"
)

func testMultiOperations(ctx context.Context, t *testing.T, db dal.Database) {
	k1r1Key := dal.NewKeyWithID(E2ETestKind1, "k1r1")
	k1r2Key := dal.NewKeyWithID(E2ETestKind1, "k1r2")
	k2r1Key := dal.NewKeyWithID(E2ETestKind2, "k2r1")
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
			t.Fatalf("failed to get 3 non exising records at once: %v", err)
		}
		assertRecordsMustNotExist(t, records)
	})
	recordsCreated := false
	testsStarted := 0
	testsDone := 0
	var m sync.Mutex

	started := func() {
		//m.Lock()
		testsStarted++
		//m.Unlock()
	}

	done := func() {
		m.Lock()
		testsDone++
		m.Unlock()
	}

	started()
	t.Run("CRUD_3_records", func(t *testing.T) {
		defer done()
		newRecord := func(key *dal.Key, value string) dal.Record {
			return dal.NewRecordWithData(key, &TestData{
				StringProp: value,
			})
		}
		t.Run("create_3_records", func(t *testing.T) {
			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				records := []dal.Record{
					newRecord(k1r1Key, "v1"),
					newRecord(k1r2Key, "v2"),
					newRecord(k2r1Key, "v3"),
				}
				if err := tx.SetMulti(ctx, records); err != nil {
					return fmt.Errorf("failed to set 3 records at once: %w", err)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set 3 records at once: %v", err)
			}
		})
		t.Run("get_3_existing_records", func(t *testing.T) {
			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				records := []dal.Record{
					newRecord(k1r1Key, ""),
					newRecord(k1r2Key, ""),
					newRecord(k2r1Key, ""),
				}
				if err := tx.GetMulti(ctx, records); err != nil {
					return fmt.Errorf("failed to get 3 records at once: %w", err)
				}
				assertRecordsMustExist(t, records)
				for i, v := range []string{"v1", "v2", "v3"} {
					assert.Equal(t, v, records[i].Data().(*TestData).StringProp,
						"record expected to load stored value")
				}
				return nil
			})
			if err != nil {
				t.Fatalf("failed to get 3 existing records at once: %v", err)
			}
		})
		recordsCreated = true
		t.Run("update_2_existing_records", func(t *testing.T) {
			if !recordsCreated {
				t.Fatal("records must be created first")
			}
			defer done()
			const newValue = "UpdateD"
			updates := []dal.Update{
				{Field: "StringProp", Value: newValue},
			}
			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tx.UpdateMulti(ctx, []*dal.Key{k1r1Key, k1r2Key}, updates)
			})
			if err != nil {
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skipf("skipping test as UpdateMulti is not supported: %v", err)
					return
				}
				t.Fatalf("failed to update 2 records at once: %v", err)
			}
			records := []dal.Record{
				newRecord(k1r1Key, ""),
				newRecord(k1r2Key, ""),
				newRecord(k2r1Key, ""),
			}
			if err := db.GetMulti(ctx, records); err != nil {
				t.Fatalf("failed to get 3 records at once: %v", err)
			}
			assertRecordsMustExist(t, records)
			for i, record := range records {
				assert.Equal(t, newValue, record.Data().(*TestData).StringProp, fmt.Sprintf("records[%d]: expected to have updated value", i))
			}
			testsDone += 1
		})
		t.Run("GetMulti_2_existing_2_missing_records", func(t *testing.T) {
			if !recordsCreated {
				t.Fatal("records must be created first")
			}
			defer done()
			keys := []*dal.Key{
				k1r1Key,
				k1r2Key,
				dal.NewKeyWithID(E2ETestKind1, "non_existing_1"),
				dal.NewKeyWithID(E2ETestKind2, "non_existing_2"),
			}
			records := make([]dal.Record, len(keys))
			for i, key := range keys {
				records[i] = newRecord(key, "")
			}
			if err := db.GetMulti(ctx, records); err != nil {
				t.Fatalf("failed to set multiple records at once: %v", err)
			}
			assertRecordsMustExist(t, records[:2])
			assertRecordsMustNotExist(t, records[2:])
			assertStringPropValue := func(record dal.Record, expected string) {
				if stringProp := record.Data().(*TestData).StringProp; stringProp != expected {
					t.Errorf("expected %v got %v, err: %v", expected, stringProp, record.Error())
				}
			}
			assertStringPropValue(records[0], "v1")
			assertStringPropValue(records[1], "v2")
		})
	})

	//t.Run("cleanup_delete", func(t *testing.T) {
	//	for {
	//		if testsDone < testsStarted {
	//			time.Sleep(time.Second)
	//		}
	//	}
	//	deleteAllRecords(ctx, t, db, allKeys)
	//	data := make([]struct{}, len(allKeys))
	//	records := make([]dal.Record, len(allKeys))
	//	for i := range records {
	//		records[i] = dal.NewRecordWithData(allKeys[i], &data[i])
	//	}
	//	if err := db.GetMulti(ctx, records); err != nil {
	//		t.Fatalf("failed to get multiple records at once: %v", err)
	//	}
	//	assertRecordsMustNotExist(t, records)
	//})

}
