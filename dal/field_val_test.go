package dal

import "testing"

func TestFieldVal_Validate(t *testing.T) {

	type test struct {
		name     string
		v        FieldVal
		expected string
	}

	tests := []test{
		{
			name:     "empty",
			v:        FieldVal{},
			expected: "missing field name",
		},
		{
			name:     "nil",
			v:        FieldVal{Name: "name1", Value: nil},
			expected: "",
		},
		{
			name:     "string",
			v:        FieldVal{Name: "name1", Value: "value1"},
			expected: "",
		},
		{
			name:     "int",
			v:        FieldVal{Name: "name1", Value: 1},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.v.Validate()
			if tt.expected == "" {
				if err != nil {
					t.Errorf("expected to be valid, got: %v", err)
				}
			} else if err == nil {
				t.Errorf("expected to be invalid, got nil")
			} else if err.Error() != tt.expected {
				t.Errorf("expected error %v, got %v", tt.expected, err.Error())
			}
		})
	}
}
