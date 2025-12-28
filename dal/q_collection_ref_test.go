package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestCollectionRef_recordsetSource(t *testing.T) {
	ref := NewCollectionRef("test", "t", nil)
	// This method is a marker method, just call it to ensure coverage
	ref.recordsetSource()
	_ = (RecordsetSource)(ref)
}

func TestCollectionRef_Alias(t *testing.T) {
	tests := []struct {
		name     string
		ref      CollectionRef
		expected string
	}{
		{
			name:     "with alias",
			ref:      NewCollectionRef("users", "u", nil),
			expected: "u",
		},
		{
			name:     "empty alias",
			ref:      NewCollectionRef("users", "", nil),
			expected: "",
		},
		{
			name:     "same name and alias becomes empty",
			ref:      NewCollectionRef("users", "users", nil),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.Alias()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectionRef_Parent(t *testing.T) {
	parentKey := NewKeyWithID("parent_collection", "parent_id")

	tests := []struct {
		name     string
		ref      CollectionRef
		expected *Key
	}{
		{
			name:     "with parent",
			ref:      NewCollectionRef("child", "c", parentKey),
			expected: parentKey,
		},
		{
			name:     "without parent",
			ref:      NewCollectionRef("root", "r", nil),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.Parent()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectionRef_Equal(t *testing.T) {
	type args struct {
		other       CollectionRef
		ignoreAlias bool
	}
	tests := []struct {
		name string
		v    CollectionRef
		args args
		want bool
	}{
		{
			name: "empty, ignoreAlias=true",
			v:    CollectionRef{},
			args: args{
				other:       CollectionRef{},
				ignoreAlias: true,
			},
			want: true,
		},
		{
			name: "empty, ignoreAlias=false",
			v:    CollectionRef{},
			args: args{
				other:       CollectionRef{},
				ignoreAlias: false,
			},
			want: true,
		},
		{
			name: "same name and alias, no parent, ignoreAlias=false",
			v:    CollectionRef{name: "books", alias: "b"},
			args: args{
				other:       CollectionRef{name: "books", alias: "b"},
				ignoreAlias: false,
			},
			want: true,
		},
		{
			name: "same name, different alias, ignoreAlias=true",
			v:    CollectionRef{name: "books", alias: "b1"},
			args: args{
				other:       CollectionRef{name: "books", alias: "b2"},
				ignoreAlias: true,
			},
			want: true,
		},
		{
			name: "same name, different alias, ignoreAlias=false",
			v:    CollectionRef{name: "books", alias: "b1"},
			args: args{
				other:       CollectionRef{name: "books", alias: "b2"},
				ignoreAlias: false,
			},
			want: false,
		},
		{
			name: "different name, same alias, ignoreAlias=true",
			v:    CollectionRef{name: "books", alias: "b"},
			args: args{
				other:       CollectionRef{name: "magazines", alias: "b"},
				ignoreAlias: true,
			},
			want: false,
		},
		{
			name: "different name, same alias, ignoreAlias=false",
			v:    CollectionRef{name: "books", alias: "b"},
			args: args{
				other:       CollectionRef{name: "magazines", alias: "b"},
				ignoreAlias: false,
			},
			want: false,
		},
		// parent-pointer-specific cases are appended below where a shared pointer can be reused
	}
	// Replace the placeholder complex cases with concrete ones using shared parent pointers
	{
		p := NewKeyWithID("parent", "1")
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "same pointer parent, same name and alias (ignoreAlias=false)",
				v:    CollectionRef{name: "books", alias: "b", parent: p},
				args: args{other: CollectionRef{name: "books", alias: "b", parent: p}, ignoreAlias: false},
				want: true,
			},
		)
	}
	{
		p := NewKeyWithID("parent", "1")
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "same pointer parent, different alias (ignoreAlias=true)",
				v:    CollectionRef{name: "books", alias: "b1", parent: p},
				args: args{other: CollectionRef{name: "books", alias: "b2", parent: p}, ignoreAlias: true},
				want: true,
			},
		)
	}
	{
		p := NewKeyWithID("parent", "1")
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "same values but different parent pointers -> not equal",
				v:    CollectionRef{name: "books", alias: "b", parent: p},
				args: args{other: CollectionRef{name: "books", alias: "b", parent: NewKeyWithID("parent", "1")}, ignoreAlias: false},
				want: false,
			},
		)
	}
	{
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "one has parent, other has nil parent -> not equal",
				v:    CollectionRef{name: "books", alias: "b", parent: NewKeyWithID("parent", "1")},
				args: args{other: CollectionRef{name: "books", alias: "b", parent: nil}, ignoreAlias: false},
				want: false,
			},
		)
	}
	{
		p := NewKeyWithID("parent", "1")
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "same pointer parent, different name -> not equal",
				v:    CollectionRef{name: "books", alias: "b", parent: p},
				args: args{other: CollectionRef{name: "magazines", alias: "b", parent: p}, ignoreAlias: true},
				want: false,
			},
		)
	}
	{
		p1 := NewKeyWithID("parent", "1")
		p2 := NewKeyWithID("parent", "2")
		tests = append(tests,
			struct {
				name string
				v    CollectionRef
				args args
				want bool
			}{
				name: "different parent pointers (different values) -> not equal",
				v:    CollectionRef{name: "books", alias: "b", parent: p1},
				args: args{other: CollectionRef{name: "books", alias: "b", parent: p2}, ignoreAlias: false},
				want: false,
			},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.Equal(tt.args.other, tt.args.ignoreAlias)
			assert.Equalf(t, tt.want, got, "Equal(%v, %v)", tt.args.other, tt.args.ignoreAlias)
		})
	}
}
