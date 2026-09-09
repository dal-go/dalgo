package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/record"
)

func TestProtectedOperationConstructorBoundaries(t *testing.T) {
	key := record.NewKeyWithID("docs", "one")
	if _, err := NewProtectedRead("x", Update, key); err == nil {
		t.Fatal("write action accepted as read")
	}
	if _, err := NewProtectedEvidenceRead("x", Exists, key, [][]string{{"name"}}); err == nil {
		t.Fatal("exists evidence fields accepted")
	}
	if _, err := NewProtectedEvidenceRead("x", Get, key, nil); err == nil {
		t.Fatal("empty evidence field list accepted")
	}
	many := make([][]string, 33)
	if _, err := NewProtectedEvidenceRead("x", Get, key, many); err == nil {
		t.Fatal("oversized evidence field list accepted")
	}
	if _, err := NewProtectedEvidenceRead("x", Get, key, [][]string{{}}); err == nil {
		t.Fatal("empty evidence path accepted")
	}
	if _, err := newProtectedOperation("", Get, key, nil, nil, ""); err == nil {
		t.Fatal("empty operation id accepted")
	}
	if _, err := newProtectedOperation("x", Get, nil, nil, nil, ""); err == nil {
		t.Fatal("nil key accepted")
	}
	if cloneKey(nil) != nil || cloneMap(nil) != nil {
		t.Fatal("nil clones changed")
	}
}

func TestInspectionSessionLifetimeAndContextBoundaries(t *testing.T) {
	closed := &inspectionSession{}
	if _, err := closed.Assess(context.Background()); err == nil {
		t.Fatal("closed assess accepted")
	}
	if _, err := closed.Evidence(context.Background()); err == nil {
		t.Fatal("closed evidence accepted")
	}
	if _, err := closed.ReadVisibility(context.Background()); err == nil {
		t.Fatal("closed visibility accepted")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	s := &inspectionSession{alive: true, ingress: context.Background()}
	if _, err := s.Assess(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("assess err=%v", err)
	}
	if _, err := s.Evidence(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("evidence err=%v", err)
	}
	if _, err := s.ReadVisibility(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("visibility err=%v", err)
	}

	ingress, stop := context.WithCancel(context.Background())
	stop()
	s = &inspectionSession{alive: true, ingress: ingress}
	if _, err := s.Assess(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("assess ingress err=%v", err)
	}
	if _, err := s.Evidence(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("evidence ingress err=%v", err)
	}
	if _, err := s.ReadVisibility(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("visibility ingress err=%v", err)
	}
}

func TestExecutionSessionLifetimeAndReceiptsBoundaries(t *testing.T) {
	closed := &executionSession{inspectionSession: &inspectionSession{}}
	closed.executionSession()
	closed.inspectionSession.isInspectionSession()
	if _, err := closed.Execute(context.Background()); err == nil {
		t.Fatal("closed execute accepted")
	}
	if _, err := closed.Receipts(context.Background()); err == nil {
		t.Fatal("closed receipts accepted")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	s := &executionSession{inspectionSession: &inspectionSession{alive: true, ingress: context.Background()}}
	if _, err := s.Execute(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("execute err=%v", err)
	}
	if _, err := s.Receipts(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("receipts err=%v", err)
	}

	s = &executionSession{inspectionSession: &inspectionSession{alive: true, ingress: context.Background()}}
	if _, err := s.Receipts(context.Background()); err == nil {
		t.Fatal("premature receipts accepted")
	}
}

func TestCoordinatorEvidenceHelpers(t *testing.T) {
	items := []ProtectedEvidence{{OperationID: "one", Exists: true}}
	if got := filterEvidence(items, "missing"); got != nil {
		t.Fatalf("missing filter=%v", got)
	}
	if evidenceExists(items, "missing") {
		t.Fatal("missing evidence exists")
	}
	if _, ok := valueAtPath(map[string]any{"a": "scalar"}, []string{"a", "b"}); ok {
		t.Fatal("traversed scalar")
	}
	if _, ok := valueAtPath(map[string]any{"a": map[string]any{}}, []string{"a", "b"}); ok {
		t.Fatal("missing nested value present")
	}
	if got := reduceOutcome(AssessmentDeny, AssessmentAllow); got != AssessmentDeny {
		t.Fatalf("reduced deny to %s", got)
	}
	(&unavailablePolicyLease{}).Release()
}

func TestReadVisibilityForRejectsInvalidRequester(t *testing.T) {
	s := &inspectionSession{alive: true, ingress: context.Background()}
	invalid := PrincipalRef{Kind: PrincipalKindUser, ID: "u"}
	if _, err := s.ReadVisibilityFor(context.Background(), Principal{Subject: &invalid}); err == nil {
		t.Fatal("invalid subject accepted")
	}
	if _, err := s.ReadVisibilityFor(context.Background(), Principal{Actor: &invalid}); err == nil {
		t.Fatal("invalid actor accepted")
	}
}

func TestInspectionAdmissionErrorsRemainDistinct(t *testing.T) {
	for name, session := range map[string]*inspectionSession{
		"denied":    {alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentDeny}},
		"admission": {alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentAllow}, admissionErr: ErrProtectedResourceUnavailable},
		"revision":  {alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentAllow}, revisionConflict: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := session.Evidence(context.Background()); err == nil {
				t.Fatal("evidence unexpectedly available")
			}
			if _, err := session.Assess(context.Background()); name != "denied" && err == nil {
				t.Fatal("assessment omitted admission error")
			}
		})
	}
}

