package dal

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewRecord(t *testing.T) {
	type args struct {
		key  *Key
		data any
	}
	tests := []struct {
		name string
		args args
		want Record
	}{
		{name: "nil", args: args{
			key:  NewKeyWithID("Kind1", "k1"),
			data: "test_data",
		}, want: &record{key: NewKeyWithID("Kind1", "k1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRecord(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_record_Data(t *testing.T) {
	type fields struct {
		key  *Key
		data any
		err  error
	}
	tests := []struct {
		name   string
		fields fields
		want   any
	}{
		{name: "string", fields: fields{data: "test"}, want: "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := record{
				key:  tt.fields.key,
				data: tt.fields.data,
				err:  tt.fields.err,
			}
			v.SetError(nil)
			if got := v.Data(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Data() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_record_Error(t *testing.T) {
	type fields struct {
		err error
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{name: "nil", fields: fields{err: nil}, wantErr: false},
		{name: "not_found", fields: fields{err: ErrRecordNotFound}, wantErr: false}, // TODO: should it return error?
		{name: "with_error", fields: fields{err: errors.New("some_error")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := record{
				err: tt.fields.err,
			}
			if err := v.Error(); (err != nil) != tt.wantErr {
				t.Errorf("Error() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_record_Key(t *testing.T) {
	type fields struct {
		key  *Key
		data any
		err  error
	}
	tests := []struct {
		name   string
		fields fields
		want   *Key
	}{
		{
			name:   "key",
			fields: fields{key: NewKeyWithID("Kind1", "k1")},
			want:   &Key{collection: "Kind1", ID: "k1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := record{
				key:  tt.fields.key,
				data: tt.fields.data,
				err:  tt.fields.err,
			}
			if got := v.Key(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Key() = %v, want %v", got, tt.want)
			}
		})
	}
}

//func Test_record_SetData(t *testing.T) {
//	type fields struct {
//		key  *Key
//		data any
//		err  error
//	}
//	type args struct {
//		data any
//	}
//	tests := []struct {
//		Name   string
//		fields fields
//		args   args
//	}{
//		{
//			Name:   "nil",
//			fields: fields{},
//			args:   args{data: "test_data"},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.Name, func(t *testing.T) {
//			v := record{
//				key:  tt.fields.key,
//				data: tt.fields.data,
//				err:  tt.fields.err,
//			}
//			v.SetData(tt.args.data)
//			if v.data != tt.args.data {
//				t.Errorf("expected %v, got: %v", tt.args.data, v.data)
//			}
//		})
//	}
//}

func Test_record_SetError(t *testing.T) {
	type fields struct {
		key  *Key
		data any
		err  error
	}
	type args struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "set_nil_over_nil",
			fields: fields{
				err: nil,
			},
			args: args{err: nil},
		},
		{
			name: "set_err_over_nil",
			fields: fields{
				err: nil,
			},
			args: args{err: errors.New("test error")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := record{
				key:  tt.fields.key,
				data: tt.fields.data,
				err:  tt.fields.err,
			}
			v.SetError(tt.args.err)
			if !(tt.args.err == nil && v.err == NoError) && v.err != tt.args.err {
				t.Errorf("expected %v, got: %v", tt.args.err, v.err)
			}
		})
	}
}

func Test_record_MarkAsChanged(t *testing.T) {
	type fields struct {
		key     *Key
		err     error
		changed bool
		data    any
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name:   "from_false_to_true",
			fields: fields{changed: false},
		},
		{
			name:   "from_true_to_true",
			fields: fields{changed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &record{
				key:     tt.fields.key,
				err:     tt.fields.err,
				changed: tt.fields.changed,
				data:    tt.fields.data,
			}
			v.MarkAsChanged()
			if v.changed != true {
				t.Errorf("failed to mark as changed")
			}
		})
	}
}

func TestRecord_Exists(t *testing.T) {
	for _, tt := range []struct {
		name        string
		r           Record
		shouldPanic bool
		expected    bool
	}{
		{name: "panics_if_exists_not_set", r: &record{key: NewKeyWithID("Kind1", "k1")}, shouldPanic: true},
		{name: "exists", r: (&record{key: NewKeyWithID("Kind1", "k1")}).SetError(nil), expected: true},
		{name: "does_not_exist", r: (&record{key: NewKeyWithID("Kind1", "k1")}).SetError(ErrRecordNotFound), expected: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic")
					}
				}()
			}
			actual := tt.r.Exists()
			if actual != tt.expected {
				t.Errorf("expected %v, got: %v", tt.expected, actual)
			}
		})
	}
}

func TestRecord_HasChanged(t *testing.T) {
	for _, tt := range []struct {
		name     string
		r        Record
		expected bool
	}{
		{name: "changed", r: &record{changed: true}, expected: true},
		{name: "not_changed", r: &record{changed: false}, expected: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.r.HasChanged()
			if actual != tt.expected {
				t.Errorf("expected %v, got: %v", tt.expected, actual)
			}
		})
	}
}

func TestRecord_Data(t *testing.T) {
	for _, tt := range []struct {
		name        string
		data        any
		r           *record
		shouldPanic bool
	}{
		{name: "panics_if_err_is_not_set", shouldPanic: true, r: &record{key: NewKeyWithID("Kind1", "k1")}},
		{name: "panics_if_is_not_found", shouldPanic: true, r: (&record{key: NewKeyWithID("Kind1", "k1")}).setError(ErrRecordNotFound)},
		{name: "nil", r: (&record{key: NewKeyWithID("Kind1", "k1")}).setError(NoError)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.r.data = tt.data
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic")
					}
				}()
			}
			data := tt.r.Data()
			if data != tt.data {
				t.Errorf("expected %v, got: %v", tt.data, data)
			}
		})
	}
}
