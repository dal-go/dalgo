package dtql

import (
	"strings"
	"testing"
)

// TestDeserialize_errorBranches drives the remaining deserialization error paths
// (expression/condition/comparison/group validation) through the public
// Deserialize entry point.
func TestDeserialize_errorBranches(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "expression sets several forms",
			yaml:    "from:\n  name: users\norderBy:\n  - field: a\n    value: 1\n",
			wantErr: "several are set",
		},
		{
			name:    "column expression sets several forms",
			yaml:    "from:\n  name: users\ncolumns:\n  - field: a\n    value: 1\n",
			wantErr: "several are set",
		},
		{
			name:    "condition with no form",
			yaml:    "from:\n  name: users\nwhere: {}\n",
			wantErr: "must be a comparison",
		},
		{
			name:    "comparison missing left",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  right:\n    value: 1\n",
			wantErr: "requires both left and right",
		},
		{
			name:    "comparison left invalid expression",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n    value: 1\n  right:\n    value: 1\n",
			wantErr: "comparison left:",
		},
		{
			name:    "comparison right invalid expression",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n  right:\n    field: b\n    value: 1\n",
			wantErr: "comparison right:",
		},
		{
			name:    "empty and group",
			yaml:    "from:\n  name: users\nwhere:\n  and: []\n",
			wantErr: "must have at least one condition",
		},
		{
			name:    "or group with invalid child",
			yaml:    "from:\n  name: users\nwhere:\n  or:\n    - op: \"!=\"\n      left:\n        field: a\n      right:\n        value: 1\n",
			wantErr: "group condition #0:",
		},
		{
			name:    "orderBy with no expression form",
			yaml:    "from:\n  name: users\norderBy:\n  - desc: true\n",
			wantErr: "orderBy #0:",
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
