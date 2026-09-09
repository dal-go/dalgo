package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

func TestPreparationFailureIsAssessedWithoutDisclosureOrExecution(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{
		OperationID: "op1", CanonicalTarget: key.String(),
		PreparationError: errors.New("secret malformed stored scalar"),
	}}}
	allow := MustPolicy("writer", Scope("docs", AnyID, Allow(Update)))
	lease, _ := NewStaticPolicyLease(allow)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedUpdate("op1", key, []update.Update{update.ByFieldName("title", "new")}, "")
	err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(session ExecutionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil || assessment.Outcome != AssessmentIndeterminate || assessment.Complete || len(assessment.Policies) != 2 {
			t.Fatalf("assessment=%+v err=%v", assessment, err)
		}
		if assessment.Policies[0].Decision.Allowed != true || assessment.Policies[1].Decision.Code != CodeEvaluationFailed {
			t.Fatalf("policies=%+v", assessment.Policies)
		}
		if strings.Contains(assessment.Policies[1].Decision.Explanation, "secret") {
			t.Fatalf("preparation cause leaked: %+v", assessment.Policies[1])
		}
		visible, err := session.ReadVisibility(context.Background())
		if err != nil || visible["op1"] {
			t.Fatalf("visibility=%v err=%v", visible, err)
		}
		_, err = session.Execute(context.Background())
		return err
	})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("err=%v", err)
	}
	if strings.Join(events, ",") != "storage-enter,evidence,storage-abort" {
		t.Fatalf("events=%v", events)
	}
}

