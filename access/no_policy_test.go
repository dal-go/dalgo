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

// allowAllOperations allows every operation on every collection.
func allowAllOperations() Policy { return MustPolicy("allow-all", Root(Allow(ReadWrite, "all"))) }

// withoutPolicy is every way of building a secured session that holds no usable
// policy: none, a nil one, an empty list and a list that holds a nil beside a
// policy that allows everything.
func withoutPolicy() map[string][]Policy {
	return map[string][]Policy{
		"no policy":             nil,
		"a nil policy":          {nil},
		"an empty list":         {},
		"a list holding a nil":  {allowAllOperations(), nil},
		"a nil before a policy": {nil, allowAllOperations()},
	}
}

func readOperations(ctx context.Context, session dal.ReadSession) map[string]func() error {
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{})
	query := dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().SelectKeysOnly(reflect.String)
	return map[string]func() error{
		"Exists":               func() error { _, err := session.Exists(ctx, rec.Key()); return err },
		"Get":                  func() error { return session.Get(ctx, rec) },
		"GetMulti":             func() error { return session.GetMulti(ctx, []record.Record{rec}) },
		"query to records":     func() error { _, err := session.ExecuteQueryToRecordsReader(ctx, query); return err },
		"query to a recordset": func() error { _, err := session.ExecuteQueryToRecordsetReader(ctx, query); return err },
	}
}

func writeOperations(ctx context.Context, session dal.WriteSession) map[string]func() error {
	key := record.NewKeyWithID("docs", "d1")
	rec := record.NewRecordWithData(key, map[string]any{"name": "one"})
	records := []record.Record{rec}
	changes := []update.Update{update.ByFieldName("name", "new")}
	return map[string]func() error{
		"Set":          func() error { return session.Set(ctx, rec) },
		"SetMulti":     func() error { return session.SetMulti(ctx, records) },
		"Insert":       func() error { return session.Insert(ctx, rec) },
		"InsertMulti":  func() error { return session.InsertMulti(ctx, records) },
		"Update":       func() error { return session.Update(ctx, key, changes) },
		"UpdateRecord": func() error { return session.UpdateRecord(ctx, rec, changes) },
		"UpdateMulti":  func() error { return session.UpdateMulti(ctx, []*record.Key{key}, changes) },
		"Delete":       func() error { return session.Delete(ctx, key) },
		"DeleteMulti":  func() error { return session.DeleteMulti(ctx, []*record.Key{key}) },
	}
}

func requireEveryOperationDenied(t *testing.T, operations map[string]func() error, wrapped *fakeSession) {
	t.Helper()
	for name, run := range operations {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, want an access denial", err)
			}
			if len(wrapped.calls) != 0 {
				t.Fatalf("the wrapped session was called: %v", wrapped.calls)
			}
		})
	}
}

func TestSecuredSessionWithoutAPolicyDeniesEveryRequest(t *testing.T) {
	ctx := context.Background()
	for label, policies := range withoutPolicy() {
		t.Run(label, func(t *testing.T) {
			t.Run("read session", func(t *testing.T) {
				wrapped := &fakeSession{}
				requireEveryOperationDenied(t, readOperations(ctx, SecureReadSession(wrapped, policies...)), wrapped)
			})
			t.Run("write session", func(t *testing.T) {
				wrapped := &fakeSession{}
				requireEveryOperationDenied(t, writeOperations(ctx, SecureWriteSession(wrapped, policies...)), wrapped)
			})
			t.Run("read-write session", func(t *testing.T) {
				wrapped := &fakeSession{}
				session := SecureReadwriteSession(wrapped, policies...)
				operations := readOperations(ctx, session)
				for name, run := range writeOperations(ctx, session) {
					operations[name] = run
				}
				requireEveryOperationDenied(t, operations, wrapped)
			})
		})
	}
}

func TestSecuredSessionWithAPolicyStillAllows(t *testing.T) {
	ctx := context.Background()
	wrapped := &fakeSession{}
	session := SecureReadwriteSession(wrapped, allowAllOperations(), MustPolicy("also-allow-all", Root(Allow(ReadWrite, "all"))))
	if err := session.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{})); err != nil {
		t.Fatalf("read with policies: %v", err)
	}
	if err := session.Delete(ctx, record.NewKeyWithID("docs", "d1")); err != nil {
		t.Fatalf("write with policies: %v", err)
	}
	if wrapped.calls["Get"] != 1 || wrapped.calls["Delete"] != 1 {
		t.Fatalf("calls = %v", wrapped.calls)
	}
}

