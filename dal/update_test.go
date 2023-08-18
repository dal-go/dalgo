package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUpdate_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		update  Update
		wantErr bool
	}{
		{name: "empty", update: Update{}, wantErr: true},
		{name: "both", update: Update{Field: "a/b", FieldPath: FieldPath{"a", "b"}}, wantErr: true},
		{name: "field_only", update: Update{Field: "a/b"}, wantErr: false},
		{name: "fieldPath_only", update: Update{FieldPath: FieldPath{"a", "b"}}, wantErr: false},
		{name: "delete", update: Update{Field: "a/b", Value: DeleteField}, wantErr: false},
		{name: "ServerTimestamp", update: Update{Field: "a/b", Value: ServerTimestamp}, wantErr: false},
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
