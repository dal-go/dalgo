package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/update"
)

type fakeSession struct {
	calls map[string]int
	err   error
}

func (f *fakeSession) call(s string) error {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[s]++
	return f.err
}
func (f *fakeSession) Exists(context.Context, *dal.Key) (bool, error) { return true, f.call("Exists") }
func (f *fakeSession) Get(context.Context, dal.Record) error          { return f.call("Get") }
func (f *fakeSession) GetMulti(context.Context, []dal.Record) error   { return f.call("GetMulti") }
func (f *fakeSession) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, f.call("QueryRecords")
}
func (f *fakeSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, f.call("QueryRecordset")
}
func (f *fakeSession) Set(context.Context, dal.Record) error        { return f.call("Set") }
func (f *fakeSession) SetMulti(context.Context, []dal.Record) error { return f.call("SetMulti") }
func (f *fakeSession) Insert(context.Context, dal.Record, ...dal.InsertOption) error {
	return f.call("Insert")
}
func (f *fakeSession) InsertMulti(context.Context, []dal.Record, ...dal.InsertOption) error {
	return f.call("InsertMulti")
}
func (f *fakeSession) Update(context.Context, *dal.Key, []update.Update, ...dal.Precondition) error {
	return f.call("Update")
}
func (f *fakeSession) UpdateRecord(context.Context, dal.Record, []update.Update, ...dal.Precondition) error {
	return f.call("UpdateRecord")
}
func (f *fakeSession) UpdateMulti(context.Context, []*dal.Key, []update.Update, ...dal.Precondition) error {
	return f.call("UpdateMulti")
}
func (f *fakeSession) Delete(context.Context, *dal.Key) error        { return f.call("Delete") }
func (f *fakeSession) DeleteMulti(context.Context, []*dal.Key) error { return f.call("DeleteMulti") }

type opaqueQ struct{}

func (opaqueQ) String() string { return "opaque" }
func (opaqueQ) Offset() int    { return 0 }
func (opaqueQ) Limit() int     { return 0 }
func (opaqueQ) GetRecordsReader(context.Context, dal.QueryExecutor) (dal.RecordsReader, error) {
	return nil, nil
}
func (opaqueQ) GetRecordsetReader(context.Context, dal.QueryExecutor) (dal.RecordsetReader, error) {
	return nil, nil
}

type requestCapturePolicy struct{ request Request }

func (p *requestCapturePolicy) Name() string { return "capture" }
func (p *requestCapturePolicy) Decide(_ context.Context, request Request) Decision {
	p.request = request
	return Decision{Allowed: true}
}
func (p *requestCapturePolicy) Authorize(ctx context.Context, request Request) error {
	p.Decide(ctx, request)
	return nil
}

func TestContextGuardComplete(t *testing.T) {
	ctx := context.Background()
	p := MustPolicy("all", Root(Allow(ReadWrite, "all")))
	var nilCtx context.Context
	if got := policiesFromContext(nilCtx); got != nil {
		t.Fatal(got)
	}
	if len(policiesFromContext(ctx)) != 0 {
		t.Fatal()
	}
	ctx = WithPolicy(ctx, p)
	if len(policiesFromContext(ctx)) != 1 {
		t.Fatal()
	}
	ctx2 := WithPolicy(ctx, p)
	if len(policiesFromContext(ctx2)) != 2 {
		t.Fatal()
	}
	for _, fn := range []func(){func() { WithPolicy(nilCtx, p) }, func() { WithPolicy(context.Background(), nil) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("panic")
				}
			}()
			fn()
		}()
	}
	g := guard{databasePolicies: []Policy{p}, requireContext: true}
	if err := g.checkContext(context.Background()); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if err := (guard{requireContext: true}).authorize(context.Background(), Get, RecordResourceForKey(dal.NewKeyWithID("x", "1"))); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	g = g.bind(ctx)
	if err := g.checkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := g.authorize(context.Background(), Get, RecordResourceForKey(dal.NewKeyWithID("x", "1"))); err != nil {
		t.Fatal(err)
	}
	deny := MustPolicy("deny", Root(Deny(Get, "no")))
	if err := (guard{databasePolicies: []Policy{deny}}).authorize(ctx, Get, RecordResourceForKey(dal.NewKeyWithID("x", "1"))); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if err := (guard{requireContext: true}).requireContextPolicy(Get, 1); err != nil {
		t.Fatal(err)
	}
}

