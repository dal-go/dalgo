package mock_dal

import (
	"errors"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestNewMockRecord(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)
	assert.NotNil(t, mockRecord)
	assert.NotNil(t, mockRecord.EXPECT())

	mockRecord.EXPECT().Key().Return(dal.NewKeyWithID("a", "v"))
	assert.NotNil(t, mockRecord.Key())

	mockRecord.EXPECT().SetError(dal.ErrRecordNotFound).Times(1)
	mockRecord.SetError(dal.ErrRecordNotFound)

	data := []byte("abc")
	mockRecord.EXPECT().Data().Return(data).Times(1)
	assert.Equal(t, data, mockRecord.Data())
}

func TestMockRecord_Data(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)
	testData := map[string]interface{}{"test": "value"}

	mockRecord.EXPECT().Data().Return(testData)

	result := mockRecord.Data()
	assert.Equal(t, testData, result)
}

func TestMockRecord_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)
	testError := errors.New("test error")

	mockRecord.EXPECT().Error().Return(testError)

	result := mockRecord.Error()
	assert.Equal(t, testError, result)
}

func TestMockRecord_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)

	t.Run("exists_true", func(t *testing.T) {
		mockRecord.EXPECT().Exists().Return(true)
		result := mockRecord.Exists()
		assert.True(t, result)
	})

	t.Run("exists_false", func(t *testing.T) {
		mockRecord.EXPECT().Exists().Return(false)
		result := mockRecord.Exists()
		assert.False(t, result)
	})
}

func TestMockRecord_HasChanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)

	t.Run("has_changed_true", func(t *testing.T) {
		mockRecord.EXPECT().HasChanged().Return(true)
		result := mockRecord.HasChanged()
		assert.True(t, result)
	})

	t.Run("has_changed_false", func(t *testing.T) {
		mockRecord.EXPECT().HasChanged().Return(false)
		result := mockRecord.HasChanged()
		assert.False(t, result)
	})
}

func TestMockRecord_Key(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)
	testKey := dal.NewKeyWithID("test", "id1")

	mockRecord.EXPECT().Key().Return(testKey)

	result := mockRecord.Key()
	assert.Equal(t, testKey, result)
}

func TestMockRecord_MarkAsChanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)

	mockRecord.EXPECT().MarkAsChanged()

	// Should not panic
	mockRecord.MarkAsChanged()
}

func TestMockRecord_SetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRecord := NewMockRecord(ctrl)
	testError := errors.New("test error")

	mockRecord.EXPECT().SetError(testError).Return(mockRecord)

	result := mockRecord.SetError(testError)
	assert.Equal(t, mockRecord, result)
}
