package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestParamRoundTrip(t *testing.T) {
	source := "from:\n  name: orders\nwhere:\n  op: ==\n  left:\n    field: tenantID\n  right:\n    param: tenant\n"
	query, err := Deserialize([]byte(source))
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	comparison, ok := query.Where().(dal.Comparison)
	if !ok {
		t.Fatalf("Where() is %T", query.Where())
	}
	if param, ok := comparison.Right.(dal.Param); !ok || param.Name != "tenant" {
		t.Fatalf("right operand = %#v", comparison.Right)
	}
	encoded, err := Serialize(query)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if string(encoded) != source {
		t.Errorf("canonical form differs:\n%s\nwant:\n%s", encoded, source)
	}
	if !strings.Contains(query.Where().String(), "$tenant") {
		t.Errorf("String() = %q", query.Where().String())
	}
}

func TestParamRejections(t *testing.T) {
	for name, source := range map[string]string{
		"invalid name":    "from:\n  name: orders\nwhere:\n  op: ==\n  left:\n    field: a\n  right:\n    param: 1x\n",
		"param and value": "from:\n  name: orders\nwhere:\n  op: ==\n  left:\n    field: a\n  right:\n    param: x\n    value: 1\n",
	} {
		if _, err := Deserialize([]byte(source)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSchemaMentionsParam(t *testing.T) {
	if !strings.Contains(string(SchemaJSON()), `"param"`) {
		t.Error("schema must describe the param expression")
	}
}
