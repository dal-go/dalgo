package dalgo2memory_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

type fact struct {
	V string
}

func wrapCollection[T any](name string, newRecord func() *T, opts ...dalgo2memory.CollectionOption) dalgo2memory.CollectionDef {
	return dalgo2memory.WithCollection[T](name, newRecord, opts...)
}

func wrapSchema(allowUndefined bool, collections ...dalgo2memory.CollectionDef) dalgo2memory.Option {
	return dalgo2memory.WithSchema(allowUndefined, collections...)
}

func TestExternalCollectionDefWrapper(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile(), wrapSchema(false, wrapCollection[fact]("c", nil)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("c", "1"), &fact{V: "ok"}))
	})
	require.NoError(t, err)

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("undeclared", "1"), &fact{V: "nope"}))
	})
	require.Error(t, err)
}