func newFakeDB(session *fakeSession) *fakeDB {
	tx := &fakeTx{fakeSession: session, opts: dal.NewTransactionOptions()}
	return &fakeDB{fakeSession: session, adapter: dal.NewAdapter("a", "1"), schema: dal.NewSchema(nil, nil), ro: tx, rw: tx}
}

// A secured database holds its policies in options, and policies may also come
// with the context of a request. With none in force at the time of a request,
// nothing is allowed.
func TestSecuredDatabaseWithoutAPolicyDeniesEveryRequest(t *testing.T) {
	ctx := context.Background()
	options := map[string][]DBOption{
		"no option":                  nil,
		"an empty policy list":       {WithDatabasePolicies()},
		"an empty policy list twice": {WithDatabasePolicies(), WithDatabasePolicies()},
	}
	for label, dbOptions := range options {
		t.Run(label, func(t *testing.T) {
			wrapped := &fakeSession{}
			raw := newFakeDB(wrapped)
			secured := MustSecureDB(dal.NewDB(raw), dbOptions...)
			operations := readOperations(ctx, secured)
			for name, run := range writeOperations(ctx, secured.(dal.WriteSession)) {
				operations[name] = run
			}
			operations["read transaction"] = func() error {
				return secured.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
					return tx.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{}))
				})
			}
			operations["read-write transaction"] = func() error {
				return secured.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					return tx.Delete(ctx, record.NewKeyWithID("docs", "d1"))
				})
			}
			requireEveryOperationDenied(t, operations, wrapped)
		})
	}
	t.Run("a policy that arrives with the context is in force", func(t *testing.T) {
		wrapped := &fakeSession{}
		secured := MustSecureDB(dal.NewDB(newFakeDB(wrapped)))
		withContextPolicy := WithPolicy(ctx, allowAllOperations())
		if err := secured.Get(withContextPolicy, record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{})); err != nil {
			t.Fatalf("error = %v, want none", err)
		}
		if err := BindDB(secured, withContextPolicy).Get(ctx, record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{})); err != nil {
			t.Fatalf("bound database: error = %v, want none", err)
		}
	})
	t.Run("a bound database without a policy", func(t *testing.T) {
		wrapped := &fakeSession{}
		bound := BindDB(dal.NewDB(newFakeDB(wrapped)), ctx)
		requireEveryOperationDenied(t, readOperations(ctx, bound), wrapped)
	})
}

// The denial says the session holds no policy; it is not a decision of a policy.
func TestNoPolicyDenialNamesTheConfiguration(t *testing.T) {
	err := SecureReadSession(&fakeSession{}).Get(context.Background(), record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{}))
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %v", err)
	}
	decision := denied.Decision
	if decision.Allowed || decision.Code != CodeConfigurationInvalid || decision.Scope != DecisionScopeConfiguration ||
		decision.Operation != Get || decision.Resource.String() != "/docs/d1" || !decision.Code.IsIndeterminate() {
		t.Fatalf("decision = %+v", decision)
	}
}

// A policy that holds no decision cannot let a request through: an assessment
// that is incomplete without any decision is a denial.
func TestIncompleteAssessmentWithoutADecisionIsADenial(t *testing.T) {
	request := Request{Operation: Query, Resources: []Resource{OpaqueQuery("x")}}
	residuals, writes, err := settleAssessment(request, policyAssessment{assessment: Assessment{Outcome: AssessmentIndeterminate, Complete: false}})
	var denied *DeniedError
	if !errors.As(err, &denied) || residuals != nil || writes != nil {
		t.Fatalf("residuals = %v, writes = %v, error = %v, want a denial", residuals, writes, err)
	}
	if denied.Decision.Code != CodeEvaluationFailed || denied.Decision.Operation != Query || denied.Decision.Resource.String() != "opaque-query:x" {
		t.Fatalf("decision = %+v", denied.Decision)
	}
	t.Run("without a resource", func(t *testing.T) {
		_, _, err := settleAssessment(Request{Operation: Get}, policyAssessment{})
		if !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("a complete assessment passes", func(t *testing.T) {
		if _, _, err := settleAssessment(request, policyAssessment{assessment: Assessment{Complete: true}}); err != nil {
			t.Fatalf("error = %v", err)
		}
	})
}

// A nil policy in a list is a decision of its own: the evaluation failed.
func TestNilPolicyIsAnEvaluationFailure(t *testing.T) {
	err := SecureReadSession(&fakeSession{}, allowAllOperations(), nil).Get(context.Background(), record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{}))
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %v", err)
	}
	if denied.Decision.Code != CodeEvaluationFailed || denied.Decision.Explanation != "nil mandatory policy" || len(denied.Decisions) != 2 {
		t.Fatalf("decision = %+v, decisions = %d", denied.Decision, len(denied.Decisions))
	}
}

