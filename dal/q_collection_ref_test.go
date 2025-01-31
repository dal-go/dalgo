package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCollectionRef(t *testing.T) {
	t.Run("panics_on_empty_name", func(t *testing.T) {
		assert.Panics(t, func() {
			NewCollectionRef("", "e", nil)
		})
	})

	someParentKey := NewKeyWithID("some_parent", "some_id")

	type args struct {
		name   string
		alias  string
		parent *Key
	}

	tests := []struct {
		name string
		args args
		want CollectionRef
	}{
		{name: "only_name", args: args{name: "some_target", alias: "", parent: nil}, want: CollectionRef{name: "some_target", alias: "", parent: nil}},
		{name: "name_and_alias", args: args{name: "some_target", alias: "st", parent: nil}, want: CollectionRef{name: "some_target", alias: "st", parent: nil}},
		{name: "same_name_and_alias", args: args{name: "some_target", alias: "some_target", parent: nil}, want: CollectionRef{name: "some_target", alias: "", parent: nil}},
		{name: "name_with_parent", args: args{name: "some_target", alias: "", parent: someParentKey}, want: CollectionRef{name: "some_target", alias: "", parent: someParentKey}},
		{name: "name_and_alias_with_parent", args: args{name: "some_target", alias: "st", parent: someParentKey}, want: CollectionRef{name: "some_target", alias: "st", parent: someParentKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, NewCollectionRef(tt.args.name, tt.args.alias, tt.args.parent), "NewCollectionRef(%v, %v, %v)", tt.args.name, tt.args.alias, tt.args.parent)
		})
	}
}

func TestCollectionRef(t *testing.T) {
	type expected struct {
		string string
		path   string
	}
	for _, tt := range []struct {
		name          string
		collectionRef CollectionRef
		expected      expected
	}{
		{
			name:          "empty",
			collectionRef: CollectionRef{},
			expected: expected{
				string: "",
				path:   "",
			},
		},
		{
			name: "name_only",
			collectionRef: CollectionRef{
				name: "collection1",
			},
			expected: expected{
				string: "collection1",
				path:   "collection1",
			},
		},
		{
			name: "no_parent_with_alias",
			collectionRef: CollectionRef{
				name:  "collection1",
				alias: "c1",
			},
			expected: expected{
				string: "collection1 AS c1",
				path:   "collection1",
			},
		},
		{
			name: "single_parent",
			collectionRef: CollectionRef{
				name:   "collection1",
				parent: &Key{collection: "collection2", ID: "id2"},
			},
			expected: expected{
				string: "collection2/id2/collection1",
				path:   "collection2/id2/collection1",
			},
		},
		{
			name: "single_parent_with_alias",
			collectionRef: CollectionRef{
				name:   "collection1",
				alias:  "c1",
				parent: &Key{collection: "collection2", ID: "id2"},
			},
			expected: expected{
				string: "collection2/id2/collection1 AS c1",
				path:   "collection2/id2/collection1",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("string", func(t *testing.T) {
				assert.Equal(t, tt.expected.string, tt.collectionRef.String())
			})
			t.Run("path", func(t *testing.T) {
				assert.Equal(t, tt.expected.path, tt.collectionRef.Path())
			})
		})
	}
}
