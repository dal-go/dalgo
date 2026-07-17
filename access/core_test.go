package access

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestOperationsComplete(t *testing.T) {
	for _, tc := range []struct {
		o Operations
		s string
	}{{0, "none"}, {Get, "get"}, {Read, "get,exists,query"}, {Operations(1 << 14), "unknown(16384)"}} {
		if got := tc.o.String(); got != tc.s {
			t.Errorf("String(%d)=%q", tc.o, got)
		}
	}
	if !Get.validLeaf() || Read.validLeaf() || Operations(0).validSet() || Operations(1<<14).validSet() {
		t.Fatal("validation")
	}
	if !Read.contains(Get) || Read.contains(Insert) || Read.contains(Read) {
		t.Fatal("contains")
	}
	for _, tc := range []struct {
		in   []string
		want Operations
		bad  bool
	}{
		{[]string{" read ", "WRITE"}, ReadWrite, false}, {[]string{"mutation"}, Write, false}, {[]string{"mutations"}, Write, false},
		{[]string{"read-write"}, ReadWrite, false}, {[]string{"read_write"}, ReadWrite, false}, {[]string{"get", "truncate"}, Get | Truncate, false},
		{[]string{"bogus"}, 0, true}, {nil, 0, true},
	} {
		got, err := parseOperations(tc.in)
		if (err != nil) != tc.bad || got != tc.want {
			t.Errorf("parse %#v=%v,%v", tc.in, got, err)
		}
	}
	for _, tc := range []struct {
		o    Operations
		want string
		bad  bool
	}{{Read, "read", false}, {Write, "write", false}, {ReadWrite, "readwrite", false}, {Get | Delete, "get,delete", false}, {0, "", true}} {
		got, err := operationNamesForDocument(tc.o)
		if (err != nil) != tc.bad || strings.Join(got, ",") != tc.want {
			t.Errorf("names %v=%v,%v", tc.o, got, err)
		}
	}
}

type stringID string
type signedID int32
type unsignedID uint16

func TestResourcesAndPatternsComplete(t *testing.T) {
	parent := dal.NewKeyWithID("spaces", stringID("a/b"))
	key := dal.NewKeyWithParentAndID(parent, "ext", signedID(7))
	for _, tc := range []struct {
		r    Resource
		kind ResourceKind
		s    string
	}{
		{RecordResourceForKey(nil), PathResource, "/"}, {RecordResourceForKey(key), PathResource, "/spaces/a%2Fb/ext/7"},
		{CollectionResourceFor(parent, "items"), PathResource, "/spaces/a%2Fb/items"}, {CollectionGroup("x"), CollectionGroupResource, "collection-group:x"},
		{OpaqueQuery(""), OpaqueQueryResource, "opaque-query"}, {OpaqueQuery("sql"), OpaqueQueryResource, "opaque-query:sql"},
	} {
		if tc.r.Kind() != tc.kind || tc.r.String() != tc.s {
			t.Errorf("resource=%v %v", tc.r, tc.r.Kind())
		}
	}
	if _, err := NewPath(7); err == nil {
		t.Error("collection type")
	}
	if _, err := NewPath(""); err == nil {
		t.Error("empty collection")
	}
	if _, err := NewPath("x", nil); err == nil {
		t.Error("nil id")
	}
	p := Path("spaces", AnyID, "ext", signedID(7))
	if p.String() != "/spaces/*/ext/7" || !patternsMatch(p, RecordResourceForKey(key)) {
		t.Fatal(p.String())
	}
	if patternsMatch(Path("x"), CollectionGroup("x")) || patternsMatch(Path("spaces", AnyID, "ext", 7, "extra"), RecordResourceForKey(key)) {
		t.Fatal("bad match")
	}
	if patternsMatch(Path("spaces", AnyID, "wrong"), RecordResourceForKey(key)) {
		t.Fatal("kind/value mismatch")
	}
	if !equalID(stringID("x"), "x") || !equalID(signedID(2), int64(2)) || !equalID(unsignedID(2), uint64(2)) {
		t.Fatal("aliases")
	}
	if equalID(nil, 2) || equalID(int64(2), uint64(2)) || equalID(2, 3) {
		t.Fatal("inequality")
	}
	if !isSignedInteger(reflectKindInt()) || !isUnsignedInteger(reflectKindUint()) {
		t.Fatal("kind helpers")
	}
}

func reflectKindInt() reflect.Kind  { return reflect.Int }
func reflectKindUint() reflect.Kind { return reflect.Uint }

