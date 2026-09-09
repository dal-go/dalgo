package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type malformedAssessmentPolicy struct{}

func (malformedAssessmentPolicy) Name() string         { return "bad" }
func (malformedAssessmentPolicy) InspectionPure() bool { return true }
func (malformedAssessmentPolicy) Decide(context.Context, Request) Decision {
	return Decision{Effect: "allow"}
}
func (p malformedAssessmentPolicy) Authorize(ctx context.Context, request Request) error {
	if p.Decide(ctx, request).Allowed {
		return nil
	}
	return ErrAccessDenied
}

func TestAssessPlanCollectsDeterministicIndependentDecisions(t *testing.T) {
	resource := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	request := Request{Operation: Get, Resources: []Resource{resource}}
	conditional := MustPolicy("conditional", Scope("docs", AnyID,
		Allow(Get, "owner").Where(dal.WhereField("ownerID", dal.Equal, "u1"))))
	denied := MustPolicy("denied", Scope("docs", AnyID, Deny(Get, "blocked")))
	described, err := WithPolicyMetadata(conditional, PolicyMetadata{ID: "owner-policy", Revision: "r7", Visibility: PolicyVisibilityPublic, Source: "owners/db/policy.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	assessment := AssessPlan(context.Background(), request, []Policy{described, denied})
	if assessment.Outcome != AssessmentDeny || !assessment.Complete {
		t.Fatalf("assessment = %+v", assessment)
	}
	if got := []string{assessment.Policies[0].Policy.ID, assessment.Policies[1].Policy.ID}; !reflect.DeepEqual(got, []string{"owner-policy", "denied"}) {
		t.Fatalf("policy order = %v", got)
	}
	if assessment.Policies[1].Decision.Code != CodeRuleDenied {
		t.Fatalf("deny code = %q", assessment.Policies[1].Decision.Code)
	}
	if len(assessment.Restrictions) != 2 || assessment.Restrictions[0].Slot != DecisionSlotWhere || assessment.Restrictions[1].Slot != DecisionSlotCheck {
		t.Fatalf("restrictions = %+v", assessment.Restrictions)
	}
	document, err := assessment.Restrictions[0].DocumentCondition()
	if err != nil || document == nil || document.Left == nil || document.Left.Field != "ownerID" {
		t.Fatalf("portable condition = %+v, %v", document, err)
	}
}

func TestAssessPlanPreservesParamExpressionAndChecksStaticColumns(t *testing.T) {
	resource := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	policy := MustPolicy("docs", Scope("docs", AnyID,
		Allow(Update, "owner").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("title")))
	ctx := WithCurrentUser(context.Background(), "secret-user-id")
	assessment := AssessPlan(ctx, Request{Operation: Update, Resources: []Resource{resource}}, []Policy{policy})
	document, err := assessment.Restrictions[0].DocumentCondition()
	if err != nil || document.Right == nil || document.Right.Param != "currentUser" || document.Right.Value != nil {
		t.Fatalf("source expression = %+v, %v", document, err)
	}
	denied := AssessPlan(ctx, Request{Operation: Update, Resources: []Resource{resource}, Columns: [][]string{{"secret"}}}, []Policy{policy})
	if denied.Outcome != AssessmentDeny {
		t.Fatalf("static column outcome = %s", denied.Outcome)
	}
	last := denied.Policies[len(denied.Policies)-1].Decision
	if last.Code != CodeColumnDenied || last.Slot != DecisionSlotFields || !reflect.DeepEqual(last.Columns, [][]string{{"secret"}}) {
		t.Fatalf("column blocker = %+v", last)
	}
}

func TestAssessPlanUsesExecutionQueryFieldGuard(t *testing.T) {
	policy := MustPolicy("users", Collection("users", Allow(Query, "list").Fields("name")))
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).WhereField("secret", dal.Equal, "guess").SelectKeysOnly(reflect.String)
	assessment := AssessPlan(context.Background(), Request{Operation: Query, Resources: []Resource{CollectionResourceFor(nil, "users")}, Query: query}, []Policy{policy})
	if assessment.Outcome != AssessmentDeny {
		t.Fatalf("hidden query probe outcome = %s", assessment.Outcome)
	}
	decision := assessment.Policies[len(assessment.Policies)-1].Decision
	if decision.Code != CodeColumnDenied || decision.Slot != DecisionSlotWhere || !reflect.DeepEqual(decision.Columns, [][]string{{"secret"}}) {
		t.Fatalf("query field blocker = %+v", decision)
	}
}

func TestAssessPlanEmptyAndMalformedAreIndeterminate(t *testing.T) {
	request := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}}
	if assessment := AssessPlan(context.Background(), request, nil); assessment.Outcome != AssessmentIndeterminate || assessment.Complete {
		t.Fatalf("empty assessment = %+v", assessment)
	}
	assessment := AssessPlan(context.Background(), request, []Policy{malformedAssessmentPolicy{}})
	if assessment.Outcome != AssessmentIndeterminate || assessment.Complete || assessment.Policies[0].Decision.Code != CodeEvaluationFailed {
		t.Fatalf("malformed assessment = %+v", assessment)
	}
}

