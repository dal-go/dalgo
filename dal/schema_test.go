package dal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSchema(t *testing.T) {
	keyToFieldsFunc := func(key *Key, data any) ([]ExtraField, error) {
		return []ExtraField{NewExtraField("test", "value")}, nil
	}

	dataToKey := func(incompleteKey *Key, data any) (key *Key, err error) {
		return
	}

	schema := NewSchema(keyToFieldsFunc, dataToKey)

	assert.NotNil(t, schema)
}

func TestSchema_KeyToField(t *testing.T) {
	tests := []struct {
		name            string
		keyToFieldsFunc KeyToFieldsFunc
		key             *Key
		expectedFields  []ExtraField
		expectedError   error
	}{
		{
			name: "successful_mapping",
			keyToFieldsFunc: func(key *Key, data any) ([]ExtraField, error) {
				return []ExtraField{NewExtraField("testField", "testValue")}, nil
			},
			key:            NewKeyWithID("TestKind", "test123"),
			expectedFields: []ExtraField{NewExtraField("testField", "testValue")},
			expectedError:  nil,
		},
		{
			name: "error_in_mapping",
			keyToFieldsFunc: func(key *Key, data any) ([]ExtraField, error) {
				return nil, errors.New("mapping error")
			},
			key:            NewKeyWithID("TestKind", "test123"),
			expectedFields: nil,
			expectedError:  errors.New("mapping error"),
		},
		{
			name: "nil_key",
			keyToFieldsFunc: func(key *Key, data any) ([]ExtraField, error) {
				if key == nil {
					return nil, errors.New("key is nil")
				}
				return []ExtraField{NewExtraField("field", "value")}, nil
			},
			key:            nil,
			expectedFields: nil,
			expectedError:  errors.New("key is nil"),
		},
		{
			name: "different_field_types",
			keyToFieldsFunc: func(key *Key, data any) ([]ExtraField, error) {
				switch key.ID {
				case "string":
					return []ExtraField{NewExtraField("stringField", "stringValue")}, nil
				case "int":
					return []ExtraField{NewExtraField("intField", 42)}, nil
				case "bool":
					return []ExtraField{NewExtraField("boolField", true)}, nil
				default:
					return []ExtraField{NewExtraField("defaultField", nil)}, nil
				}
			},
			key:            NewKeyWithID("TestKind", "int"),
			expectedFields: []ExtraField{NewExtraField("intField", 42)},
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewSchema(tt.keyToFieldsFunc, nil)

			fields, err := schema.KeyToFields(tt.key, nil)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, fields)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, fields)
				assert.Equal(t, len(tt.expectedFields), len(fields))
				if len(tt.expectedFields) > 0 && len(fields) > 0 {
					assert.Equal(t, tt.expectedFields[0].Name(), fields[0].Name())
					assert.Equal(t, tt.expectedFields[0].Value(), fields[0].Value())
				}
			}
		})
	}
}

func TestSchema_KeyToField_WithComplexKey(t *testing.T) {
	keyToFieldFunc := func(key *Key, data any) ([]ExtraField, error) {
		// Create field based on key properties
		fieldName := key.Collection()
		fieldValue := key.ID
		return []ExtraField{NewExtraField(fieldName, fieldValue)}, nil
	}

	schema := NewSchema(keyToFieldFunc, nil)

	// Test with parent-child key relationship
	parentKey := NewKeyWithID("Parent", "parent123")
	complexKey := NewKeyWithParentAndID(parentKey, "Child", "child456")

	fields, err := schema.KeyToFields(complexKey, nil)

	assert.NoError(t, err)
	assert.NotNil(t, fields)
	assert.Equal(t, 1, len(fields))
	assert.Equal(t, "Child", fields[0].Name())
	assert.Equal(t, "child456", fields[0].Value())
}

func TestSchema_KeyToField_WithDataParameter(t *testing.T) {
	keyToFieldFunc := func(key *Key, data any) ([]ExtraField, error) {
		// Use data parameter to modify field value
		if data != nil {
			if prefix, ok := data.(string); ok {
				return []ExtraField{NewExtraField(key.Collection(), prefix+key.ID.(string))}, nil
			}
		}
		return []ExtraField{NewExtraField(key.Collection(), key.ID)}, nil
	}

	schema := NewSchema(keyToFieldFunc, nil)
	testKey := NewKeyWithID("TestKind", "test123")

	// Test with nil data
	fields1, err1 := schema.KeyToFields(testKey, nil)
	assert.NoError(t, err1)
	assert.Equal(t, 1, len(fields1))
	assert.Equal(t, "TestKind", fields1[0].Name())
	assert.Equal(t, "test123", fields1[0].Value())

	// Test with string data
	fields2, err2 := schema.KeyToFields(testKey, "prefix_")
	assert.NoError(t, err2)
	assert.Equal(t, 1, len(fields2))
	assert.Equal(t, "TestKind", fields2[0].Name())
	assert.Equal(t, "prefix_test123", fields2[0].Value())

	// Test with non-string data (should use default behavior)
	fields3, err3 := schema.KeyToFields(testKey, 42)
	assert.NoError(t, err3)
	assert.Equal(t, 1, len(fields3))
	assert.Equal(t, "TestKind", fields3[0].Name())
	assert.Equal(t, "test123", fields3[0].Value())
}

func TestSchema_DataToKey(t *testing.T) {
	tests := []struct {
		name          string
		dataToKeyFunc DataToKeyFunc
		incompleteKey *Key
		data          any
		expectedKey   *Key
		expectedError error
	}{
		{
			name: "successful_mapping",
			dataToKeyFunc: func(incompleteKey *Key, data any) (*Key, error) {
				if dataMap, ok := data.(map[string]any); ok {
					if id, exists := dataMap["id"]; exists {
						return NewKeyWithID("TestKind", id), nil
					}
				}
				return nil, errors.New("id not found in data")
			},
			incompleteKey: NewKeyWithID("TestKind", ""),
			data:          map[string]any{"id": "test123", "name": "test"},
			expectedKey:   NewKeyWithID("TestKind", "test123"),
			expectedError: nil,
		},
		{
			name: "error_in_mapping",
			dataToKeyFunc: func(incompleteKey *Key, data any) (*Key, error) {
				return nil, errors.New("mapping error")
			},
			incompleteKey: NewKeyWithID("TestKind", ""),
			data:          map[string]any{"name": "test"},
			expectedKey:   nil,
			expectedError: errors.New("mapping error"),
		},
		{
			name: "use_incomplete_key_collection",
			dataToKeyFunc: func(incompleteKey *Key, data any) (*Key, error) {
				if dataMap, ok := data.(map[string]any); ok {
					if id, exists := dataMap["id"]; exists {
						return NewKeyWithID(incompleteKey.Collection(), id), nil
					}
				}
				return nil, errors.New("id not found in data")
			},
			incompleteKey: NewKeyWithID("CustomKind", ""),
			data:          map[string]any{"id": "custom123"},
			expectedKey:   NewKeyWithID("CustomKind", "custom123"),
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewSchema(nil, tt.dataToKeyFunc)

			key, err := schema.DataToKey(tt.incompleteKey, tt.data)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, key)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, key)
				assert.Equal(t, tt.expectedKey.Collection(), key.Collection())
				assert.Equal(t, tt.expectedKey.ID, key.ID)
			}
		})
	}
}
