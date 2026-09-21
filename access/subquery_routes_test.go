package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func TestSecuredRecursiveRoutesDenyNestedLeaf(t *testing.T) {
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for collection, data := range map[string]map[string]any{
			"Customer": {"id": 1}, "Invoice": {"customer": 1},
		} {
			if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID(collection, "1"), data)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	secured := MustSecureDB(raw, WithDatabasePolicies(MustPolicy("nested",
		Collection("Customer", Allow(Query, "read")), Collection("Invoice", Deny(Query, "deny")),
	)))
	inner := dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	query := dal.From(dal.NewRootCollectionRef("Customer", "c")).NewQuery().Where(dal.NewExistsCondition(inner)).SelectIntoRecord(nil)
	if _, err := secured.ExecuteQueryToRecordsReader(ctx, query); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("records nested denial = %v", err)
	}
	if _, err := secured.ExecuteQueryToRecordsetReader(ctx, query); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("recordset nested denial = %v", err)
	}
	if _, err := secured.(interface {
		Select(context.Context, dal.Query) (dal.Reader, error)
	}).Select(ctx, query); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("DB Select nested denial = %v", err)
	}
	if err := secured.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		_, err := tx.(interface {
			Select(context.Context, dal.Query) (dal.Reader, error)
		}).Select(ctx, query)
		return err
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("readonly Select nested denial = %v", err)
	}
	if err := secured.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, err := tx.(interface {
			Select(context.Context, dal.Query) (dal.Reader, error)
		}).Select(ctx, query)
		return err
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("readwrite Select nested denial = %v", err)
	}
}
