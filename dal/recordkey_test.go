package dal

import "testing"

func TestGetRecordKeyPath(t *testing.T) {
	type args struct {
		key *Key
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no_parent-string_id",
			args: args{key: NewKeyWithID("Kind1", "id1")},
			want: "Kind1/id1",
		},
		{
			name: "single_parent-string_id",
			args: args{
				key: NewKeyWithParentAndID(NewKeyWithID("Parent1", "p1"), "Kind1", "id1"),
			},
			want: "Parent1/p1/Kind1/id1",
		},
		{
			name: "two_parents-string_id",
			args: args{
				key: NewKeyWithParentAndID(
					NewKeyWithParentAndID(
						NewKeyWithID("Parent2", "p2"),
						"Parent1", "p1",
					),
					"Kind1",
					"id1",
				),
			},
			want: "Parent2/p2/Parent1/p1/Kind1/id1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.args.key.String(); got != tt.want {
				t.Errorf("key.String() = %v, want %v, child:%+v, Parent: %+v", got, tt.want, tt.args.key, tt.args.key.parent)
			}
		})
	}
}

func TestKeyCollectionPath(t *testing.T) {
	type args struct {
		key *Key
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no_parent-string_id",
			args: args{key: NewKeyWithID("Kind1", "id1")},
			want: "Kind1",
		},
		{
			name: "single_parent-string_id",
			args: args{key: NewKeyWithParentAndID(NewKeyWithID("Parent1", "p1"), "Kind1", "id1")},
			want: "Parent1/Kind1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.args.key.CollectionPath(); got != tt.want {
				t.Errorf("CollectionPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReverseStringsJoin(t *testing.T) {
	type args struct {
		elems []string
		sep   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "empty",
			args: args{elems: []string{}, sep: "/"},
			want: "",
		},
		{
			name: "single",
			args: args{elems: []string{"el1"}, sep: "/"},
			want: "el1",
		},
		{
			name: "two",
			args: args{elems: []string{"el1", "el2"}, sep: "/"},
			want: "el2/el1",
		},
		{
			name: "three",
			args: args{elems: []string{"el1", "el2", "el3"}, sep: "/"},
			want: "el3/el2/el1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reverseStringsJoin(tt.args.elems, tt.args.sep); got != tt.want {
				t.Errorf("reverseStringsJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}
