package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestCollectionGroupRef_recordsetSource(t *testing.T) {
	ref := NewCollectionGroupRef("test", "t")
	// This method is a marker method, just call it to ensure coverage
	ref.recordsetSource()
}

func TestCollectionGroupRef_Name(t *testing.T) {
	tests := []struct {
		name     string
		ref      CollectionGroupRef
		expected string
	}{
		{
			name:     "with name",
			ref:      NewCollectionGroupRef("users", "u"),
			expected: "users",
		},
		{
			name:     "different name",
			ref:      NewCollectionGroupRef("orders", "o"),
			expected: "orders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.Name()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectionGroupRef_Alias(t *testing.T) {
	tests := []struct {
		name     string
		ref      CollectionGroupRef
		expected string
	}{
		{
			name:     "with alias",
			ref:      NewCollectionGroupRef("users", "u"),
			expected: "u",
		},
		{
			name:     "empty alias",
			ref:      NewCollectionGroupRef("users", ""),
			expected: "",
		},
		{
			name:     "different alias",
			ref:      NewCollectionGroupRef("orders", "ord"),
			expected: "ord",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.Alias()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectionGroupRef_String(t *testing.T) {
	tests := []struct {
		name     string
		ref      CollectionGroupRef
		expected string
	}{
		{
			name:     "with alias",
			ref:      NewCollectionGroupRef("users", "u"),
			expected: `dal.CollectionGroupRef{name="users",alias="u"}`,
		},
		{
			name:     "without alias",
			ref:      NewCollectionGroupRef("users", ""),
			expected: `dal.CollectionGroupRef{name="users"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
