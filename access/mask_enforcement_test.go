package access

import (
	"context"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
	"testing"
)

func restoredAddressMask() Mask {
	return Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"address"}}, {Include: []string{"address.city"}}}}
}

func TestMaskReadAndQueryEnforcement(t *testing.T) {
	ctx := context.Background()
	stub := &stubReadwriteSession{rows: map[string]map[string]any{"u1": {"name": "Ann", "address": map[string]any{"city": "Dublin", "secret": "hidden"}}}}
	policy := MustPolicy("masked", Scope("users", AnyID, Allow(ReadWrite, "row").WithFieldMask(restoredAddressMask())), Collection("users", Allow(Query, "list").WithFieldMask(restoredAddressMask())))
	session := SecureReadwriteSession(stub, policy)
	data := map[string]any{}
	require.NoError(t, session.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "u1"), &data)))
	require.Equal(t, map[string]any{"name": "Ann", "address": map[string]any{"city": "Dublin"}}, data)
	for _, path := range []string{"address", "address.secret"} {
		q := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectColumns(dal.Column{Expression: dal.Field(path)})
		_, err := session.ExecuteQueryToRecordsReader(ctx, q)
		require.ErrorIs(t, err, ErrAccessDenied)
	}
	q := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectColumns(dal.Column{Expression: dal.Field("name")})
	_, err := session.ExecuteQueryToRecordsetReader(ctx, q)
	require.NoError(t, err)
	_, err = MarshalAccessPolicyYAML(policy)
	require.ErrorIs(t, err, ErrNotSerializable)
}

func TestMaskMutationDescendants(t *testing.T) {
	compiled, err := CompileMask(restoredAddressMask(), FieldMask)
	require.NoError(t, err)
	sets := fieldSets{&fieldSet{mask: compiled}}
	pre := map[string]any{"address": map[string]any{"city": "Dublin", "secret": "hidden"}}
	post := map[string]any{"address": map[string]any{"city": "Cork"}}
	whole := writeImages{pre: pre, post: post, updates: []update.Update{update.ByFieldName("address", post["address"])}}
	refused, unsupported := sets.disallowedMaskedMutation(whole, Update)
	require.False(t, unsupported)
	require.Contains(t, refused, "address.secret")
	require.Contains(t, refused, "address")
	leaf := writeImages{pre: pre, post: post, updates: []update.Update{update.ByFieldPath(update.FieldPath{"address", "city"}, "Cork")}}
	refused, unsupported = sets.disallowedMaskedMutation(leaf, Update)
	require.Empty(t, refused)
	require.False(t, unsupported)
	for _, operation := range []Operations{Set, Delete} {
		refused, unsupported = sets.disallowedMaskedMutation(writeImages{pre: pre, post: post}, operation)
		require.False(t, unsupported)
		require.Contains(t, refused, "address.secret")
	}
	// Arrays are opaque: a child restoration does not authorize the whole array.
	data := map[string]any{"address": []any{map[string]any{"city": "Dublin", "secret": "hidden"}}}
	sets.redactMap("", data)
	require.Empty(t, data)
}

func TestPortableMasksLoadAndDenyCollections(t *testing.T) {
	for _, bound := range []bool{false, true} {
		root := t.TempDir()
		scopes := `[{path: /users, rules: [{id: q, effect: allow, operations: [query], fieldMask: {stages: [{include: ['*']}, {exclude: ['secret*']}]}}]}]`
		body := "scopes: " + scopes + "\n"
		if bound {
			body = "ruleSets: {reader: " + scopes + "}\nbindings: {everyone: [reader]}\n"
		}
		body += "collectionMask: {stages: [{include: ['*']}, {exclude: ['sys_*']}] }\n"
		writePolicyFile(t, root, "p.yaml", portablePolicy("p", "public", body))
		policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"p.yaml"}})
		require.NoError(t, err)
		for _, name := range []string{"users", "sys_users"} {
			d := policies[0].Decide(context.Background(), Request{Operation: Query, Resources: []Resource{CollectionResourceFor(nil, name)}})
			require.Equal(t, name == "users", d.Allowed)
			if !d.Allowed {
				require.Equal(t, "p", d.Policy)
				require.Equal(t, "p.yaml", d.PolicySource)
			}
		}
	}
}

func TestMaskWritesReachBackendOnlyWhenAllTouchedFieldsAllow(t *testing.T) {
	ctx := context.Background()
	stub := &stubReadwriteSession{rows: map[string]map[string]any{"u1": {"address": map[string]any{"city": "Dublin", "secret": "hidden"}}}}
	policy := MustPolicy("masked", Scope("users", AnyID, Allow(ReadWrite, "row").WithFieldMask(restoredAddressMask())))
	session := SecureReadwriteSession(stub, policy)
	key := record.NewKeyWithID("users", "u1")
	err := session.Update(ctx, key, []update.Update{update.ByFieldName("address", map[string]any{"city": "Cork"})})
	require.ErrorIs(t, err, ErrAccessDenied)
	require.Empty(t, stub.writes)
	err = session.Update(ctx, key, []update.Update{update.ByFieldPath(update.FieldPath{"address", "city"}, "Cork")})
	require.NoError(t, err)
	require.NotEmpty(t, stub.writes)
}

func TestMaskOpaqueContainersDoNotLeakDescendants(t *testing.T) {
	c, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"address.secret"}}}}, FieldMask)
	require.NoError(t, err)
	sets := fieldSets{&fieldSet{mask: c}}
	for _, value := range []any{map[string]string{"secret": "hidden"}, struct{ Secret string }{"hidden"}, []any{"hidden"}} {
		data := map[string]any{"address": value}
		sets.redactMap("", data)
		require.Empty(t, data)
	}
	require.True(t, c.CompleteSubtree("name"))
	require.False(t, c.CompleteSubtree("address"))
}

func TestMaskMutationDistinguishesOpaqueCoverageFromDefiniteExclusion(t *testing.T) {
	c, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"address.secret"}}}}, FieldMask)
	require.NoError(t, err)
	w := writeResidual{policy: "p", residual: &WriteResidual{Terminal: &WriteAlternative{Rule: "r", fields: &fieldSet{mask: c}}}}
	for name, value := range map[string]any{"opaque": struct{ Secret string }{"hidden"}, "enumerable": map[string]any{"secret": "hidden"}} {
		t.Run(name, func(t *testing.T) {
			err := checkFields(Set, writeImages{post: map[string]any{"address": value}}, w, *w.residual.Terminal)
			var denied *DeniedError
			require.ErrorAs(t, err, &denied)
			if name == "opaque" {
				require.Equal(t, CodeEnforcementUnsupported, denied.Decision.Code)
			} else {
				require.Equal(t, CodeColumnDenied, denied.Decision.Code)
			}
		})
	}
}

func TestMaskedNestedUpdateHandlesMissingAndScalarParents(t *testing.T) {
	c, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}}}, FieldMask)
	require.NoError(t, err)
	sets := fieldSets{&fieldSet{mask: c}}
	images := writeImages{pre: map[string]any{"address": "opaque"}, post: map[string]any{}, updates: []update.Update{update.ByFieldPath(update.FieldPath{"address", "city"}, "x")}}
	refused, unsupported := sets.disallowedMaskedMutation(images, Update)
	require.Empty(t, refused)
	require.False(t, unsupported)
}
