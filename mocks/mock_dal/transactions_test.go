package mock_dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewMockTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockTransaction(ctrl)
	assert.NotNil(t, mockTx)
	assert.NotNil(t, mockTx.EXPECT())
}

func TestMockTransaction_Options(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockTransaction(ctrl)

	mockTx.EXPECT().Options().Return(nil)

	options := mockTx.Options()
	assert.Nil(t, options)
}

func TestNewMockReadTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)
	assert.NotNil(t, mockTx)
	assert.NotNil(t, mockTx.EXPECT())
}

func TestMockReadTransaction_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)
	ctx := context.Background()
	key := &dal.Key{}

	t.Run("exists returns true", func(t *testing.T) {
		mockTx.EXPECT().Exists(ctx, key).Return(true, nil)

		exists, err := mockTx.Exists(ctx, key)
		assert.True(t, exists)
		assert.NoError(t, err)
	})

	t.Run("exists returns error", func(t *testing.T) {
		expectedErr := errors.New("exists error")
		mockTx.EXPECT().Exists(ctx, key).Return(false, expectedErr)

		exists, err := mockTx.Exists(ctx, key)
		assert.False(t, exists)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadTransaction_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)
	ctx := context.Background()

	t.Run("get success", func(t *testing.T) {
		mockTx.EXPECT().Get(ctx, gomock.Any()).Return(nil)

		err := mockTx.Get(ctx, NewMockRecord(ctrl))
		assert.NoError(t, err)
	})

	t.Run("get error", func(t *testing.T) {
		expectedErr := errors.New("get error")
		mockTx.EXPECT().Get(ctx, gomock.Any()).Return(expectedErr)

		err := mockTx.Get(ctx, NewMockRecord(ctrl))
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadTransaction_Options(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)

	mockTx.EXPECT().Options().Return(nil)

	options := mockTx.Options()
	assert.Nil(t, options)
}

func TestMockReadTransaction_GetMulti(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)
	ctx := context.Background()

	records := []dal.Record{NewMockRecord(ctrl), NewMockRecord(ctrl)}

	t.Run("get multi success", func(t *testing.T) {
		mockTx.EXPECT().GetMulti(ctx, gomock.Any()).Return(nil)
		err := mockTx.GetMulti(ctx, records)
		assert.NoError(t, err)
	})

	t.Run("get multi error", func(t *testing.T) {
		expectedErr := errors.New("get multi error")
		mockTx.EXPECT().GetMulti(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.GetMulti(ctx, records)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadTransaction_QueryMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadTransaction(ctrl)
	ctx := context.Background()

	t.Run("QueryReader success", func(t *testing.T) {
		mockTx.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, nil)
		reader, err := mockTx.ExecuteQueryToRecordsReader(ctx, nil)
		assert.NoError(t, err)
		assert.Nil(t, reader)
	})

	t.Run("QueryReader error", func(t *testing.T) {
		expectedErr := errors.New("query reader error")
		mockTx.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, expectedErr)
		reader, err := mockTx.ExecuteQueryToRecordsReader(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, expectedErr, err)
	})
}

func TestNewMockReadwriteTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	assert.NotNil(t, mockTx)
	assert.NotNil(t, mockTx.EXPECT())
}

func TestMockReadwriteTransaction_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()
	key := &dal.Key{}

	t.Run("delete success", func(t *testing.T) {
		mockTx.EXPECT().Delete(ctx, key).Return(nil)

		err := mockTx.Delete(ctx, key)
		assert.NoError(t, err)
	})

	t.Run("delete error", func(t *testing.T) {
		expectedErr := errors.New("delete error")
		mockTx.EXPECT().Delete(ctx, key).Return(expectedErr)

		err := mockTx.Delete(ctx, key)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("delete multi success", func(t *testing.T) {
		keys := []*dal.Key{key}
		mockTx.EXPECT().DeleteMulti(ctx, gomock.Any()).Return(nil)
		err := mockTx.DeleteMulti(ctx, keys)
		assert.NoError(t, err)
	})

	t.Run("delete multi error", func(t *testing.T) {
		keys := []*dal.Key{key}
		expectedErr := errors.New("delete multi error")
		mockTx.EXPECT().DeleteMulti(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.DeleteMulti(ctx, keys)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()
	key := &dal.Key{}

	t.Run("exists returns true", func(t *testing.T) {
		mockTx.EXPECT().Exists(ctx, key).Return(true, nil)
		exists, err := mockTx.Exists(ctx, key)
		assert.True(t, exists)
		assert.NoError(t, err)
	})

	t.Run("exists returns error", func(t *testing.T) {
		expectedErr := errors.New("exists error")
		mockTx.EXPECT().Exists(ctx, key).Return(false, expectedErr)
		exists, err := mockTx.Exists(ctx, key)
		assert.False(t, exists)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_ID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	expectedID := "test-tx-id"

	mockTx.EXPECT().ID().Return(expectedID)

	id := mockTx.ID()
	assert.Equal(t, expectedID, id)
}

func TestMockReadwriteTransaction_GetAndGetMulti(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()
	records := []dal.Record{NewMockRecord(ctrl)}

	t.Run("get success", func(t *testing.T) {
		mockTx.EXPECT().Get(ctx, gomock.Any()).Return(nil)
		err := mockTx.Get(ctx, NewMockRecord(ctrl))
		assert.NoError(t, err)
	})

	t.Run("get error", func(t *testing.T) {
		expectedErr := errors.New("get error")
		mockTx.EXPECT().Get(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.Get(ctx, NewMockRecord(ctrl))
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("get multi success", func(t *testing.T) {
		mockTx.EXPECT().GetMulti(ctx, gomock.Any()).Return(nil)
		err := mockTx.GetMulti(ctx, records)
		assert.NoError(t, err)
	})

	t.Run("get multi error", func(t *testing.T) {
		expectedErr := errors.New("get multi error")
		mockTx.EXPECT().GetMulti(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.GetMulti(ctx, records)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_Insert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()

	t.Run("insert success", func(t *testing.T) {
		mockTx.EXPECT().Insert(ctx, gomock.Any()).Return(nil)

		err := mockTx.Insert(ctx, NewMockRecord(ctrl))
		assert.NoError(t, err)
	})
	// Cover varargs branch (opts)
	var insOpt dal.InsertOption = nil
	t.Run("insert with option", func(t *testing.T) {
		mockTx.EXPECT().Insert(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockTx.Insert(ctx, NewMockRecord(ctrl), insOpt)
		assert.NoError(t, err)
	})

	t.Run("insert error", func(t *testing.T) {
		expectedErr := errors.New("insert error")
		mockTx.EXPECT().Insert(ctx, gomock.Any()).Return(expectedErr)

		err := mockTx.Insert(ctx, NewMockRecord(ctrl))
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("insert multi success", func(t *testing.T) {
		records := []dal.Record{NewMockRecord(ctrl)}
		mockTx.EXPECT().InsertMulti(ctx, gomock.Any()).Return(nil)
		err := mockTx.InsertMulti(ctx, records)
		assert.NoError(t, err)
	})
	// Cover varargs branch (opts) for InsertMulti
	var insOptMulti dal.InsertOption = nil
	t.Run("insert multi with option", func(t *testing.T) {
		records := []dal.Record{NewMockRecord(ctrl)}
		mockTx.EXPECT().InsertMulti(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockTx.InsertMulti(ctx, records, insOptMulti)
		assert.NoError(t, err)
	})

	t.Run("insert multi error", func(t *testing.T) {
		records := []dal.Record{NewMockRecord(ctrl)}
		expectedErr := errors.New("insert multi error")
		mockTx.EXPECT().InsertMulti(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.InsertMulti(ctx, records)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_QueryMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()

	t.Run("QueryReader success", func(t *testing.T) {
		mockTx.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, nil)
		reader, err := mockTx.ExecuteQueryToRecordsReader(ctx, nil)
		assert.NoError(t, err)
		assert.Nil(t, reader)
	})

	t.Run("QueryReader error", func(t *testing.T) {
		expectedErr := errors.New("query reader error")
		mockTx.EXPECT().GetRecordsReader(ctx, gomock.Any()).Return(nil, expectedErr)
		reader, err := mockTx.ExecuteQueryToRecordsReader(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_Set(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()

	t.Run("set success", func(t *testing.T) {
		mockTx.EXPECT().Set(ctx, gomock.Any()).Return(nil)

		err := mockTx.Set(ctx, NewMockRecord(ctrl))
		assert.NoError(t, err)
	})

	t.Run("set error", func(t *testing.T) {
		expectedErr := errors.New("set error")
		mockTx.EXPECT().Set(ctx, gomock.Any()).Return(expectedErr)

		err := mockTx.Set(ctx, NewMockRecord(ctrl))
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("set multi success", func(t *testing.T) {
		records := []dal.Record{NewMockRecord(ctrl)}
		mockTx.EXPECT().SetMulti(ctx, gomock.Any()).Return(nil)
		err := mockTx.SetMulti(ctx, records)
		assert.NoError(t, err)
	})

	t.Run("set multi error", func(t *testing.T) {
		records := []dal.Record{NewMockRecord(ctrl)}
		expectedErr := errors.New("set multi error")
		mockTx.EXPECT().SetMulti(ctx, gomock.Any()).Return(expectedErr)
		err := mockTx.SetMulti(ctx, records)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()
	key := &dal.Key{}
	updates := []update.Update{}

	t.Run("update success", func(t *testing.T) {
		mockTx.EXPECT().Update(ctx, key, updates).Return(nil)

		err := mockTx.Update(ctx, key, updates)
		assert.NoError(t, err)
	})
	// Cover varargs branch (preconditions)
	var pre dal.Precondition = nil
	t.Run("update with precondition", func(t *testing.T) {
		mockTx.EXPECT().Update(ctx, key, updates, gomock.Any()).Return(nil)
		err := mockTx.Update(ctx, key, updates, pre)
		assert.NoError(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		expectedErr := errors.New("update error")
		mockTx.EXPECT().Update(ctx, key, updates).Return(expectedErr)

		err := mockTx.Update(ctx, key, updates)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("update multi success", func(t *testing.T) {
		keys := []*dal.Key{{}}
		mockTx.EXPECT().UpdateMulti(ctx, gomock.Any(), gomock.Any()).Return(nil)
		err := mockTx.UpdateMulti(ctx, keys, updates)
		assert.NoError(t, err)
	})
	// Cover varargs branch (preconditions) for UpdateMulti
	var pre2 dal.Precondition = nil
	t.Run("update multi with precondition", func(t *testing.T) {
		keys := []*dal.Key{{}}
		mockTx.EXPECT().UpdateMulti(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		err := mockTx.UpdateMulti(ctx, keys, updates, pre2)
		assert.NoError(t, err)
	})

	t.Run("update multi error", func(t *testing.T) {
		keys := []*dal.Key{{}}
		expectedErr := errors.New("update multi error")
		mockTx.EXPECT().UpdateMulti(ctx, gomock.Any(), gomock.Any()).Return(expectedErr)
		err := mockTx.UpdateMulti(ctx, keys, updates)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("update record success", func(t *testing.T) {
		record := NewMockRecord(ctrl)
		mockTx.EXPECT().UpdateRecord(ctx, gomock.Any(), updates).Return(nil)
		err := mockTx.UpdateRecord(ctx, record, updates)
		assert.NoError(t, err)
	})
	// Cover varargs branch (preconditions) for UpdateRecord
	var pre3 dal.Precondition = nil
	t.Run("update record with precondition", func(t *testing.T) {
		record := NewMockRecord(ctrl)
		mockTx.EXPECT().UpdateRecord(ctx, gomock.Any(), updates, gomock.Any()).Return(nil)
		err := mockTx.UpdateRecord(ctx, record, updates, pre3)
		assert.NoError(t, err)
	})

	t.Run("update record error", func(t *testing.T) {
		record := NewMockRecord(ctrl)
		expectedErr := errors.New("update record error")
		mockTx.EXPECT().UpdateRecord(ctx, gomock.Any(), updates).Return(expectedErr)
		err := mockTx.UpdateRecord(ctx, record, updates)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMockReadwriteTransaction_Options(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := NewMockReadwriteTransaction(ctrl)

	mockTx.EXPECT().Options().Return(nil)

	options := mockTx.Options()
	assert.Nil(t, options)
}

// Additional coverage for variadic arguments on readwrite transaction methods
func TestMockReadwriteTransaction_VariadicArgs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	tx := NewMockReadwriteTransaction(ctrl)
	ctx := context.Background()
	key := &dal.Key{}
	rec := NewMockRecord(ctrl)
	updates := []update.Update{}

	// Insert with one option
	tx.EXPECT().Insert(ctx, gomock.Any(), gomock.Any()).Return(nil)
	opt := dal.InsertOption(nil)
	assert.NoError(t, tx.Insert(ctx, rec, opt))

	// InsertMulti with one option
	tx.EXPECT().InsertMulti(ctx, gomock.Any(), gomock.Any()).Return(nil)
	records := []dal.Record{rec}
	assert.NoError(t, tx.InsertMulti(ctx, records, opt))

	// Update with one precondition
	tx.EXPECT().Update(ctx, key, updates, gomock.Any()).Return(nil)
	var pc dal.Precondition
	assert.NoError(t, tx.Update(ctx, key, updates, pc))

	// UpdateMulti with one precondition
	tx.EXPECT().UpdateMulti(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	keys := []*dal.Key{key}
	assert.NoError(t, tx.UpdateMulti(ctx, keys, updates, pc))

	// UpdateRecord with one precondition
	tx.EXPECT().UpdateRecord(ctx, gomock.Any(), updates, gomock.Any()).Return(nil)
	assert.NoError(t, tx.UpdateRecord(ctx, rec, updates, pc))
}
