package dalgo

import (
	"github.com/pkg/errors"
	"reflect"
	"testing"
)

func TestNewRecord(t *testing.T) {
	type args struct {
		key  *Key
		data interface{}
	}
	tests := []struct {
		name string
		args args
		want Record
	}{
		{name: "nil", args: args{
			key:  NewKeyWithStrID("Kind1", "k1"),
			data: "test_data",
		}, want: &record{key: NewKeyWithStrID("Kind1", "k1")}},
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
		data interface{}
		err  error
	}
	tests := []struct {
		name   string
		fields fields
		want   interface{}
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
		{name: "", fields: fields{err: nil}, wantErr: false},
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
		data interface{}
		err  error
	}
	tests := []struct {
		name   string
		fields fields
		want   *Key
	}{
		{
			name:   "key",
			fields: fields{key: NewKeyWithStrID("Kind1", "k1")},
			want:   &Key{kind: "Kind1", ID: "k1"},
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
//		data interface{}
//		err  error
//	}
//	type args struct {
//		data interface{}
//	}
//	tests := []struct {
//		name   string
//		fields fields
//		args   args
//	}{
//		{
//			name:   "nil",
//			fields: fields{},
//			args:   args{data: "test_data"},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
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
		data interface{}
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := record{
				key:  tt.fields.key,
				data: tt.fields.data,
				err:  tt.fields.err,
			}
			err := errors.New("test error")
			v.SetError(err)
			if actualErr := v.Error(); actualErr != err {
				t.Errorf("expected %v, got: %v", err, actualErr)
			}
		})
	}
}
