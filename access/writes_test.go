package access

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// stubReadwriteSession records writes the wrapper lets through and serves
// pre-images from a map keyed by record ID.
type stubReadwriteSession struct {
	stubReadSession
	rows   map[string]map[string]any
	writes []string
}

func (s *stubReadwriteSession) Get(_ context.Context, rec record.Record) error {
	s.gets++
	if s.getErr != nil {
		return s.getErr
	}
	row, ok := s.rows[rec.Key().ID.(string)]
	if !ok {
		rec.SetError(record.ErrRecordNotFound)
		return nil
	}
	if target, ok := rec.Data().(*map[string]any); ok {
		*target = row
	}
	rec.SetError(nil)
	return nil
}

func (s *stubReadwriteSession) Set(context.Context, record.Record) error {
	s.writes = append(s.writes, "set")
	return nil
}
func (s *stubReadwriteSession) SetMulti(context.Context, []record.Record) error {
	s.writes = append(s.writes, "setMulti")
	return nil
}
func (s *stubReadwriteSession) Insert(context.Context, record.Record, ...dal.InsertOption) error {
	s.writes = append(s.writes, "insert")
	return nil
}
func (s *stubReadwriteSession) InsertMulti(context.Context, []record.Record, ...dal.InsertOption) error {
	s.writes = append(s.writes, "insertMulti")
	return nil
}
func (s *stubReadwriteSession) Update(context.Context, *record.Key, []update.Update, ...dal.Precondition) error {
	s.writes = append(s.writes, "update")
	return nil
}
func (s *stubReadwriteSession) UpdateRecord(context.Context, record.Record, []update.Update, ...dal.Precondition) error {
	s.writes = append(s.writes, "updateRecord")
	return nil
}
func (s *stubReadwriteSession) UpdateMulti(context.Context, []*record.Key, []update.Update, ...dal.Precondition) error {
	s.writes = append(s.writes, "updateMulti")
	return nil
}
func (s *stubReadwriteSession) Delete(context.Context, *record.Key) error {
	s.writes = append(s.writes, "delete")
	return nil
}
func (s *stubReadwriteSession) DeleteMulti(context.Context, []*record.Key) error {
	s.writes = append(s.writes, "deleteMulti")
	return nil
}

func ownership() dal.Condition {
	return dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))
}

func TestCheckRuleCompilation(t *testing.T) {
	for name, rules := range map[string][]Rule{
		"check on deny":   {Root(Deny(Get, "d").Check(ownership()))},
		"check on scope":  {Under(Path("x"), Allow(Get)).Check(ownership())},
		"invalid check":   {Root(Allow(Set, "s").Check(dal.NewGroupCondition(dal.And)))},
		"check on opaque": {OpaqueQueryScope(Allow(Query, "q").Check(ownership()))},
	} {
		if _, err := NewPolicy("p", rules...); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
	// A check-only rule compiles; the generated name mentions it; params are
	// de-duplicated across where and check.
	policy := MustPolicy("p", Scope("docs", AnyID, Allow(Set|Update).Where(ownership()).Check(dal.NewGroupCondition(dal.And, ownership(), dal.WhereField("status", dal.Equal, dal.Constant{Value: "draft"})))))
	ctx := WithCurrentUser(context.Background(), "u1")
	decision := policy.Decide(ctx, Request{Operation: Set, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}})
	mustDecideAllowed(t, decision)
	if !strings.Contains(decision.Rule, "check (ownerID = $currentUser AND status = 'draft')") || len(policy.compiled[0].params) != 1 {
		t.Errorf("decision = %+v params = %v", decision, policy.compiled[0].params)
	}
	if decision.Writes == nil || decision.Writes[0].Alternatives[0].Check == nil || decision.Writes[0].Alternatives[0].CheckText == "" {
		t.Errorf("write residual = %+v", decision.Writes)
	}
}

