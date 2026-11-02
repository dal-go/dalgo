package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func nativeBindingRequest(name string) Request {
	q := dal.From(dal.NewQualifiedRootCollectionRef("main", name, "")).NewQuery().SelectIntoRecord(nil)
	return Request{Operation: Query, Query: q, Resources: ResourcesForQuery(q)}
}
func nativeBindingContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := WithNativeTableNamespace(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
func nativeLegacyPolicy() Policy {
	return MustPolicy("legacy", Root(Allow(Read)), Collection("Invoice", Deny(Read)), OpaqueQueryScope(Allow(Query)))
}
func TestNativeTableNamespaceConfiguration(t *testing.T) {
	for _, name := range []string{"", " main", "main ", "\xff", "m\x00n"} {
		if ctx, err := WithNativeTableNamespace(context.Background(), name); err == nil || ctx != nil {
			t.Fatalf("invalid namespace %q accepted", name)
		}
	}
	ctx := nativeBindingContext(t)
	again, err := WithNativeTableNamespace(ctx, "main")
	if err != nil || again != ctx {
		t.Fatal("binding not stable")
	}
	if next, err := WithNativeTableNamespace(ctx, "temp"); err == nil || next != nil {
		t.Fatal("binding replaced")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if next, err := WithNativeTableNamespace(canceled, "main"); err != nil || next.Err() != context.Canceled {
		t.Fatal("original cancellation lost")
	}
}
func TestNativeTableNamespaceOriginalDecisionAndLegacyDenialIntersect(t *testing.T) {
	ctx := nativeBindingContext(t)
	for _, tc := range []struct {
		name   string
		policy Policy
		want   AssessmentOutcome
	}{
		{"qualified-legacy-deny", nativeLegacyPolicy(), AssessmentDeny},
		{"table-allow-collection-deny", MustPolicy("table-and-deny", TableScope(TableName{Schema: "main", Name: "Invoice"}, Allow(Read)), Collection("Invoice", Deny(Read))), AssessmentDeny},
		{"qualified-table-no-rule", MustPolicy("table-a-only", Root(Allow(Read)), TableScope(TableName{Schema: "main", Name: "Customer"}, Allow(Read))), AssessmentDeny},
		{"original-opaque-no-match", MustPolicy("path-only", Root(Allow(Read))), AssessmentDeny},
		{"explicit-table-allow", MustPolicy("table-allow", TableScope(TableName{Schema: "main", Name: "Invoice"}, Allow(Read))), AssessmentAllow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := nativeBindingRequest("Invoice")
			got := AssessPlan(ctx, req, []Policy{tc.policy})
			if got.Outcome != tc.want || len(got.Policies) != 2 {
				t.Fatalf("%+v", got)
			}
		})
	}
	baseline := AssessPlan(context.Background(), nativeBindingRequest("Invoice"), []Policy{nativeLegacyPolicy()})
	if baseline.Outcome != AssessmentAllow || len(baseline.Policies) != 1 {
		t.Fatal("default qualified compatibility changed", baseline)
	}
	if got := AssessPlan(ctx, nativeBindingRequest("Customer"), []Policy{nativeLegacyPolicy()}); got.Outcome != AssessmentAllow {
		t.Fatal("allowed table refused", got)
	}
}
func TestNativeTableNamespaceDoesNotMutateOrGuessResources(t *testing.T) {
	ctx := nativeBindingContext(t)
	q := nativeBindingRequest("Customer")
	unknown := OpaqueQuery("unrecognized")
	foreign := nativeBindingRequest("Invoice").Resources[0]
	foreign.table = &TableName{Database: "elsewhere", Schema: "main", Name: "Invoice"}
	other := nativeBindingRequest("Invoice").Resources[0]
	other.table = &TableName{Schema: "other", Name: "Invoice"}
	empty := nativeBindingRequest("Invoice").Resources[0]
	empty.table = &TableName{Schema: "main"}
	q.Resources = append(q.Resources, unknown, foreign, other, empty, CollectionResourceFor(nil, "plain"))
	before := append([]Resource(nil), q.Resources...)
	policies := []Policy{nativeLegacyPolicy(), nil}
	augmented, bound := nativeTablePolicies(ctx, q, policies)
	if !bound || len(augmented) != 3 || !reflect.DeepEqual(q.Resources, before) {
		t.Fatal("request changed or nil policy lost")
	}
	delegate := augmented[2].(nativeTablePolicy)
	if delegate.resources[0].Kind() != PathResource || delegate.resources[0].String() != "/Customer" {
		t.Fatal("no canonical path")
	}
	identity, ok := delegate.resources[0].Table()
	if !ok || identity.Schema != "main" || identity.Name != "Customer" {
		t.Fatal("table identity lost")
	}
	if !reflect.DeepEqual(delegate.resources[1:], before[1:]) {
		t.Fatal("unknown or foreign resource reinterpreted")
	}
	for _, request := range []Request{{Operation: Get, Resources: before}, {Operation: Query, Resources: []Resource{unknown, foreign, other, empty}}, {Operation: Query}} {
		result, bound := nativeTablePolicies(ctx, request, policies)
		if bound || !reflect.DeepEqual(result, policies) {
			t.Fatal("unbound resource changed")
		}
	}
	result, bound := nativeTablePolicies(context.Background(), q, policies)
	if bound || !reflect.DeepEqual(result, policies) {
		t.Fatal("default changed")
	}
	if got := AssessPlan(ctx, q, policies); got.Complete || got.Outcome != AssessmentIndeterminate {
		t.Fatal("nil policy did not fail closed", got)
	}
}

type nativeQueryWitness struct{ dal.StructuredQuery }

type nativeRequestPolicy struct {
	seen     *[]Request
	expected context.Context
	pure     bool
	calls    *int
}

func (p nativeRequestPolicy) Name() string         { return "native-request" }
func (p nativeRequestPolicy) InspectionPure() bool { return p.pure }
func (p nativeRequestPolicy) Decide(ctx context.Context, request Request) Decision {
	if ctx != p.expected {
		panic("policy context changed")
	}
	*p.calls++
	*p.seen = append(*p.seen, request)
	return Decision{Allowed: true, Effect: "allow"}
}
func (p nativeRequestPolicy) Authorize(ctx context.Context, request Request) error {
	p.Decide(ctx, request)
	return nil
}
func TestNativeTableNamespacePreservesCompleteRequestAndPurity(t *testing.T) {
	ctx := nativeBindingContext(t)
	request := nativeBindingRequest("Customer")
	request.Query = &nativeQueryWitness{request.Query.(dal.StructuredQuery)}
	request.Columns = [][]string{{"id"}}
	request.Execution = &ExecutionTarget{}
	seen := []Request{}
	calls := 0
	policy := nativeRequestPolicy{seen: &seen, expected: ctx, pure: true, calls: &calls}
	if got := AssessPlan(ctx, request, []Policy{policy}); got.Outcome != AssessmentAllow || calls != 2 {
		t.Fatal(got, calls)
	}
	for _, r := range seen {
		if r.Query != request.Query || r.Execution != request.Execution || !reflect.DeepEqual(r.Columns, request.Columns) || r.Operation != request.Operation {
			t.Fatal("request obligation replaced")
		}
	}
	p := nativeTablePolicy{Policy: policy, resources: []Resource{CollectionResourceFor(nil, "Customer")}}
	if err := p.Authorize(ctx, request); err != nil || calls != 3 || seen[2].Resources[0].Kind() != PathResource {
		t.Fatal("authorize delegate differs")
	}
	if p.PolicyMetadata().ID != policy.Name() || !p.InspectionPure() {
		t.Fatal("metadata/purity lost")
	}
	calls = 0
	policy.pure = false
	g := guard{databasePolicies: []Policy{policy}}
	if _, _, err := g.authorizeRequest(ctx, request); !errors.Is(err, ErrAccessDenied) || calls != 0 {
		t.Fatal("impure policy executed", err, calls)
	}
}
func TestNativeTableNamespaceSameOwnerSnapshotAndAllMandatoryLayers(t *testing.T) {
	request := nativeBindingRequest("Invoice")
	calls := 0
	provider := func(context.Context) ([]Policy, error) {
		calls++
		if calls > 1 {
			t.Fatal("provider fetched for canonical reassessment")
		}
		return []Policy{nativeLegacyPolicy()}, nil
	}
	ctx := nativeBindingContext(t)
	assessment, err := AssessContextPlan(ctx, request, WithDatabasePolicies(MustPolicy("outer", Root(Allow(Read)), OpaqueQueryScope(Allow(Query)))), WithDatabasePolicyProvider(provider))
	if err != nil || assessment.Outcome != AssessmentDeny || calls != 1 || len(assessment.Policies) != 4 {
		t.Fatal(assessment, err, calls)
	}
	calls = 0
	g := guard{policyProvider: provider, boundPolicies: []Policy{MustPolicy("captured", Root(Allow(Read)), OpaqueQueryScope(Allow(Query)))}}
	_, _, err = g.authorizeRequest(ctx, request)
	if !errors.Is(err, ErrAccessDenied) || calls != 1 {
		t.Fatal("execution differs", err, calls)
	}
	ctx = WithPolicy(ctx, nativeLegacyPolicy())
	g = guard{databasePolicies: []Policy{MustPolicy("db", Root(Allow(Read)), OpaqueQueryScope(Allow(Query)))}}
	if _, _, err = g.authorizeRequest(ctx, request); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("context deny dropped", err)
	}
	g = guard{databasePolicies: []Policy{nativeLegacyPolicy()}, requireContext: true}
	if _, _, err = g.authorizeRequest(nativeBindingContext(t), request); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("native option satisfied required context")
	}
	cause := errors.New("provider unavailable")
	g = guard{policyProvider: func(context.Context) ([]Policy, error) { return nil, cause }}
	if _, _, err = g.authorizeRequest(ctx, request); !errors.Is(err, cause) {
		t.Fatal("provider error lost", err)
	}
}
func TestNativeTableNamespaceKeepsCanonicalRowAndFieldObligations(t *testing.T) {
	ctx := nativeBindingContext(t)
	request := nativeBindingRequest("Customer")
	row := MustPolicy("legacy-rows", Collection("Customer", Allow(Query).Where(dal.WhereField("id", dal.GreaterThen, 1))), OpaqueQueryScope(Allow(Query)))
	fields := MustPolicy("legacy-fields", Collection("Customer", Allow(Query).Fields("id")), OpaqueQueryScope(Allow(Query)))
	original := MustPolicy("table-row", TableScope(TableName{Schema: "main", Name: "Customer"}, Allow(Query).Where(dal.WhereField("id", dal.LessThen, 3))))
	got := AssessPlan(ctx, request, []Policy{row, fields, original})
	if got.Outcome != AssessmentConditional || !got.Complete || len(got.Restrictions) < 4 {
		t.Fatalf("lost obligations %+v", got)
	}
	residuals, writes, err := (guard{databasePolicies: []Policy{row, fields, original}}).authorizeRequest(ctx, request)
	if err != nil || len(residuals) != 1 || len(residuals[0]) != 3 || len(writes) != 1 || !queryFields(writes[0]).restrictive() || !queryFields(writes[0]).allowsWhole("id") || queryFields(writes[0]).allowsWhole("label") {
		t.Fatalf("residual/field intersection changed %v %v %v", residuals, writes, err)
	}
}

