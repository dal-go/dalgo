package access

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func docsRuleSets() map[string][]Rule {
	return map[string][]Rule{
		"read-docs":     {Scope("docs", AnyID, Allow(Read, "read"))},
		"write-docs":    {Scope("docs", AnyID, Allow(Insert|Set|Update, "write"))},
		"content-write": {Scope("docs", AnyID, Allow(Write, "publish"))},
		"suspend":       {Scope("docs", AnyID, Deny(Write, "no-writes"))},
		"own-docs":      {Scope("docs", AnyID, Allow(Update, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))))},
		"by-role":       {Scope("docs", AnyID, Allow(Get).Where(dal.WhereField("audience", dal.In, dal.NewParam("principal.roles"))))},
		"public":        {Scope("public", AnyID, Allow(Read, "anyone"))},
	}
}

func docsBindings() Bindings {
	return Bindings{
		Roles:    map[string][]string{"reader": {"read-docs"}, "writer": {"write-docs"}, "editor": {"content-write"}, "admin": {"content-write", "read-docs"}, "member": {"own-docs", "by-role"}},
		Groups:   map[string][]string{"suspended": {"suspend"}},
		Users:    map[string][]string{"u9": {"write-docs"}},
		Everyone: []string{"public"},
	}
}

func TestPrincipalContext(t *testing.T) {
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Error("no principal expected")
	}
	var nilContext context.Context
	if _, ok := PrincipalFrom(nilContext); ok {
		t.Error("nil context has no principal")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("WithPrincipal(nil) must panic")
			}
		}()
		WithPrincipal(nilContext, Principal{})
	}()
	ctx := WithPrincipal(context.Background(), Principal{ID: "u1", Roles: []string{"member"}, Groups: []string{"staff"}})
	variables := variablesFromContext(ctx)
	if variables["currentUser"] != "u1" || len(variables["principal.roles"].([]string)) != 1 || len(variables["principal.groups"].([]string)) != 1 {
		t.Errorf("principal-derived variables = %v", variables)
	}
	// Explicit variables take precedence over the principal.
	variables = variablesFromContext(WithVariables(ctx, map[string]any{"currentUser": "impersonated", "principal.roles": []string{"x"}, "principal.groups": []string{}}))
	if variables["currentUser"] != "impersonated" || variables["principal.roles"].([]string)[0] != "x" {
		t.Errorf("explicit variables must win: %v", variables)
	}
}

func TestPrincipalPolicySetConstruction(t *testing.T) {
	for name, tc := range map[string]struct {
		name     string
		sets     map[string][]Rule
		bindings Bindings
	}{
		"empty name":       {"", docsRuleSets(), docsBindings()},
		"no rule sets":     {"p", nil, docsBindings()},
		"bad set name":     {"p", map[string][]Rule{"a/b": {Root(Allow(Get))}}, Bindings{}},
		"set fails":        {"p", map[string][]Rule{"bad": {Root(Deny(Get, "d").Where(dal.WhereField("a", dal.Equal, dal.Constant{Value: 1})))}}, Bindings{}},
		"unknown role set": {"p", docsRuleSets(), Bindings{Roles: map[string][]string{"x": {"nope"}}}},
		"unknown group":    {"p", docsRuleSets(), Bindings{Groups: map[string][]string{"x": {"nope"}}}},
		"unknown user":     {"p", docsRuleSets(), Bindings{Users: map[string][]string{"x": {"nope"}}}},
		"unknown everyone": {"p", docsRuleSets(), Bindings{Everyone: []string{"nope"}}},
	} {
		if _, err := NewPrincipalPolicySet(tc.name, tc.sets, tc.bindings); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("MustPrincipalPolicySet must panic on an invalid set")
			}
		}()
		MustPrincipalPolicySet("", nil, Bindings{})
	}()
	set := MustPrincipalPolicySet("docs", docsRuleSets(), docsBindings())
	if set.Name() != "docs" || set.Source() != "" {
		t.Errorf("name/source = %q/%q", set.Name(), set.Source())
	}
}