type impureAssessmentPolicy struct{ calls *int }

func (p impureAssessmentPolicy) Name() string { return "custom" }
func (p impureAssessmentPolicy) Decide(_ context.Context, request Request) Decision {
	*p.calls++
	return Decision{Allowed: true, Operation: request.Operation}
}
func (p impureAssessmentPolicy) Authorize(ctx context.Context, request Request) error {
	if p.Decide(ctx, request).Allowed {
		return nil
	}
	return ErrAccessDenied
}

func TestAssessPlanRequiresExplicitCustomPolicyPurity(t *testing.T) {
	request := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}}
	calls := 0
	custom := impureAssessmentPolicy{calls: &calls}
	assessment := AssessPlan(context.Background(), request, []Policy{custom})
	if calls != 0 || assessment.Outcome != AssessmentIndeterminate || assessment.Complete || assessment.Policies[0].Decision.Code != CodeEnforcementUnsupported {
		t.Fatalf("calls=%d assessment=%+v", calls, assessment)
	}
	pure, err := DeclareInspectionPure(custom)
	if err != nil || !CanInspectPolicy(pure) {
		t.Fatalf("pure=%T err=%v", pure, err)
	}
	described, err := WithPolicyMetadata(pure, PolicyMetadata{ID: "public-custom", Revision: "r1", Visibility: PolicyVisibilityPublic})
	if err != nil || !CanInspectPolicy(described) {
		t.Fatalf("described purity lost: %T err=%v", described, err)
	}
	if metadata := DescribePolicy(pure); metadata.ID != "custom" || metadata.Visibility != PolicyVisibilityPrivate {
		t.Fatalf("declared policy metadata=%+v", metadata)
	}
	assessment = AssessPlan(context.Background(), request, []Policy{described})
	if calls != 1 || assessment.Outcome != AssessmentAllow || assessment.Policies[0].Policy.ID != "public-custom" {
		t.Fatalf("calls=%d assessment=%+v", calls, assessment)
	}
	if _, err := DeclareInspectionPure(nil); err == nil {
		t.Fatal("nil custom policy accepted")
	}
}

func TestPolicyMetadataDefaultsPrivateAndIsDefensive(t *testing.T) {
	policy := MustPolicy("legacy", Root(Allow(Get)))
	if metadata := DescribePolicy(policy); metadata.Visibility != PolicyVisibilityPrivate || metadata.ID != "legacy" {
		t.Fatalf("metadata = %+v", metadata)
	}
	_, err := WithPolicyMetadata(policy, PolicyMetadata{Visibility: "secret"})
	if err == nil {
		t.Fatal("invalid visibility accepted")
	}
}

