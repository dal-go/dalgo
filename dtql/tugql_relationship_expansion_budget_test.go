package dtql

import (
	"fmt"
	"testing"
)

func TestTugQLPublicInferredCompositeRelationshipExpansionStaysBounded(t *testing.T) {
	fields := make([]TugQLField, 1000)
	pairs := make([]TugQLRelationshipPair, 1000)
	for i := range fields {
		name := fmt.Sprintf("F%d", i)
		fields[i] = TugQLField{Name: name, Type: "integer", Authorized: true}
		pairs[i] = TugQLRelationshipPair{FromField: name, ToField: name}
	}
	ctx := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{Name: "A", Fields: fields}, {Name: "B", Fields: fields}}}},
		Relationships:     []TugQLRelationship{{ID: "large-composite", Version: "r1", From: TugQLRelationEndpoint{Table: "A"}, To: TugQLRelationEndpoint{Table: "B"}, Pairs: pairs, ExactTypedEquality: true}},
	}
	for _, source := range []string{
		"from A as a\njoin B as b\nselect a.F0\n",
		"with Joined as (\n  from A as a\n  join B as b\n  select a.F0\n)\nfrom Joined\nselect F0\n",
	} {
		d, ds := ParseTugQL(source)
		if len(ds) != 0 || d.Tree == nil {
			t.Fatalf("small authored query failed parse: %+v", ds)
		}
		r, ds := ResolveTugQL(d, ctx)
		if r.Query != nil || len(ds) != 1 || ds[0].Code != "document_node_limit" {
			t.Fatalf("expanded relationship escaped budget: executable=%v diagnostics=%+v", r.Query != nil, ds)
		}
	}
}
