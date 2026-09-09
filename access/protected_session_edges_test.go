package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/record"
)

func TestProtectedWriteNormalizationRejectsMalformedInputs(t *testing.T) {
	s := securedWriteSession{coordinator: &EnforcementCoordinator{}}
	bad := record.NewRecordWithData(record.NewKeyWithID("docs", "bad"), make(chan int))
	if err := s.Set(context.Background(), bad); err == nil {
		t.Fatal("unmappable set data accepted")
	}
	if err := s.SetMulti(context.Background(), []record.Record{bad}); err == nil {
		t.Fatal("unmappable set batch accepted")
	}
	if err := s.Insert(context.Background(), bad); err == nil {
		t.Fatal("unmappable insert data accepted")
	}
	if err := s.InsertMulti(context.Background(), []record.Record{bad}); err == nil {
		t.Fatal("unmappable insert batch accepted")
	}
	if err := s.Update(context.Background(), nil, nil); err == nil {
		t.Fatal("nil update key accepted")
	}
	if err := s.UpdateMulti(context.Background(), []*record.Key{nil}, nil); err == nil {
		t.Fatal("nil update batch key accepted")
	}
	if err := s.Delete(context.Background(), nil); err == nil {
		t.Fatal("nil delete key accepted")
	}
	if err := s.DeleteMulti(context.Background(), []*record.Key{nil}); err == nil {
		t.Fatal("nil delete batch key accepted")
	}
}

func TestProtectedWriteReturnsConcretePolicyDenial(t *testing.T) {
	storage := &automaticCoordinatorStorage{}
	deny := MustPolicy("owner", Scope("docs", AnyID, Deny(Set, "blocked")))
	lease, err := NewStaticPolicyLease(deny)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
	if err != nil {
		t.Fatal(err)
	}
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"name": "x"})
	err = (securedWriteSession{coordinator: coordinator}).Set(context.Background(), rec)
	var denied *DeniedError
	if !errors.As(err, &denied) || denied.Decision.Policy != "owner" || denied.Decision.Rule != "blocked" {
		t.Fatalf("denial=%+v err=%v", denied, err)
	}
}