func TestPolicyHierarchyErrorsAndAudit(t *testing.T) {
	if _, err := NewPolicy(""); err == nil {
		t.Fatal("name")
	}
	if _, err := NewAuditPolicy(""); err == nil {
		t.Fatal("audit name")
	}
	if _, err := NewPolicy("duplicate", Root(Allow(Get, "same")), Collection("users", Allow(Get, "same"))); err == nil {
		t.Fatal("duplicate rule name")
	}
	for _, rules := range [][]Rule{{Audit(Get)}, {Allow(0)}, {Under(Path("x"), CollectionGroupScope("g", Allow(Get)))}, {Scope("x", 1, CollectionGroupScope("g", Allow(Get)))}, {CollectionGroupScope("", Allow(Get))}, {CollectionGroupScope("g", Under(Path("x"), Allow(Get)))}, {OpaqueQueryScope(Under(Path("x"), Allow(Get)))}, {{kind: 99}}} {
		if _, err := NewPolicy("bad", rules...); err == nil {
			t.Errorf("expected compile error %#v", rules)
		}
	}
	if _, err := NewAuditPolicy("bad", Allow(Get)); err == nil {
		t.Fatal("access effect in audit")
	}
	p := MustPolicy("ext",
		Root(Deny(ReadWrite, "root-deny")),
		Scope("spaces", AnyID, Allow(ReadWrite, "space-allow"), Scope("ext", "mine", Allow(ReadWrite, "mine"))),
		Scope("spaces", "one", Deny(Delete, "specific-deny")),
		CollectionGroupScope("events", Allow(Query, "group")), OpaqueQueryScope(Allow(Query, "opaque")),
	)
	if p.Name() != "ext" || p.Source() != "" {
		t.Fatal("identity")
	}
	ctx := context.Background()
	mine := RecordResourceForKey(dal.NewKeyWithParentAndID(dal.NewKeyWithID("spaces", "one"), "ext", "mine"))
	if d := p.Decide(ctx, Request{Operation: Get, Resources: []Resource{mine}}); !d.Allowed || d.Rule != "mine" {
		t.Fatalf("allow: %+v", d)
	}
	if d := p.Decide(ctx, Request{Operation: Delete, Resources: []Resource{RecordResourceForKey(dal.NewKeyWithID("spaces", "one"))}}); d.Allowed || d.Rule != "specific-deny" {
		t.Fatalf("deny %+v", d)
	}
	if !p.Decide(ctx, Request{Operation: Query, Resources: []Resource{CollectionGroup("events")}}).Allowed || !p.Decide(ctx, Request{Operation: Query, Resources: []Resource{OpaqueQuery("q")}}).Allowed {
		t.Fatal("special")
	}
	for _, req := range []Request{{Operation: Read}, {Operation: Get}, {Operation: Get, Resources: []Resource{mine, RecordResourceForKey(dal.NewKeyWithID("root", "x"))}}} {
		d := p.Decide(ctx, req)
		if d.Allowed {
			t.Fatalf("should deny %+v", d)
		}
		err := p.Authorize(ctx, req)
		var de *DeniedError
		if !errors.As(err, &de) || !errors.Is(err, ErrAccessDenied) || de.Unwrap() != ErrAccessDenied {
			t.Fatal(err)
		}
	}
	err := (&DeniedError{Decision: Decision{Policy: "p", PolicySource: "store", Rule: "r", Operation: Get, Resource: mine, Explanation: "why"}}).Error()
	if !strings.Contains(err, "source=\"store\"") || !strings.Contains(err, "rule=\"r\"") {
		t.Fatal(err)
	}
	if !effectIsRestrictive(effectDeny) || !effectIsRestrictive(effectIgnoreAudit) || effectIsRestrictive(effectAllow) {
		t.Fatal("restrictive")
	}
	a := MustAuditPolicy("audit", Root(Audit(Write, "all")), Scope("audit", AnyID, IgnoreAudit(Write, "self")))
	if a.Name() != "audit" || a.Source() != "" {
		t.Fatal("audit identity")
	}
	if !a.Classify(ctx, Request{Operation: Insert, Resources: []Resource{RecordResourceForKey(dal.NewKeyWithID("users", "u"))}}).Audit {
		t.Fatal("audit")
	}
	if a.Classify(ctx, Request{Operation: Insert, Resources: []Resource{RecordResourceForKey(dal.NewKeyWithID("audit", "a"))}}).Audit {
		t.Fatal("ignore")
	}
	for _, r := range []Request{{Operation: Read}, {Operation: Get}, {Operation: Get, Resources: []Resource{RecordResourceForKey(dal.NewKeyWithID("x", "1"))}}} {
		_ = a.Classify(ctx, r)
	}
	defer func() {
		if recover() == nil {
			t.Error("MustAuditPolicy panic")
		}
	}()
	MustAuditPolicy("")
}

func TestMustPolicyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("panic")
		}
	}()
	MustPolicy("")
}

