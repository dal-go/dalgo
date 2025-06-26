package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCollectionGroupRef(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name          string
		args          args
		expectedPanic string
		want          CollectionGroupRef
	}{
		{
			name: "empty_name",
			args: args{
				name: "",
			},
			want:          CollectionGroupRef{},
			expectedPanic: "name of recordsetSource group reference cannot be empty",
		},
		{
			name: "should_pass",
			args: args{
				name: "collection_1",
			},
			want: CollectionGroupRef{
				name: "collection_1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedPanic != "" {
				defer func() {
					r := recover()
					if _, ok := r.(string); !ok || r != tt.expectedPanic {
						t.Fatalf(`expect panic: "%s", got: %v`, tt.expectedPanic, r)
					}
				}()
			}
			actual := NewCollectionGroupRef(tt.args.name)
			assert.Equal(t, tt.want, actual)
		})
	}
}
