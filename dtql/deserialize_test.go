package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

func TestSerializeAndBack(t *testing.T) {
	out, err := Serialize(fullQuery())
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	// The document is plain YAML parseable by a standard library.
	var generic any
	if err := yaml.Unmarshal(out, &generic); err != nil {
		t.Fatalf("not plain YAML: %v", err)
	}
	q, err := Deserialize(out)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	if q == nil {
		t.Fatal("Deserialize returned a nil query")
	}
	if got := q.From().Base().(dal.CollectionRef).Name(); got != "users" {
		t.Errorf("From name = %q, want users", got)
	}
	if len(q.Columns()) != 2 {
		t.Errorf("Columns = %d, want 2", len(q.Columns()))
	}
	if q.Limit() != 10 || q.Offset() != 20 {
		t.Errorf("Limit/Offset = %d/%d, want 10/20", q.Limit(), q.Offset())
	}
}

func TestDeserialize_invalidInputRejected(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "unknown key",
			yaml:    "from:\n  name: users\nbogus: 1\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "missing from name",
			yaml:    "limit: 5\n",
			wantErr: "from.name is required",
		},
		{
			name:    "wrong value type for limit",
			yaml:    "from:\n  name: users\nlimit: notanint\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "unknown operator",
			yaml:    "from:\n  name: users\nwhere:\n  op: \"!=\"\n  left:\n    field: a\n  right:\n    value: 1\n",
			wantErr: "unknown comparison operator",
		},
		{
			name:    "comparison missing right",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n",
			wantErr: "requires both left and right",
		},
		{
			name:    "mixed comparison and group",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n  right:\n    value: 1\n  and:\n    - op: ==\n      left:\n        field: b\n      right:\n        value: 2\n",
			wantErr: "mixes comparison and group",
		},
		{
			name:    "expression with no field/value/values",
			yaml:    "from:\n  name: users\ncolumns:\n  - as: x\n",
			wantErr: "exactly one of field, value or values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Deserialize([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if q != nil {
				t.Errorf("expected nil query on error, got %#v", q)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