func TestPreparationFailurePreservesStaticDenialAndOtherOperations(t *testing.T) {
	events := []string{}
	badKey := record.NewKeyWithID("docs", "bad")
	goodKey := record.NewKeyWithID("docs", "good")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{
		{OperationID: "bad", CanonicalTarget: badKey.String(), PreparationError: errors.New("private")},
		{OperationID: "good", CanonicalTarget: goodKey.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, Complete: true},
	}}
	deny := MustPolicy("deny", Scope("docs", "bad", Deny(Get)), Scope("docs", "good", Allow(Get)))
	lease, _ := NewStaticPolicyLease(deny)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	bad, _ := NewProtectedRead("bad", Get, badKey)
	good, _ := NewProtectedRead("good", Get, goodKey)
	err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{bad, good}, func(session InspectionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil || assessment.Outcome != AssessmentDeny || assessment.Complete {
			t.Fatalf("assessment=%+v err=%v", assessment, err)
		}
		seen := map[string]bool{}
		for _, policy := range assessment.Policies {
			seen[policy.OperationID] = true
		}
		if !seen["bad"] || !seen["good"] {
			t.Fatalf("missing operation assessments: %+v", assessment.Policies)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreparationFailureCannotCarryPrivateEvidence(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{
		OperationID: "read", CanonicalTarget: key.String(), PreparationError: errors.New("bad"), PreImage: map[string]any{"secret": "value"},
	}}}
	allow := MustPolicy("reader", Scope("docs", AnyID, Allow(Get)))
	lease, _ := NewStaticPolicyLease(allow)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedRead("read", Get, key)
	called := false
	err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(InspectionSession) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestCoordinatorDoesNotReplayImpurePolicyForInspectionOrVisibility(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	evidence := []ProtectedEvidence{{OperationID: "op1", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, CandidateImage: map[string]any{"title": "new"}, CandidateRevision: "v2", Complete: true}}
	storage := &coordinatorStorage{events: &events, evidence: evidence}
	calls := 0
	custom := impureAssessmentPolicy{calls: &calls}
	lease, _ := NewStaticPolicyLease(custom)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedSet("op1", key, map[string]any{"title": "new"}, "")
	err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(session InspectionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil || calls != 0 || assessment.Outcome != AssessmentIndeterminate || assessment.Policies[0].Decision.Code != CodeEnforcementUnsupported {
			t.Fatalf("calls=%d assessment=%+v err=%v", calls, assessment, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	events = nil
	err = coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(session ExecutionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil || assessment.Outcome != AssessmentAllow || calls != 1 {
			t.Fatalf("calls=%d assessment=%+v err=%v", calls, assessment, err)
		}
		visible, err := session.ReadVisibility(context.Background())
		if err != nil || visible["op1"] || calls != 1 {
			t.Fatalf("visibility=%v calls=%d err=%v", visible, calls, err)
		}
		_, err = session.Execute(context.Background())
		return err
	})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

type coordinatorStorage struct {
	events      *[]string
	evidence    []ProtectedEvidence
	evidenceErr error
	executeErr  error
	callbackErr error
}
type coordinatorInspectionStorage struct{ parent *coordinatorStorage }

func (s coordinatorInspectionStorage) Evidence(context.Context) ([]ProtectedEvidence, error) {
	*s.parent.events = append(*s.parent.events, "evidence")
	if s.parent.evidenceErr != nil {
		return nil, s.parent.evidenceErr
	}
	return cloneProtectedEvidence(s.parent.evidence), nil
}

type coordinatorExecutionStorage struct{ coordinatorInspectionStorage }

func (s coordinatorExecutionStorage) Execute(context.Context) error {
	*s.parent.events = append(*s.parent.events, "execute")
	return s.parent.executeErr
}
func (s *coordinatorStorage) WithinProtectedInspection(_ context.Context, _ []ProtectedOperation, fn func(ProtectedInspectionStorage) error) error {
	*s.events = append(*s.events, "storage-enter")
	err := fn(coordinatorInspectionStorage{s})
	if err != nil {
		*s.events = append(*s.events, "storage-abort")
		return err
	}
	*s.events = append(*s.events, "storage-close")
	return nil
}
func (s *coordinatorStorage) WithinProtectedExecution(_ context.Context, _ []ProtectedOperation, fn func(ProtectedExecutionStorage) error) error {
	*s.events = append(*s.events, "storage-enter")
	err := fn(coordinatorExecutionStorage{coordinatorInspectionStorage{s}})
	if err != nil {
		*s.events = append(*s.events, "storage-abort")
		return err
	}
	*s.events = append(*s.events, "storage-commit")
	return nil
}

type automaticCoordinatorStorage struct{ operations []ProtectedOperation }

func (s *automaticCoordinatorStorage) WithinProtectedInspection(_ context.Context, operations []ProtectedOperation, fn func(ProtectedInspectionStorage) error) error {
	s.operations = operations
	return fn(automaticInspectionStorage{s})
}
func (s *automaticCoordinatorStorage) WithinProtectedExecution(_ context.Context, operations []ProtectedOperation, fn func(ProtectedExecutionStorage) error) error {
	s.operations = operations
	return fn(automaticExecutionStorage{automaticInspectionStorage{s}})
}

type automaticInspectionStorage struct{ storage *automaticCoordinatorStorage }

func (s automaticInspectionStorage) Evidence(context.Context) ([]ProtectedEvidence, error) {
	result := make([]ProtectedEvidence, len(s.storage.operations))
	for i, op := range s.storage.operations {
		exists := op.Action() != Insert
		var pre map[string]any
		if exists {
			pre = map[string]any{"name": "old"}
		}
		var candidate map[string]any
		switch op.Action() {
		case Insert, Set:
			candidate = op.Data()
		case Update:
			candidate = map[string]any{"name": "new"}
		}
		result[i] = ProtectedEvidence{OperationID: op.ID(), CanonicalTarget: op.CanonicalTarget(), SnapshotToken: "s", DataRevision: "r1", CandidateRevision: "r2", Exists: exists, Complete: true, PreImage: pre, CandidateImage: candidate}
	}
	return result, nil
}

type automaticExecutionStorage struct{ automaticInspectionStorage }

func (automaticExecutionStorage) Execute(context.Context) error { return nil }

func TestProtectedOperationConstructionIsDefensive(t *testing.T) {
	parent := record.NewKeyWithID("accounts", "a1")
	key, err := record.NewKeyWithOptions("docs", record.WithKeyID("d1"), record.WithParentKey(parent))
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"nested": map[string]any{"items": []any{map[string]any{"value": "original"}}}}
	op, err := NewProtectedSet("set", key, data, "r1")
	if err != nil {
		t.Fatal(err)
	}
	data["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "mutated"
	got := op.Data()
	if got["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] != "original" {
		t.Fatal("constructor retained mutable input")
	}
	got["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "returned"
	if op.Data()["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] != "original" {
		t.Fatal("Data exposed mutable operation state")
	}
	if op.ID() != "set" || op.Action() != Set || op.IfDataRevision() != "r1" || op.CanonicalTarget() != key.String() || op.Key().Parent().ID != "a1" {
		t.Fatalf("operation accessors changed values: %+v", op)
	}

	changes := []update.Update{update.ByFieldPath(update.FieldPath{"profile", "name"}, []any{map[string]any{"v": "x"}}), update.DeleteByFieldPath("obsolete")}
	updated, err := NewProtectedUpdate("update", key, changes, "r2")
	if err != nil {
		t.Fatal(err)
	}
	first := updated.Updates()
	first[0].Path[0] = "changed"
	first[0].Value.([]any)[0].(map[string]any)["v"] = "changed"
	if again := updated.Updates(); again[0].Path[0] != "profile" || again[0].Value.([]any)[0].(map[string]any)["v"] != "x" || !again[1].Delete {
		t.Fatalf("Updates exposed mutable state: %+v", again)
	}
	if deleted, err := NewProtectedDelete("delete", key, "r3"); err != nil || deleted.Action() != Delete {
		t.Fatalf("delete=%+v err=%v", deleted, err)
	}
	if _, err := NewProtectedRead("bad", Update, key); err == nil {
		t.Fatal("invalid protected read action accepted")
	}
	if _, err := NewProtectedUpdate("bad", key, []update.Update{nil}, ""); err == nil {
		t.Fatal("nil update accepted")
	}
}

func TestReadVisibilityForRequiresBothPrincipals(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "read", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"ownerID": "u1"}, Complete: true}}}
	policy := MustPolicy("owner", Scope("docs", AnyID, Allow(Get).Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser")))))
	lease, _ := NewStaticPolicyLease(policy)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedRead("read", Get, key)
	err := coordinator.WithinInspection(WithCurrentUser(context.Background(), "u1"), []ProtectedOperation{op}, func(session InspectionSession) error {
		other, owner := "u2", "u1"
		visible, err := session.ReadVisibilityFor(context.Background(), Principal{ID: &other})
		if err != nil || visible["read"] {
			t.Fatalf("other visibility=%v err=%v", visible, err)
		}
		visible, err = session.ReadVisibilityFor(context.Background(), Principal{ID: &owner})
		if err != nil || !visible["read"] {
			t.Fatalf("owner visibility=%v err=%v", visible, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type eventLease struct {
	policies []Policy
	events   *[]string
}

func (l *eventLease) Policies() []Policy { return append([]Policy(nil), l.policies...) }
func (*eventLease) Revision() string     { return "r1" }
func (l *eventLease) Release()           { *l.events = append(*l.events, "lease-release") }

func TestCoordinatorLockLeaseExecuteAndLifetime(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "op1", CanonicalTarget: key.String(), SnapshotToken: "snapshot", Exists: true, PreImage: map[string]any{"ownerID": "u1"}, CandidateImage: map[string]any{"ownerID": "u1", "title": "new"}, DataRevision: "v1", CandidateRevision: "v2", Complete: true}}}
	policy := MustPolicy("owner", Scope("docs", AnyID, Allow(Update, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("title")))
	coordinator, err := NewValidatedEnforcementCoordinator(storage, func(_ context.Context, _ ProtectedOperation, image map[string]any) error {
		events = append(events, "validate")
		image["ownerID"] = "mutated-copy"
		return nil
	}, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) {
		events = append(events, "lease")
		return &eventLease{policies: []Policy{policy}, events: &events}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	op, err := NewProtectedUpdate("op1", key, []update.Update{update.ByFieldName("title", "new")}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithCurrentUser(context.Background(), "u1")
	var captured InspectionSession
	err = coordinator.WithinExecution(ctx, []ProtectedOperation{op}, func(session ExecutionSession) error {
		captured = session
		if _, ok := any(session).(ProtectedExecutionStorage); ok {
			t.Fatal("execution session exposes storage")
		}
		assessment, err := session.Execute(ctx)
		if err != nil || assessment.Outcome != AssessmentAllow {
			t.Fatalf("execute assessment=%+v err=%v", assessment, err)
		}
		revisions, err := session.Revisions(ctx)
		if err != nil || revisions["op1"] != "v2" {
			t.Fatalf("revisions=%v err=%v", revisions, err)
		}
		if _, err = session.Execute(ctx); err == nil {
			t.Fatal("second execute succeeded")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"storage-enter", "lease", "evidence", "validate", "execute", "storage-commit", "lease-release"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
	if _, err := captured.Assess(ctx); err == nil {
		t.Fatal("retained session remained usable")
	}
}

func TestInspectionCannotExecuteAndDenialHidesEvidence(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "read", CanonicalTarget: key.String(), SnapshotToken: "snapshot", Exists: true, PreImage: map[string]any{"ownerID": "other", "secret": "value"}, DataRevision: "whole-image", Complete: true}}}
	policy := MustPolicy("owner", Scope("docs", AnyID, Allow(Get, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("secret")))
	lease, _ := NewStaticPolicyLease(policy)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedEvidenceRead("read", Get, key, [][]string{{"secret"}})
	err := coordinator.WithinInspection(WithCurrentUser(context.Background(), "u1"), []ProtectedOperation{op}, func(session InspectionSession) error {
		if _, ok := any(session).(ExecutionSession); ok {
			t.Fatal("inspection implements execution")
		}
		assessment, err := session.Assess(context.Background())
		if err != nil || assessment.Outcome != AssessmentDeny {
			t.Fatalf("assessment=%+v err=%v", assessment, err)
		}
		if _, err := session.Evidence(context.Background()); !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("evidence err=%v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizedPointEvidenceReturnsOnlyDeclaredFields(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "read", CanonicalTarget: key.String(), SnapshotToken: "snapshot", Exists: true, PreImage: map[string]any{"title": "visible", "secret": "hidden"}, DataRevision: "whole", Complete: true}}}
	policy := MustPolicy("reader", Scope("docs", AnyID, Allow(Get).Fields("title")))
	lease, _ := NewStaticPolicyLease(policy)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedEvidenceRead("read", Get, key, [][]string{{"title"}})
	err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(session InspectionSession) error {
		facts, err := session.Evidence(context.Background())
		if err != nil {
			return err
		}
		if len(facts) != 1 || facts[0].DataRevision != "whole" || len(facts[0].Fields) != 1 || facts[0].Fields[0].Value != "visible" {
			t.Fatalf("facts=%+v", facts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecutionWithoutExecuteAbortsAndReleasesLease(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "op1", CanonicalTarget: key.String(), SnapshotToken: "snapshot", Exists: true, PreImage: map[string]any{}, CandidateImage: map[string]any{"title": "x"}, CandidateRevision: "v2", Complete: true}}}
	policy := MustPolicy("writer", Scope("docs", AnyID, Allow(Set)))
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) {
		events = append(events, "lease")
		return &eventLease{policies: []Policy{policy}, events: &events}, nil
	}})
	op, _ := NewProtectedSet("op1", key, map[string]any{"title": "x"}, "")
	err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(ExecutionSession) error { return nil })
	if err == nil {
		t.Fatal("callback without Execute committed")
	}
	want := []string{"storage-enter", "lease", "evidence", "storage-abort", "lease-release"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v", events)
	}
}

func TestProtectedProfileRejectsDynamicWorkerBeforeInvocation(t *testing.T) {
	// A concrete DB stub is unnecessary: the coordinator option is tested on
	// the existing stub DB used throughout this package.
	fs := &fakeSession{}
	tx := &fakeTx{fakeSession: &fakeSession{}, opts: dal.NewTransactionOptions()}
	raw := &fakeDB{fakeSession: fs, adapter: dal.NewAdapter("a", "1"), schema: dal.NewSchema(nil, nil), ro: tx, rw: tx}
	db := dal.NewDB(raw)
	events := []string{}
	storage := &coordinatorStorage{events: &events}
	policy := MustPolicy("p", Root(Allow(Update)))
	lease, _ := NewStaticPolicyLease(policy)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	secured, err := SecureDB(db, WithEnforcementCoordinator(coordinator))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = secured.RunReadwriteTransaction(context.Background(), func(context.Context, dal.ReadwriteTransaction) error { called = true; return nil })
	if !errors.Is(err, ErrAccessDenied) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestProtectedDatabaseRoutesEveryWriteThroughCoordinator(t *testing.T) {
	fs := &fakeSession{}
	tx := &fakeTx{fakeSession: &fakeSession{}, opts: dal.NewTransactionOptions()}
	raw := &fakeDB{fakeSession: fs, adapter: dal.NewAdapter("a", "1"), schema: dal.NewSchema(nil, nil), ro: tx, rw: tx}
	storage := &automaticCoordinatorStorage{}
	policy := MustPolicy("writer", Root(Allow(Set|Insert|Update|Delete)))
	lease, _ := NewStaticPolicyLease(policy)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	secured, err := SecureDB(dal.NewDB(raw), WithEnforcementCoordinator(coordinator))
	if err != nil {
		t.Fatal(err)
	}
	writer := secured.(dal.WriteSession)
	key1, key2 := record.NewKeyWithID("docs", "d1"), record.NewKeyWithID("docs", "d2")
	rec1, rec2 := record.NewRecordWithData(key1, map[string]any{"name": "one"}), record.NewRecordWithData(key2, map[string]any{"name": "two"})
	change := []update.Update{update.ByFieldName("name", "new")}
	for name, call := range map[string]func() error{
		"set":           func() error { return writer.Set(context.Background(), rec1) },
		"set multi":     func() error { return writer.SetMulti(context.Background(), []record.Record{rec1, rec2}) },
		"insert":        func() error { return writer.Insert(context.Background(), rec1) },
		"insert multi":  func() error { return writer.InsertMulti(context.Background(), []record.Record{rec1, rec2}) },
		"update":        func() error { return writer.Update(context.Background(), key1, change) },
		"update record": func() error { return writer.UpdateRecord(context.Background(), rec1, change) },
		"update multi":  func() error { return writer.UpdateMulti(context.Background(), []*record.Key{key1, key2}, change) },
		"delete":        func() error { return writer.Delete(context.Background(), key1) },
		"delete multi":  func() error { return writer.DeleteMulti(context.Background(), []*record.Key{key1, key2}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if len(fs.calls) != 0 {
		t.Fatalf("raw write session was used: %v", fs.calls)
	}
	if err := writer.Insert(context.Background(), rec1, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("insert option err=%v", err)
	}
	if err := writer.InsertMulti(context.Background(), []record.Record{rec1}, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("insert multi option err=%v", err)
	}
	if err := writer.Update(context.Background(), key1, change, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("update precondition err=%v", err)
	}
	if err := writer.UpdateMulti(context.Background(), []*record.Key{key1}, change, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("update multi precondition err=%v", err)
	}
}

func TestValidationOnlyParticipantDoesNotCreateACLLayer(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "op1", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: false, CandidateImage: map[string]any{"title": "x"}, CandidateRevision: "v1", Complete: true}}}
	allow := MustPolicy("allow", Scope("docs", AnyID, Allow(Insert)))
	lease, _ := NewStaticPolicyLease(allow)
	validated := false
	coordinator, err := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "schema", Validator: func(context.Context, ProtectedOperation, map[string]any) error { validated = true; return nil }}, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	if err != nil {
		t.Fatal(err)
	}
	op, _ := NewProtectedInsert("op1", key, map[string]any{"title": "x"})
	err = coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(session InspectionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil {
			return err
		}
		if len(assessment.Policies) != 1 || assessment.Policies[0].LayerID != "owner" {
			t.Fatalf("policies=%+v", assessment.Policies)
		}
		return nil
	})
	if err != nil || !validated {
		t.Fatalf("err=%v validated=%v", err, validated)
	}
}

func TestUnavailableProviderStillCollectsDefinitiveDenial(t *testing.T) {
	events := []string{}
	key := record.NewKeyWithID("docs", "d1")
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "read", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, Complete: true}}}
	deny := MustPolicy("deny", Scope("docs", AnyID, Deny(Get)))
	lease, _ := NewStaticPolicyLease(deny)
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "offline", Provider: func(context.Context) (PolicyLease, error) { return nil, errors.New("offline detail") }}, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	op, _ := NewProtectedRead("read", Get, key)
	err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(session InspectionSession) error {
		assessment, err := session.Assess(context.Background())
		if err != nil {
			return err
		}
		if assessment.Outcome != AssessmentDeny || len(assessment.Policies) != 2 || assessment.Policies[0].Decision.Code != CodeSourceUnavailable || assessment.Policies[1].Decision.Code != CodeRuleDenied {
			t.Fatalf("assessment=%+v", assessment)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorRejectsInvalidRegistrationAndOperationBatches(t *testing.T) {
	events := []string{}
	storage := &coordinatorStorage{events: &events}
	allow := MustPolicy("allow", Root(Allow(ReadWrite)))
	lease, _ := NewStaticPolicyLease(allow)
	provider := func(context.Context) (PolicyLease, error) { return lease, nil }
	for name, participants := range map[string][]MandatoryParticipant{
		"none":              nil,
		"empty layer":       {{Provider: provider}},
		"empty participant": {{LayerID: "owner"}},
		"duplicate layer":   {{LayerID: "owner", Provider: provider}, {LayerID: "owner", Validator: func(context.Context, ProtectedOperation, map[string]any) error { return nil }}},
		"validation only":   {{LayerID: "schema", Validator: func(context.Context, ProtectedOperation, map[string]any) error { return nil }}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewEnforcementCoordinator(storage, participants...); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if _, err := NewEnforcementCoordinator(nil, MandatoryParticipant{LayerID: "owner", Provider: provider}); err == nil {
		t.Fatal("nil storage accepted")
	}
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: provider})
	key := record.NewKeyWithID("docs", "d1")
	op, _ := NewProtectedRead("same", Get, key)
	if err := coordinator.WithinInspection(context.Background(), nil, func(InspectionSession) error { return nil }); err == nil {
		t.Fatal("empty inspection batch accepted")
	}
	if err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, nil); err == nil {
		t.Fatal("nil inspection callback accepted")
	}
	if err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op, op}, func(InspectionSession) error { return nil }); err == nil {
		t.Fatal("duplicate operation id accepted")
	}
	other, _ := NewProtectedRead("other", Get, key)
	if err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op, other}, func(ExecutionSession) error { return nil }); err == nil {
		t.Fatal("duplicate execution target accepted")
	}
	if err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, nil); err == nil {
		t.Fatal("nil execution callback accepted")
	}
}

func TestCoordinatorDefersAdmissionFailuresUntilAfterAuthorization(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	allow := MustPolicy("allow", Scope("docs", AnyID, Allow(Insert|Set|Update|Delete|Get)))
	lease, _ := NewStaticPolicyLease(allow)
	provider := func(context.Context) (PolicyLease, error) { return lease, nil }
	tests := []struct {
		name     string
		op       ProtectedOperation
		evidence ProtectedEvidence
		want     error
	}{
		{"missing update", mustProtectedUpdate(t, "op", key, "r1"), ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", DataRevision: "r1", CandidateRevision: "r2", CandidateImage: map[string]any{"name": "n"}, Complete: true}, ErrProtectedResourceUnavailable},
		{"existing insert", mustProtectedInsert(t, "op", key), ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"name": "old"}, CandidateImage: map[string]any{"name": "new"}, CandidateRevision: "r2", Complete: true}, ErrProtectedRecordExists},
		{"stale revision", mustProtectedSet(t, "op", key, "wanted"), ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"name": "old"}, DataRevision: "actual", CandidateImage: map[string]any{"name": "new"}, CandidateRevision: "r2", Complete: true}, ErrDataRevisionConflict},
		{"missing candidate revision", mustProtectedSet(t, "op", key, ""), ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"name": "old"}, CandidateImage: map[string]any{"name": "new"}, Complete: true}, ErrAccessDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{}
			storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{tc.evidence}}
			coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: provider})
			err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{tc.op}, func(session InspectionSession) error {
				assessment, err := session.Assess(context.Background())
				if assessment.Outcome != AssessmentAllow {
					t.Fatalf("authorization was not completed: %+v", assessment)
				}
				return err
			})
			if tc.name == "missing candidate revision" {
				if err == nil || errors.Is(err, ErrAccessDenied) {
					t.Fatalf("validation error=%v", err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

func mustProtectedUpdate(t *testing.T, id string, key *record.Key, revision string) ProtectedOperation {
	t.Helper()
	op, err := NewProtectedUpdate(id, key, []update.Update{update.ByFieldName("name", "n")}, revision)
	if err != nil {
		t.Fatal(err)
	}
	return op
}
func mustProtectedInsert(t *testing.T, id string, key *record.Key) ProtectedOperation {
	t.Helper()
	op, err := NewProtectedInsert(id, key, map[string]any{"name": "new"})
	if err != nil {
		t.Fatal(err)
	}
	return op
}
func mustProtectedSet(t *testing.T, id string, key *record.Key, revision string) ProtectedOperation {
	t.Helper()
	op, err := NewProtectedSet(id, key, map[string]any{"name": "new"}, revision)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func TestCoordinatorSessionLifetimeCancellationAndReceipts(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	allow := MustPolicy("allow", Scope("docs", AnyID, Allow(Set|Get).Fields("name")))
	lease, _ := NewStaticPolicyLease(allow)
	provider := func(context.Context) (PolicyLease, error) { return lease, nil }
	evidence := []ProtectedEvidence{{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"name": "old"}, DataRevision: "r1", CandidateImage: map[string]any{"name": "new"}, CandidateRevision: "r2", Complete: true}}
	events := []string{}
	storage := &coordinatorStorage{events: &events, evidence: evidence}
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: provider})
	op, _ := NewProtectedSet("op", key, map[string]any{"name": "new"}, "")
	ctx, cancel := context.WithCancel(context.Background())
	var retained ExecutionSession
	err := coordinator.WithinExecution(ctx, []ProtectedOperation{op}, func(session ExecutionSession) error {
		retained = session
		if _, err := session.Receipts(context.Background()); err == nil {
			t.Fatal("receipts available before execution")
		}
		canceled, stop := context.WithCancel(context.Background())
		stop()
		if _, err := session.Assess(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("Assess canceled err=%v", err)
		}
		if _, err := session.ReadVisibility(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadVisibility canceled err=%v", err)
		}
		if _, err := session.Evidence(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("Evidence canceled err=%v", err)
		}
		if _, err := session.Execute(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute canceled err=%v", err)
		}
		assessment, err := session.Execute(context.Background())
		if err != nil || assessment.Outcome != AssessmentAllow {
			t.Fatalf("Execute assessment=%+v err=%v", assessment, err)
		}
		receipts, err := session.Receipts(context.Background())
		if err != nil || len(receipts) != 1 || receipts[0].DataRevision != "r2" {
			t.Fatalf("receipts=%v err=%v", receipts, err)
		}
		receipts[0].DataRevision = "changed"
		again, _ := session.Receipts(context.Background())
		if again[0].DataRevision != "r2" {
			t.Fatal("receipt result was mutable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := retained.Assess(context.Background()); err == nil {
		t.Fatal("closed Assess succeeded")
	}
	if _, err := retained.ReadVisibility(context.Background()); err == nil {
		t.Fatal("closed ReadVisibility succeeded")
	}
	if _, err := retained.Evidence(context.Background()); err == nil {
		t.Fatal("closed Evidence succeeded")
	}
	if _, err := retained.Execute(context.Background()); err == nil {
		t.Fatal("closed Execute succeeded")
	}
	if _, err := retained.Receipts(context.Background()); err == nil {
		t.Fatal("closed Receipts succeeded")
	}
	if _, err := retained.Revisions(context.Background()); err == nil {
		t.Fatal("closed Revisions succeeded")
	}
}

func TestCoordinatorStorageAndExecutionFailuresAbort(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	op, _ := NewProtectedSet("op", key, map[string]any{"name": "new"}, "")
	allow := MustPolicy("allow", Scope("docs", AnyID, Allow(Set)))
	lease, _ := NewStaticPolicyLease(allow)
	provider := func(context.Context) (PolicyLease, error) { return lease, nil }
	evidence := []ProtectedEvidence{{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, CandidateImage: map[string]any{"name": "new"}, CandidateRevision: "r2", Complete: true}}
	for name, storage := range map[string]*coordinatorStorage{
		"evidence": {events: &[]string{}, evidenceErr: errors.New("evidence failed")},
		"execute":  {events: &[]string{}, evidence: evidence, executeErr: errors.New("execute failed")},
	} {
		t.Run(name, func(t *testing.T) {
			coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: provider})
			err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(session ExecutionSession) error {
				_, err := session.Execute(context.Background())
				return err
			})
			if err == nil || !strings.Contains(strings.Join(*storage.events, ","), "storage-abort") {
				t.Fatalf("err=%v events=%v", err, *storage.events)
			}
		})
	}
}

func TestProtectedEvidenceBoundaryRejectsMalformedAdapterOutput(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	op, _ := NewProtectedRead("op", Get, key)
	allow := MustPolicy("allow", Scope("docs", AnyID, Allow(Get)))
	lease, _ := NewStaticPolicyLease(allow)
	participants := []MandatoryParticipant{{LayerID: "owner"}}
	valid := ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, Complete: true}
	tests := map[string][]ProtectedEvidence{
		"missing":          nil,
		"empty id":         {{CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true}},
		"duplicate":        {valid, valid},
		"incomplete":       {{OperationID: "op", CanonicalTarget: key.String()}},
		"failed complete":  {{OperationID: "op", CanonicalTarget: key.String(), PreparationError: errors.New("private"), Complete: true}},
		"failed snapshot":  {{OperationID: "op", CanonicalTarget: key.String(), PreparationError: errors.New("private"), SnapshotToken: "secret"}},
		"wrong target":     {{OperationID: "op", CanonicalTarget: "wrong", SnapshotToken: "s", Complete: true}},
		"present no image": {{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, Complete: true}},
		"absent image":     {{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", PreImage: map[string]any{}, Complete: true}},
	}
	for name, evidence := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := assessProtected(context.Background(), []ProtectedOperation{op}, evidence, participants, []PolicyLease{lease}, true); err == nil {
				t.Fatal("malformed evidence accepted")
			}
		})
	}
}