// The adapter deliberately supplies a fresh worker context. The constraint must
// be owned by the secured handle, rather than inherited by lucky context lineage.
type nativeFreshContextDB struct{ *fakeDB }

func (f nativeFreshContextDB) RunReadonlyTransaction(_ context.Context, w dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return w(context.Background(), f.ro)
}
func (f nativeFreshContextDB) RunReadwriteTransaction(_ context.Context, w dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return w(context.Background(), f.rw)
}
func TestNativeTableNamespaceBoundHandlesAndTransactionsKeepConstraint(t *testing.T) {
	for _, route := range []string{"bound", "readonly", "readwrite", "bound-readonly", "bound-readwrite"} {
		t.Run(route, func(t *testing.T) {
			leaf := &fakeSession{}
			tx := &fakeTx{fakeSession: leaf, opts: dal.NewTransactionOptions()}
			raw := nativeFreshContextDB{&fakeDB{fakeSession: leaf, ro: tx, rw: tx}}
			db := MustSecureDB(dal.NewDB(raw), WithDatabasePolicies(nativeLegacyPolicy()))
			ctx := nativeBindingContext(t)
			if route == "bound" || route == "bound-readonly" || route == "bound-readwrite" {
				db = BindDB(db, ctx)
				ctx = context.Background()
			}
			check := func(session dal.ReadSession) error {
				if _, err := session.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Invoice").Query); !errors.Is(err, ErrAccessDenied) {
					t.Fatalf("denied table escaped binding: %v", err)
				}
				if _, err := session.ExecuteQueryToRecordsetReader(context.Background(), nativeBindingRequest("Invoice").Query); !errors.Is(err, ErrAccessDenied) {
					t.Fatalf("recordset escaped binding: %v", err)
				}
				if len(leaf.calls) != 0 {
					t.Fatal("denied table dispatched", leaf.calls)
				}
				_, err := session.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Customer").Query)
				return err
			}
			var err error
			switch route {
			case "bound":
				err = check(db)
			case "readonly", "bound-readonly":
				err = db.RunReadonlyTransaction(ctx, func(_ context.Context, tx dal.ReadTransaction) error { return check(tx) })
			default:
				err = db.RunReadwriteTransaction(ctx, func(_ context.Context, tx dal.ReadwriteTransaction) error { return check(tx) })
			}
			if err != nil || leaf.calls["QueryRecords"] != 1 {
				t.Fatal("positive control failed", err, leaf.calls)
			}
		})
	}
}
func TestNativeTableNamespaceBoundConflictsAndContextPolicyRequirement(t *testing.T) {
	var nilContext context.Context
	leaf := &fakeSession{}
	raw := dal.NewDB(&fakeDB{fakeSession: leaf})
	db := MustSecureDB(raw, WithDatabasePolicies(nativeLegacyPolicy()))
	bound := BindDB(db, nativeBindingContext(t))
	other, err := WithNativeTableNamespace(context.Background(), "temp")
	if err != nil {
		t.Fatal(err)
	}
	for _, handle := range []dal.DB{bound, BindDB(bound, other)} {
		if _, err := handle.ExecuteQueryToRecordsReader(other, nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
			t.Fatal("conflict accepted", err)
		}
	}
	if _, err := bound.ExecuteQueryToRecordsReader(nilContext, nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("nil operation accepted", err)
	}
	required := BindDB(MustSecureDB(raw, WithDatabasePolicies(nativeLegacyPolicy()), RequireContextPolicy()), nativeBindingContext(t))
	if _, err := required.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("namespace counted as policy", err)
	}
	if _, err := bound.ExecuteQueryToRecordsetReader(other, nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("recordset conflict accepted", err)
	}
	g := (guard{databasePolicies: []Policy{nativeLegacyPolicy()}}).bind(nativeBindingContext(t))
	if _, _, err := g.authorizeRequest(other, nativeBindingRequest("Customer")); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("guard conflict accepted", err)
	}
	if len(leaf.calls) != 0 {
		t.Fatal("denied request dispatched", leaf.calls)
	}
	// Capturing no context remains harmless to the existing bind helper.
	if len((guard{}).bind(nilContext).nativeNamespaces) != 0 {
		t.Fatal("nil bound a namespace")
	}
}
func TestNativeTableNamespaceNilDefaultCompatibility(t *testing.T) {
	var nilContext context.Context
	if ctx, err := WithNativeTableNamespace(nilContext, "main"); err == nil || ctx != nil {
		t.Fatal("nil option context accepted")
	}
	seen := []Request{}
	calls := 0
	policy := nativeRequestPolicy{seen: &seen, expected: nil, pure: true, calls: &calls}
	if got := AssessPlan(nilContext, nativeBindingRequest("Customer"), []Policy{policy}); got.Outcome != AssessmentAllow || calls != 1 {
		t.Fatal("nil default changed", got, calls)
	}
}

