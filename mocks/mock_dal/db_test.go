package mock_dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewMockDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	assert.NotNil(t, mockDB)
	assert.NotNil(t, mockDB.EXPECT())
}

func TestMockDB_Adapter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)

	mockDB.EXPECT().Adapter().Return(nil)

	result := mockDB.Adapter()
	assert.Nil(t, result)
}

func TestMockDB_ID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	expectedID := "test-db-id"

	mockDB.EXPECT().ID().Return(expectedID)

	result := mockDB.ID()
	assert.Equal(t, expectedID, result)
}

func TestMockDB_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()
	key := &dal.Key{}

	t.Run("exists returns true", func(t *testing.T) {
		mockDB.EXPECT().Exists(ctx, key).Return(true, nil)

		exists, err := mockDB.Exists(ctx, key)
		assert.True(t, exists)
		assert.NoError(t, err)
	})

	t.Run("exists returns false", func(t *testing.T) {
		mockDB.EXPECT().Exists(ctx, key).Return(false, nil)

		exists, err := mockDB.Exists(ctx, key)
		assert.False(t, exists)
		assert.NoError(t, err)
	})

	t.Run("exists returns error", func(t *testing.T) {
		expectedErr := errors.New("exists error")
		mockDB.EXPECT().Exists(ctx, key).Return(false, expectedErr)

		exists, err := mockDB.Exists(ctx, key)
		assert.False(t, exists)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockDB_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()

	t.Run("get success", func(t *testing.T) {
		mockDB.EXPECT().Get(ctx, gomock.Any()).Return(nil)

		err := mockDB.Get(ctx, NewMockRecord(ctrl))
		assert.NoError(t, err)
	})

	t.Run("get error", func(t *testing.T) {
		expectedErr := errors.New("get error")
		mockDB.EXPECT().Get(ctx, gomock.Any()).Return(expectedErr)

		err := mockDB.Get(ctx, NewMockRecord(ctrl))
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockDB_GetMulti(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()
	records := []dal.Record{NewMockRecord(ctrl), NewMockRecord(ctrl)}

	t.Run("get multi success", func(t *testing.T) {
		mockDB.EXPECT().GetMulti(ctx, gomock.Any()).Return(nil)

		err := mockDB.GetMulti(ctx, records)
		assert.NoError(t, err)
	})

	t.Run("get multi error", func(t *testing.T) {
		expectedErr := errors.New("get multi error")
		mockDB.EXPECT().GetMulti(ctx, gomock.Any()).Return(expectedErr)

		err := mockDB.GetMulti(ctx, records)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockDB_QueryReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()

	t.Run("query reader success", func(t *testing.T) {
		mockDB.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, nil)

		reader, err := mockDB.GetRecordsReader(ctx, nil)
		assert.NoError(t, err)
		assert.Nil(t, reader)
	})

	t.Run("query reader error", func(t *testing.T) {
		expectedErr := errors.New("query reader error")
		mockDB.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, expectedErr)

		reader, err := mockDB.GetRecordsReader(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockDB_RunReadonlyTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadTransaction) error { return nil }

	t.Run("readonly transaction success", func(t *testing.T) {
		mockDB.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(nil)

		err := mockDB.RunReadonlyTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction with options", func(t *testing.T) {
		mockDB.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		opts := []dal.TransactionOption{dal.TransactionOption(nil)}
		err := mockDB.RunReadonlyTransaction(ctx, txFunc, opts...)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockDB.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockDB.RunReadonlyTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Also cover the varargs branch by passing a typed nil TransactionOption
	var opt dal.TransactionOption = nil
	t.Run("readonly transaction with option", func(t *testing.T) {
		mockDB.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockDB.RunReadonlyTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}

func TestMockDB_RunReadwriteTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := NewMockDB(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadwriteTransaction) error { return nil }

	t.Run("readwrite transaction success", func(t *testing.T) {
		mockDB.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(nil)

		err := mockDB.RunReadwriteTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readwrite transaction with options", func(t *testing.T) {
		mockDB.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		opts := []dal.TransactionOption{dal.TransactionOption(nil)}
		err := mockDB.RunReadwriteTransaction(ctx, txFunc, opts...)
		assert.NoError(t, err)
	})

	t.Run("readwrite transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockDB.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockDB.RunReadwriteTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Cover varargs branch as well
	var opt dal.TransactionOption = nil
	t.Run("readwrite transaction with option", func(t *testing.T) {
		mockDB.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockDB.RunReadwriteTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}
