package dal

import (
	"errors"
	"fmt"
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
				parent: NewKeyWithParentAndID(NewKeyWithID("P2", "p2"), "P1", "p1"),
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

type validatableID struct {
	isValid bool
}

func (v validatableID) Validate() error {
	if v.isValid {
		return nil
	}
	return errors.New("invalid")
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
		{name: "no_parent-validatable_id-valid", wantErr: false, fields: fields{
			kind: "Kind1", ID: validatableID{isValid: true},
		}},
		{name: "no_parent-validatable_id-invalid", wantErr: true, fields: fields{
			kind: "Kind1", ID: validatableID{isValid: false},
		}},
		{name: "no_parent-field_val", wantErr: false, fields: fields{
			kind: "Kind1",
			ID: []FieldVal{
				{Name: "f1", Value: "v1"},
				{Name: "f2", Value: "v2"},
			},
		}},
		{name: "no_parent-invalid_field_val", wantErr: true, fields: fields{
			kind: "Kind1",
			ID: []FieldVal{
				{Name: ""},
			},
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

func TestNewKeyWithOptions(t *testing.T) {
	type args struct {
		collection string
		options    []KeyOption
	}
	tests := []struct {
		name    string
		args    args
		wantKey *Key
		wantErr bool
	}{
		{
			name:    "single_with_string_id",
			args:    args{collection: "Kind1", options: []KeyOption{WithStringID("k1")}},
			wantKey: &Key{collection: "Kind1", ID: "k1"},
		},
		{
			name:    "empty_collection_arg",
			args:    args{collection: ""},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := NewKeyWithOptions(tt.args.collection, tt.args.options...)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Nil(t, key)
			} else {
				assert.Nil(t, err)
				if !reflect.DeepEqual(key, tt.wantKey) {
					t.Errorf("NewKey() = %v, want %v", key, tt.wantKey)
				}
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
		name        string
		args        args
		want        *Key
		shouldPanic string
	}{
		{name: "valid", args: args{collection: "kind1", id: "k1"}, want: &Key{collection: "kind1", ID: "k1"}},
		{name: "empty_collection", shouldPanic: "collection is a required parameter", args: args{collection: "", id: "k1"}, want: &Key{collection: "kind1", ID: "k1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic != "" {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("NewKeyWithID() did not panic")
					} else {
						switch s := r.(type) {
						case string:
							if s != tt.shouldPanic {
								t.Errorf("NewKeyWithID() panic = %v, want %v", s, tt.shouldPanic)
							}
						}
					}
				}()
			}
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
		name        string
		fields      fields
		want        string
		shouldPanic bool
	}{
		{name: "should_panic_on_invalid_key", shouldPanic: true, fields: fields{ID: nil, collection: "", IDKind: reflect.Invalid}, want: "Kind1/1"},
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
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected to panic")
					}
				}()
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

func TestKey_CollectionPath(t *testing.T) {
	type fields struct {
		parent     *Key
		collection string
		ID         any
		IDKind     reflect.Kind
	}
	tests := []struct {
		name        string
		fields      fields
		want        string
		shouldPanic bool
	}{
		{name: "should_panic_on_invalid_key", shouldPanic: true, fields: fields{ID: nil, collection: "", IDKind: reflect.Invalid}, want: "Kind1/1"},
		{name: "no_parent-string_id", fields: fields{ID: "k1", collection: "Kind1"}, want: "Kind1"},
		{name: "no_parent-int_id", fields: fields{ID: 1, collection: "Kind1"}, want: "Kind1"},
		{name: "no_parent_string_id-escaped", fields: fields{ID: "k1/k2", collection: "Kind1"}, want: "Kind1"},
		{name: "single_parent-string_id", fields: fields{ID: "k1", collection: "Kind1", parent: NewKeyWithID("Parent1", "p1")}, want: "Parent1/Kind1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &Key{
				parent:     tt.fields.parent,
				collection: tt.fields.collection,
				ID:         tt.fields.ID,
				IDKind:     tt.fields.IDKind,
			}
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected to panic")
					}
				}()
			}
			assert.Equal(t, tt.want, k.CollectionPath())
		})
	}
}