func TestCandidateValidatorsFailClosedWithoutMutatingEvidence(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	set, _ := NewProtectedSet("op", key, map[string]any{"name": "new"}, "")
	del, _ := NewProtectedDelete("op", key, "")
	base := ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{}, CandidateImage: map[string]any{"name": "new"}, CandidateRevision: "r2", Complete: true}
	events := []string{}
	storage := &coordinatorStorage{events: &events}
	allow := MustPolicy("allow", Root(Allow(Set|Delete)))
	lease, _ := NewStaticPolicyLease(allow)
	provider := func(context.Context) (PolicyLease, error) { return lease, nil }
	coordinator, _ := NewValidatedEnforcementCoordinator(storage, func(_ context.Context, _ ProtectedOperation, image map[string]any) error {
		image["name"] = "mutated"
		return errors.New("invalid candidate")
	}, MandatoryParticipant{LayerID: "schema", Validator: func(context.Context, ProtectedOperation, map[string]any) error { return nil }}, MandatoryParticipant{LayerID: "owner", Provider: provider})
	if err := coordinator.validateCandidates(context.Background(), []ProtectedOperation{set}, []ProtectedEvidence{base}); err == nil || base.CandidateImage["name"] != "new" {
		t.Fatalf("err=%v evidence=%v", err, base.CandidateImage)
	}
	for name, opAndEvidence := range map[string]struct {
		op ProtectedOperation
		ev []ProtectedEvidence
	}{
		"missing":      {set, nil},
		"candidate":    {set, []ProtectedEvidence{{OperationID: "op"}}},
		"revision":     {set, []ProtectedEvidence{{OperationID: "op", CandidateImage: map[string]any{}}}},
		"delete image": {del, []ProtectedEvidence{{OperationID: "op", CandidateImage: map[string]any{}, CandidateRevision: "r"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := coordinator.validateCandidates(context.Background(), []ProtectedOperation{opAndEvidence.op}, opAndEvidence.ev); err == nil {
				t.Fatal("invalid candidate evidence accepted")
			}
		})
	}
	deleteEvidence := base
	deleteEvidence.CandidateImage = nil
	if err := coordinator.validateCandidates(context.Background(), []ProtectedOperation{del}, []ProtectedEvidence{deleteEvidence}); err != nil {
		t.Fatalf("valid delete rejected: %v", err)
	}
}

