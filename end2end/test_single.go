package end2end

import (
	"context"
	"github.com/strongo/dalgo/dal"
	"testing"
)

func assertRecordDoesNotExists(t *testing.T, record dal.Record, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected to get 'record not found' error, got nil")
	} else if !dal.IsNotFound(err) {
		t.Errorf("expected to get 'record not found' error, got: %v", err)
	}
	if record.Exists() {
		t.Error("expected record to not exist, but Exists() returned true")
	}
}

func testSingleOperations(ctx context.Context, t *testing.T, db dal.Database) {
	t.Run("single", func(t *testing.T) {
		const id = "r0"
		key := dal.NewKeyWithID(E2ETestKind1, id)
		t.Run("get", func(t *testing.T) {
			t.Run("db", func(t *testing.T) {
				var data TestData
				record := dal.NewRecordWithData(key, &data)
				err := db.Get(ctx, record)
				assertRecordDoesNotExists(t, record, err)
			})
			t.Run("readonly_transaction", func(t *testing.T) {
				data := TestData{}
				record := dal.NewRecordWithData(key, &data)
				err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
					return tx.Get(ctx, record)
				})
				assertRecordDoesNotExists(t, record, err)
			})
			t.Run("readwrite_transaction", func(t *testing.T) {
				data := TestData{}
				record := dal.NewRecordWithData(key, &data)
				err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					return tx.Get(ctx, record)
				})
				assertRecordDoesNotExists(t, record, err)
			})
		})
		t.Run("transaction", func(t *testing.T) {
			t.Run("delete_non_existing", func(t *testing.T) {
				if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					return tx.Delete(ctx, key)
				}); err != nil {
					t.Errorf("Failed to delete: %v", err)
				}
			})
			t.Run("create", func(t *testing.T) {
				t.Run("with_predefined_id", func(t *testing.T) {
					data := TestData{
						StringProp:  "str1",
						IntegerProp: 1,
					}
					record := dal.NewRecordWithData(key, &data)
					if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
						return tx.Insert(ctx, record)
					}); err != nil {
						t.Errorf("Failed to insert: %v", err)
					}
				})
			})
			t.Run("delete_created", func(t *testing.T) {
				t.Run("with_predefined_id", func(t *testing.T) {
					if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
						return tx.Delete(ctx, key)
					}); err != nil {
						t.Errorf("Failed to delete: %v", err)
					}
				})
			})
		})
	})
}
