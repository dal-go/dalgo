package mock_dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewMockTransactionCoordinator(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockTransactionCoordinator(ctrl)
	assert.NotNil(t, mockCoordinator)
	assert.NotNil(t, mockCoordinator.EXPECT())
}

func TestMockTransactionCoordinator_RunReadonlyTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockTransactionCoordinator(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadTransaction) error { return nil }

	t.Run("readonly transaction success", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(nil)

		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction with options", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		opts := []dal.TransactionOption{dal.TransactionOption(nil)}
		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc, opts...)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Cover varargs branch by passing a typed nil TransactionOption
	var opt dal.TransactionOption = nil
	t.Run("readonly transaction with option", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}

func TestMockTransactionCoordinator_RunReadwriteTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockTransactionCoordinator(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadwriteTransaction) error { return nil }

	t.Run("readwrite transaction success", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(nil)

		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readwrite transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Cover varargs branch with option
	var opt dal.TransactionOption = nil
	t.Run("readwrite transaction with option", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}

func TestNewMockReadTransactionCoordinator(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockReadTransactionCoordinator(ctrl)
	assert.NotNil(t, mockCoordinator)
	assert.NotNil(t, mockCoordinator.EXPECT())
}

func TestMockReadTransactionCoordinator_RunReadonlyTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockReadTransactionCoordinator(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadTransaction) error { return nil }

	t.Run("readonly transaction success", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(nil)

		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction with options", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		opts := []dal.TransactionOption{dal.TransactionOption(nil)}
		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc, opts...)
		assert.NoError(t, err)
	})

	t.Run("readonly transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Cover varargs branch by passing option
	var opt dal.TransactionOption = nil
	t.Run("readonly transaction with option", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadonlyTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockCoordinator.RunReadonlyTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}

func TestNewMockReadwriteTransactionCoordinator(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockReadwriteTransactionCoordinator(ctrl)
	assert.NotNil(t, mockCoordinator)
	assert.NotNil(t, mockCoordinator.EXPECT())
}

func TestMockReadwriteTransactionCoordinator_RunReadwriteTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockReadwriteTransactionCoordinator(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadwriteTransaction) error { return nil }

	t.Run("readwrite transaction success", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(nil)

		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc)
		assert.NoError(t, err)
	})

	t.Run("readwrite transaction with options", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		opts := []dal.TransactionOption{dal.TransactionOption(nil)}
		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc, opts...)
		assert.NoError(t, err)
	})

	t.Run("readwrite transaction error", func(t *testing.T) {
		expectedErr := errors.New("transaction error")
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any()).Return(expectedErr)

		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Cover varargs branch by passing option
	var opt dal.TransactionOption = nil
	t.Run("readwrite transaction with option", func(t *testing.T) {
		mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc, opt)
		assert.NoError(t, err)
	})
}

func TestMockTransactionCoordinator_RunReadwriteTransaction_WithOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCoordinator := NewMockTransactionCoordinator(ctrl)
	ctx := context.Background()
	txFunc := func(context.Context, dal.ReadwriteTransaction) error { return nil }

	mockCoordinator.EXPECT().RunReadwriteTransaction(ctx, gomock.Any(), gomock.Any()).Return(nil)
	opts := []dal.TransactionOption{dal.TransactionOption(nil)}
	err := mockCoordinator.RunReadwriteTransaction(ctx, txFunc, opts...)
	assert.NoError(t, err)
}
