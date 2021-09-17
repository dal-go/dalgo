package dalgo

import (
	"reflect"
	"testing"
)

func TestField_Validate(t *testing.T) {
	type fields struct {
		Name  string
		Value interface{}
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
			v := Field{
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
//		kind  string
//		ID    interface{}
//	}
//	type args struct {
//		child *Key
//	}
//	type want struct {
//		fields
//		parent *fields
//	}
//	tests := []struct {
//		name   string
//		fields fields
//		args   args
//		want   want
//	}{
//		{
//			name: "single_parent",
//			fields: fields{
//				kind: "Parent1",
//				ID:   "p1",
//			},
//			args: args{
//				child: NewKeyWithStrID("Kind1", "k1"),
//			},
//			want: want{
//				fields: fields{
//					kind:  "Kind1",
//					ID:    "k1",
//					level: 1,
//				},
//				parent: &fields{kind: "Parent1", ID: "p1", level: 0},
//			},
//		},
//		{
//			name: "two_parents",
//			fields: fields{
//				kind: "Parent1",
//				ID:   "p1",
//			},
//			args: args{
//				child: NewKeyWithStrID("Parent2", "p2").Child(NewKeyWithStrID("Kind1", "k1")),
//			},
//			want: want{
//				parent: &fields{kind: "Parent1", ID: "p1", level: 1},
//				fields: fields{
//					kind:  "Kind1",
//					ID:    "k1",
//					level: 2,
//				},
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			v := &Key{
//				kind: tt.fields.kind,
//				ID:   tt.fields.ID,
//			}
//			got := v.Child(tt.args.child)
//			if got.Level() != tt.want.level {
//				t.Errorf("Child().level = %v, want %v, got %v, parent %+v", got.Level(), tt.want.level, got, got.parent)
//			}
//			if got.parent == nil && tt.want.parent != nil {
//				t.Errorf("Child().parent = nil, want %+v", tt.want.parent)
//			}
//			if got.parent != nil && tt.want.parent == nil {
//				t.Error("Child().parent != nil, want nil")
//			}
//			if got.parent != nil && tt.want.parent != nil {
//				if got.parent.kind != tt.want.parent.kind {
//					t.Errorf("Child().parent.kind = %v, want %v", got.parent.kind, tt.want.parent.kind)
//				}
//				if got.parent.ID != tt.want.parent.ID {
//					t.Errorf("Child().parent.ID = %v, want %v", got.parent.ID, tt.want.parent.ID)
//				}
//				if got.parent.Level() != tt.want.parent.level {
//					t.Errorf("Child().parent.level = %v, want %v, parent %+v", got.parent.Level(), tt.want.parent.level, got.parent)
//				}
//			}
//		})
//	}
//}

func TestKey_Kind(t *testing.T) {
	type fields struct {
		kind string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{name: "empty", fields: fields{kind: ""}, want: ""},
		{name: "kind1", fields: fields{kind: "kind1"}, want: "kind1"},
		{name: "kind2", fields: fields{kind: "kind2"}, want: "kind2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Key{
				kind: tt.fields.kind,
			}
			if got := v.Kind(); got != tt.want {
				t.Errorf("Kind() = %v, want %v", got, tt.want)
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
				t.Errorf("Level() = %v, want %v, for %+v, parent %+v", got, tt.want, v, v.parent)
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
		level  int
		parent *Key
		kind   string
		ID     interface{}
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
				parent: tt.fields.parent,
				kind:   tt.fields.kind,
				ID:     tt.fields.ID,
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
		kind string
		id   int
	}
	type want struct {
		level  int
		parent *Key
		kind   string
		id     int
	}
	tests := []struct {
		name string
		args args
		want *Key
	}{
		{name: "valid", args: args{kind: "kind1", id: 1}, want: &Key{kind: "kind1", ID: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewKeyWithIntID(tt.args.kind, tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewKeyWithIntID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewKeyWithStrID(t *testing.T) {
	type args struct {
		kind string
		id   string
	}
	type want struct {
		level  int
		parent *Key
		kind   string
		id     string
	}
	tests := []struct {
		name string
		args args
		want *Key
	}{
		{name: "valid", args: args{kind: "kind1", id: "k1"}, want: &Key{kind: "kind1", ID: "k1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewKeyWithStrID(tt.args.kind, tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewKeyWithStrID() = %v, want %v", got, tt.want)
			}
		})
	}
}
