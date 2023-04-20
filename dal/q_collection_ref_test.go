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
		{name: "only_name", args: args{name: "some_target", alias: "", parent: nil}, want: CollectionRef{Name: "some_target", Alias: "", Parent: nil}},
		{name: "name_and_alias", args: args{name: "some_target", alias: "st", parent: nil}, want: CollectionRef{Name: "some_target", Alias: "st", Parent: nil}},
		{name: "same_name_and_alias", args: args{name: "some_target", alias: "some_target", parent: nil}, want: CollectionRef{Name: "some_target", Alias: "", Parent: nil}},
		{name: "name_with_parent", args: args{name: "some_target", alias: "", parent: someParentKey}, want: CollectionRef{Name: "some_target", Alias: "", Parent: someParentKey}},
		{name: "name_and_alias_with_parent", args: args{name: "some_target", alias: "st", parent: someParentKey}, want: CollectionRef{Name: "some_target", Alias: "st", Parent: someParentKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, NewCollectionRef(tt.args.name, tt.args.alias, tt.args.parent), "NewCollectionRef(%v, %v, %v)", tt.args.name, tt.args.alias, tt.args.parent)
		})
	}
}