func TestNativeTableNamespaceBoundOuterPreservesInnerMandatoryDenial(t *testing.T) {
	for _, route := range []string{"direct", "readonly", "readwrite"} {
		t.Run(route, func(t *testing.T) {
			leaf := &fakeSession{}
			tx := &fakeTx{fakeSession: leaf, opts: dal.NewTransactionOptions()}
			raw := nativeFreshContextDB{&fakeDB{fakeSession: leaf, ro: tx, rw: tx}}
			inner := MustSecureDB(dal.NewDB(raw), WithDatabasePolicies(nativeLegacyPolicy()))
			outerPolicy := MustPolicy("outer", Root(Allow(Read)), OpaqueQueryScope(Allow(Query)))
			bound := BindDB(MustSecureDB(inner, WithDatabasePolicies(outerPolicy)), nativeBindingContext(t))
			check := func(session dal.ReadSession) error {
				if _, err := session.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Invoice").Query); !errors.Is(err, ErrAccessDenied) {
					t.Fatal("inner record denial lost", err)
				}
				if _, err := session.ExecuteQueryToRecordsetReader(context.Background(), nativeBindingRequest("Invoice").Query); !errors.Is(err, ErrAccessDenied) {
					t.Fatal("inner recordset denial lost", err)
				}
				if len(leaf.calls) != 0 {
					t.Fatal("denied query reached leaf", leaf.calls)
				}
				if _, err := session.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Customer").Query); err != nil {
					return err
				}
				_, err := session.ExecuteQueryToRecordsetReader(context.Background(), nativeBindingRequest("Customer").Query)
				return err
			}
			var err error
			switch route {
			case "direct":
				err = check(bound)
			case "readonly":
				err = bound.RunReadonlyTransaction(context.Background(), func(_ context.Context, tx dal.ReadTransaction) error { return check(tx) })
			default:
				err = bound.RunReadwriteTransaction(context.Background(), func(_ context.Context, tx dal.ReadwriteTransaction) error { return check(tx) })
			}
			if err != nil || leaf.calls["QueryRecords"] != 1 || leaf.calls["QueryRecordset"] != 1 {
				t.Fatal("positive controls failed", err, leaf.calls)
			}
		})
	}
}
func TestNativeTableNamespaceNestedConflictAndPurity(t *testing.T) {
	leaf := &fakeSession{}
	outerPolicy := MustPolicy("outer", Root(Allow(Read)), OpaqueQueryScope(Allow(Query)))
	temp, err := WithNativeTableNamespace(context.Background(), "temp")
	if err != nil {
		t.Fatal(err)
	}
	inner := BindDB(MustSecureDB(dal.NewDB(&fakeDB{fakeSession: leaf}), WithDatabasePolicies(nativeLegacyPolicy())), temp)
	bound := BindDB(MustSecureDB(inner, WithDatabasePolicies(outerPolicy)), nativeBindingContext(t))
	if _, err := bound.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("inner conflict ignored", err)
	}
	calls := 0
	seen := []Request{}
	impure := nativeRequestPolicy{seen: &seen, calls: &calls, pure: false}
	inner = MustSecureDB(dal.NewDB(&fakeDB{fakeSession: leaf}), WithDatabasePolicies(impure))
	bound = BindDB(MustSecureDB(inner, WithDatabasePolicies(outerPolicy)), nativeBindingContext(t))
	if _, err := bound.ExecuteQueryToRecordsReader(context.Background(), nativeBindingRequest("Customer").Query); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("inner impure policy accepted", err)
	}
	if calls != 0 || len(leaf.calls) != 0 {
		t.Fatal("impure policy or denied leaf called", calls, leaf.calls)
	}
}

