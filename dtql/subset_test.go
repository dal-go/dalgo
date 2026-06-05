package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestCheckInScope_accepts(t *testing.T) {
	q := fakeQuery{
		from: rootFrom(),
		where: dal.NewGroupCondition(dal.And,
			dal.WhereField("age", dal.GreaterOrEqual, 18),
			dal.WhereField("status", dal.In, []string{"active", "pending"}),
		),
		columns: []dal.Column{{Expression: dal.Field("name")}},
		orderBy: []dal.OrderExpression{dal.AscendingField("name")},
		limit:   10,
		offset:  20,
	}
	if err := checkInScope(q); err != nil {
		t.Fatalf("expected in-scope query to be accepted, got: %v", err)
	}
}

func TestCheckInScope_rejects(t *testing.T) {
	parentKey := dal.NewKeyWithID("orgs", "org1")
	tests := []struct {
		name    string
		q       dal.StructuredQuery
		wantErr string
	}{
		{
			name:    "nil from",
			q:       fakeQuery{},
			wantErr: "no From source",
		},
		{
			name:    "join",
			q:       fakeQuery{from: rootFrom().Join(dal.JoinedSource{RecordsetSource: dal.NewRootCollectionRef("orders", "")})},
			wantErr: "joins are not supported",
		},
		{
			name:    "collection group ref",
			q:       fakeQuery{from: dal.From(dal.NewCollectionGroupRef("users", ""))},
			wantErr: "collection group references are not supported",
		},
		{
			name:    "parented collection ref",
			q:       fakeQuery{from: dal.From(dal.NewCollectionRef("users", "", parentKey))},
			wantErr: "parented collection reference",
		},
		{
			name:    "group by",
			q:       fakeQuery{from: rootFrom(), groupBy: []dal.Expression{dal.Field("country")}},
			wantErr: "GroupBy is not supported",
		},
		{
			name:    "cursor",
			q:       fakeQuery{from: rootFrom(), startFrom: dal.Cursor("c1")},
			wantErr: "cursor (StartFrom) is not supported",
		},
		{
			name:    "aggregate function column",
			q:       fakeQuery{from: rootFrom(), columns: []dal.Column{dal.CountAs(dal.Field("id"), "cnt")}},
			wantErr: "unsupported expression",
		},
		{
			name:    "unsupported comparison operator",
			q:       fakeQuery{from: rootFrom(), where: dal.Comparison{Operator: "!=", Left: dal.Field("a"), Right: dal.Constant{Value: 1}}},
			wantErr: "unsupported comparison operator",
		},
		{
			name:    "unsupported group operator",
			q:       fakeQuery{from: rootFrom(), where: dal.NewGroupCondition("XOR", dal.WhereField("a", dal.Equal, 1))},
			wantErr: "unsupported group operator",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkInScope(tt.q)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
