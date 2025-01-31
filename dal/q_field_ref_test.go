package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFieldRef_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    FieldRef
		b    FieldRef
		want bool
	}{
		{
			name: "empty",
			want: true,
		},
		{
			name: "same_name",
			a:    FieldRef{name: "n1"},
			b:    FieldRef{name: "n1"},
			want: true,
		},
		{
			name: "different_names",
			a:    FieldRef{name: "n1"},
			b:    FieldRef{name: "n2"},
			want: false,
		},
		{
			name: "different_isID",
			a:    FieldRef{isID: true},
			b:    FieldRef{isID: false},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.Equal(tt.b))
		})
	}
}

func TestFieldRef_EqualTo(t *testing.T) {
	var nilValue any = nil
	tests := []struct {
		name     string
		fieldRef FieldRef
		input    any
		want     Condition
	}{
		{
			name:     "empty_nil",
			fieldRef: FieldRef{name: "f1"},
			input:    nil,
			want:     Comparison{Left: Field("f1"), Operator: Equal, Right: Constant{Value: nilValue}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			actual := tt.fieldRef.EqualTo(tt.input)
			assert.Equalf(t, tt.want, actual, "EqualTo(%v)", tt.input)
		})
	}
}

func TestFieldRef_String(t *testing.T) {
	tests := []struct {
		name     string
		fieldRef FieldRef
		want     string
	}{
		{
			name:     "empty",
			fieldRef: FieldRef{},
			want:     "[]",
		},
		{
			name:     "no_escaping",
			fieldRef: FieldRef{name: "f1"},
			want:     "f1",
		},
		{
			name:     "with_escaping",
			fieldRef: FieldRef{name: "f 1"},
			want:     "[f 1]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.fieldRef.String()
			assert.Equalf(t, tt.want, actual, "String()")
		})
	}
}

func TestRequiresEscaping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty",
			input: "",
			want:  true,
		},
		{
			name:  "letter",
			input: "a",
			want:  false,
		},
		{
			name:  "number",
			input: "123",
			want:  false,
		},
		{
			name:  "alpha_numeric",
			input: "a1b2",
			want:  false,
		},
		{
			name:  "space",
			input: "a 1",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := RequiresEscaping(tt.input)
			assert.Equalf(t, tt.want, actual, "RequiresEscaping(%v)", tt.input)
		})
	}
}

func TestWhereField(t *testing.T) {
	type args struct {
		name     string
		operator Operator
		v        any
	}
	tests := []struct {
		name        string
		args        args
		want        Condition
		shouldPanic bool
	}{
		{
			name: "string",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        "v1",
			},
			want: Comparison{Left: Field("f1"), Operator: Equal, Right: Constant{Value: "v1"}},
		},
		{
			name: "int",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        123,
			},
			want: Comparison{Left: Field("f1"), Operator: Equal, Right: Constant{Value: 123}},
		},
		{
			name: "nil",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        nil,
			},
			want: Comparison{Left: Field("f1"), Operator: Equal, Right: Constant{Value: nil}},
		},
		{
			name: "Constant",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        Constant{Value: 123},
			},
			want: Comparison{Left: Field("f1"), Operator: Equal, Right: Constant{Value: 123}},
		},
		{
			name: "FieldRef",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        FieldRef{name: "f2"},
			},
			want: Comparison{Left: Field("f1"), Operator: Equal, Right: FieldRef{name: "f2"}},
		},
		{
			name: "key",
			args: args{
				name:     "f1",
				operator: Equal,
				v:        &Key{},
			},
			shouldPanic: true, // TODO: might be wrong, we might want to  filter by key value?
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("WhereField() should have panicked!")
					}
				}()
			}
			condition := WhereField(tt.args.name, tt.args.operator, tt.args.v)
			assert.Equal(t, tt.want, condition)
		})
	}
}