func TestPolicyMetadataAndRestrictionValidationFailures(t *testing.T) {
	policy := MustPolicy("p", Root(Allow(Get)))
	for name, metadata := range map[string]PolicyMetadata{
		"id":         {ID: " bad ", Visibility: PolicyVisibilityPublic},
		"visibility": {ID: "p", Visibility: "future"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := WithPolicyMetadata(policy, metadata); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	if _, err := WithPolicyMetadata(nil, PolicyMetadata{ID: "p", Visibility: PolicyVisibilityPublic}); err == nil {
		t.Fatal("nil policy accepted")
	}
	if document, err := (AssessmentRestriction{}).DocumentCondition(); err != nil || document != nil {
		t.Fatalf("empty restriction document=%v err=%v", document, err)
	}
	if _, err := (AssessmentRestriction{Condition: fakeCond{}}).DocumentCondition(); !errors.Is(err, ErrNotSerializable) {
		t.Fatalf("opaque condition err=%v", err)
	}
}

func TestValidateDocumentConditionShapes(t *testing.T) {
	field := &DocumentExpression{Field: "owner"}
	value := &DocumentExpression{Value: "u1"}
	valid := []DocumentCondition{
		{Op: "==", Left: field, Right: value},
		{And: []DocumentCondition{{Op: "==", Left: field, Right: value}}},
		{Or: []DocumentCondition{{Op: "In", Left: field, Right: &DocumentExpression{Values: []any{"u1"}}}}},
	}
	for _, condition := range valid {
		if err := ValidateDocumentCondition(condition); err != nil {
			t.Fatalf("valid condition %+v: %v", condition, err)
		}
	}
	invalid := []DocumentCondition{
		{},
		{Op: "==", Left: field, Right: value, And: []DocumentCondition{{}}},
		{Op: "future", Left: field, Right: value},
		{Op: "==", Left: field},
		{Op: "==", Left: &DocumentExpression{Field: "owner", Param: "x"}, Right: value},
		{Op: "==", Left: field, Right: &DocumentExpression{Param: "bad name"}},
		{And: []DocumentCondition{}},
		{Or: []DocumentCondition{{}}},
	}
	for _, condition := range invalid {
		if err := ValidateDocumentCondition(condition); err == nil {
			t.Fatalf("invalid condition accepted: %+v", condition)
		}
	}
}

func TestDecisionsFromErrorDeepCopiesSlices(t *testing.T) {
	original := Decision{Residuals: []dal.Condition{dal.WhereField("id", dal.Equal, "x")}, Columns: [][]string{{"profile", "name"}}, Writes: []*WriteResidual{{Terminal: &WriteAlternative{Rule: "allow", Fields: []string{"name"}}}}}
	err := &DeniedError{Decision: original, Decisions: []Decision{original}}
	got := DecisionsFromError(err)
	got[0].Residuals[0] = nil
	got[0].Columns[0][0] = "changed"
	got[0].Writes[0].Terminal.Fields[0] = "changed"
	again := DecisionsFromError(err)
	if again[0].Residuals[0] == nil || again[0].Columns[0][0] != "profile" || again[0].Writes[0].Terminal.Rule != "allow" || again[0].Writes[0].Terminal.Fields[0] != "name" || !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("nested slices alias source: %+v", again)
	}
}

type emptyMetadataPolicy struct{ malformedAssessmentPolicy }

func (emptyMetadataPolicy) PolicyMetadata() PolicyMetadata { return PolicyMetadata{} }

func TestAssessmentDefensiveDefaults(t *testing.T) {
	metadata := DescribePolicy(emptyMetadataPolicy{})
	if metadata.ID != "bad" || metadata.Visibility != PolicyVisibilityPrivate {
		t.Fatalf("metadata=%+v", metadata)
	}
	p, err := WithPolicyMetadata(malformedAssessmentPolicy{}, PolicyMetadata{ID: "bad"})
	if err != nil || DescribePolicy(p).Visibility != PolicyVisibilityPrivate {
		t.Fatalf("policy=%v err=%v", p, err)
	}
	if writeMayAllowField(nil, "secret") != true {
		t.Fatal("nil residual restricted fields")
	}
	a := assessPolicies(context.Background(), Request{Operation: Get}, []Policy{nil})
	if a.assessment.Complete || a.assessment.Outcome != AssessmentIndeterminate {
		t.Fatalf("assessment=%+v", a.assessment)
	}
	_ = namedPolicy("x").Decide(context.Background(), Request{})
	_ = namedPolicy("x").Authorize(context.Background(), Request{})
}
