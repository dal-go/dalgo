package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCollectionGroupRef(t *testing.T) {
	type args struct {
		name  string
		alias string
	}
	tests := []struct {
		name          string
		args          args
		expectedPanic string
		want          CollectionGroupRef
	}{
		{
			name:          "empty",
			args:          args{},
			want:          CollectionGroupRef{},
			expectedPanic: "name of collection group reference cannot be empty",
		},
		{
			name: "should_pass",
			args: args{
				name:  "collection_1",
				alias: "c1",
			},
			want: CollectionGroupRef{
				name:  "collection_1",
				alias: "c1",
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
			actual := NewCollectionGroupRef(tt.args.name, tt.args.alias)
			assert.Equal(t, tt.want, actual)
		})
	}
}
