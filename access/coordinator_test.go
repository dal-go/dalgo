package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type coordinatorStorage struct {
	events      *[]string
	evidence    []ProtectedEvidence
	executeErr  error
	callbackErr error
}
type coordinatorInspectionStorage struct{ parent *coordinatorStorage }

func (s coordinatorInspectionStorage) Evidence(context.Context) ([]ProtectedEvidence, error) {
	*s.parent.events = append(*s.parent.events, "evidence")
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
