package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// TestSerialize_acceptsInScope confirms a fully in-scope query serializes.
func TestSerialize_acceptsInScope(t *testing.T) {
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
	if _, err := Serialize(q); err != nil {
		t.Fatalf("expected in-scope query to serialize, got: %v", err)
	}
}

// TestSerialize_rejectsOutOfScopeConstructs covers every rejection branch of the
// single validating build pass via the public Serialize entry point.
func TestSerialize_rejectsOutOfScopeConstructs(t *testing.T) {
	parentKey := record.NewKeyWithID("orgs", "org1")
	badCmp := dal.Comparison{Operator: "!=", Left: dal.Field("a"), Right: dal.Constant{Value: 1}}
	tests := []struct {
		name    string
		q       dal.StructuredQuery
		wantErr string
	}{
		{"nil query", nil, "query is nil"},
		{"nil from", fakeQuery{}, "no From source"},
		{
			"join",
			fakeQuery{from: rootFrom().Join(dal.JoinedSource{RecordsetSource: dal.NewRootCollectionRef("orders", "")})},
			"joins are not supported",
		},
		{"collection group ref", fakeQuery{from: dal.From(dal.NewCollectionGroupRef("users", ""))}, "unsupported From source"},
		{"parented collection ref", fakeQuery{from: dal.From(dal.NewCollectionRef("users", "", parentKey))}, "parented collection reference"},
		{"group by", fakeQuery{from: rootFrom(), groupBy: []dal.Expression{dal.Field("country")}}, "GroupBy is not supported"},
		{"cursor", fakeQuery{from: rootFrom(), startFrom: dal.Cursor("c1")}, "cursor (StartFrom) is not supported"},
		{"aggregate function column", fakeQuery{from: rootFrom(), columns: []dal.Column{dal.CountAs(dal.Field("id"), "cnt")}}, "column #0: unsupported expression"},
		{"unsupported column expression", fakeQuery{from: rootFrom(), columns: []dal.Column{{Expression: unsupportedExpr{}}}}, "column #0: unsupported expression"},
		{"unsupported orderBy expression", fakeQuery{from: rootFrom(), orderBy: []dal.OrderExpression{dal.Ascending(unsupportedExpr{})}}, "orderBy #0: unsupported expression"},
		{"unsupported condition type", fakeQuery{from: rootFrom(), where: unsupportedCond{}}, "unsupported condition"},
		{"unsupported comparison operator", fakeQuery{from: rootFrom(), where: badCmp}, "unsupported comparison operator"},
		{"unsupported group operator", fakeQuery{from: rootFrom(), where: dal.NewGroupCondition("XOR", dal.WhereField("a", dal.Equal, 1))}, "unsupported group operator"},
		{"unsupported expression in comparison left", fakeQuery{from: rootFrom(), where: dal.NewComparison(unsupportedExpr{}, dal.Equal, dal.Constant{Value: 1})}, "comparison left: unsupported expression"},
		{"unsupported expression in comparison right", fakeQuery{from: rootFrom(), where: dal.NewComparison(dal.Field("a"), dal.Equal, unsupportedExpr{})}, "comparison right: unsupported expression"},
		{"unsupported expression nested in group", fakeQuery{from: rootFrom(), where: dal.NewGroupCondition(dal.And, unsupportedCond{})}, "group condition #0: unsupported condition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Serialize(tt.q)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (out=%s)", tt.wantErr, out)
			}
			if out != nil {
				t.Errorf("expected no document on rejection, got: %s", out)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
