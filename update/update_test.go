package update

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdate_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		update  update
		wantErr bool
	}{
		{name: "empty", update: update{}, wantErr: true},
		{name: "both", update: update{fieldName: "a/b", fieldPath: FieldPath{"a", "b"}}, wantErr: true},
		{name: "field_only", update: update{fieldName: "a/b"}, wantErr: false},
		{name: "fieldPath_only", update: update{fieldPath: FieldPath{"a", "b"}}, wantErr: false},
		{name: "delete", update: update{fieldName: "a/b", value: DeleteField}, wantErr: false},
		{name: "ServerTimestamp", update: update{fieldName: "a/b", value: ServerTimestamp}, wantErr: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.update.Validate()
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestByFieldName(t *testing.T) {
	type args struct {
		fieldName string
		value     any
	}
	tests := []struct {
		name string
		args args
		want Update
	}{
		{
			name: "normal",
			args: args{fieldName: "a", value: 1},
			want: update{fieldName: "a", value: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, ByFieldName(tt.args.fieldName, tt.args.value), "ByFieldName(%v, %v)", tt.args.fieldName, tt.args.value)
		})
	}
}

func TestByFieldPath(t *testing.T) {
	type args struct {
		fieldPath FieldPath
		value     any
	}
	tests := []struct {
		name string
		args args
		want Update
	}{
		{
			name: "normal",
			args: args{fieldPath: FieldPath{"a", "b"}, value: 1},
			want: update{fieldPath: FieldPath{"a", "b"}, value: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, ByFieldPath(tt.args.fieldPath, tt.args.value), "ByFieldPath(%v, %v)", tt.args.fieldPath, tt.args.value)
		})
	}
}

func TestDeleteByFieldName(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		want      Update
	}{
		{
			name:      "normal",
			fieldName: "testField",
			want:      update{fieldName: "testField", value: DeleteField},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeleteByFieldName(tt.fieldName)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestDeleteByFieldPath(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want Update
	}{
		{
			name: "single_path",
			path: []string{"field1"},
			want: update{fieldPath: FieldPath{"field1"}, value: DeleteField},
		},
		{
			name: "nested_path",
			path: []string{"field1", "field2", "field3"},
			want: update{fieldPath: FieldPath{"field1", "field2", "field3"}, value: DeleteField},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeleteByFieldPath(tt.path...)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestUpdate_FieldName(t *testing.T) {
	u := update{fieldName: "testField", value: "testValue"}
	assert.Equal(t, "testField", u.FieldName())
}

func TestUpdate_FieldPath(t *testing.T) {
	path := FieldPath{"field1", "field2"}
	u := update{fieldPath: path, value: "testValue"}
	assert.Equal(t, path, u.FieldPath())
}

func TestUpdate_Value(t *testing.T) {
	testValue := "testValue"
	u := update{fieldName: "testField", value: testValue}
	assert.Equal(t, testValue, u.Value())
}

func TestByFieldName_WithDots(t *testing.T) {
	result := ByFieldName("field1.field2", "value")
	expected := update{fieldPath: FieldPath{"field1", "field2"}, value: "value"}
	assert.Equal(t, expected, result)
}

func TestByFieldName_Panics(t *testing.T) {
	assert.Panics(t, func() {
		ByFieldName("", "value")
	}, "should panic with empty fieldName")
}

func TestByFieldPath_Panics(t *testing.T) {
	assert.Panics(t, func() {
		ByFieldPath(FieldPath{}, "value")
	}, "should panic with empty fieldPath")
}

func TestDeleteByFieldName_Panics(t *testing.T) {
	assert.Panics(t, func() {
		DeleteByFieldName("")
	}, "should panic with empty fieldName")
}

func TestDeleteByFieldPath_Panics(t *testing.T) {
	assert.Panics(t, func() {
		DeleteByFieldPath()
	}, "should panic with empty path")
}

func TestUpdate_Validate_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		update  update
		wantErr bool
		errMsg  string
	}{
		{
			name:    "fieldName_with_dot",
			update:  update{fieldName: "field.name"},
			wantErr: true,
			errMsg:  "fieldName contains '.' character",
		},
		{
			name:    "empty_fieldPath_component",
			update:  update{fieldPath: FieldPath{"field1", "", "field3"}},
			wantErr: true,
			errMsg:  "empty field path component at index 1",
		},
		{
			name:    "whitespace_only_fieldPath_component",
			update:  update{fieldPath: FieldPath{"field1", "   ", "field3"}},
			wantErr: true,
			errMsg:  "empty field path component at index 1",
		},
		{
			name:    "both_fieldName_and_fieldPath",
			update:  update{fieldName: "field", fieldPath: FieldPath{"path"}},
			wantErr: true,
			errMsg:  "both FieldVal and fieldPath are provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.update.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), strings.Split(tt.errMsg, ":")[0])
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSentinelValues(t *testing.T) {
	// Test that sentinel values are distinct
	assert.NotEqual(t, DeleteField, ServerTimestamp)

	// Test using sentinel values in updates
	deleteUpdate := update{fieldName: "field", value: DeleteField}
	timestampUpdate := update{fieldName: "field", value: ServerTimestamp}

	assert.Equal(t, DeleteField, deleteUpdate.Value())
	assert.Equal(t, ServerTimestamp, timestampUpdate.Value())
}

func TestByFieldName_WithDots_InvalidPath_Panics(t *testing.T) {
	// "a..b" will split into ["a", "", "b"] and Validate should fail, causing panic inside ByFieldName
	assert.Panics(t, func() {
		ByFieldName("a..b", 123)
	})
}

func TestByFieldPath_InvalidPath_Panics(t *testing.T) {
	// FieldPath with empty component should cause Validate to fail and ByFieldPath to panic
	assert.Panics(t, func() {
		ByFieldPath(FieldPath{"a", ""}, 1)
	})
}