// nilPolicyLease is a policy snapshot with a nil policy in it.
type nilPolicyLease struct{ policies []Policy }

func (l nilPolicyLease) Policies() []Policy { return l.policies }
func (nilPolicyLease) Revision() string     { return "r" }
func (nilPolicyLease) Release()             {}

// Every other entry point that takes policies refuses none, a nil one or an
// empty list the same way: by an error, a panic, or a denial.
func TestEveryEntryPointThatTakesPoliciesRefusesNoPolicy(t *testing.T) {
	ctx := context.Background()
	request := Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "d1"))}}
	t.Run("WithDatabasePolicies refuses a nil policy", func(t *testing.T) {
		if _, err := SecureDB(dal.NewDB(newFakeDB(&fakeSession{})), WithDatabasePolicies(nil)); err == nil {
			t.Fatal("want an error")
		}
		if _, err := SecureDB(dal.NewDB(newFakeDB(&fakeSession{})), WithDatabasePolicies(allowAllOperations(), nil)); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("WithPolicy refuses a nil policy", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("want a panic")
			}
		}()
		WithPolicy(ctx, nil)
	})
	t.Run("a policy provider that returns none, or a nil one, denies", func(t *testing.T) {
		for label, policies := range map[string][]Policy{"none": nil, "an empty list": {}, "a nil policy": {nil}, "a nil beside a policy": {allowAllOperations(), nil}} {
			wrapped := &fakeSession{}
			secured := MustSecureDB(dal.NewDB(newFakeDB(wrapped)), WithDatabasePolicyProvider(func(context.Context) ([]Policy, error) { return policies, nil }))
			if err := secured.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{})); !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("%s: error = %v, want an access denial", label, err)
			}
			if len(wrapped.calls) != 0 {
				t.Fatalf("%s: the wrapped session was called: %v", label, wrapped.calls)
			}
		}
	})
	t.Run("NewStaticPolicyLease and NewStaticParticipant refuse none and a nil policy", func(t *testing.T) {
		for _, policies := range [][]Policy{nil, {}, {nil}, {allowAllOperations(), nil}} {
			if _, err := NewStaticPolicyLease(policies...); err == nil {
				t.Fatalf("lease of %v: want an error", policies)
			}
			if _, err := NewStaticParticipant("layer", policies...); err == nil {
				t.Fatalf("participant of %v: want an error", policies)
			}
		}
	})
	t.Run("a participant whose snapshot holds a nil policy denies a protected write", func(t *testing.T) {
		raw := newFakeDB(&fakeSession{})
		storage := &automaticCoordinatorStorage{}
		for label, lease := range map[string]PolicyLease{
			"a nil policy":          nilPolicyLease{policies: []Policy{nil}},
			"a nil beside a policy": nilPolicyLease{policies: []Policy{MustPolicy("writer", Root(Allow(Set, "all"))), nil}},
			"no policy":             nilPolicyLease{},
		} {
			coordinator, err := NewEnforcementCoordinator(storage, MandatoryParticipant{LayerID: "owner", Provider: func(context.Context) (PolicyLease, error) { return lease, nil }})
			if err != nil {
				t.Fatal(err)
			}
			secured := MustSecureDB(dal.NewDB(raw), WithEnforcementCoordinator(coordinator))
			rec := record.NewRecordWithData(record.NewKeyWithID("docs", "d1"), map[string]any{"name": "one"})
			if err := secured.(dal.WriteSession).Set(ctx, rec); !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("%s: error = %v, want an access denial", label, err)
			}
		}
		if len(raw.calls) != 0 {
			t.Fatalf("the raw session was called: %v", raw.calls)
		}
	})
	t.Run("AssessPlan does not allow none or a nil policy", func(t *testing.T) {
		for label, policies := range map[string][]Policy{"none": nil, "a nil policy": {nil}, "a nil beside a policy": {allowAllOperations(), nil}} {
			assessment := AssessPlan(ctx, request, policies)
			if assessment.Outcome != AssessmentIndeterminate || assessment.Complete {
				t.Fatalf("%s: assessment = %+v, want indeterminate and incomplete", label, assessment)
			}
		}
	})
}
