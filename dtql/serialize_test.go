package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// fullQuery builds an in-scope query exercising every in-scope node: a root
// From, columns, a Where combining Comparison (with inline Constant and Array)
// under And/Or groups, OrderBy, Limit and Offset.
func fullQuery() dal.StructuredQuery {
	return fakeQuery{
		from: rootFrom(),
		columns: []dal.Column{
			{Expression: dal.Field("name")},
			{Expression: dal.Field("age"), Alias: "years"},
		},
		where: dal.NewGroupCondition(dal.And,
			dal.WhereField("age", dal.GreaterOrEqual, 18),
			dal.NewGroupCondition(dal.Or,
				dal.WhereField("status", dal.In, []string{"active", "pending"}),
				dal.WhereField("country", dal.Equal, "US"),
			),
		),
		orderBy: []dal.OrderExpression{
			dal.AscendingField("name"),
			dal.DescendingField("age"),
		},
		limit:  10,
		offset: 20,
	}
}

func TestSerialize_subsetNodesRepresented(t *testing.T) {
	out, err := Serialize(fullQuery())
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	yamlStr := string(out)
	t.Logf("DTQL-YAML:\n%s", yamlStr)

	// The output must be plain YAML parseable by a standard library.
	var generic map[string]any
	if err := yaml.Unmarshal(out, &generic); err != nil {
		t.Fatalf("serializer output is not plain YAML: %v", err)
	}

	// Every in-scope node must have a representation present.
	for _, want := range []string{
		"from:", "name: users",
		"columns:", "field: name", "as: years",
		"where:", "and:", "or:",
		"op: '>='", "op: In", "op: ==",
		"value: 18", "values:", "active", "pending",
		"orderBy:", "desc: true",
		"limit: 10", "offset: 20",
	} {
		if !strings.Contains(yamlStr, want) {
			t.Errorf("serialized DTQL is missing representation %q", want)
		}
	}
}

func TestSerialize_rejectsOutOfScope(t *testing.T) {
	q := fakeQuery{from: dal.From(dal.NewCollectionGroupRef("users", ""))}
	if _, err := Serialize(q); err == nil {
		t.Fatal("expected Serialize to reject an out-of-scope query")
	}
}
