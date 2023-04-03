package demo

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/strongo/dalgo/dal"
	"github.com/strongo/dalgo/mock_dal"
	"reflect"
	"testing"
)

func TestSelectUserByEmail(t *testing.T) {
	type args struct {
		ctx   context.Context
		db    dal.ReadSession
		email string
	}
	mockCtrl := gomock.NewController(t)
	dbMock := mock_dal.NewMockDatabase(mockCtrl)
	tests := []struct {
		name         string
		args         args
		selectResult mock_dal.SelectResult
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
			selectResult: mock_dal.NewSelectResult(
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
			selectResult: mock_dal.NewSelectResult(
				func(into func() interface{}) dal.Reader {
					return mock_dal.NewSingleRecordReader(
						dal.NewKeyWithID("users", 1),
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
			dbMock.EXPECT().
				Select(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query dal.Select) (dal.Reader, error) {
					return tt.selectResult.Reader(query.Into), tt.selectResult.Err
				})
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
