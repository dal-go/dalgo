package mock_dal

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewMockSchema(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	schema := NewMockSchema(ctrl)
	assert.NotNil(t, schema)
	assert.NotNil(t, schema.EXPECT())
}

func TestMockSchema_Methods(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	schema := NewMockSchema(ctrl)

	// DataToKey
	incomplete := &dal.Key{}
	data := map[string]any{"x": 1}
	expectedKey := &dal.Key{}
	schema.EXPECT().DataToKey(incomplete, data).Return(expectedKey, nil)
	key, err := schema.DataToKey(incomplete, data)
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)

	// KeyToFields
	expectedFields := []dal.ExtraField{dal.NewExtraField("f", 123)}
	schema.EXPECT().KeyToFields(expectedKey, data).Return(expectedFields, nil)
	fields, err := schema.KeyToFields(expectedKey, data)
	assert.NoError(t, err)
	assert.Len(t, fields, 1)
	assert.Equal(t, expectedFields[0].Name(), fields[0].Name())
	assert.Equal(t, expectedFields[0].Value(), fields[0].Value())
}

func TestMockDB_Schema(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDB(ctrl)
	schema := NewMockSchema(ctrl)

	db.EXPECT().Schema().Return(schema)

	result := db.Schema()
	assert.Equal(t, schema, result)
}
