package access

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type failingCodec struct{ decode, encode error }

func (f failingCodec) Decode(io.Reader, *Document) error { return f.decode }
func (f failingCodec) Encode(io.Writer, Document) error  { return f.encode }

type badWriter struct{}

func (badWriter) Write([]byte) (int, error) { return 0, errors.New("write") }

type staticCodec struct{ d Document }

func (s staticCodec) Decode(_ io.Reader, out *Document) error { *out = s.d; return nil }
func (staticCodec) Encode(io.Writer, Document) error          { return nil }

func TestDocumentErrorBranches(t *testing.T) {
	if _, err := DecodeAccessPolicy(strings.NewReader("x"), failingCodec{decode: errors.New("decode")}); err == nil {
		t.Fatal()
	}
	validDoc := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny", Scopes: []DocumentScope{{Path: "/", Rules: []DocumentRule{{ID: "x", Effect: "allow", Operations: []string{"get"}}}}}}
	for _, d := range []Document{
		{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{}, Scopes: []DocumentScope{{Path: "/", Rules: []DocumentRule{{ID: "x", Effect: "allow", Operations: []string{"get"}}}}}},
		func() Document { x := validDoc; x.Default = "allow"; return x }(),
		func() Document { x := validDoc; x.Scopes[0].Rules[0].Operations = []string{"bad"}; return x }(),
		{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny", Scopes: []DocumentScope{{CollectionGroup: "g", Scopes: []DocumentScope{{Path: "/x", Rules: []DocumentRule{{ID: "x", Effect: "allow", Operations: []string{"get"}}}}}}}},
	} {
		_, _ = DecodeAccessPolicy(strings.NewReader(""), staticCodec{d: d})
	}
	badAudit := Document{APIVersion: DocumentAPIVersion, Kind: AuditPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "ignore-audit", Scopes: []DocumentScope{{CollectionGroup: "g", Rules: []DocumentRule{{ID: "g", Effect: "audit", Operations: []string{"get"}}}, Scopes: []DocumentScope{{Path: "/x", Rules: []DocumentRule{{ID: "x", Effect: "audit", Operations: []string{"get"}}}}}}}}
	_, _ = DecodeAuditPolicy(strings.NewReader(""), failingCodec{decode: errors.New("bad")})
	_, _ = DecodeAuditPolicy(strings.NewReader(""), staticCodec{d: badAudit})
	if err := EncodeAccessPolicy(&bytes.Buffer{}, failingCodec{encode: errors.New("encode")}, MustPolicy("p", Allow(Get, "a"))); err == nil {
		t.Fatal()
	}
	if err := (YAMLCodec{}).Encode(badWriter{}, Document{}); err == nil {
		t.Fatal()
	}
	if err := (JSONCodec{}).Encode(badWriter{}, Document{}); err == nil {
		t.Fatal()
	}
	for _, tc := range []struct {
		codec Codec
		data  string
	}{
		{YAMLCodec{}, "bad: ["}, {JSONCodec{}, "{"}, {JSONCodec{}, `{"unknown":1}`}, {JSONCodec{}, `{} {}`}, {YAMLCodec{}, "---\n{}\n---\n{}"},
	} {
		var d Document
		if err := tc.codec.Decode(strings.NewReader(tc.data), &d); err == nil {
			t.Errorf("expected %q", tc.data)
		}
	}
	base := `{"apiVersion":"` + DocumentAPIVersion + `","kind":"%s","metadata":{"name":"x"},"default":"%s","scopes":[{"path":"/","rules":[{"id":"x","effect":"%s","operations":["%s"]}]}]}`
	for _, s := range []string{
		fmt.Sprintf(base, AuditPolicyKind, "ignore-audit", "audit", "get"),
		fmt.Sprintf(base, AuditPolicyKind, "deny", "audit", "get"),
		fmt.Sprintf(base, AuditPolicyKind, "ignore-audit", "allow", "get"),
		fmt.Sprintf(base, AuditPolicyKind, "ignore-audit", "wat", "get"),
		fmt.Sprintf(base, AuditPolicyKind, "ignore-audit", "audit", "wat"),
	} {
		_, _ = UnmarshalAccessPolicyJSON([]byte(s))
		_, _ = UnmarshalAuditPolicyJSON([]byte(s))
	}
	validAudit := fmt.Sprintf(base, AuditPolicyKind, "ignore-audit", "audit", "get")
	if _, err := UnmarshalAuditPolicyJSON([]byte(validAudit)); err != nil {
		t.Fatal(err)
	}
	wrong := fmt.Sprintf(base, AccessPolicyKind, "deny", "allow", "get")
	if _, err := UnmarshalAuditPolicyJSON([]byte(wrong)); err == nil {
		t.Fatal()
	}
	emptyScopes := `{"apiVersion":"` + DocumentAPIVersion + `","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[]}`
	if _, err := UnmarshalAccessPolicyJSON([]byte(emptyScopes)); err == nil {
		t.Fatal()
	}
	nested := `apiVersion: ` + DocumentAPIVersion + `
kind: AccessPolicy
metadata: {name: nested}
default: deny
scopes:
  - path: /spaces/*
    scopes:
      - path: /ext/mine
        rules: [{id: own, effect: allow, operations: [get]}]
`
	if _, err := UnmarshalAccessPolicyYAML([]byte(nested)); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []DocumentScope{{CollectionGroup: "g", Rules: []DocumentRule{{ID: "g", Effect: "allow", Operations: []string{"query"}}}}, {OpaqueQuery: true, Rules: []DocumentRule{{ID: "o", Effect: "allow", Operations: []string{"query"}}}}, {Path: "/", Scopes: []DocumentScope{{}}}} {
		_, _ = ruleFromDocumentScope(scope, map[effect]bool{effectAllow: true, effectDeny: true})
	}
	_, _ = ruleFromDocumentScope(DocumentScope{Path: "/"}, map[effect]bool{effectAllow: true})
	_, _ = ruleFromDocumentScope(DocumentScope{Path: "bad", Rules: []DocumentRule{{ID: "x", Effect: "allow", Operations: []string{"get"}}}}, map[effect]bool{effectAllow: true})
	_, _ = ruleFromDocumentRule(DocumentRule{ID: " ", Effect: "allow", Operations: []string{"get"}}, map[effect]bool{effectAllow: true})
	for _, r := range []Rule{Allow(Get, "root"), Collection("x", Allow(Get, "c")), Rule{kind: scopeRule, pattern: Path("x"), children: []Rule{{kind: 99}}}} {
		_, _ = documentScopeFromRule(r)
	}
	_, _ = documentScopeFromRule(Under(Path("x"), Under(Path("y"), Allow(Get, "nested"))))
	_, _ = documentScopeFromRule(Under(Path("x"), Allow(Get)))
	if _, err := documentRuleFromRule(Rule{kind: directiveRule, name: "x", operations: 0, effect: effectAllow}); err == nil {
		t.Fatal()
	}
	if _, err := marshalAuditPolicy(MustAuditPolicy("a", Audit(Get)), YAMLCodec{}); !errors.Is(err, ErrNotSerializable) {
		t.Fatal(err)
	}
}

func TestRemainingCoreBranches(t *testing.T) {
	for _, e := range []effect{effectAllow, effectDeny, effectAudit, effectIgnoreAudit, effect(99)} {
		_ = e.String()
	}
	for _, r := range []Rule{Collection("x", Allow(Get)), CollectionGroupScope("g", Allow(Query)), OpaqueQueryScope(Allow(Query))} {
		_, _ = NewPolicy("p", r)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Path panic")
			}
		}()
		Path(1)
	}()
	resource := Resource{kind: ResourceKind("bad")}
	cr := compiledRule{kind: resource.kind, operations: Get}
	if ruleMatchesResource(cr, resource) {
		t.Fatal()
	}
	_ = resourceDescription(CollectionGroupResource, "g", PathPattern{})
	_ = resourceDescription(OpaqueQueryResource, "", PathPattern{})
	// Exercise literal and lexical tie breakers as well as the no-match path.
	p := MustPolicy("ties", Scope("x", AnyID, Allow(Get, "z")), Scope("x", "one", Allow(Get, "b"), Allow(Get, "a")))
	if d := p.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("x", "one"))}}); d.Rule != "a" {
		t.Fatal(d)
	}
	if err := p.Authorize(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("x", "one"))}}); err != nil {
		t.Fatal(err)
	}
	if MustPolicy("none").Decide(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("x", "1"))}}).Allowed {
		t.Fatal()
	}
	_ = MustPolicy("tie-deny", Root(Allow(Get, "a"), Deny(Get, "d"))).Decide(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("x", "1"))}})
	badPattern := PathPattern{segments: []pathSegment{{kind: idSegment, value: "x"}}}
	_ = patternsMatch(badPattern, CollectionResourceFor(nil, "x"))
	_ = patternsMatch(Path("x", "no"), RecordResourceForKey(record.NewKeyWithID("x", "yes")))
	_, _ = NewPolicy("bad", CollectionGroupScope("g", OpaqueQueryScope(Allow(Query))))
	var unknown dal.RecordsetSource
	_ = resourceForRecordsetSource(unknown)
}

