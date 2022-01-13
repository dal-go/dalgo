package demo

import (
	"context"
	"errors"
	"github.com/strongo/dalgo/dal"
	"github.com/strongo/dalgo/dalmock"
	"reflect"
	"testing"
)

func TestSelectUserByEmail(t *testing.T) {
	type args struct {
		ctx   context.Context
		db    dal.ReadSession
		email string
	}
	dbMock := dalmock.NewDbMock()
	tests := []struct {
		name         string
		args         args
		selectResult dalmock.SelectResult
		want         *userData
		wantErr      error
	}{
		{
			name: "should return nil",
			args: args{
				db:    dbMock,
				ctx:   context.Background(),
				email: "unknown@example.com",
			},
			selectResult: dalmock.NewSelectResult(
				nil,
				dal.ErrRecordNotFound,
			),
			want:    nil,
			wantErr: dal.ErrRecordNotFound,
		},
		{
			name: "should succeed",
			args: args{
				db:    dbMock,
				ctx:   context.Background(),
				email: "test@example.com",
			},
			selectResult: dalmock.NewSelectResult(
				func(into func() interface{}) dal.Reader {
					return dalmock.NewSingleRecordReader(
						dal.NewKeyWithIntID("users", 1),
						`{"email":"test@example.com"}`,
						into,
					)
				}, nil,
			),
			want:    &userData{Email: "test@example.com"},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbMock.ForSelect(dal.Select{
				From: User.Collection(),
			}).Return(tt.selectResult)
			got := &userData{}
			err := SelectUserByEmail(tt.args.ctx, tt.args.db, tt.args.email, got)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("SelectUserByEmail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("SelectUserByEmail() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
