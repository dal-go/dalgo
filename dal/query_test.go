package dal

import (
	"github.com/dal-go/dalgo/query"
	"github.com/dal-go/dalgo/query/constant"
	"testing"
)

func TestSelect_String(t *testing.T) {
	type fields struct {
		From    *CollectionRef
		Where   query.Condition
		GroupBy []query.Expression
		OrderBy []query.Expression
		Columns []query.Column
		Into    func() interface{}
		Limit   int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "select_1",
			fields: fields{
				Columns: []query.Column{
					{Expression: constant.Int(1)},
				},
			},
			want: "SELECT 1",
		},
		{
			name: "select_'abc'_AS_first_col",
			fields: fields{
				Columns: []query.Column{
					{Expression: constant.Str("abc"), Alias: "first_col"},
				},
			},
			want: "SELECT 'abc' AS first_col",
		},
		{
			name: "select_*_from_User",
			fields: fields{
				From: &CollectionRef{Name: "User"},
			},
			want: "SELECT * FROM [User]",
		},
		{
			name: "select_*_from_Users_where_SomeID_=_123",
			fields: fields{
				From:  &CollectionRef{Name: "Users"},
				Where: query.ID("SomeID", 123),
			},
			want: "SELECT * FROM [Users] WHERE [SomeID] = 123",
		},
		{
			name: "select_*_from_User_where_Email_=_'test@example.com'",
			fields: fields{
				From:  &CollectionRef{Name: "User"},
				Where: query.NewComparison(query.Equal, query.Field("Email"), query.String("test@example.com")),
			},
			want: "SELECT * FROM [User] WHERE [Email] = 'test@example.com'",
		},
		{
			name: "select top 7 * from User",
			fields: fields{
				From:  &CollectionRef{Name: "User"},
				Limit: 7,
			},
			want: "SELECT TOP 7 * FROM [User]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Select{
				From:    tt.fields.From,
				Where:   tt.fields.Where,
				GroupBy: tt.fields.GroupBy,
				OrderBy: tt.fields.OrderBy,
				Columns: tt.fields.Columns,
				Into:    tt.fields.Into,
				Limit:   tt.fields.Limit,
			}
			if got := q.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			} else {
				t.Log(got)
			}
		})
	}
}
