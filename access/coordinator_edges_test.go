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
