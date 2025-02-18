package dal

//import (
//	"errors"
//	"testing"
//)
//
//func TestNewReturnError(t *testing.T) {
//	type args struct {
//		err error
//	}
//	tests := []struct {
//		name string
//		args args
//	}{
//		{
//			name: "nil",
//			args: args{
//				err: nil,
//			},
//		},
//		{name: "not_nil",
//			args: args{
//				err: errors.New("test"),
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			err := ReturnError(tt.args.err)
//			if err == nil {
//				t.Errorf("ReturnError(%v) = nil", tt.args.err)
//				return
//			}
//			var returnErr returnError
//			if ok := errors.As(err, &returnErr); !ok {
//				t.Errorf("ReturnError(%v) = %T; want returnError", tt.args.err, err)
//				return
//			}
//			if !errors.Is(returnErr.err, tt.args.err) {
//				t.Errorf("ReturnError(%v) = %v; want %v", tt.args.err, returnErr.err, tt.args.err)
//				return
//			}
//		})
//	}
//}