func TestNativeTableNamespaceCanonicalUsesExistingRulePrecedence(t *testing.T) {
	ctx := nativeBindingContext(t)
	request := nativeBindingRequest("Customer")
	tableWins := MustPolicy("table-wins", TableScope(TableName{Schema: "main", Name: "Customer"}, Allow(Query)), Collection("Customer", Allow(Query).Fields("id").Where(dal.WhereField("id", dal.GreaterThen, 1))))
	assessment := AssessPlan(ctx, request, []Policy{tableWins})
	if assessment.Outcome != AssessmentAllow || len(assessment.Restrictions) != 0 {
		t.Fatal("existing table precedence changed", assessment)
	}
	opaqueOnly := MustPolicy("opaque-only", OpaqueQueryScope(Allow(Query)))
	if got := AssessPlan(context.Background(), request, []Policy{opaqueOnly}); got.Outcome != AssessmentAllow {
		t.Fatal("baseline opaque grant lost", got)
	}
	if got := AssessPlan(ctx, request, []Policy{opaqueOnly}); got.Outcome != AssessmentDeny {
		t.Fatal("missing canonical grant accepted", got)
	}
}
func TestNativeTableNamespaceConfigurationDiagnostics(t *testing.T) {
	leaf := &fakeSession{}
	bound := BindDB(MustSecureDB(dal.NewDB(&fakeDB{fakeSession: leaf}), WithDatabasePolicies(nativeLegacyPolicy())), nativeBindingContext(t))
	other, err := WithNativeTableNamespace(context.Background(), "temp")
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	for _, ctx := range []context.Context{other, nilContext} {
		for _, recordset := range []bool{false, true} {
			if recordset {
				_, err = bound.ExecuteQueryToRecordsetReader(ctx, nativeBindingRequest("Customer").Query)
			} else {
				_, err = bound.ExecuteQueryToRecordsReader(ctx, nativeBindingRequest("Customer").Query)
			}
			var denied *DeniedError
			if !errors.As(err, &denied) || denied.Decision.Code != CodeConfigurationInvalid || denied.Decision.Explanation != "native table namespace cannot be applied" {
				t.Fatal("configuration diagnostic changed", err)
			}
			// Query entry rejects conflicting configuration before it walks resources;
			// the existing zero-resource rendering is explicitly accepted here.
			if denied.Decision.Resource.String() != "/" {
				t.Fatal("unexpected diagnostic resource", denied.Decision.Resource)
			}
		}
	}
	if len(leaf.calls) != 0 {
		t.Fatal("configuration refusal dispatched", leaf.calls)
	}
}
