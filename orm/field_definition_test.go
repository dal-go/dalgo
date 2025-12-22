package orm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewField(t *testing.T) {
	tests := []struct {
		name        string
		newField    func(name string) FieldDefinition[string]
		assertField func(t *testing.T, f FieldDefinition[string])
		shouldPanic bool
	}{
		{
			name:        "empty_name",
			shouldPanic: true,
			newField: func(_ string) FieldDefinition[string] {
				return NewField[string]("")
			},
		},
		{
			name: "string_field_with_just_name",
			newField: func(name string) FieldDefinition[string] {
				return NewField[string](name)
			},
			assertField: func(t *testing.T, f FieldDefinition[string]) {
				assert.False(t, f.IsRequired())
			},
		},
		{
			name: "required_string_field_1",
			newField: func(name string) FieldDefinition[string] {
				return NewField(name, Required[string]())
			},
			assertField: func(t *testing.T, f FieldDefinition[string]) {
				assert.True(t, f.IsRequired())
			},
		},
		{
			name: "required_string_field_with_default_value",
			newField: func(name string) FieldDefinition[string] {
				return NewField[string](name, Required[string](), Default[string]("default_value_1"))
			},
			assertField: func(t *testing.T, f FieldDefinition[string]) {
				assert.True(t, f.IsRequired())
				assert.Equal(t, "default_value_1", f.DefaultValue())
			},
		},
	}

	for _, tt := range tests {
		if tt.shouldPanic {
			assert.Panics(t, func() {
				tt.newField(tt.name)
			})
			continue
		}
		f := tt.newField(tt.name)
		assert.Equal(t, tt.name, f.Name())
		assert.Equal(t, "string", f.Type())
		tt.assertField(t, f)
	}
}

func TestNewFieldWithType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("NewFieldWithType() did not panic")
		}
	}()
	NewFieldWithType[string]("name", "ABC")
}
