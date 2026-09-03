package access

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type customer struct {
	Name       string `json:"name"`
	AssignedTo string `json:"assignedTo"`
	TenantID   string `json:"tenantID"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

func assignedToCurrentUser() dal.Condition {
	return dal.WhereField("assignedTo", dal.Equal, dal.NewParam("currentUser"))
}

func customersPolicy(t *testing.T) *AccessPolicy {
	t.Helper()
	policy, err := NewPolicy("customers",
		Scope("customers", AnyID,
			Allow(Read, "read-all"),
			Allow(Update|Delete, "edit-assigned").Where(assignedToCurrentUser()),
		),
	)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	return policy
}

func mustDecideAllowed(t *testing.T, decision Decision) {
	t.Helper()
	if !decision.Allowed {
		t.Fatalf("expected allowed, got %+v", decision)
	}
}

func TestConditionalRuleCompilation(t *testing.T) {
	condition := assignedToCurrentUser()
	for name, rules := range map[string][]Rule{
		"conditional deny":         {Root(Deny(Get, "d").Where(condition))},
		"condition on scope":       {Under(Path("x"), Allow(Get)).Where(condition)},
		"collection group":         {CollectionGroupScope("g", Allow(Query, "q").Where(condition))},
		"opaque query":             {OpaqueQueryScope(Allow(Query, "q").Where(condition))},
		"explicit truncate":        {Root(Allow(Get|Truncate, "t").Where(condition))},
		"invalid condition":        {Root(Allow(Get, "bad").Where(dal.Comparison{Operator: dal.Equal, Left: dal.Constant{Value: 1}, Right: dal.Constant{Value: 1}}))},
		"invalid nested condition": {Root(Allow(Get, "bad").Where(dal.NewGroupCondition(dal.And)))},
	} {
		if _, err := NewPolicy("p", rules...); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
	if _, err := NewAuditPolicy("a", Root(Audit(Get, "x").Where(condition))); err == nil {
		t.Error("conditional audit rule must be rejected")
	}
	// The write and readwrite groups drop Truncate rather than failing.
	for _, operations := range []Operations{Write, ReadWrite} {
		policy := MustPolicy("p", Scope("notes", AnyID, Allow(operations, "own").Where(condition)))
		ctx := WithCurrentUser(context.Background(), "u1")
		key := RecordResourceForKey(record.NewKeyWithID("notes", "n1"))
		if decision := policy.Decide(ctx, Request{Operation: Truncate, Resources: []Resource{key}}); decision.Allowed {
			t.Errorf("%s: truncate must not be authorised by a conditional rule", operations)
		}
		decision := policy.Decide(ctx, Request{Operation: Delete, Resources: []Resource{key}})
		mustDecideAllowed(t, decision)
		if decision.Residuals == nil || decision.Residuals[0] == nil {
			t.Errorf("%s: delete must carry a residual", operations)
		}
	}
	// A generated name mentions the condition.
	policy := MustPolicy("p", Scope("notes", AnyID, Allow(Get).Where(condition)))
	decision := policy.Decide(WithCurrentUser(context.Background(), "u1"), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("notes", "n1"))}})
	if !strings.Contains(decision.Rule, "where assignedTo = $currentUser") {
		t.Errorf("generated rule name = %q", decision.Rule)
	}
	// Explanations and condition text carry parameter names, never values.
	if strings.Contains(decision.Explanation, "u1") || strings.Contains(decision.Condition, "u1") {
		t.Errorf("decision leaks a resolved value: %+v", decision)
	}
}

func TestConditionalDecisions(t *testing.T) {
	policy := customersPolicy(t)
	c1 := RecordResourceForKey(record.NewKeyWithID("customers", "c1"))
	ctx := WithCurrentUser(context.Background(), "u1")

	get := policy.Decide(ctx, Request{Operation: Get, Resources: []Resource{c1}})
	mustDecideAllowed(t, get)
	if get.Residuals != nil || get.Rule != "read-all" {
		t.Errorf("read must be unconditional: %+v", get)
	}

	edit := policy.Decide(ctx, Request{Operation: Update, Resources: []Resource{c1}})
	mustDecideAllowed(t, edit)
	if edit.Rule != "edit-assigned" || edit.Condition != "assignedTo = $currentUser" || len(edit.Residuals) != 1 || edit.Residuals[0] == nil {
		t.Errorf("edit decision = %+v", edit)
	}
	if got := edit.Residuals[0].String(); got != "assignedTo = 'u1'" {
		t.Errorf("residual = %q", got)
	}

	missing := policy.Decide(context.Background(), Request{Operation: Update, Resources: []Resource{c1}})
	if missing.Allowed || !strings.Contains(missing.Explanation, "$currentUser") || missing.Condition == "" {
		t.Errorf("missing variable must deny and name the parameter: %+v", missing)
	}
	if err := policy.Authorize(context.Background(), Request{Operation: Update, Resources: []Resource{c1}}); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "$currentUser") {
		t.Errorf("Authorize error = %v", err)
	}

	// A conditional allow deeper than an unconditional deny yields a residual.
	deeper := MustPolicy("deeper", Root(Deny(Get, "deny-root")), Scope("docs", AnyID, Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, dal.NewParam("currentUser")))))
	doc := RecordResourceForKey(record.NewKeyWithID("docs", "d1"))
	decision := deeper.Decide(ctx, Request{Operation: Get, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals[0] == nil {
		t.Errorf("expected residual: %+v", decision)
	}
	// An unconditional deny at equal specificity wins over a conditional allow.
	tie := MustPolicy("tie", Scope("docs", AnyID, Deny(Get, "no"), Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, dal.NewParam("currentUser")))))
	if decision := tie.Decide(ctx, Request{Operation: Get, Resources: []Resource{doc}}); decision.Allowed {
		t.Errorf("deny must win the tie: %+v", decision)
	}
	// An unconditional allow makes a deeper condition moot.
	moot := MustPolicy("moot", Root(Allow(Get, "all")), Scope("docs", AnyID, Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, dal.NewParam("currentUser")))))
	decision = moot.Decide(ctx, Request{Operation: Get, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Residuals != nil || decision.Rule != "all" {
		t.Errorf("unconditional allow must make the condition moot: %+v", decision)
	}
	// Two conditional allows at one specificity combine with OR.
	two := MustPolicy("two", Scope("docs", AnyID,
		Allow(Get, "a").Where(dal.WhereField("owner", dal.Equal, dal.NewParam("currentUser"))),
		Allow(Get, "b").Where(dal.WhereField("public", dal.Equal, dal.Constant{Value: true})),
	))
	decision = two.Decide(ctx, Request{Operation: Get, Resources: []Resource{doc}})
	mustDecideAllowed(t, decision)
	if decision.Rule != "a, b" || decision.Condition != "(owner = $currentUser OR public = true)" {
		t.Errorf("combined decision = %+v", decision)
	}
	if got := decision.Residuals[0].String(); got != "(owner = 'u1' OR public = true)" {
		t.Errorf("combined residual = %q", got)
	}
	// One unresolved parameter among several conditional rules denies.
	if decision := two.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{doc}}); decision.Allowed {
		t.Errorf("unresolved parameter must deny: %+v", decision)
	}
	// Multi-resource requests carry residuals per resource.
	mixed := MustPolicy("mixed", Scope("docs", AnyID, Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, dal.NewParam("currentUser")))), Scope("docs", "shared", Allow(Get, "shared")))
	resources := []Resource{RecordResourceForKey(record.NewKeyWithID("docs", "shared")), doc}
	decision = mixed.Decide(ctx, Request{Operation: Get, Resources: resources})
	mustDecideAllowed(t, decision)
	if len(decision.Residuals) != 2 || decision.Residuals[0] != nil || decision.Residuals[1] == nil {
		t.Errorf("per-resource residuals = %+v", decision.Residuals)
	}
}

func TestVariables(t *testing.T) {
	ctx := WithVariables(context.Background(), map[string]any{"tenant": "t1", "currentUser": "u0"})
	ctx = WithCurrentUser(ctx, "u1")
	variables := variablesFromContext(ctx)
	if variables["tenant"] != "t1" || variables["currentUser"] != "u1" {
		t.Errorf("variables = %v", variables)
	}
	var nilContext context.Context
	if got := variablesFromContext(nilContext); len(got) != 0 {
		t.Errorf("nil context variables = %v", got)
	}
	for _, bad := range []func(){
		func() { WithVariables(nilContext, nil) },
		func() { WithVariables(context.Background(), map[string]any{"not valid": 1}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			bad()
		}()
	}
	resolver := newVariableResolver(WithVariables(context.Background(), map[string]any{"now": "fixed"}))
	if value, ok := resolver.resolve("now"); !ok || value != "fixed" {
		t.Errorf("explicit now = %v, %v", value, ok)
	}
	resolver = newVariableResolver(context.Background())
	if value, ok := resolver.resolve("now"); !ok || value.(time.Time).IsZero() {
		t.Errorf("built-in now = %v, %v", value, ok)
	}
	if _, ok := resolver.resolve("missing"); ok {
		t.Error("missing variable must not resolve")
	}
}

// stubReadSession records how the secured wrapper talks to an adapter.
type stubReadSession struct {
	exists      bool
	load        map[string]any
	getErr      error
	queryErr    error
	gets        int
	existsCalls int
	queries     []dal.Query
}

func (s *stubReadSession) Exists(context.Context, *record.Key) (bool, error) {
	s.existsCalls++
	return s.exists, nil
}

func (s *stubReadSession) Get(_ context.Context, rec record.Record) error {
	s.gets++
	if s.getErr != nil {
		return s.getErr
	}
	if !s.exists {
		rec.SetError(record.ErrRecordNotFound)
		return nil
	}
	if s.load != nil && rec.Data() != nil {
		encoded, _ := json.Marshal(s.load)
		_ = json.Unmarshal(encoded, rec.Data())
	}
	rec.SetError(nil)
	return nil
}

func (s *stubReadSession) GetMulti(ctx context.Context, records []record.Record) error {
	for _, rec := range records {
		if err := s.Get(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

func (s *stubReadSession) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	s.queries = append(s.queries, query)
	return nil, s.queryErr
}

func (s *stubReadSession) ExecuteQueryToRecordsetReader(_ context.Context, query dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	s.queries = append(s.queries, query)
	return nil, s.queryErr
}

func ownGetPolicy() *AccessPolicy {
	return MustPolicy("own", Scope("customers", AnyID, Allow(Read, "own").Where(assignedToCurrentUser())))
}

func TestConditionalPointReadsThroughStub(t *testing.T) {
	ctx := WithCurrentUser(context.Background(), "u1")
	key := record.NewKeyWithID("customers", "c2")

	// Get: the adapter loads the record, the wrapper denies and clears it.
	stub := &stubReadSession{exists: true, load: map[string]any{"name": "Bob", "assignedTo": "u2"}}
	session := SecureReadSession(stub, ownGetPolicy())
	data := &customer{}
	rec := record.NewRecordWithData(key, data)
	err := session.Get(ctx, rec)
	var denied *DeniedError
	if !errors.As(err, &denied) || !strings.Contains(err.Error(), "own") || strings.Contains(err.Error(), "u1") || strings.Contains(err.Error(), "Bob") {
		t.Fatalf("Get error = %v", err)
	}
	if *data != (customer{}) || !errors.Is(rec.Error(), ErrAccessDenied) {
		t.Errorf("denied record must be cleared: data=%+v err=%v", *data, rec.Error())
	}
	if denied.Decision.Condition != "assignedTo = $currentUser" {
		t.Errorf("Condition = %q", denied.Decision.Condition)
	}
	// Get of an own record keeps the data.
	stub.load = map[string]any{"name": "Ann", "assignedTo": "u1"}
	rec = record.NewRecordWithData(key, data)
	if err := session.Get(ctx, rec); err != nil || data.Name != "Ann" {
		t.Errorf("own record: err=%v data=%+v", err, *data)
	}
	// A missing record needs no evaluation.
	stub.exists = false
	rec = record.NewRecordWithData(key, &customer{})
	if err := session.Get(ctx, rec); err != nil || rec.Exists() {
		t.Errorf("missing record: err=%v exists=%v", err, rec.Exists())
	}
	// Adapter errors propagate.
	stub.getErr = errors.New("boom")
	if err := session.Get(ctx, record.NewRecordWithData(key, &customer{})); err == nil || err.Error() != "boom" {
		t.Errorf("adapter error = %v", err)
	}
	if err := session.GetMulti(ctx, []record.Record{record.NewRecordWithData(key, &customer{})}); err == nil {
		t.Error("GetMulti must propagate adapter errors")
	}
	stub.getErr = nil
	stub.exists = true

	// Exists is upgraded to a read; the adapter's Exists is never called.
	stub.load = map[string]any{"assignedTo": "u2"}
	if ok, err := session.Exists(ctx, key); ok || !errors.Is(err, ErrAccessDenied) {
		t.Errorf("Exists(other's) = %v, %v", ok, err)
	}
	stub.load = map[string]any{"assignedTo": "u1"}
	if ok, err := session.Exists(ctx, key); !ok || err != nil {
		t.Errorf("Exists(own) = %v, %v", ok, err)
	}
	stub.exists = false
	if ok, err := session.Exists(ctx, key); ok || err != nil {
		t.Errorf("Exists(missing) = %v, %v", ok, err)
	}
	stub.exists = true
	if stub.existsCalls != 0 {
		t.Errorf("adapter Exists called %d times under a conditional rule", stub.existsCalls)
	}
	stub.getErr = errors.New("boom")
	if _, err := session.Exists(ctx, key); err == nil || err.Error() != "boom" {
		t.Errorf("Exists adapter error = %v", err)
	}
	stub.getErr = nil
	// Without a conditional rule the adapter's Exists is used.
	plain := SecureReadSession(stub, MustPolicy("plain", Root(Allow(Read, "all"))))
	if _, err := plain.Exists(ctx, key); err != nil || stub.existsCalls != 1 {
		t.Errorf("plain Exists: err=%v calls=%d", err, stub.existsCalls)
	}
	// A denied authorization short-circuits before any read.
	if _, err := SecureReadSession(stub, MustPolicy("none", Root(Deny(Read, "no")))).Exists(ctx, key); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("denied Exists = %v", err)
	}

	// GetMulti: one denied record clears the whole batch.
	stub.load = map[string]any{"assignedTo": "u2"}
	first := &customer{}
	second := &customer{}
	records := []record.Record{record.NewRecordWithData(record.NewKeyWithID("customers", "c1"), first), record.NewRecordWithData(key, second)}
	if err := session.GetMulti(ctx, records); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("GetMulti = %v", err)
	}
	if *first != (customer{}) || *second != (customer{}) {
		t.Errorf("batch must be cleared: %+v %+v", *first, *second)
	}
	stub.load = map[string]any{"assignedTo": "u1"}
	records = []record.Record{record.NewRecordWithData(record.NewKeyWithID("customers", "c1"), first), record.NewRecordWithData(key, second)}
	if err := session.GetMulti(ctx, records); err != nil || first.AssignedTo != "u1" {
		t.Errorf("own batch: err=%v first=%+v", err, *first)
	}
	records = []record.Record{record.NewRecordWithData(record.NewKeyWithID("customers", "c1"), first)}
	if err := plain.GetMulti(ctx, records); err != nil {
		t.Errorf("plain GetMulti = %v", err)
	}
}

func TestConditionalQueriesThroughStub(t *testing.T) {
	ctx := WithVariables(context.Background(), map[string]any{"tenant": "t1"})
	policy := MustPolicy("tenant", Collection("orders", Allow(Query, "tenant-slice").Where(dal.WhereField("tenantID", dal.Equal, dal.NewParam("tenant")))))
	stub := &stubReadSession{}
	session := SecureReadSession(stub, policy)
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", ""))).
		WhereField("status", dal.Equal, "open").Limit(10).OrderBy(dal.Ascending(dal.Field("createdAt"))).
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithData(record.NewKeyWithID("orders", ""), &map[string]any{})
		})
	if _, err := session.ExecuteQueryToRecordsReader(ctx, query); err != nil {
		t.Fatalf("query: %v", err)
	}
	if _, err := session.ExecuteQueryToRecordsetReader(ctx, query); err != nil {
		t.Fatalf("recordset query: %v", err)
	}
	if len(stub.queries) != 2 {
		t.Fatalf("adapter saw %d queries", len(stub.queries))
	}
	for _, delegated := range stub.queries {
		structured, ok := delegated.(dal.StructuredQuery)
		if !ok {
			t.Fatalf("delegated query is %T", delegated)
		}
		where := structured.Where().String()
		if where != "(status = 'open' AND tenantID = 't1')" {
			t.Errorf("delegated where = %q", where)
		}
		if strings.Contains(where, "$") {
			t.Errorf("a parameter reached the adapter: %q", where)
		}
		if structured.Limit() != 10 || len(structured.OrderBy()) != 1 || !strings.Contains(delegated.String(), "WHERE (status = 'open' AND tenantID = 't1')") {
			t.Errorf("rewritten query lost parts: limit=%d order=%d string=%q", structured.Limit(), len(structured.OrderBy()), delegated.String())
		}
	}
	// The rewritten query is an ordinary structured query that renders its
	// residual in String(), which SQL-text adapters emit directly.
	rewritten := stub.queries[0]
	stub.queries = nil
	if _, err := rewritten.GetRecordsReader(ctx, stub); err != nil {
		t.Errorf("GetRecordsReader: %v", err)
	}
	if _, err := rewritten.GetRecordsetReader(ctx, stub); err != nil {
		t.Errorf("GetRecordsetReader: %v", err)
	}
	for i, delegated := range stub.queries {
		if !strings.Contains(delegated.String(), "tenantID = 't1'") {
			t.Errorf("executor call %d received a query without the residual: %s", i, delegated.String())
		}
	}
	// Without a caller condition the residual stands alone.
	bare := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", ""))).SelectKeysOnly(reflect.String)
	stub.queries = nil
	if _, err := session.ExecuteQueryToRecordsReader(ctx, bare); err != nil {
		t.Fatalf("bare query: %v", err)
	}
	if got := stub.queries[0].(dal.StructuredQuery).Where().String(); got != "tenantID = 't1'" {
		t.Errorf("bare where = %q", got)
	}
	// Missing variable denies before the adapter is called.
	stub.queries = nil
	if _, err := session.ExecuteQueryToRecordsReader(context.Background(), bare); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "$tenant") {
		t.Errorf("missing variable = %v", err)
	}
	if len(stub.queries) != 0 {
		t.Error("adapter must not be called on denial")
	}
	// Residuals from two policies conjoin.
	ctx2 := WithPolicy(ctx, MustPolicy("status", Collection("orders", Allow(Query, "open-only").Where(dal.WhereField("status", dal.Equal, dal.Constant{Value: "open"})))))
	stub.queries = nil
	if _, err := session.ExecuteQueryToRecordsReader(ctx2, bare); err != nil {
		t.Fatalf("two policies: %v", err)
	}
	if got := stub.queries[0].(dal.StructuredQuery).Where().String(); got != "(tenantID = 't1' AND status = 'open')" {
		t.Errorf("conjoined where = %q", got)
	}
	// A residual on a joined source is refused.
	joined := MustPolicy("join", Collection("orders", Allow(Query, "all")), Collection("users", Allow(Query, "own").Where(assignedToCurrentUser())))
	joinQuery := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", "o")).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("users", "u"), dal.JoinInner))).SelectKeysOnly(reflect.String)
	if _, err := SecureReadSession(stub, joined).ExecuteQueryToRecordsReader(WithCurrentUser(ctx, "u1"), joinQuery); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "joined source") {
		t.Errorf("joined residual = %v", err)
	}
	// Adapter errors propagate through the rewrite.
	stub.queryErr = errors.New("boom")
	if _, err := session.ExecuteQueryToRecordsReader(ctx, bare); err == nil || err.Error() != "boom" {
		t.Errorf("adapter query error = %v", err)
	}
}

func TestRewriteQueryEdges(t *testing.T) {
	bare := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", ""))).SelectKeysOnly(reflect.String)
	if q, err := rewriteQuery(bare, [][]residual{{}}); err != nil || q.(dal.StructuredQuery).Where() != nil {
		t.Errorf("empty base residuals must return the caller's query unchanged: %v, %v", q, err)
	}
	r := residual{rule: "r", text: "x = $y", condition: dal.WhereField("x", dal.Equal, dal.Constant{Value: 1})}
	if _, err := rewriteQuery(opaqueQ{}, [][]residual{{r}}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("opaque query with residual = %v", err)
	}
	q, err := rewriteQuery(bare, [][]residual{{r, r}})
	if err != nil || q.(dal.StructuredQuery).Where().String() != "(x = 1 AND x = 1)" {
		t.Errorf("two base residuals = %v, %v", q, err)
	}
}

func TestCheckRecordEdges(t *testing.T) {
	key := record.NewKeyWithID("customers", "c1")
	bad := residual{rule: "bad", text: "fake", condition: fakeCond{}}
	rec := record.NewRecordWithData(key, &customer{Name: "x"})
	rec.SetError(nil)
	if err := checkRecord(Get, rec, []residual{bad}); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "could not be evaluated") {
		t.Errorf("unevaluable condition = %v", err)
	}
	unserialisable := record.NewRecordWithData(key, &struct{ C chan int }{C: make(chan int)})
	unserialisable.SetError(nil)
	if err := checkRecord(Get, unserialisable, []residual{bad}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("unserialisable data = %v", err)
	}
	// Map data is emptied on denial; nil data is left alone.
	m := map[string]any{"assignedTo": "u2"}
	mapRecord := record.NewRecordWithData(key, m)
	mapRecord.SetError(nil)
	if err := checkRecord(Get, mapRecord, []residual{{rule: "own", text: "t", condition: dal.WhereField("assignedTo", dal.Equal, dal.Constant{Value: "u1"})}}); !errors.Is(err, ErrAccessDenied) || len(m) != 0 {
		t.Errorf("map record: err=%v map=%v", err, m)
	}
	nilData := record.NewRecordWithData(key, (*customer)(nil))
	nilData.SetError(nil)
	clearRecord(nilData, errors.New("x"))
	if err := checkRecords(Get, nil, nil); err != nil {
		t.Errorf("no residuals = %v", err)
	}
}

type fakeCond struct{}

func (fakeCond) String() string { return "fake" }

func TestConditionalWritesFailClosed(t *testing.T) {
	policy := customersPolicy(t)
	ctx := WithCurrentUser(context.Background(), "u1")
	key := record.NewKeyWithID("customers", "c1")
	session := SecureWriteSession(&stubWriteSession{}, policy)
	for name, op := range map[string]func() error{
		"update":       func() error { return session.Update(ctx, key, []update.Update{update.ByFieldName("name", "x")}) },
		"delete":       func() error { return session.Delete(ctx, key) },
		"delete multi": func() error { return session.DeleteMulti(ctx, []*record.Key{key}) },
	} {
		err := op()
		var denied *DeniedError
		if !errors.As(err, &denied) || !strings.Contains(err.Error(), "not enforced") || denied.Decision.Condition != "assignedTo = $currentUser" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := session.Insert(ctx, record.NewRecordWithData(key, &customer{})); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("insert without a rule = %v", err)
	}
}

type stubWriteSession struct{ dal.WriteSession }

func TestResidualsBoundedByResources(t *testing.T) {
	// A custom policy returning more residuals than resources must not panic.
	policy := oversizedPolicy{}
	g := guard{databasePolicies: []Policy{policy}}
	residuals, err := g.authorizeRequest(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("x", "1"))}})
	if err != nil || len(residuals) != 1 || len(residuals[0]) != 1 {
		t.Errorf("residuals = %v, %v", residuals, err)
	}
}

type oversizedPolicy struct{}

func (oversizedPolicy) Name() string { return "oversized" }
func (oversizedPolicy) Decide(context.Context, Request) Decision {
	condition := dal.WhereField("a", dal.Equal, dal.Constant{Value: 1})
	return Decision{Allowed: true, Policy: "oversized", Residuals: []dal.Condition{condition, condition, condition}}
}
func (p oversizedPolicy) Authorize(ctx context.Context, request Request) error {
	if d := p.Decide(ctx, request); !d.Allowed {
		return &DeniedError{Decision: d}
	}
	return nil
}

func TestConditionalReadsOnMemoryDB(t *testing.T) {
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	seed := map[string]customer{
		"c1": {Name: "Ann", AssignedTo: "u1", TenantID: "t1"},
		"c2": {Name: "Bob", AssignedTo: "u2", TenantID: "t1"},
		"c3": {Name: "Cy", AssignedTo: "u1", TenantID: "t2"},
	}
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for id, c := range seed {
			c := c
			if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("customers", id), &c)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Read all customers, edit only assigned ones: reads are unconditional.
	db := MustSecureDB(raw, WithDatabasePolicies(customersPolicy(t)))
	user1 := WithCurrentUser(ctx, "u1")
	other := &customer{}
	if err := db.Get(user1, record.NewRecordWithData(record.NewKeyWithID("customers", "c2"), other)); err != nil || other.Name != "Bob" {
		t.Errorf("read-all get: err=%v data=%+v", err, *other)
	}

	// Tenant slice: the engine filters, paging and order intact.
	tenantDB := MustSecureDB(raw, WithDatabasePolicies(MustPolicy("tenant", Collection("customers", Allow(Query, "tenant-slice").Where(dal.WhereField("tenantID", dal.Equal, dal.NewParam("tenant")))))))
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("customers", ""))).Limit(10).OrderBy(dal.Ascending(dal.Field("name"))).
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithData(record.NewKeyWithID("customers", ""), &customer{})
		})
	reader, err := tenantDB.ExecuteQueryToRecordsReader(WithVariables(ctx, map[string]any{"tenant": "t1"}), query)
	if err != nil {
		t.Fatalf("tenant query: %v", err)
	}
	var names []string
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		c := rec.Data().(*customer)
		if c.TenantID != "t1" {
			t.Errorf("row outside tenant: %+v", *c)
		}
		names = append(names, c.Name)
	}
	if !reflect.DeepEqual(names, []string{"Ann", "Bob"}) {
		t.Errorf("tenant rows = %v", names)
	}

	// Own-record reads through the memory adapter.
	ownDB := MustSecureDB(raw, WithDatabasePolicies(ownGetPolicy()))
	mine := &customer{}
	if err := ownDB.Get(user1, record.NewRecordWithData(record.NewKeyWithID("customers", "c1"), mine)); err != nil || mine.Name != "Ann" {
		t.Errorf("own get: err=%v data=%+v", err, *mine)
	}
	theirs := &customer{}
	if err := ownDB.Get(user1, record.NewRecordWithData(record.NewKeyWithID("customers", "c2"), theirs)); !errors.Is(err, ErrAccessDenied) || *theirs != (customer{}) {
		t.Errorf("other's get: err=%v data=%+v", err, *theirs)
	}
	if ok, err := ownDB.Exists(user1, record.NewKeyWithID("customers", "c2")); ok || !errors.Is(err, ErrAccessDenied) {
		t.Errorf("other's exists = %v, %v", ok, err)
	}
	if ok, err := ownDB.Exists(user1, record.NewKeyWithID("customers", "c1")); !ok || err != nil {
		t.Errorf("own exists = %v, %v", ok, err)
	}
	// Inside a read-only transaction the same rules apply.
	if err := ownDB.RunReadonlyTransaction(user1, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("customers", "c2"), &customer{}))
	}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("transactional get = %v", err)
	}
}