func TestPolicyLeaseSnapshotFailuresAndCancellation(t *testing.T) {
	events := []string{}
	policy := MustPolicy("allow", Root(Allow(Get)))
	valid := &eventLease{policies: []Policy{policy}, events: &events}
	tooMany := make([]Policy, 101)
	for i := range tooMany {
		tooMany[i] = policy
	}
	coordinator := &EnforcementCoordinator{participants: []MandatoryParticipant{
		{LayerID: "nil", Provider: func(context.Context) (PolicyLease, error) { return nil, nil }},
		{LayerID: "empty", Provider: func(context.Context) (PolicyLease, error) { return &eventLease{events: &events}, nil }},
		{LayerID: "large", Provider: func(context.Context) (PolicyLease, error) {
			return &eventLease{policies: tooMany, events: &events}, nil
		}},
	}}
	leases, err := coordinator.acquire(context.Background())
	if err != nil || len(leases) != 3 {
		t.Fatalf("leases=%v err=%v", leases, err)
	}
	for _, lease := range leases {
		if got := lease.Policies()[0].Decide(context.Background(), Request{Operation: Get}).Code; got != CodeConfigurationInvalid {
			t.Fatalf("code=%s", got)
		}
	}
	releaseLeases(leases)
	ctx, cancel := context.WithCancel(context.Background())
	canceled := &EnforcementCoordinator{participants: []MandatoryParticipant{{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { cancel(); return valid, nil }}}}
	if _, err := canceled.acquire(ctx); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(events[len(events)-1:], []string{"lease-release"}) {
		t.Fatalf("err=%v events=%v", err, events)
	}
	if _, err := NewStaticPolicyLease(); err == nil {
		t.Fatal("empty static lease accepted")
	}
	if _, err := NewStaticPolicyLease(nil); err == nil {
		t.Fatal("nil static policy accepted")
	}
	participant, err := NewStaticParticipant("owner", policy)
	if err != nil {
		t.Fatal(err)
	}
	static, _ := participant.Provider(context.Background())
	if static.Revision() != "static" || len(static.Policies()) != 1 {
		t.Fatalf("static lease=%+v", static)
	}
	static.Release()
	if _, err := NewStaticParticipant("", policy); err == nil {
		t.Fatal("empty static participant layer accepted")
	}
}

