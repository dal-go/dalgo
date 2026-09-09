package access

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func TestExecutionGateAssessmentAndClassification(t *testing.T) {
	root := t.TempDir()
	body := portablePolicy("gated", "public", `scopes: [{path: /customers, rules: [{id: query, effect: allow, operations: [query]}]}]
execution:
  allow:
    - class: dtql
    - class: stored_procedure
      namespace: public
      mask:
        stages:
          - include: ["User_*"]
          - exclude: ["User_Delete*"]
          - include: ["User_DeleteDraft"]
`)
	writePolicyFile(t, root, "gate.yaml", body)
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"gate.yaml"}})
	require.NoError(t, err)
	request := Request{Operation: Query, Resources: []Resource{CollectionResourceFor(nil, "customers")}}
	for _, test := range []struct {
		target  ExecutionTarget
		allowed bool
	}{
		{ExecutionTarget{Class: ExecutionDTQL}, true},
		{ExecutionTarget{Class: ExecutionNativeSQL}, false},
		{ExecutionTarget{Class: ExecutionNativeGraphQL}, false},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: "User_Read"}, true},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: "User_Delete"}, false},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: "User_DeleteDraft"}, true},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "sys", Name: "User_Read"}, false},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: "User_*"}, false},
		{ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: "Other_DeleteDraft"}, false},
	} {
		request.Execution = &test.target
		d := policies[0].Decide(context.Background(), request)
		require.Equal(t, test.allowed, d.Allowed, test.target)
		if !test.allowed && test.target.Class == ExecutionStoredProcedure && test.target.Namespace == "public" && test.target.Name != "User_*" {
			require.Equal(t, CodeCallableDenied, d.Code)
		}
		require.Equal(t, "gated", d.Policy)
		require.Equal(t, "gate.yaml", d.PolicySource)
	}
	request.Query = dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("customers", ""))).SelectColumns(dal.Column{Expression: dal.Field("name")})
	request.Execution = &ExecutionTarget{Class: ExecutionNativeSQL}
	require.False(t, policies[0].Decide(context.Background(), request).Allowed, "caller label cannot change a structured query")
	request.Execution = nil
	require.True(t, policies[0].Decide(context.Background(), request).Allowed, "internal SQL compilation remains typed DTQL")
	request.Query = dal.NewTextQuery("CALL public.User_Read()", nil)
	request.Execution = &ExecutionTarget{Class: ExecutionDTQL}
	require.False(t, policies[0].Decide(context.Background(), request).Allowed, "opaque SQL cannot claim the DTQL surface")

}

func TestExecutionGateEmptyAndLayerIntersection(t *testing.T) {
	root := t.TempDir()
	for name, entries := range map[string]string{"deny": "[]", "allow": "[{class: dtql}]"} {
		writePolicyFile(t, root, name+".yaml", portablePolicy(name, "public", `scopes: [{path: /users, rules: [{id: query, effect: allow, operations: [query]}]}]
execution: {allow: `+entries+"}\n"))
	}
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"allow.yaml", "deny.yaml"}})
	require.NoError(t, err)
	stub := &stubReadwriteSession{}
	session := SecureReadSession(SecureReadSession(stub, policies[1]), policies[0])
	q := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectColumns(dal.Column{Expression: dal.Field("name")})
	_, err = session.ExecuteQueryToRecordsReader(context.Background(), q)
	require.ErrorIs(t, err, ErrAccessDenied)
	require.Empty(t, stub.queries)
	_, err = MarshalAccessPolicyYAML(policies[0].(*AccessPolicy))
	require.ErrorIs(t, err, ErrNotSerializable)
}

func TestExecutionGateAllExceptSystemProcedures(t *testing.T) {
	gate, err := compileExecutionGate(&ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Namespace: "public", Mask: &Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"sys_*"}}}}}}})
	require.NoError(t, err)
	for name, allowed := range map[string]bool{"User_Read": true, "sys_Read": false} {
		require.Equal(t, allowed, gate.allows(Request{Execution: &ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "public", Name: name}}))
	}
}

func TestExecutionClassificationBoundaries(t *testing.T) {
	if gate, err := compileExecutionGate(nil); err != nil || gate != nil {
		t.Fatalf("nil gate=%v err=%v", gate, err)
	}
	badMask := Mask{}
	if _, err := compileExecutionGate(&ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Mask: &badMask}}}); err == nil {
		t.Fatal("invalid callable mask accepted")
	}
	structured := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("x", ""))).SelectKeysOnly(reflect.String)
	for name, request := range map[string]Request{
		"opaque":                  {Query: dal.NewTextQuery("select 1", nil)},
		"structured relabeled":    {Query: structured, Execution: &ExecutionTarget{Class: ExecutionNativeSQL}},
		"nonprocedure names":      {Execution: &ExecutionTarget{Class: ExecutionDTQL, Name: "x"}},
		"bad procedure namespace": {Execution: &ExecutionTarget{Class: ExecutionStoredProcedure, Namespace: "bad/name", Name: "x"}},
		"unknown class":           {Execution: &ExecutionTarget{Class: ExecutionClass("future")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := classifyExecution(request); err == nil {
				t.Fatal("invalid execution accepted")
			}
		})
	}
}
