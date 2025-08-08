package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewField(t *testing.T) {
	tests := []struct {
		name          string
		fieldName     string
		fieldValue    any
		expectedName  string
		expectedValue any
	}{
		{
			name:          "string_field",
			fieldName:     "stringField",
			fieldValue:    "testValue",
			expectedName:  "stringField",
			expectedValue: "testValue",
		},
		{
			name:          "int_field",
			fieldName:     "intField",
			fieldValue:    42,
			expectedName:  "intField",
			expectedValue: 42,
		},
		{
			name:          "bool_field",
			fieldName:     "boolField",
			fieldValue:    true,
			expectedName:  "boolField",
			expectedValue: true,
		},
		{
			name:          "nil_value",
			fieldName:     "nilField",
			fieldValue:    nil,
			expectedName:  "nilField",
			expectedValue: nil,
		},
		{
			name:          "empty_name",
			fieldName:     "",
			fieldValue:    "value",
			expectedName:  "",
			expectedValue: "value",
		},
		{
			name:          "complex_value",
			fieldName:     "complexField",
			fieldValue:    map[string]interface{}{"key": "value"},
			expectedName:  "complexField",
			expectedValue: map[string]interface{}{"key": "value"},
		},
		{
			name:          "slice_value",
			fieldName:     "sliceField",
			fieldValue:    []string{"a", "b", "c"},
			expectedName:  "sliceField",
			expectedValue: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewExtraField(tt.fieldName, tt.fieldValue)

			assert.NotNil(t, field)
			assert.Equal(t, tt.expectedName, field.Name())
			assert.Equal(t, tt.expectedValue, field.Value())
		})
	}
}

func TestField_Name(t *testing.T) {
	field := NewExtraField("testName", "testValue")

	name := field.Name()

	assert.Equal(t, "testName", name)
}

func TestField_Value(t *testing.T) {
	testValue := "testValue"
	field := NewExtraField("testName", testValue)

	value := field.Value()

	assert.Equal(t, testValue, value)
}

func TestField_Interface(t *testing.T) {
	// Test that field implements ExtraField interface
	var f ExtraField = NewExtraField("test", "value")

	assert.NotNil(t, f)
	assert.Equal(t, "test", f.Name())
	assert.Equal(t, "value", f.Value())
}

func TestField_ValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		checkType func(any) bool
	}{
		{
			name:  "string_type",
			value: "string",
			checkType: func(v any) bool {
				_, ok := v.(string)
				return ok
			},
		},
		{
			name:  "int_type",
			value: 123,
			checkType: func(v any) bool {
				_, ok := v.(int)
				return ok
			},
		},
		{
			name:  "float_type",
			value: 3.14,
			checkType: func(v any) bool {
				_, ok := v.(float64)
				return ok
			},
		},
		{
			name:  "bool_type",
			value: true,
			checkType: func(v any) bool {
				_, ok := v.(bool)
				return ok
			},
		},
		{
			name:  "struct_type",
			value: struct{ Name string }{Name: "test"},
			checkType: func(v any) bool {
				_, ok := v.(struct{ Name string })
				return ok
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewExtraField("testField", tt.value)

			value := field.Value()
			assert.True(t, tt.checkType(value), "Value should maintain its original type")
			assert.Equal(t, tt.value, value)
		})
	}
}

func TestField_Immutability(t *testing.T) {
	// Test that field values are not modified after creation
	originalSlice := []string{"a", "b", "c"}
	field := NewExtraField("sliceField", originalSlice)

	// Modify the original slice
	originalSlice[0] = "modified"

	// Field should still have the original reference (Go slices are reference types)
	fieldValue := field.Value().([]string)
	assert.Equal(t, "modified", fieldValue[0], "Field should reference the same slice")

	// Test with map
	originalMap := map[string]string{"key": "value"}
	mapField := NewExtraField("mapField", originalMap)

	// Modify the original map
	originalMap["key"] = "modified"

	// Field should still have the original reference
	fieldMapValue := mapField.Value().(map[string]string)
	assert.Equal(t, "modified", fieldMapValue["key"], "Field should reference the same map")
}