func TestAllSessionFailureBranches(t *testing.T) {
	ctx := context.Background()
	key := record.NewKeyWithID("x", "1")
	r := record.NewRecord(key)
	rs := []record.Record{r}
	ks := []*record.Key{key}
	q := opaqueQ{}
	for _, policy := range []*AccessPolicy{MustPolicy("deny", Root(Deny(ReadWrite, "no")), OpaqueQueryScope(Deny(Query, "noq"))), MustPolicy("allow", Root(Allow(ReadWrite, "yes")), OpaqueQueryScope(Allow(Query, "yesq")))} {
		f := &fakeSession{}
		if policy.Name() == "allow" {
			f.err = errors.New("delegate")
		}
		rw := SecureReadwriteSession(f, policy)
		_, _ = rw.Exists(ctx, key)
		_ = rw.Get(ctx, r)
		_ = rw.GetMulti(ctx, rs)
		_, _ = rw.ExecuteQueryToRecordsReader(ctx, q)
		_, _ = rw.ExecuteQueryToRecordsetReader(ctx, q)
		_ = rw.Set(ctx, r)
		_ = rw.SetMulti(ctx, rs)
		_ = rw.Insert(ctx, r)
		_ = rw.InsertMulti(ctx, rs)
		_ = rw.Update(ctx, key, nil)
		_ = rw.UpdateRecord(ctx, r, nil)
		_ = rw.UpdateMulti(ctx, ks, nil)
		_ = rw.Delete(ctx, key)
		_ = rw.DeleteMulti(ctx, ks)
	}
}