func TestExecutionAdmissionErrorsAndReuse(t *testing.T) {
	for name, session := range map[string]*executionSession{
		"denied":    {inspectionSession: &inspectionSession{alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentDeny}}},
		"admission": {inspectionSession: &inspectionSession{alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentAllow}, admissionErr: ErrProtectedRecordExists}},
		"revision":  {inspectionSession: &inspectionSession{alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentAllow}, revisionConflict: true}},
		"reused":    {inspectionSession: &inspectionSession{alive: true, ingress: context.Background(), assessment: Assessment{Outcome: AssessmentAllow}}, executed: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := session.Execute(context.Background()); err == nil {
				t.Fatal("invalid execution accepted")
			}
		})
	}
}

func TestCoordinatorPropagatesBoundaryFailures(t *testing.T) {
	key := record.NewKeyWithID("docs", "one")
	op, _ := NewProtectedRead("one", Get, key)
	allow := MustPolicy("p", Scope("docs", AnyID, Allow(Get)))
	lease, _ := NewStaticPolicyLease(allow)
	participant := MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }}
	boom := errors.New("boom")
	for _, execution := range []bool{false, true} {
		events := []string{}
		coordinator, _ := NewEnforcementCoordinator(&coordinatorStorage{events: &events, evidenceErr: boom}, participant)
		var err error
		if execution {
			err = coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(ExecutionSession) error { return nil })
		} else {
			err = coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(InspectionSession) error { return nil })
		}
		if !errors.Is(err, boom) {
			t.Fatalf("execution=%v err=%v", execution, err)
		}
	}
	events := []string{}
	storage := &coordinatorStorage{events: &events, evidence: []ProtectedEvidence{{OperationID: "one", CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true, Exists: true, PreImage: map[string]any{}}}}
	coordinator, _ := NewEnforcementCoordinator(storage, participant)
	if err := coordinator.WithinInspection(context.Background(), []ProtectedOperation{op}, func(InspectionSession) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("callback err=%v", err)
	}
	if err := coordinator.WithinExecution(context.Background(), []ProtectedOperation{op}, func(ExecutionSession) error { return nil }); err == nil {
		t.Fatal("execution without Execute accepted")
	}
}

func TestCoordinatorCancellationDuringLeaseAcquisition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	key := record.NewKeyWithID("docs", "one")
	op, _ := NewProtectedRead("one", Get, key)
	events := []string{}
	storage := &coordinatorStorage{events: &events}
	lease, _ := NewStaticPolicyLease(MustPolicy("p", Root(Allow(Get))))
	coordinator, _ := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { cancel(); return lease, nil }})
	if err := coordinator.WithinInspection(ctx, []ProtectedOperation{op}, func(InspectionSession) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestProtectedEvidenceValidationRejectsIncompleteOrMismatchedFacts(t *testing.T) {
	key := record.NewKeyWithID("docs", "one")
	op, _ := NewProtectedRead("one", Get, key)
	valid := ProtectedEvidence{OperationID: "one", CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true, Exists: true, PreImage: map[string]any{}}
	cases := map[string][]ProtectedEvidence{
		"missing":               nil,
		"duplicate":             {valid, valid},
		"empty id":              {{CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true}},
		"incomplete":            {{OperationID: "one", CanonicalTarget: key.String(), SnapshotToken: "s"}},
		"wrong target":          {{OperationID: "one", CanonicalTarget: "wrong", SnapshotToken: "s", Complete: true}},
		"present without image": {{OperationID: "one", CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true, Exists: true}},
		"absent with image":     {{OperationID: "one", CanonicalTarget: key.String(), SnapshotToken: "s", Complete: true, PreImage: map[string]any{}}},
		"failed carrying facts": {{OperationID: "one", CanonicalTarget: key.String(), PreparationError: errors.New("private"), Complete: true}},
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := assessProtected(context.Background(), []ProtectedOperation{op}, evidence, nil, nil, true); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