func TestPrincipalPolicySetDecisions(t *testing.T) {
	set := MustPrincipalPolicySet("docs", docsRuleSets(), docsBindings())
	doc := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	get := Request{Operation: Get, Resources: []Resource{doc}}
	insert := Request{Operation: Insert, Resources: []Resource{doc}}
	as := func(id string, roles ...string) context.Context {
		return WithPrincipal(context.Background(), Principal{ID: id, Roles: roles})
	}
	// roles-union: reader + writer together read and insert.
	both := as("u1", "reader", "writer")
	mustDecideAllowed(t, set.Decide(both, get))
	mustDecideAllowed(t, set.Decide(both, insert))
	if decision := set.Decide(as("u1", "reader"), insert); decision.Allowed {
		t.Errorf("reader alone must not insert: %+v", decision)
	}
	// shared-rule-set: editor and admin share content-write, same rule id.
	editor, admin := set.Decide(as("u2", "editor"), insert), set.Decide(as("u3", "admin"), insert)
	mustDecideAllowed(t, editor)
	mustDecideAllowed(t, admin)
	if editor.Rule != "content-write/publish" || admin.Rule != editor.Rule {
		t.Errorf("shared rule ids = %q / %q", editor.Rule, admin.Rule)
	}
	if !strings.Contains(editor.Explanation, "via role:editor") || !strings.Contains(admin.Explanation, "via role:admin") {
		t.Errorf("explanations must name the binding: %q / %q", editor.Explanation, admin.Explanation)
	}
	// specific-deny-across-bindings: a suspended writer cannot write.
	suspended := WithPrincipal(context.Background(), Principal{ID: "u4", Roles: []string{"writer"}, Groups: []string{"suspended"}})
	if decision := set.Decide(suspended, insert); decision.Allowed || !strings.Contains(decision.Explanation, "group:suspended") {
		t.Errorf("suspended writer = %+v", decision)
	}
	// users binding and everyone.
	mustDecideAllowed(t, set.Decide(as("u9"), insert))
	public := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("public", "p1"))}}
	anonymous := set.Decide(context.Background(), public)
	mustDecideAllowed(t, anonymous)
	if !strings.Contains(anonymous.Explanation, "via everyone") {
		t.Errorf("everyone attribution: %q", anonymous.Explanation)
	}
	// unbound-principal-denied.
	// everyone applies (public set), so a docs read by an unknown role is denied by "no matching allow rule".
	if decision := set.Decide(as("nobody", "unknown-role"), get); decision.Allowed || !strings.Contains(decision.Explanation, "no matching allow rule") {
		t.Errorf("unknown role must not read docs: %+v", decision)
	}
	none := MustPrincipalPolicySet("strict", map[string][]Rule{"r": {Root(Allow(Get, "g"))}}, Bindings{Roles: map[string][]string{"reader": {"r"}}})
	if decision := none.Decide(as("nobody"), get); decision.Allowed || !strings.Contains(decision.Explanation, `no binding applies to principal "nobody"`) {
		t.Errorf("no binding = %+v", decision)
	}
	if decision := none.Decide(context.Background(), get); decision.Allowed || !strings.Contains(decision.Explanation, "<none>") {
		t.Errorf("absent principal = %+v", decision)
	}
	if err := none.Authorize(as("nobody"), get); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("Authorize = %v", err)
	}
	if err := none.Authorize(as("x", "reader"), get); err != nil {
		t.Errorf("Authorize allowed = %v", err)
	}
	// Row conditions resolve $currentUser and $principal.roles from the principal.
	member := WithPrincipal(context.Background(), Principal{ID: "u5", Roles: []string{"member", "vip"}})
	decision := set.Decide(member, Request{Operation: Update, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals[0].String() != "ownerID = 'u5'" {
		t.Errorf("principal-resolved residual = %q", decision.Residuals[0])
	}
	decision = set.Decide(member, get)
	mustDecideAllowed(t, decision)
	if got := decision.Residuals[0].String(); got != "audience In ('member','vip')" {
		t.Errorf("roles residual = %q", got)
	}
	// The compiled union is cached per binding combination.
	first := set.effective([]string{"read-docs", "write-docs"})
	if second := set.effective([]string{"read-docs", "write-docs"}); second != first {
		t.Error("same combination must reuse the compiled policy")
	}
	if other := set.effective([]string{"read-docs"}); other == first {
		t.Error("different combination must compile separately")
	}
}

func TestPrincipalPolicySetIntersection(t *testing.T) {
	set := MustPrincipalPolicySet("docs", map[string][]Rule{"admin": {Root(Allow(ReadWrite, "all"))}}, Bindings{Roles: map[string][]string{"admin": {"admin"}}})
	database := MustPolicy("db", Root(Allow(ReadWrite, "all"), Scope("audit", AnyID, Deny(Delete, "keep-audit"))))
	session := SecureReadwriteSession(&stubReadwriteSession{rows: map[string]map[string]any{}}, database, set)
	admin := WithPrincipal(context.Background(), Principal{ID: "root", Roles: []string{"admin"}})
	if err := session.Delete(admin, record.NewKeyWithID("audit", "a1")); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("binding must not widen a database denial: %v", err)
	}
	if err := session.Delete(admin, record.NewKeyWithID("docs", "d1")); err != nil {
		t.Errorf("admin delete = %v", err)
	}
	if err := session.Delete(context.Background(), record.NewKeyWithID("docs", "d1")); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("unbound principal must be denied: %v", err)
	}
}