func TestSecuredSessionsAllMethods(t *testing.T) {
	ctx := context.Background()
	f := &fakeSession{}
	allow := MustPolicy("all", Root(Allow(ReadWrite, "all")), OpaqueQueryScope(Allow(Query, "opaque")), CollectionGroupScope("items", Allow(Query, "group")))
	rw := SecureReadwriteSession(f, allow)
	key := dal.NewKeyWithID("items", "1")
	r := dal.NewRecord(key)
	rs := []dal.Record{r}
	keys := []*dal.Key{key}
	if ok, err := rw.Exists(ctx, key); !ok || err != nil {
		t.Fatal(err)
	}
	if err := rw.Get(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := rw.GetMulti(ctx, rs); err != nil {
		t.Fatal(err)
	}
	q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().SelectKeysOnly(reflect.String)
	if _, err := rw.ExecuteQueryToRecordsReader(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := rw.ExecuteQueryToRecordsetReader(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := rw.ExecuteQueryToRecordsReader(ctx, opaqueQ{}); err != nil {
		t.Fatal(err)
	}
	capture := new(requestCapturePolicy)
	if _, err := SecureReadSession(f, capture).ExecuteQueryToRecordsReader(ctx, q); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capture.request.Query, q) || capture.request.Operation != Query {
		t.Fatalf("query metadata was not preserved: %#v", capture.request)
	}
	for name, fn := range map[string]func() error{
		"Set": func() error { return rw.Set(ctx, r) }, "SetMulti": func() error { return rw.SetMulti(ctx, rs) }, "Insert": func() error { return rw.Insert(ctx, r) }, "InsertMulti": func() error { return rw.InsertMulti(ctx, rs) },
		"Update": func() error { return rw.Update(ctx, key, nil) }, "UpdateRecord": func() error { return rw.UpdateRecord(ctx, r, nil) }, "UpdateMulti": func() error { return rw.UpdateMulti(ctx, keys, nil) }, "Delete": func() error { return rw.Delete(ctx, key) }, "DeleteMulti": func() error { return rw.DeleteMulti(ctx, keys) },
	} {
		if err := fn(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(f.calls) < 14 {
		t.Fatalf("calls: %#v", f.calls)
	}
	_ = SecureReadSession(f, allow)
	_ = SecureWriteSession(f, allow)
	deny := MustPolicy("deny", Root(Deny(ReadWrite, "deny")))
	dr := SecureReadwriteSession(f, deny)
	before := len(f.calls)
	if err := dr.Get(ctx, r); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if err := dr.Set(ctx, r); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if len(f.calls) != before {
		t.Fatal("delegate called")
	}
	if got := resourcesForRecords(rs); len(got) != 1 {
		t.Fatal()
	}
	if got := resourcesForKeys(keys); len(got) != 1 {
		t.Fatal()
	}
	if got := resourcesForQuery(opaqueQ{}); got[0].Kind() != OpaqueQueryResource {
		t.Fatal(got)
	}
	c := dal.NewRootCollectionRef("items", "")
	cg := dal.NewCollectionGroupRef("items", "")
	for _, src := range []dal.RecordsetSource{c, &c, cg, &cg} {
		if got := resourceForRecordsetSource(src); got.String() == "" {
			t.Fatal()
		}
	}
	from := dal.From(c)
	from.Join(dal.NewJoinedSource(cg, dal.JoinInner))
	jq := from.NewQuery().SelectKeysOnly(reflect.String)
	if len(resourcesForQuery(jq)) != 2 {
		t.Fatal()
	}
}

type fakeTx struct {
	*fakeSession
	opts dal.TransactionOptions
}

func (f *fakeTx) ID() string                      { return "tx" }
func (f *fakeTx) Options() dal.TransactionOptions { return f.opts }

type fakeDB struct {
	*fakeSession
	adapter dal.Adapter
	schema  dal.Schema
	ro      *fakeTx
	rw      *fakeTx
}

func (f *fakeDB) ID() string                          { return "db" }
func (f *fakeDB) Adapter() dal.Adapter                { return f.adapter }
func (f *fakeDB) Schema() dal.Schema                  { return f.schema }
func (f *fakeDB) SupportsConcurrentConnections() bool { return true }
func (f *fakeDB) RunReadonlyTransaction(ctx context.Context, w dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return w(dal.NewContextWithTransaction(ctx, f.ro), f.ro)
}
func (f *fakeDB) RunReadwriteTransaction(ctx context.Context, w dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return w(dal.NewContextWithTransaction(ctx, f.rw), f.rw)
}

func TestSecureDBAndTransactions(t *testing.T) {
	fs := &fakeSession{}
	tx := &fakeTx{fakeSession: &fakeSession{}, opts: dal.NewTransactionOptions()}
	raw := &fakeDB{fakeSession: fs, adapter: dal.NewAdapter("a", "1"), schema: dal.NewSchema(nil, nil), ro: tx, rw: tx}
	if _, err := SecureDB(nil); err == nil {
		t.Fatal()
	}
	if _, err := SecureDB(raw, nil); err == nil {
		t.Fatal()
	}
	if _, err := SecureDB(raw, WithDatabasePolicies(nil)); err == nil {
		t.Fatal()
	}
	bad := func(*secureDBOptions) error { return errors.New("bad") }
	if _, err := SecureDB(raw, bad); err == nil {
		t.Fatal()
	}
	allow := MustPolicy("all", Root(Allow(ReadWrite, "all")))
	db, err := SecureDB(raw, WithDatabasePolicies(allow), RequireContextPolicy())
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic")
			}
		}()
		MustSecureDB(nil)
	}()
	db2 := MustSecureDB(raw, WithDatabasePolicies(allow))
	if db2 == nil {
		t.Fatal()
	}
	ctx := WithPolicy(context.Background(), allow)
	bound := BindDB(db, ctx)
	_ = BindDB(raw, ctx)
	if bound.ID() != "db" || bound.Adapter().Name() != "a" || bound.Schema() == nil || !bound.SupportsConcurrentConnections() {
		t.Fatal("metadata")
	}
	r := dal.NewRecord(dal.NewKeyWithID("x", "1"))
	if _, err := bound.Exists(context.Background(), r.Key()); err != nil {
		t.Fatal(err)
	}
	if err := bound.Get(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := bound.GetMulti(context.Background(), []dal.Record{r}); err != nil {
		t.Fatal(err)
	}
	q := dal.From(dal.NewRootCollectionRef("x", "")).NewQuery().SelectKeysOnly(reflect.String)
	if _, err := bound.ExecuteQueryToRecordsReader(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := bound.ExecuteQueryToRecordsetReader(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if err := db.RunReadonlyTransaction(context.Background(), func(context.Context, dal.ReadTransaction) error { return nil }); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if err := db.RunReadwriteTransaction(context.Background(), func(context.Context, dal.ReadwriteTransaction) error { return nil }); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if err := bound.RunReadonlyTransaction(context.Background(), func(c context.Context, st dal.ReadTransaction) error {
		if st.Options() != tx.opts || dal.GetTransaction(c) != st {
			t.Fatal("secured ro")
		}
		return st.Get(c, r)
	}); err != nil {
		t.Fatal(err)
	}
	if err := bound.RunReadwriteTransaction(context.Background(), func(c context.Context, st dal.ReadwriteTransaction) error {
		if st.ID() != "tx" || st.Options() != tx.opts || dal.GetTransaction(c) != st {
			t.Fatal("secured rw")
		}
		return st.Set(c, r)
	}); err != nil {
		t.Fatal(err)
	}
}