func TestKey_Collection(t *testing.T) {
	for _, tt := range []struct {
		name       string
		collection string
	}{
		{name: "empty", collection: ""},
		{name: "with_value", collection: "records"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := Key{collection: tt.collection}
			assert.Equal(t, tt.collection, key.Collection())
		})
	}
}

func TestWithID(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   string
	}{
		{name: "empty_id", id: ""},
		{name: "non_empty_id", id: "id1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := Key{}
			err := WithID(tt.id)(&key)
			assert.Nil(t, err)
			assert.Equal(t, tt.id, key.ID)
		})
	}
}

func TestWithFields(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   []FieldVal
	}{
		{name: "nil", id: nil},
		{name: "single_field", id: []FieldVal{{Name: "f1", Value: "v1"}}},
		{name: "multiple_fields", id: []FieldVal{{Name: "f1", Value: "v1"}, {Name: "f2", Value: "v2"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := Key{}
			err := WithFields(tt.id)(&key)
			assert.Nil(t, err)
			assert.Equal(t, tt.id, key.ID)
		})
	}
}

func TestNewKeyWithFields(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fields []FieldVal
	}{
		{name: "nil", fields: nil},
		{name: "single_field", fields: []FieldVal{{Name: "f1", Value: "v1"}}},
		{name: "multiple_fields", fields: []FieldVal{{Name: "f1", Value: "v1"}, {Name: "f2", Value: "v2"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := NewKeyWithFields("collection1", tt.fields...)
			assert.Equal(t, "collection1", key.Collection())
			assert.Equal(t, tt.fields, key.ID.([]FieldVal))
		})
	}
}

func TestKey_Equal(t *testing.T) {
	k11 := &Key{collection: "collection1", ID: "id1"}
	k12 := &Key{collection: "collection1", ID: "id2"}

	k21 := &Key{collection: "collection2", ID: "id1"}

	for _, tt := range []struct {
		name     string
		k1       *Key
		k2       *Key
		expected bool
	}{
		{
			name:     "both nil",
			k1:       nil,
			k2:       nil,
			expected: true,
		},
		{
			name:     "arg_nil",
			k1:       k11,
			k2:       nil,
			expected: false,
		},
		{
			name:     "same",
			k1:       k11,
			k2:       k11,
			expected: true,
		},
		{
			name:     "equal",
			k1:       k11,
			k2:       &Key{collection: "collection1", ID: "id1"},
			expected: true,
		},
		{
			name:     "same_id_different_collection",
			k1:       k11,
			k2:       k21,
			expected: false,
		},
		{
			name:     "same_collection_different_id",
			k1:       k11,
			k2:       k12,
			expected: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.k1.Equal(tt.k2)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestReverseStringsJoin(t *testing.T) {
	type args struct {
		elems []string
		sep   string
	}
	tests := []struct {
		name        string
		args        args
		want        string
		shouldPanic int
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
		{
			name:        "panics1",
			args:        args{elems: []string{"el1", "el2", "el3"}, sep: "/"},
			shouldPanic: 1,
		},
		{
			name:        "panics2",
			args:        args{elems: []string{"el1", "el2", "el3"}, sep: "/"},
			shouldPanic: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if tt.shouldPanic > 0 {
					r := recover()
					if r == nil {
						t.Errorf("panic expected")
						return
					}
					s := fmt.Sprintf("%v", r)
					assert.Equal(t, fmt.Sprintf("force panic %d", tt.shouldPanic), s)
				}
			}()
			var forcePanic []bool
			if tt.shouldPanic > 0 {
				forcePanic = make([]bool, tt.shouldPanic)
			}
			got := reverseStringsJoin(tt.args.elems, tt.args.sep, forcePanic...)
			if got != tt.want {
				t.Errorf("reverseStringsJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}