const principalPolicyYAML = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: docs
default: deny
ruleSets:
  read-docs:
    - path: /docs/*
      rules:
        - id: read
          effect: allow
          operations: [read]
  own-docs:
    - path: /docs/*
      rules:
        - id: own
          effect: allow
          operations: [update]
          where:
            op: "=="
            left: { field: ownerID }
            right: { param: currentUser }
bindings:
  roles:
    reader: [read-docs]
    member: [own-docs, read-docs]
  groups:
    staff: [read-docs]
  users:
    u1: [own-docs]
  everyone: [read-docs]
`

func TestPrincipalPolicySetDocuments(t *testing.T) {
	set, err := UnmarshalPrincipalPolicySetYAML([]byte(principalPolicyYAML), WithSource("policies/docs.yaml"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if set.Source() != "policies/docs.yaml" {
		t.Errorf("source = %q", set.Source())
	}
	doc := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	member := WithPrincipal(context.Background(), Principal{ID: "u7", Roles: []string{"member"}})
	decision := set.Decide(member, Request{Operation: Update, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals[0].String() != "ownerID = 'u7'" || decision.PolicySource != "policies/docs.yaml" {
		t.Errorf("decoded decision = %+v", decision)
	}
	for name, marshal := range map[string]func(*PrincipalPolicySet) ([]byte, error){"yaml": MarshalPrincipalPolicySetYAML, "json": MarshalPrincipalPolicySetJSON} {
		encoded, err := marshal(set)
		if err != nil || !strings.Contains(string(encoded), "bindings") || !strings.Contains(string(encoded), "own-docs") {
			t.Fatalf("%s encode: err=%v\n%s", name, err, encoded)
		}
		var again *PrincipalPolicySet
		if name == "yaml" {
			again, err = UnmarshalPrincipalPolicySetYAML(encoded)
		} else {
			again, err = UnmarshalPrincipalPolicySetJSON(encoded)
		}
		if err != nil {
			t.Fatalf("%s decode again: %v", name, err)
		}
		second := again.Decide(member, Request{Operation: Update, Resources: []Resource{doc}})
		if second.Allowed != decision.Allowed || second.Rule != decision.Rule || second.Condition != decision.Condition {
			t.Errorf("%s round trip differs: %+v vs %+v", name, decision, second)
		}
	}
	// DecodePolicy routes by shape.
	if policy, err := DecodePolicy(strings.NewReader(principalPolicyYAML), YAMLCodec{}); err != nil {
		t.Errorf("DecodePolicy(bindings) = %v", err)
	} else if _, ok := policy.(*PrincipalPolicySet); !ok {
		t.Errorf("DecodePolicy(bindings) = %T", policy)
	}
	if policy, err := DecodePolicy(strings.NewReader(conditionalPolicyYAML), YAMLCodec{}); err != nil {
		t.Errorf("DecodePolicy(scopes) = %v", err)
	} else if _, ok := policy.(*AccessPolicy); !ok {
		t.Errorf("DecodePolicy(scopes) = %T", policy)
	}
	if _, err := DecodePolicy(strings.NewReader("x"), failingCodec{decode: errors.New("decode")}); err == nil {
		t.Error("DecodePolicy must propagate decode errors")
	}
	// Error paths.
	if _, err := UnmarshalAccessPolicyYAML([]byte(principalPolicyYAML)); err == nil || !strings.Contains(err.Error(), "principal policy set") {
		t.Errorf("AccessPolicy decode of a bindings document = %v", err)
	}
	if _, err := DecodePrincipalPolicySet(strings.NewReader("x"), failingCodec{decode: errors.New("decode")}); err == nil {
		t.Error("DecodePrincipalPolicySet must propagate decode errors")
	}
	base := func() Document {
		return Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny",
			RuleSets: map[string][]DocumentScope{"r": {{Path: "/x/*", Rules: []DocumentRule{{ID: "g", Effect: "allow", Operations: []string{"get"}}}}}},
			Bindings: &DocumentBindings{Everyone: []string{"r"}}}
	}
	for name, mutate := range map[string]func(*Document){
		"wrong kind":    func(d *Document) { d.Kind = AuditPolicyKind },
		"wrong default": func(d *Document) { d.Default = "allow" },
		"no bindings":   func(d *Document) { d.Bindings = nil },
		"top-level scopes": func(d *Document) {
			d.Scopes = []DocumentScope{{Path: "/y/*", Rules: []DocumentRule{{ID: "y", Effect: "allow", Operations: []string{"get"}}}}}
		},
		"bad rule in set":        func(d *Document) { d.RuleSets["r"][0].Rules[0].Operations = []string{"bogus"} },
		"unknown set referenced": func(d *Document) { d.Bindings.Everyone = []string{"nope"} },
	} {
		document := base()
		mutate(&document)
		if _, err := DecodePrincipalPolicySet(strings.NewReader(""), staticCodec{d: document}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	empty := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny"}
	if _, err := DecodePrincipalPolicySet(strings.NewReader(""), staticCodec{d: empty}); err == nil {
		t.Error("a document with neither scopes nor bindings must fail")
	}
	// Encoding requires named rules and a non-nil set.
	if err := EncodePrincipalPolicySet(nil, YAMLCodec{}, nil); err == nil {
		t.Error("nil set must fail")
	}
	unnamed := MustPrincipalPolicySet("u", map[string][]Rule{"r": {Root(Allow(Get))}}, Bindings{Everyone: []string{"r"}})
	if _, err := MarshalPrincipalPolicySetYAML(unnamed); !errors.Is(err, ErrNotSerializable) {
		t.Errorf("unnamed rule encode = %v", err)
	}
	if _, err := MarshalPrincipalPolicySetJSON(unnamed); !errors.Is(err, ErrNotSerializable) {
		t.Errorf("unnamed rule JSON encode = %v", err)
	}
	// Unnamed rules still get unique attributable ids inside the union.
	decision = unnamed.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Rule != "r/rule-1" {
		t.Errorf("generated rule id = %q", decision.Rule)
	}
}
