package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type fixedDecisionPolicy struct {
	name     string
	decision Decision
	calls    *int
}

func TestPolicyProviderRejectsNilMembersAndWrapsCause(t *testing.T) {
	g := guard{policyProvider: func(context.Context) ([]Policy, error) { return []Policy{nil}, nil }}
	if _, err := g.pinDatabasePolicies(context.Background()); err == nil {
		t.Fatal("nil policy accepted")
	}
	cause := errors.New("source")
	err := &PolicyProviderError{Err: cause}
	if err.Error() == "" || !errors.Is(err, cause) || !errors.Is(err, ErrAccessDenied) {
		t.Fatal("provider error contract changed")
	}
}

func (p fixedDecisionPolicy) Name() string { return p.name }
func (p fixedDecisionPolicy) Decide(context.Context, Request) Decision {
	*p.calls++
	d := p.decision
	d.Policy = p.name
	return d
}
func (p fixedDecisionPolicy) Authorize(ctx context.Context, request Request) error {
	d := p.Decide(ctx, request)
	if !d.Allowed {
		return &DeniedError{Decision: d}
	}
	return nil
}

func TestGuardCollectsIndependentPolicyDecisions(t *testing.T) {
	calls := 0
	request := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}}
	g := guard{databasePolicies: []Policy{
		fixedDecisionPolicy{name: "deny-first", decision: Decision{Effect: "deny", Explanation: "first"}, calls: &calls},
		fixedDecisionPolicy{name: "allow", decision: Decision{Allowed: true, Effect: "allow"}, calls: &calls},
		fixedDecisionPolicy{name: "deny-last", decision: Decision{Effect: "deny", Explanation: "last"}, calls: &calls},
	}}
	_, _, err := g.authorizeRequest(context.Background(), request)
	if !errors.Is(err, ErrAccessDenied) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	decisions := DecisionsFromError(err)
	if len(decisions) != 3 || decisions[0].Policy != "deny-first" || decisions[1].Policy != "allow" || decisions[2].Policy != "deny-last" {
		t.Fatalf("decisions=%+v", decisions)
	}
	var denied *DeniedError
	if !errors.As(err, &denied) || denied.Decision.Policy != "deny-first" {
		t.Fatalf("legacy decision=%+v", denied)
	}
	decisions[0].Policy = "changed"
	if DecisionsFromError(err)[0].Policy != "deny-first" {
		t.Fatal("decision list aliases error")
	}
}

func TestPolicyProviderFailsClosedAndPinsTransaction(t *testing.T) {
	cause := errors.New("snapshot failed")
	g := guard{policyProvider: func(context.Context) ([]Policy, error) { return nil, cause }}
	_, _, err := g.authorizeRequest(context.Background(), Request{Operation: Get})
	if !errors.Is(err, ErrAccessDenied) || !errors.Is(err, cause) {
		t.Fatalf("provider error=%v", err)
	}
	g = guard{policyProvider: func(context.Context) ([]Policy, error) { return nil, nil }}
	if _, _, err = g.authorizeRequest(context.Background(), Request{Operation: Get}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("empty provider=%v", err)
	}

	calls := 0
	provider := func(context.Context) ([]Policy, error) {
		calls++
		return []Policy{MustPolicy("all", Root(Allow(Read, "read")))}, nil
	}
	db := MustSecureDB(dalgo2memory.New(dalgo2memory.FirestoreProfile()), WithDatabasePolicyProvider(provider))
	err = db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		key := record.NewKeyWithID("docs", "missing")
		if _, err := tx.Exists(ctx, key); err != nil {
			return err
		}
		_, err := tx.Exists(ctx, key)
		return err
	})
	if err != nil || calls != 1 {
		t.Fatalf("transaction err=%v provider calls=%d", err, calls)
	}
}