func TestWriteDecisions(t *testing.T) {
	ctx := WithCurrentUser(context.Background(), "u1")
	doc := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	// A terminal allow with only a check: reads unconditional, writes checked.
	checkOnly := MustPolicy("check-only", Scope("docs", AnyID, Allow(ReadWrite, "keep-mine").Check(ownership())))
	decision := checkOnly.Decide(ctx, Request{Operation: Get, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals != nil || decision.Writes == nil || decision.Writes[0].Terminal == nil || decision.Writes[0].Terminal.Check == nil || decision.Condition != "ownerID = $currentUser" {
		t.Errorf("check-only decision = %+v", decision)
	}
	// Conditional alternatives before a terminal allow keep both.
	both := MustPolicy("both", Root(Allow(ReadWrite, "all")), Scope("docs", AnyID, Allow(Update, "own").Where(ownership())))
	decision = both.Decide(ctx, Request{Operation: Update, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals != nil || len(decision.Writes[0].Alternatives) != 1 || decision.Writes[0].Terminal == nil || decision.Rule != "all" {
		t.Errorf("both decision = %+v", decision)
	}
	// Unresolved parameters in a check or in the terminal's check deny.
	for name, policy := range map[string]*AccessPolicy{
		"check-only":     checkOnly,
		"terminal check": MustPolicy("t", Root(Allow(ReadWrite, "all").Check(ownership()))),
	} {
		if decision := policy.Decide(context.Background(), Request{Operation: Set, Resources: []Resource{doc}}); decision.Allowed || decision.Condition != "ownerID = $currentUser" {
			t.Errorf("%s: unresolved parameter must deny with the condition text: %+v", name, decision)
		}
	}
	// Multi-resource requests carry write residuals per resource.
	decision = both.Decide(ctx, Request{Operation: Update, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("other", "o1")), doc}})
	mustDecideAllowed(t, decision)
	if len(decision.Writes) != 2 || decision.Writes[0] != nil || decision.Writes[1] == nil {
		t.Errorf("per-resource writes = %+v", decision.Writes)
	}
}

func newWriteFixture(t *testing.T, policies ...Policy) (*stubReadwriteSession, dal.ReadwriteSession) {
	t.Helper()
	stub := &stubReadwriteSession{rows: map[string]map[string]any{
		"mine":   {"ownerID": "u1", "status": "draft", "title": "a"},
		"theirs": {"ownerID": "u2", "status": "draft", "title": "b"},
	}}
	return stub, SecureReadwriteSession(stub, policies...)
}

func TestConditionalWritesThroughStub(t *testing.T) {
	ctx := WithCurrentUser(context.Background(), "u1")
	key := func(id string) *record.Key { return record.NewKeyWithID("docs", id) }
	// Where-only rule: check defaults to where.
	own := MustPolicy("own", Scope("docs", AnyID, Allow(Write, "own").Where(ownership())))
	stub, session := newWriteFixture(t, own)
	mustDeny := func(t *testing.T, err error, fragment string) {
		t.Helper()
		if !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), fragment) || strings.Contains(err.Error(), "u1") {
			t.Fatalf("expected denial containing %q without values, got %v", fragment, err)
		}
	}
	// Insert: admitted by the alternative's check, refused otherwise.
	if err := session.Insert(ctx, record.NewRecordWithData(key("new"), map[string]any{"ownerID": "u1"})); err != nil {
		t.Fatalf("insert own: %v", err)
	}
	mustDeny(t, session.Insert(ctx, record.NewRecordWithData(key("new"), map[string]any{"ownerID": "u2"})), "no rule admits the new row")
	// Set: missing row behaves like insert; existing row checks pre and post.
	if err := session.Set(ctx, record.NewRecordWithData(key("fresh"), map[string]any{"ownerID": "u1"})); err != nil {
		t.Fatalf("set new own: %v", err)
	}
	mustDeny(t, session.Set(ctx, record.NewRecordWithData(key("theirs"), map[string]any{"ownerID": "u1"})), "row condition not satisfied")
	mustDeny(t, session.Set(ctx, record.NewRecordWithData(key("mine"), map[string]any{"ownerID": "u2"})), "post-image check not satisfied")
	if err := session.Set(ctx, record.NewRecordWithData(key("mine"), map[string]any{"ownerID": "u1", "title": "c"})); err != nil {
		t.Fatalf("set own: %v", err)
	}
	// Update: cannot take ownership, cannot give it away, may edit own.
	mustDeny(t, session.Update(ctx, key("theirs"), []update.Update{update.ByFieldName("ownerID", "u1")}), "row condition not satisfied")
	mustDeny(t, session.Update(ctx, key("mine"), []update.Update{update.ByFieldName("ownerID", "u2")}), "post-image check not satisfied")
	if err := session.Update(ctx, key("mine"), []update.Update{update.ByFieldName("title", "d")}); err != nil {
		t.Fatalf("update own: %v", err)
	}
	if err := session.UpdateRecord(ctx, record.NewRecordWithData(key("mine"), map[string]any{}), []update.Update{update.ByFieldName("title", "e")}); err != nil {
		t.Fatalf("update record own: %v", err)
	}
	mustDeny(t, session.Update(ctx, key("missing"), []update.Update{update.ByFieldName("title", "x")}), "does not exist")
	mustDeny(t, session.Update(ctx, key("mine"), []update.Update{update.ByFieldPath(update.FieldPath{"title", "nested"}, "x")}), "post-image could not be computed")
	// Delete: own yes, theirs no.
	if err := session.Delete(ctx, key("mine")); err != nil {
		t.Fatalf("delete own: %v", err)
	}
	mustDeny(t, session.Delete(ctx, key("theirs")), "row condition not satisfied")
	// Batches are refused whole and never reach the adapter.
	before := len(stub.writes)
	mustDeny(t, session.SetMulti(ctx, []record.Record{record.NewRecordWithData(key("mine"), map[string]any{"ownerID": "u1"}), record.NewRecordWithData(key("theirs"), map[string]any{"ownerID": "u1"})}), "row condition not satisfied")
	mustDeny(t, session.InsertMulti(ctx, []record.Record{record.NewRecordWithData(key("a"), map[string]any{"ownerID": "u1"}), record.NewRecordWithData(key("b"), map[string]any{"ownerID": "u2"})}), "no rule admits")
	mustDeny(t, session.UpdateMulti(ctx, []*record.Key{key("mine"), key("theirs")}, []update.Update{update.ByFieldName("title", "x")}), "row condition not satisfied")
	mustDeny(t, session.DeleteMulti(ctx, []*record.Key{key("mine"), key("theirs")}), "row condition not satisfied")
	if len(stub.writes) != before {
		t.Errorf("refused batches reached the adapter: %v", stub.writes[before:])
	}
	if err := session.SetMulti(ctx, []record.Record{record.NewRecordWithData(key("mine"), map[string]any{"ownerID": "u1"})}); err != nil {
		t.Fatalf("set multi own: %v", err)
	}
	// A batch mixing an unconditional target with a conditional one evaluates only the latter.
	mixed := SecureReadwriteSession(stub, MustPolicy("mixed", Scope("docs", "shared", Allow(Write, "shared")), Scope("docs", AnyID, Allow(Write, "own").Where(ownership()))))
	if err := mixed.SetMulti(ctx, []record.Record{record.NewRecordWithData(key("shared"), map[string]any{"ownerID": "u9"}), record.NewRecordWithData(key("mine"), map[string]any{"ownerID": "u1"})}); err != nil {
		t.Fatalf("mixed batch: %v", err)
	}
	if err := session.InsertMulti(ctx, []record.Record{record.NewRecordWithData(key("a"), map[string]any{"ownerID": "u1"})}); err != nil {
		t.Fatalf("insert multi own: %v", err)
	}
	if err := session.UpdateMulti(ctx, []*record.Key{key("mine")}, []update.Update{update.ByFieldName("title", "x")}); err != nil {
		t.Fatalf("update multi own: %v", err)
	}
	if err := session.DeleteMulti(ctx, []*record.Key{key("mine")}); err != nil {
		t.Fatalf("delete multi own: %v", err)
	}
	// Row data that cannot be evaluated is refused.
	mustDeny(t, session.Insert(ctx, record.NewRecordWithData(key("chan"), &struct{ C chan int }{C: make(chan int)})), "could not be evaluated")
	// Pre-image read errors propagate.
	stub.getErr = errors.New("boom")
	if err := session.Delete(ctx, key("mine")); err == nil || err.Error() != "boom" {
		t.Errorf("pre-image error = %v", err)
	}
	stub.getErr = nil

	// A terminal allow admits what no alternative decided; its own check
	// still applies to non-delete writes; a missing row is left to the adapter.
	loose := MustPolicy("loose", Root(Allow(Write, "all").Check(dal.WhereField("status", dal.Equal, dal.Constant{Value: "draft"}))), Scope("docs", AnyID, Allow(Write, "own").Where(ownership())))
	stub, session = newWriteFixture(t, loose)
	if err := session.Update(ctx, key("theirs"), []update.Update{update.ByFieldName("title", "x")}); err != nil {
		t.Fatalf("terminal admits theirs: %v", err)
	}
	mustDeny(t, session.Update(ctx, key("theirs"), []update.Update{update.ByFieldName("status", "final")}), "post-image check not satisfied")
	mustDeny(t, session.Insert(ctx, record.NewRecordWithData(key("n"), map[string]any{"ownerID": "u3", "status": "final"})), "post-image check not satisfied")
	if err := session.Delete(ctx, key("theirs")); err != nil {
		t.Fatalf("terminal delete: %v", err)
	}
	if err := session.Update(ctx, key("missing"), []update.Update{update.ByFieldName("title", "x")}); err != nil {
		t.Fatalf("missing row under terminal allow: %v", err)
	}
	// Two policies both apply; the stricter one refuses.
	strict := WithPolicy(ctx, MustPolicy("draft-only", Scope("docs", AnyID, Allow(Write, "drafts").Where(dal.WhereField("status", dal.Equal, dal.Constant{Value: "draft"})))))
	stub.rows["mine"]["status"] = "final"
	mustDeny(t, session.Update(strict, key("mine"), []update.Update{update.ByFieldName("title", "x")}), "drafts")
}