type pureDecisionPolicy struct{ decision Decision }

func (pureDecisionPolicy) Name() string                               { return "custom" }
func (pureDecisionPolicy) InspectionPure() bool                       { return true }
func (p pureDecisionPolicy) Decide(context.Context, Request) Decision { return p.decision }
func (p pureDecisionPolicy) Authorize(context.Context, Request) error {
	if p.decision.Allowed {
		return nil
	}
	return ErrAccessDenied
}

func TestProtectedEvidenceEvaluatesRowAndWriteObligations(t *testing.T) {
	key := record.NewKeyWithID("docs", "d1")
	resource := RecordResourceForKey(key)
	get, _ := NewProtectedRead("op", Get, key)
	readEvidence := ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"owner": "u2"}, Complete: true}
	for name, policy := range map[string]Policy{
		"row false": MustPolicy("row", Scope("docs", AnyID, Allow(Get).Where(dal.WhereField("owner", dal.Equal, "u1")))),
		"row error": pureDecisionPolicy{decision: Decision{Allowed: true, Operation: Get, Resource: resource, Effect: "allow", Residuals: []dal.Condition{fakeCond{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			lease, _ := NewStaticPolicyLease(policy)
			assessment, err := assessProtected(context.Background(), []ProtectedOperation{get}, []ProtectedEvidence{readEvidence}, []MandatoryParticipant{{LayerID: "owner"}}, []PolicyLease{lease}, false)
			if err != nil || assessment.Outcome == AssessmentAllow {
				t.Fatalf("assessment=%+v err=%v", assessment, err)
			}
		})
	}
	updateOp, _ := NewProtectedUpdate("op", key, []update.Update{update.DeleteByFieldPath("obsolete")}, "")
	writeEvidence := ProtectedEvidence{OperationID: "op", CanonicalTarget: key.String(), SnapshotToken: "s", Exists: true, PreImage: map[string]any{"owner": "u2", "obsolete": "x"}, CandidateImage: map[string]any{"owner": "u2"}, CandidateRevision: "r2", Complete: true}
	writePolicy := MustPolicy("write", Scope("docs", AnyID, Allow(Update).Where(dal.WhereField("owner", dal.Equal, "u1"))))
	lease, _ := NewStaticPolicyLease(writePolicy)
	assessment, err := assessProtected(context.Background(), []ProtectedOperation{updateOp}, []ProtectedEvidence{writeEvidence}, []MandatoryParticipant{{LayerID: "owner"}}, []PolicyLease{lease}, false)
	if err != nil || assessment.Outcome != AssessmentDeny || assessment.Policies[0].Decision.Code != CodeRowPredicateFailed {
		t.Fatalf("assessment=%+v err=%v", assessment, err)
	}
}