func TestDocumentRoundTripsAndValidation(t *testing.T) {
	p := MustPolicy("portable", Root(Deny(Delete, "deny-delete")), Scope("spaces", AnyID, Allow(ReadWrite, "own")), CollectionGroupScope("events", Allow(Query, "group")), OpaqueQueryScope(Deny(Query, "opaque")))
	for _, yamlMode := range []bool{true, false} {
		var data []byte
		var err error
		if yamlMode {
			data, err = MarshalAccessPolicyYAML(p)
		} else {
			data, err = MarshalAccessPolicyJSON(p)
		}
		if err != nil {
			t.Fatal(err)
		}
		var got *AccessPolicy
		if yamlMode {
			got, err = UnmarshalAccessPolicyYAML(data, WithSource(" store "), nil)
		} else {
			got, err = UnmarshalAccessPolicyJSON(data, WithSource(" store "))
		}
		if err != nil || got.Source() != "store" {
			t.Fatalf("%s %v", data, err)
		}
	}
	a := MustAuditPolicy("a", Root(Audit(Write, "mut")), Scope("logs", AnyID, IgnoreAudit(Write, "self")))
	for _, codec := range []Codec{YAMLCodec{}, JSONCodec{}} {
		var b bytes.Buffer
		if err := EncodeAuditPolicy(&b, codec, a); err != nil {
			t.Fatal(err)
		}
		got, err := DecodeAuditPolicy(&b, codec)
		if err != nil || got.Name() != "a" {
			t.Fatal(err)
		}
	}
	if _, err := MarshalAuditPolicyYAML(a); err != nil {
		t.Fatal(err)
	}
	if _, err := MarshalAuditPolicyJSON(a); err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalAuditPolicyYAML(mustBytes(MarshalAuditPolicyYAML(a))); err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalAuditPolicyJSON(mustBytes(MarshalAuditPolicyJSON(a))); err != nil {
		t.Fatal(err)
	}

	badDocs := []string{
		`{}`, `{"apiVersion":"bad","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/","rules":[{"id":"x","effect":"allow","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"Wrong","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/","rules":[{"id":"x","effect":"allow","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":""},"default":"deny","scopes":[]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"allow","scopes":[{"path":"/","rules":[{"id":"x","effect":"allow","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/","collectionGroup":"x","rules":[{"id":"x","effect":"allow","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/"}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/","rules":[{"id":"","effect":"allow","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"/","rules":[{"id":"x","effect":"audit","operations":["get"]}]}]}`,
		`{"apiVersion":"dalgo.org/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"path":"bad","rules":[{"id":"x","effect":"allow","operations":["get"]}]}]}`,
	}
	for i, s := range badDocs {
		if _, err := UnmarshalAccessPolicyJSON([]byte(s)); err == nil {
			t.Errorf("bad %d", i)
		}
	}
	for _, s := range []string{"/", "/**", "/spaces/*/**", "/bad//x", "/spaces/%zz", "/*/x"} {
		_, _ = parseDocumentPath(s)
	}
	if _, err := parseEffect("wat"); err == nil {
		t.Fatal("effect")
	}
	for _, s := range []string{"allow", "deny", "audit", "ignore-audit"} {
		if _, err := parseEffect(s); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := decodeDocument(nil, YAMLCodec{}, nil); err == nil {
		t.Fatal("nil reader")
	}
	if _, _, err := decodeDocument(strings.NewReader("x"), nil, nil); err == nil {
		t.Fatal("nil codec")
	}
	if err := EncodeAccessPolicy(nil, YAMLCodec{}, p); err == nil {
		t.Fatal("nil writer")
	}
	if err := EncodeAccessPolicy(&bytes.Buffer{}, nil, p); err == nil {
		t.Fatal("nil codec")
	}
	if err := EncodeAccessPolicy(&bytes.Buffer{}, YAMLCodec{}, nil); err == nil {
		t.Fatal("nil policy")
	}
	if err := EncodeAuditPolicy(&bytes.Buffer{}, YAMLCodec{}, nil); err == nil {
		t.Fatal("nil audit")
	}
	if _, err := MarshalAccessPolicyYAML(MustPolicy("unnamed", Allow(Get))); !errors.Is(err, ErrNotSerializable) {
		t.Fatal(err)
	}
	if _, err := MarshalAccessPolicyYAML(MustPolicy("typed", Scope("x", 1, Allow(Get, "a")))); !errors.Is(err, ErrNotSerializable) {
		t.Fatal(err)
	}
	if _, err := documentScopeFromRule(Rule{kind: 99}); !errors.Is(err, ErrNotSerializable) {
		t.Fatal(err)
	}
	if _, err := documentRuleFromRule(Under(Path("x"))); !errors.Is(err, ErrNotSerializable) {
		t.Fatal(err)
	}
	if err := ensureSingleDocument(func(any) error { return fmt.Errorf("boom") }); err == nil {
		t.Fatal("extra err")
	}
	n := 0
	if err := ensureSingleDocument(func(any) error { n++; return nil }); err == nil {
		t.Fatal("extra")
	}
	var b bytes.Buffer
	if err := (YAMLCodec{}).Encode(&b, Document{}); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if err := (JSONCodec{}).Encode(&b, Document{}); err != nil {
		t.Fatal(err)
	}
}

func mustBytes(b []byte, e error) []byte {
	if e != nil {
		panic(e)
	}
	return b
}
