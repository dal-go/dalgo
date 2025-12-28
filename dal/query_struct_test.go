package dal

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/constant"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/assert"
)

type mockQueryExecutor struct {
	QueryExecutor
}

func (m mockQueryExecutor) ExecuteQueryToRecordsReader(ctx context.Context, query Query) (RecordsReader, error) {
	return nil, nil
}

func (m mockQueryExecutor) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	return nil, nil
}

func TestSelect(t *testing.T) {
	tests := []struct {
		name string
		q    structuredQuery
		want string
	}{
		{
			name: "select_1",
			q: structuredQuery{
				columns: []Column{
					{Expression: constant.Int(1)},
				},
			},
			want: "SELECT 1",
		},
		{
			name: "select_'abc'_AS_first_col",
			q: structuredQuery{
				columns: []Column{
					{Expression: constant.Str("abc"), Alias: "first_col"},
				},
			},
			want: "SELECT 'abc' AS first_col",
		},
		{
			name: "select_*_from_User",
			q: structuredQuery{
				from: From(&CollectionRef{name: "User"}),
			},
			want: "SELECT * FROM [User]",
		},
		{
			name: "select_top_10_*_from_User",
			q: structuredQuery{
				from:  From(&CollectionRef{name: "User"}),
				limit: 10,
			},
			want: "SELECT TOP 10 * FROM [User]",
		},
		{
			name: "select_top_10_*_from_User_offset_20",
			q: structuredQuery{
				from:   From(&CollectionRef{name: "User"}),
				limit:  10,
				offset: 20,
			},
			want: "SELECT TOP 10 * FROM [User]\nOFFSET 20",
		},
		{
			name: "select_*_from_Users_where_SomeID_=_123",
			q: structuredQuery{
				from:  From(&CollectionRef{name: "Users"}),
				where: ID("SomeID", 123),
			},
			want: "SELECT * FROM [Users] WHERE SomeID = 123",
		},
		{
			name: "select_*_from_User_where_Email_=_'test@example.com'",
			q: structuredQuery{
				from: From(&CollectionRef{name: "User"}),
				columns: []Column{
					{Expression: Field("id")},
					{Expression: Field("name")},
					{Expression: Field("email")},
				},
				where: NewComparison(FieldRef{name: "Email"}, Equal, String("test@example.com")),
			},
			want: "SELECT\n\tid,\n\tname,\n\temail\nFROM [User]\nWHERE Email = 'test@example.com'",
		},
		{
			name: "select top 7 * from User",
			q: structuredQuery{
				from:  From(&CollectionRef{name: "User"}),
				limit: 7,
			},
			want: "SELECT TOP 7 * FROM [User]",
		},
		{
			name: "select top 7 * from User order by Email, Created DESC",
			q: structuredQuery{
				from:  From(&CollectionRef{name: "User"}),
				limit: 7,
				orderBy: []OrderExpression{
					Ascending(Field("Email")),
					Descending(Field("Created")),
				},
			},
			want: "SELECT TOP 7 * FROM [User]\nORDER BY Email, Created DESC",
		},
		{ // TODO: Demo test generation
			name: "select top 7 * from User order by Email, Created DESC group by Email, Created",
			q: structuredQuery{
				from:  From(&CollectionRef{name: "User"}),
				limit: 7,
				orderBy: []OrderExpression{
					Ascending(Field("Email")),
					Descending(Field("Created")),
				},
				groupBy: []Expression{
					Field("Email"),
					Field("Created"),
				},
			},
			want: "SELECT TOP 7 * FROM [User]\nGROUP BY Email, Created\nORDER BY Email, Created DESC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("From", func(t *testing.T) {
				assert.Equal(t, tt.q.from, tt.q.From())
			})
			t.Run("Where", func(t *testing.T) {
				assert.Equal(t, tt.q.where, tt.q.Where())
			})
			t.Run("GroupBy", func(t *testing.T) {
				assert.Equal(t, tt.q.groupBy, tt.q.GroupBy())
			})
			t.Run("OrderBy", func(t *testing.T) {
				assert.Equal(t, tt.q.orderBy, tt.q.OrderBy())
			})
			t.Run("Columns", func(t *testing.T) {
				assert.Equal(t, tt.q.columns, tt.q.Columns())
			})
			t.Run("IntoRecord", func(t *testing.T) {
				if tt.q.intoRecord == nil {
					assert.Nil(t, tt.q.IntoRecord())
					return
				}
				assert.Equal(t, tt.q.intoRecord(), tt.q.IntoRecord())
			})
			t.Run("IDKind", func(t *testing.T) {
				assert.Equal(t, tt.q.idKind, tt.q.IDKind())
			})
			t.Run("StartFrom", func(t *testing.T) {
				assert.Equal(t, tt.q.startCursor, tt.q.StartFrom())
			})
			t.Run("Offset", func(t *testing.T) {
				assert.Equal(t, tt.q.offset, tt.q.Offset())
			})
			t.Run("Limit", func(t *testing.T) {
				assert.Equal(t, tt.q.limit, tt.q.Limit())
			})
			t.Run("String", func(t *testing.T) {
				if got := tt.q.String(); got != tt.want {
					t.Errorf("Got:\n%v\n\nWant:\n%v", got, tt.want)
				}
			})
			t.Run("And", func(t *testing.T) {
				q := tt.q.And(NewComparison(FieldRef{name: "Email"}, Equal, String("test@example.com")))
				assert.NotEqual(t, tt.q, q)
			})
			t.Run("Or", func(t *testing.T) {
				q := tt.q.Or(NewComparison(FieldRef{name: "Email"}, Equal, String("test@example.com")))
				assert.NotEqual(t, tt.q, q)
			})
			t.Run("GetRecordsReader", func(t *testing.T) {
				_, _ = tt.q.GetRecordsReader(context.Background(), mockQueryExecutor{})
			})
			t.Run("GetRecordsetReader", func(t *testing.T) {
				_, _ = tt.q.GetRecordsetReader(context.Background(), mockQueryExecutor{})
			})
			t.Run("Text", func(t *testing.T) {
				_ = tt.q.Text()
			})
			t.Run("IntoRecord", func(t *testing.T) {
				_ = tt.q.IntoRecord()
			})
		})
	}
}
