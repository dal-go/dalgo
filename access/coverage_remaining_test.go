package access

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type invalidKeyID struct{}

func (invalidKeyID) Validate() error { return errors.New("invalid id") }

type unsupportedCondition struct{}

func (unsupportedCondition) String() string { return "unsupported" }

type readOnlyBackend struct{ dal.Backend }

func TestRemainingFailClosedBoundaries(t *testing.T) {
	invalid := record.NewKeyWithID("docs", invalidKeyID{})
	if _, err := newProtectedOperation("op", Get, invalid, nil, nil, ""); err == nil {
		t.Fatal("invalid key accepted")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("zero key clone did not fail closed")
			}
		}()
		_ = cloneKey(&record.Key{})
	}()
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("docs", ""))).Where(unsupportedCondition{}).SelectKeysOnly(reflect.String)
	set, err := parseFieldPatterns([]string{"name"})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRequestedQueryFields(query, fieldSets{set}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unsupported query condition err=%v", err)
	}

	raw := dal.NewDB(&fakeDB{fakeSession: &fakeSession{}})
	secured, err := SecureDB(dal.NewDB(readOnlyBackend{Backend: raw}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := secured.(*securedWriteDB); ok {
		t.Fatal("read-only backend gained write capability")
	}
}

func TestRemainingAssessmentAndPrincipalBranches(t *testing.T) {
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("docs", ""))).SelectKeysOnly(reflect.String)
	a := policyAssessment{writes: [][]writeResidual{{{residual: &WriteResidual{Terminal: &WriteAlternative{}}}}}}
	a.applyStaticFieldValidation(Request{Operation: Query, Query: query})
	if got := namedPolicy("named").Name(); got != "named" {
		t.Fatalf("name=%q", got)
	}

	request := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "one"))}}
	set := MustPrincipalPolicySet("p", map[string][]Rule{"r": {Scope("docs", AnyID, Allow(Get))}}, Bindings{Groups: map[string][]string{"staff": {"r"}}})
	set.realm = "realm"
	set.execution = &compiledExecutionGate{}
	subject := PrincipalRef{Realm: "realm", Kind: PrincipalKindUser, ID: "u"}
	ctx := WithPrincipal(context.Background(), Principal{Subject: &subject, Groups: []string{"staff"}})
	if d := set.Decide(ctx, request); d.Allowed || d.Code != CodeExecutionClassDenied {
		t.Fatalf("execution decision=%+v", d)
	}
	set.execution = nil
	if d := set.Decide(ctx, request); !d.Allowed {
		t.Fatalf("typed group decision=%+v", d)
	}
}

func TestRemainingCoordinatorHelpers(t *testing.T) {
	(&unavailablePolicyLease{}).Release()
	(&pinnedPolicyLease{PolicyLease: &unavailablePolicyLease{}}).Release()
	inspection := &inspectionSession{}
	inspection.isInspectionSession()
	execution := &executionSession{}
	execution.executionSession()
	if fmt.Sprint(inspection) == "" || fmt.Sprint(execution) == "" {
		t.Fatal("sessions unexpectedly empty")
	}

	key := record.NewKeyWithID("docs", "one")
	op, _ := NewProtectedRead("one", Get, key)
	s := &inspectionSession{
		alive:      true,
		ingress:    context.Background(),
		operations: []ProtectedOperation{op},
		assessment: Assessment{Outcome: AssessmentAllow},
	}
	evidence, err := s.Evidence(context.Background())
	if err != nil || len(evidence) != 0 {
		t.Fatalf("metadata-only evidence=%v err=%v", evidence, err)
	}

	decision := Decision{Allowed: true, Effect: "allow", Operation: Get, Resource: RecordResourceForKey(key), Writes: []*WriteResidual{{Alternatives: []WriteAlternative{{Rule: "bad", Where: unsupportedCondition{}}}}}}
	assessed := Assessment{Outcome: AssessmentAllow, Complete: true, Policies: []PolicyAssessment{{Decision: decision}}}
	op.columns = [][]string{{"name"}}
	got := evaluateEvidence(op, ProtectedEvidence{Exists: true, PreImage: map[string]any{"name": "n"}}, assessed)
	if got.Outcome != AssessmentIndeterminate || got.Policies[0].Decision.Code != CodeEvaluationFailed {
		t.Fatalf("malformed deciding condition=%+v", got)
	}
}

func TestPolicyEnvelopeDefaultsVisibility(t *testing.T) {
	document, err := ParseDTQLPolicy([]byte(portablePolicy("default-visibility", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	document.Metadata.Visibility = ""
	policy, err := policyFromDTQLDocument(document, "db1", "policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if metadata := DescribePolicy(policy); metadata.Visibility != PolicyVisibilityPublic {
		t.Fatalf("metadata=%+v", metadata)
	}
}
