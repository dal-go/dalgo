package dal

import "testing"

func TestFieldName_String(t *testing.T) {
	tests := []struct {
		name     string
		field    FieldName
		expected string
	}{
		{
			name:     "simple field name",
			field:    FieldName("username"),
			expected: "username",
		},
		{
			name:     "empty field name",
			field:    FieldName(""),
			expected: "",
		},
		{
			name:     "field name with special characters",
			field:    FieldName("user_name"),
			expected: "user_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.field.String()
			if result != tt.expected {
				t.Errorf("FieldName.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}
