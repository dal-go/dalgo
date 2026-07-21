package end2end

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/mocks/mock_dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestErrorReturnBranches(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	t.Run("update_not_supported", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Return(dal.ErrNotSupported)
		update2records(t, db,
			record.NewKeyWithID(E2ETestKind1, "k1"),
			record.NewKeyWithID(E2ETestKind1, "k2"),
			record.NewKeyWithID(E2ETestKind2, "k3"),
		)
	})

	t.Run("delete_all_cities_query_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
				tx := mock_dal.NewMockReadwriteTransaction(ctrl)
				tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).Return(nil, boom)
				return f(ctx, tx)
			},
		)
		require.ErrorContains(t, deleteAllCities(ctx, db), "failed to query all cities")
	})

	t.Run("delete_all_cities_reader_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
				tx := mock_dal.NewMockReadwriteTransaction(ctrl)
				tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).Return(dal.NewRecordsReader(nil), nil)
				return f(ctx, tx)
			},
		)
		require.ErrorContains(t, deleteAllCities(ctx, db), "failed to query all cities")
	})

	t.Run("delete_all_cities_delete_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
				tx := mock_dal.NewMockReadwriteTransaction(ctrl)
				tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).Return(dal.NewRecordsReader([]record.Record{
					record.NewRecord(record.NewKeyWithID("Cities", "one")).SetError(nil),
				}), nil)
				tx.EXPECT().DeleteMulti(gomock.Any(), gomock.Any()).Return(boom)
				return f(ctx, tx)
			},
		)
		require.ErrorContains(t, deleteAllCities(ctx, db), "failed to delete all cities")
	})

	t.Run("delete_all_cities_transaction_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Return(boom)
		require.ErrorContains(t, deleteAllCities(ctx, db), "failed to delete all cities")
	})

	t.Run("setup_data_propagates_delete_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Return(boom)
		require.Error(t, setupDataForQueryTests(ctx, db))
	})

	t.Run("select_all_cities_query_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		db.EXPECT().RunReadonlyTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, f dal.ROTxWorker, _ ...dal.TransactionOption) error {
				tx := mock_dal.NewMockReadTransaction(ctrl)
				tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).Return(nil, boom)
				return f(ctx, tx)
			},
		)
		records, err := selectAllCities(ctx, db)
		require.Nil(t, records)
		require.Error(t, err)
	})

	t.Run("select_all_cities_into_record", func(t *testing.T) {
		db := dalgo2memory.NewDB()
		require.NoError(t, setupDataForQueryTests(ctx, db))
		records, err := selectAllCities(ctx, db)
		require.NoError(t, err)
		require.NotEmpty(t, records)
	})
}
