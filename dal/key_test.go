package dal

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestField_Validate(t *testing.T) {
	type fields struct {
		Name  string
		Value any
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "valid",
			fields: fields{
				Name:  "field1",
				Value: "value1",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := FieldVal{
				Name:  tt.fields.Name,
				Value: tt.fields.Value,
			}
			if err := v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

//func TestKey_Child(t *testing.T) {
//	type fields struct {
//		level int
//		collection  string
//		ID    any
//	}
//	type args struct {
//		child *Key
//	}
//	type want struct {
//		fields
//		Parent *fields
//	}
//	tests := []struct {
//		Name   string
//		fields fields
//		args   args
//		want   want
//	}{
//		{
//			Name: "single_parent",
//			fields: fields{
//				collection: "Parent1",
//				ID:   "p1",
//			},
//			args: args{
//				child: NewKeyWithID("Kind1", "k1"),
//			},
//			want: want{
//				fields: fields{
//					collection:  "Kind1",
//					ID:    "k1",
//					level: 1,
//				},
//				Parent: &fields{collection: "Parent1", ID: "p1", level: 0},
//			},
//		},
//		{
//			Name: "two_parents",
//			fields: fields{
//				collection: "Parent1",
//				ID:   "p1",
//			},
//			args: args{
//				child: NewKeyWithID("Parent2", "p2").Child(NewKeyWithID("Kind1", "k1")),
//			},
//			want: want{
//				Parent: &fields{collection: "Parent1", ID: "p1", level: 1},
//				fields: fields{
//					collection:  "Kind1",
//					ID:    "k1",
//					level: 2,
//				},
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.Name, func(t *testing.T) {
//			v := &Key{
//				collection: tt.fields.collection,
//				ID:   tt.fields.ID,
//			}
//			got := v.Child(tt.args.child)
//			if got.Level() != tt.want.level {
//				t.Errorf("Child().level = %v, want %v, got %v, Parent %+v", got.Level(), tt.want.level, got, got.Parent)
//			}
//			if got.Parent == nil && tt.want.Parent != nil {
//				t.Errorf("Child().Parent = nil, want %+v", tt.want.Parent)
//			}
//			if got.Parent != nil && tt.want.Parent == nil {
//				t.Error("Child().Parent != nil, want nil")
//			}
//			if got.Parent != nil && tt.want.Parent != nil {
//				if got.Parent.collection != tt.want.Parent.collection {
//					t.Errorf("Child().Parent.collection = %v, want %v", got.Parent.collection, tt.want.Parent.collection)
//				}
//				if got.Parent.ID != tt.want.Parent.ID {
//					t.Errorf("Child().Parent.ID = %v, want %v", got.Parent.ID, tt.want.Parent.ID)
//				}
//				if got.Parent.Level() != tt.want.Parent.level {
//					t.Errorf("Child().Parent.level = %v, want %v, Parent %+v", got.Parent.Level(), tt.want.Parent.level, got.Parent)
//				}
//			}
//		})
//	}
//}

func TestKey_Kind(t *testing.T) {
	type fields struct {
		collection string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{name: "empty", fields: fields{collection: ""}, want: ""},
		{name: "kind1", fields: fields{collection: "kind1"}, want: "kind1"},
		{name: "kind2", fields: fields{collection: "kind2"}, want: "kind2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Key{
				collection: tt.fields.collection,
			}
			if got := v.Collection(); got != tt.want {
				t.Errorf("Collection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKey_Level(t *testing.T) {
	type fields struct {
		parent *Key
	}
	tests := []struct {
		name   string
		fields fields
		want   int
	}{
		{name: "zero", fields: fields{parent: nil}, want: 0},
		{
			name: "one",
			fields: fields{
				parent: NewKeyWithID("P1", "p1"),
			},
			want: 1,
		},
		{
			name: "two",
			fields: fields{
				parent: NewKeyWithID("P1", "p1", WithParent("P2", "p2")),
			},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Key{
				parent: tt.fields.parent,
			}
			if got := v.Level(); got != tt.want {
				t.Errorf("Level() = %v, want %v, for %+v, Parent %+v", got, tt.want, v, v.parent)
			}
		})
	}
}

func TestKey_Parent(t *testing.T) {
	type fields struct {
		parent *Key
	}
	parent := NewKeyWithID("Parent1", "p1")
	tests := []struct {
		name   string
		fields fields
		want   *Key
	}{
		{name: "no_parent", fields: fields{parent: nil}, want: nil},
		{name: "single_parent", fields: fields{parent: parent}, want: parent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Key{
				parent: tt.fields.parent,
			}
			if got := v.Parent(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKey_Validate(t *testing.T) {
	type fields struct {
		//level  int
		parent *Key
		kind   string
		ID     any
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{name: "no_parent-string_id", wantErr: false, fields: fields{
			kind: "Kind1", ID: "k1",
		}},
		{name: "no_parent-int_id", wantErr: false, fields: fields{
			kind: "Kind1", ID: 1,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Key{
				parent:     tt.fields.parent,
				collection: tt.fields.kind,
				ID:         tt.fields.ID,
			}
			if err := v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewKey(t *testing.T) {
	type args struct {
		kind    string
		options []KeyOption
	}
	tests := []struct {
		name    string
		args    args
		wantKey *Key
	}{
		{
			name:    "single_with_string_id",
			args:    args{kind: "Kind1", options: []KeyOption{WithStringID("k1")}},
			wantKey: NewKeyWithID("Kind1", "k1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotKey := NewKey(tt.args.kind, tt.args.options...); !reflect.DeepEqual(gotKey, tt.wantKey) {
				t.Errorf("NewKey() = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}

func TestNewKeyWithIntID(t *testing.T) {
	type args struct {
		collection string
		id         int
	}
	tests := []struct {
		name string
		args args
		want *Key
	}{
		{name: "valid", args: args{collection: "kind1", id: 1}, want: &Key{collection: "kind1", ID: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewKeyWithID(tt.args.collection, tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewKeyWithID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewKeyWithID(t *testing.T) {
	type args struct {
		collection string
		id         string
	}
	tests := []struct {
		name string
		args args
		want *Key
	}{
		{name: "valid", args: args{collection: "kind1", id: "k1"}, want: &Key{collection: "kind1", ID: "k1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewKeyWithID(tt.args.collection, tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewKeyWithID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKey_String(t *testing.T) {
	type fields struct {
		parent     *Key
		collection string
		ID         any
		IDKind     reflect.Kind
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{name: "no_parent-string_id", fields: fields{ID: "k1", collection: "Kind1"}, want: "Kind1/k1"},
		{name: "no_parent-int_id", fields: fields{ID: 1, collection: "Kind1"}, want: "Kind1/1"},
		{name: "no_parent_string_id-escaped", fields: fields{ID: "k1/k2", collection: "Kind1"}, want: "Kind1/k1%2Fk2"},
		{name: "single_parent-string_id", fields: fields{ID: "k1", collection: "Kind1", parent: NewKeyWithID("Parent1", "p1")}, want: "Parent1/p1/Kind1/k1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &Key{
				parent:     tt.fields.parent,
				collection: tt.fields.collection,
				ID:         tt.fields.ID,
				IDKind:     tt.fields.IDKind,
			}
			assert.Equalf(t, tt.want, k.String(), "String()")
		})
	}
}

func TestNewIncompleteKey(t *testing.T) {
	type args struct {
		collection string
		idKind     reflect.Kind
		parent     *Key
	}
	type test struct {
		name        string
		args        args
		shouldPanic bool
	}
	for _, tt := range []test{
		{name: "valid", args: args{collection: "Kind1", idKind: reflect.String, parent: nil}, shouldPanic: false},
		{name: "invalid_id_kind", args: args{collection: "Kind1", idKind: reflect.Invalid, parent: nil}, shouldPanic: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil && tt.shouldPanic {
						t.Errorf("NewIncompleteKey() did not panic")
					}
				}()
			}
			NewIncompleteKey(tt.args.collection, tt.args.idKind, tt.args.parent)
		})
	}
}
