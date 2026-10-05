package dal

import (
	"context"
	"strings"
	"testing"
)

func TestNilNestedQueryIsRefusedBeforeProviderReads(t *testing.T) {
	base := NewRootCollectionRef("Invoice", "i")
	queries := []struct {
		name  string
		query StructuredQuery
	}{
		{"exists", From(base).NewQuery().Where(NewExistsCondition(nil)).SelectColumns(Column{Expression: Field("value")})},
		{"scalar", From(base).NewQuery().SelectColumns(Column{Expression: NewQueryExpression(nil, "v")})},
		{"derived source", From(NewQuerySource(nil, "d")).NewQuery().SelectColumns(Column{Expression: Field("value")})},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			if queryTreeHoldsOrderedAggregate(tc.query) {
				t.Fatal("nil nested query was reported as an ordered aggregate")
			}
			if err := ValidateAggregation(tc.query); err == nil || !strings.Contains(err.Error(), "query_shape") {
				t.Fatalf("validation = %v, want query_shape", err)
			}
			stub := &orderedStub{}
			db := NewDB(stub)
			if _, err := db.ExecuteQueryToRecordsReader(context.Background(), tc.query); err == nil || !strings.Contains(err.Error(), "query_shape") {
				t.Fatalf("records reader = %v, want query_shape", err)
			}
			if _, err := db.ExecuteQueryToRecordsetReader(context.Background(), tc.query); err == nil || !strings.Contains(err.Error(), "query_shape") {
				t.Fatalf("recordset reader = %v, want query_shape", err)
			}
			if reads := stub.readsReached(); reads != 0 {
				t.Fatalf("provider reads = %d, want 0", reads)
			}
		})
	}
}
