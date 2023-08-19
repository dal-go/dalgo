package dal

import (
	"github.com/dal-go/dalgo/constant"
	"testing"
)

func TestSelect_String(t *testing.T) {
	type fields struct {
		From    *CollectionRef
		Where   Condition
		GroupBy []Expression
		OrderBy []OrderExpression
		Columns []Column
		Into    func() Record
		limit   int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "select_1",
			fields: fields{
				Columns: []Column{
					{Expression: constant.Int(1)},
				},
			},
			want: "SELECT 1",
		},
		{
			name: "select_'abc'_AS_first_col",
			fields: fields{
				Columns: []Column{
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
				Where: ID("SomeID", 123),
			},
			want: "SELECT * FROM [Users] WHERE SomeID = 123",
		},
		{
			name: "select_*_from_User_where_Email_=_'test@example.com'",
			fields: fields{
				From:  &CollectionRef{Name: "User"},
				Where: NewComparison(FieldRef{Name: "Email"}, Equal, String("test@example.com")),
			},
			want: "SELECT * FROM [User] WHERE Email = 'test@example.com'",
		},
		{
			name: "select top 7 * from User",
			fields: fields{
				From:  &CollectionRef{Name: "User"},
				limit: 7,
			},
			want: "SELECT TOP 7 * FROM [User]",
		},
		{
			name: "select top 7 * from User order by Email, Created DESC",
			fields: fields{
				From:  &CollectionRef{Name: "User"},
				limit: 7,
				OrderBy: []OrderExpression{
					Ascending(Field("Email")),
					Descending(Field("Created")),
				},
			},
			want: "SELECT TOP 7 * FROM [User]\nORDER BY Email, Created DESC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := theQuery{
				from:    tt.fields.From,
				where:   tt.fields.Where,
				groupBy: tt.fields.GroupBy,
				orderBy: tt.fields.OrderBy,
				columns: tt.fields.Columns,
				into:    tt.fields.Into,
				limit:   tt.fields.limit,
			}
			if got := q.String(); got != tt.want {
				t.Errorf("Got:\n%v\n\nWant:\n%v", got, tt.want)
			} else {
				t.Log(got)
			}
		})
	}
}