func TestEvaluateWriteErrorBranches(t *testing.T) {
	bad := WriteAlternative{Rule: "bad", Where: fakeCond{}, Check: fakeCond{}, WhereText: "w", CheckText: "c"}
	good := WriteAlternative{Rule: "good", Where: dal.WhereField("ownerID", dal.Equal, dal.Constant{Value: "u1"}), WhereText: "ownerID = $currentUser"}
	images := writeImages{exists: true, pre: map[string]any{"ownerID": "u1"}, post: map[string]any{"ownerID": "u1"}}
	for name, tc := range map[string]struct {
		operation Operations
		images    writeImages
		residual  WriteResidual
		fragment  string
	}{
		"new row check error":        {Insert, images, WriteResidual{Alternatives: []WriteAlternative{bad}}, "could not be evaluated"},
		"pre-image where error":      {Update, images, WriteResidual{Alternatives: []WriteAlternative{bad}}, "could not be evaluated"},
		"post-image check error":     {Update, images, WriteResidual{Alternatives: []WriteAlternative{{Rule: "g", Where: good.Where, Check: fakeCond{}, CheckText: "c"}}}, "could not be evaluated"},
		"terminal check error":       {Insert, images, WriteResidual{Terminal: &WriteAlternative{Rule: "t", Check: fakeCond{}}}, "could not be evaluated"},
		"terminal check false":       {Set, images, WriteResidual{Terminal: &WriteAlternative{Rule: "t", Check: dal.WhereField("ownerID", dal.Equal, dal.Constant{Value: "u2"}), CheckText: "ownerID = 'u2'"}}, "not satisfied"},
		"no alternative no terminal": {Delete, images, WriteResidual{Alternatives: []WriteAlternative{{Rule: "a", Where: dal.WhereField("ownerID", dal.Equal, dal.Constant{Value: "u2"}), WhereText: "a"}, {Rule: "b", Where: dal.WhereField("ownerID", dal.Equal, dal.Constant{Value: "u3"}), WhereText: "b"}}}, "(a OR b)"},
	} {
		err := evaluateWrite(tc.operation, tc.images, writeResidual{policy: "p", resource: RecordResourceForKey(record.NewKeyWithID("docs", "d1")), residual: &tc.residual})
		var denied *DeniedError
		if !errors.As(err, &denied) || (!strings.Contains(err.Error(), tc.fragment) && denied.Decision.Condition != tc.fragment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := evaluateWrite(Delete, images, writeResidual{residual: &WriteResidual{Alternatives: []WriteAlternative{good}}}); err != nil {
		t.Errorf("delete own = %v", err)
	}
}

func TestReadAllEditAssignedOnMemoryDB(t *testing.T) {
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	key := func(id string) *record.Key { return record.NewKeyWithID("customers", id) }
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for id, c := range map[string]customer{"c1": {Name: "Ann", AssignedTo: "u1"}, "c2": {Name: "Bob", AssignedTo: "u2"}} {
			c := c
			if err := tx.Set(ctx, record.NewRecordWithData(key(id), &c)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db := MustSecureDB(raw, WithDatabasePolicies(customersPolicy(t)))
	user1 := WithCurrentUser(ctx, "u1")
	run := func(worker func(ctx context.Context, tx dal.ReadwriteTransaction) error) error {
		return db.RunReadwriteTransaction(user1, worker)
	}
	// Read all: the other user's customer is readable.
	other := &customer{}
	if err := db.Get(user1, record.NewRecordWithData(key("c2"), other)); err != nil || other.Name != "Bob" {
		t.Fatalf("read all: err=%v data=%+v", err, *other)
	}
	// Edit only assigned: own update lands, other's is refused, reassignment away is refused.
	if err := run(func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key("c1"), []update.Update{update.ByFieldName("name", "Ann B")})
	}); err != nil {
		t.Fatalf("update own: %v", err)
	}
	if err := run(func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key("c2"), []update.Update{update.ByFieldName("name", "x")})
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("update other's = %v", err)
	}
	if err := run(func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key("c1"), []update.Update{update.ByFieldName("assignedTo", "u2")})
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("reassign away = %v", err)
	}
	if err := run(func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Delete(ctx, key("c2"))
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("delete other's = %v", err)
	}
	if err := run(func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Delete(ctx, key("c1"))
	}); err != nil {
		t.Fatalf("delete own: %v", err)
	}
	// The stored data reflects only the admitted writes.
	stored := &customer{}
	if err := raw.Get(ctx, record.NewRecordWithData(key("c2"), stored)); err != nil || stored.Name != "Bob" {
		t.Errorf("c2 must be untouched: err=%v data=%+v", err, *stored)
	}
	if exists, err := raw.Exists(ctx, key("c1")); err != nil || exists {
		t.Errorf("c1 must be deleted: exists=%v err=%v", exists, err)
	}
}

