package update

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
