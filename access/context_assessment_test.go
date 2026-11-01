package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func metadataAssessmentRequest() Request {
	// Pure canonical mapping only; actual non-query assessment is Get/Query=nil.
	descriptor := dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")).NewQuery().SelectIntoRecordset()
	return Request{Operation: Get, Resources: ResourcesForQuery(descriptor), Columns: [][]string{{"name"}}}
}
func assessmentPolicy(t *testing.T, name string, rules ...Rule) Policy {
	t.Helper()
	policy, err := NewPolicyForRealm("owned-realm", name, rules...)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
func assessmentAgent(t *testing.T) context.Context {
	t.Helper()
	principal, err := NewPrincipal(PrincipalRef{Realm: "owned-realm", Kind: PrincipalKindAgent, ID: "same"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return WithPrincipal(context.Background(), principal)
}
func TestAssessContextPlanNeverDropsInheritedDeny(t *testing.T) {
	request := metadataAssessmentRequest()
	allow := assessmentPolicy(t, "core-baseline", TableScope(TableName{Schema: "sales", Name: "Customer"}, Allow(Read)))
	deny := assessmentPolicy(t, "inherited-mandatory-deny", TableScope(TableName{Schema: "sales", Name: "Customer"}, Deny(Read)))
	ctx := WithPolicy(assessmentAgent(t), deny)
	positive := AssessPlan(ctx, request, []Policy{allow})
	if positive.Outcome != AssessmentAllow {
		t.Fatalf("ordinary explicit-only positivecontrol=%+v", positive)
	}
	actual, err := AssessContextPlan(ctx, request, WithDatabasePolicies(allow))
	if err != nil || actual.Outcome != AssessmentDeny || len(actual.Policies) < 2 {
		t.Fatalf("inherited mandatory denial lost: %+v,%v", actual, err)
	}
	bound := guard{databasePolicies: []Policy{allow}}.bind(ctx)
	policies, err := bound.requestPolicies(assessmentAgent(t), request)
	if err != nil || AssessPlan(assessmentAgent(t), request, policies).Outcome != AssessmentDeny {
		t.Fatalf("shared collector lost bound-session policy: %v,%v", policies, err)
	}
}
func TestAssessContextPlanPreservesRowAndFieldObligations(t *testing.T) {
	table := TableName{Schema: "sales", Name: "Customer"}
	rows := assessmentPolicy(t, "region-constraint", TableScope(table, Allow(Read).Where(dal.WhereField("region", dal.Equal, "eu"))))
	fields := assessmentPolicy(t, "field-list", TableScope(table, Allow(Read).Fields("name")))
	ctx := WithPolicy(assessmentAgent(t), rows, fields)
	request := metadataAssessmentRequest()
	actual, err := AssessContextPlan(ctx, request)
	if err != nil || !actual.Complete || actual.Outcome != AssessmentConditional || len(actual.Restrictions) == 0 {
		t.Fatalf("constraints dropped: %+v,%v", actual, err)
	}
	request.Columns = [][]string{{"secret"}}
	actual, err = AssessContextPlan(ctx, request)
	if err != nil || actual.Outcome != AssessmentDeny {
		t.Fatalf("forbidden field allowed: %+v,%v", actual, err)
	}
}
func TestAssessContextPlanProvidersAndCancellationFailClosed(t *testing.T) {
	request := metadataAssessmentRequest()
	allow := assessmentPolicy(t, "baseline", TableScope(TableName{Schema: "sales", Name: "Customer"}, Allow(Read)))
	for _, provider := range []PolicyProvider{
		func(context.Context) ([]Policy, error) { return nil, errors.New("provider unavailable") },
		func(context.Context) ([]Policy, error) { return nil, nil },
		func(context.Context) ([]Policy, error) { return []Policy{nil}, nil },
		func(context.Context) ([]Policy, error) { return []Policy{(*AccessPolicy)(nil)}, nil },
	} {
		actual, err := AssessContextPlan(assessmentAgent(t), request, WithDatabasePolicyProvider(provider))
		if err == nil || actual.Complete || actual.Outcome != AssessmentIndeterminate {
			t.Fatalf("bad provider did not fail closed: %+v,%v", actual, err)
		}
	}
	calls := 0
	provider := func(context.Context) ([]Policy, error) { calls++; return []Policy{allow}, nil }
	actual, err := AssessContextPlan(assessmentAgent(t), request, WithDatabasePolicyProvider(provider))
	if err != nil || actual.Outcome != AssessmentAllow || calls != 1 {
		t.Fatalf("provider snapshot not used once: %+v,%v,%d", actual, err, calls)
	}
	ctx, cancel := context.WithCancel(assessmentAgent(t))
	cancel()
	actual, err = AssessContextPlan(ctx, request, WithDatabasePolicyProvider(provider))
	if !errors.Is(err, context.Canceled) || actual.Complete || calls != 1 {
		t.Fatalf("canceled provider ran: %+v,%v,%d", actual, err, calls)
	}
	ctx, cancel = context.WithCancel(assessmentAgent(t))
	actual, err = AssessContextPlan(ctx, request, WithDatabasePolicyProvider(func(context.Context) ([]Policy, error) { cancel(); return []Policy{allow}, nil }))
	if !errors.Is(err, context.Canceled) || actual.Complete {
		t.Fatalf("provider cancellation lost: %+v,%v", actual, err)
	}
	for _, options := range [][]DBOption{{nil}, {WithDatabasePolicies(nil)}, {WithDatabasePolicyProvider(nil)}, {WithDatabasePolicyProvider(provider), WithDatabasePolicyProvider(provider)}, {WithDatabasePolicies(allow), RequireContextPolicy()}} {
		actual, err = AssessContextPlan(assessmentAgent(t), request, options...)
		if err == nil || actual.Complete {
			t.Fatalf("invalid options admitted: %+v,%v", actual, err)
		}
	}
	var nilContext context.Context
	actual, err = AssessContextPlan(nilContext, request)
	if err == nil || actual.Complete {
		t.Fatalf("nil context admitted: %+v,%v", actual, err)
	}
	actual, err = AssessContextPlan(assessmentAgent(t), request)
	if err == nil || actual.Complete {
		t.Fatalf("no policy admitted: %+v,%v", actual, err)
	}
	actual, err = AssessContextPlan(WithPolicy(assessmentAgent(t), allow), request, RequireContextPolicy())
	if err != nil || actual.Outcome != AssessmentAllow {
		t.Fatalf("required context policy ignored: %+v,%v", actual, err)
	}
}

type assessmentImpure struct {
	calls  *int
	cancel context.CancelFunc
}

func (p assessmentImpure) Name() string { return "unknown-custom-policy" }
func (p assessmentImpure) Decide(_ context.Context, request Request) Decision {
	*p.calls++
	if p.cancel != nil {
		p.cancel()
	}
	return Decision{Allowed: true, Operation: request.Operation}
}
func (p assessmentImpure) Authorize(ctx context.Context, request Request) error {
	p.Decide(ctx, request)
	return nil
}
func TestAssessContextPlanPurityAndFixedAgent(t *testing.T) {
	request := metadataAssessmentRequest()
	calls := 0
	actual, err := AssessContextPlan(WithPolicy(assessmentAgent(t), assessmentImpure{calls: &calls}), request)
	if err != nil || actual.Complete || actual.Outcome != AssessmentIndeterminate || calls != 0 {
		t.Fatalf("impure policy evaluated/ignored: %+v,%v,%d", actual, err, calls)
	}
	humanGrant, err := NewPrincipalPolicySetForRealm("owned-realm", "human-only", map[string][]Rule{"read": {TableScope(TableName{Schema: "sales", Name: "Customer"}, Allow(Read))}}, Bindings{Users: map[string][]string{"same": {"read"}}, Roles: map[string][]string{"admin": {"read"}}})
	if err != nil {
		t.Fatal(err)
	}
	human, err := NewPrincipal(PrincipalRef{Realm: "owned-realm", Kind: PrincipalKindUser, ID: "same"}, []string{"admin"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, err = AssessContextPlan(WithPolicy(WithPrincipal(context.Background(), human), humanGrant), request)
	if err != nil || actual.Outcome != AssessmentAllow {
		t.Fatalf("human positivecontrol=%+v,%v", actual, err)
	}
	actual, err = AssessContextPlan(WithPolicy(assessmentAgent(t), humanGrant), request)
	if err != nil || actual.Outcome != AssessmentDeny {
		t.Fatalf("Agent borrowed human grant: %+v,%v", actual, err)
	}
	legacy := MustPolicy("legacy-unscoped", TableScope(TableName{Schema: "sales", Name: "Customer"}, Allow(Read)))
	actual, err = AssessContextPlan(WithPolicy(assessmentAgent(t), legacy), request)
	if err != nil || actual.Outcome != AssessmentDeny {
		t.Fatalf("legacy realm-less denial dropped: %+v,%v", actual, err)
	}
	ctx, cancel := context.WithCancel(assessmentAgent(t))
	pure, err := DeclareInspectionPure(assessmentImpure{calls: &calls, cancel: cancel})
	if err != nil {
		t.Fatal(err)
	}
	actual, err = AssessContextPlan(WithPolicy(ctx, pure), request)
	if !errors.Is(err, context.Canceled) || actual.Complete {
		t.Fatalf("post-assessment cancellation lost: %+v,%v", actual, err)
	}
}