func TestCheckDocumentRoundTrip(t *testing.T) {
	source := `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: docs
default: deny
scopes:
  - path: /docs/*
    rules:
      - id: keep-drafts
        effect: allow
        operations: [write]
        where:
          op: "=="
          left: { field: ownerID }
          right: { param: currentUser }
        check:
          and:
            - op: "=="
              left: { field: ownerID }
              right: { param: currentUser }
            - op: "=="
              left: { field: status }
              right: { value: draft }
`
	policy, err := UnmarshalAccessPolicyYAML([]byte(source))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	decision := policy.Decide(WithCurrentUser(context.Background(), "u1"), Request{Operation: Set, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}})
	mustDecideAllowed(t, decision)
	if got := decision.Writes[0].Alternatives[0].CheckText; got != "(ownerID = $currentUser AND status = 'draft')" {
		t.Errorf("check text = %q", got)
	}
	for name, marshal := range map[string]func(*AccessPolicy) ([]byte, error){"yaml": MarshalAccessPolicyYAML, "json": MarshalAccessPolicyJSON} {
		encoded, err := marshal(policy)
		if err != nil || !strings.Contains(string(encoded), "check") {
			t.Fatalf("%s: err=%v encoded=%s", name, err, encoded)
		}
	}
	bad := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny",
		Scopes: []DocumentScope{{Path: "/x/*", Rules: []DocumentRule{{ID: "r", Effect: "allow", Operations: []string{"set"}, Check: &DocumentCondition{}}}}}}
	if _, err := DecodeAccessPolicy(strings.NewReader(""), staticCodec{d: bad}); err == nil || !strings.Contains(err.Error(), "check:") {
		t.Errorf("invalid check must fail with the slot named: %v", err)
	}
	unserialisable := &AccessPolicy{name: "p", rules: []Rule{Root(Rule{kind: directiveRule, effect: effectAllow, operations: Set, name: "r", check: fakeCond{}})}}
	if _, err := MarshalAccessPolicyYAML(unserialisable); !errors.Is(err, ErrNotSerializable) {
		t.Errorf("unserialisable check = %v", err)
	}
}
