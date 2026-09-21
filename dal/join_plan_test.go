package dal

import (
	"context"
	"errors"
	"testing"
)

type testNativeJoinProvider struct {
	decline error
	calls   int
}

func (p *testNativeJoinProvider) CanExecuteJoin(_ context.Context, _ StructuredQuery) error {
	p.calls++
	return p.decline
}

func TestPlanJoinQuerySpecificNativeDecision(t *testing.T) {
	q := joinTestQuery()
	provider := &testNativeJoinProvider{}
	plan, err := PlanJoin(context.Background(), q, provider)
	if err != nil || plan.Strategy != JoinNative || provider.calls != 1 {
		t.Fatalf("native plan=%+v err=%v calls=%d", plan, err, provider.calls)
	}
	provider.decline = errors.New("correlated subtree")
	plan, err = PlanJoin(context.Background(), q, provider)
	if err != nil || plan.Strategy != JoinGeneric || provider.calls != 2 {
		t.Fatalf("generic plan=%+v err=%v calls=%d", plan, err, provider.calls)
	}
	plan, err = PlanJoin(context.Background(), q, nil)
	if err != nil || plan.Strategy != JoinGeneric {
		t.Fatalf("no-capability plan=%+v err=%v", plan, err)
	}
	single := From(NewRootCollectionRef("A", "")).NewQuery().SelectIntoRecord(nil)
	plan, err = PlanJoin(context.Background(), single, provider)
	if err != nil || plan.Strategy != JoinNative || provider.calls != 2 {
		t.Fatalf("single plan=%+v err=%v calls=%d", plan, err, provider.calls)
	}
	if _, err = PlanJoin(context.Background(), nil, provider); err == nil {
		t.Fatal("nil query accepted")
	}
	bad := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinRight, joinOn("a", "id", "b", "aid"))).NewQuery().SelectIntoRecord(nil)
	if _, err = PlanJoin(context.Background(), bad, provider); err == nil {
		t.Fatal("invalid tree accepted")
	}
}
