package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

func TestFieldPatternMatching(t *testing.T) {
	set, err := parseFieldPatterns([]string{"name", "address.*", "public_*", "*_id", "meta.*.value", "tags"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for path, want := range map[string]bool{
		"name": true, "address": true, "address.city": true, "address.geo.lat": true,
		"public_bio": true, "public_": true, "user_id": true, "meta.a.value": true, "meta.a.other": false,
		"tags": true, "tags.0": true, "email": false, "nam": false, "namex": false, "passwordHash": false,
	} {
		if got := set.allows(path); got != want {
			t.Errorf("allows(%q) = %v, want %v", path, got, want)
		}
	}
	if names, ok := set.enumerable(); ok {
		t.Errorf("wildcard first segments must not be enumerable: %v", names)
	}
	literal, _ := parseFieldPatterns([]string{"id", "name", "address.*", "id"})
	if names, ok := literal.enumerable(); !ok || !reflect.DeepEqual(names, []string{"address", "id", "name"}) {
		t.Errorf("enumerable = %v, %v", names, ok)
	}
	for name, patterns := range map[string][]string{
		"empty list":    {},
		"empty pattern": {" "},
		"empty segment": {"a..b"},
		"middle star":   {"a*b"},
		"two stars":     {"*a*"},
	} {
		if _, err := parseFieldPatterns(patterns); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	var none *fieldSet
	if !none.allows("anything") || (fieldSets{none}).restrictive() {
		t.Error("a nil set allows everything and is not restrictive")
	}
	if _, ok := none.enumerable(); ok {
		t.Error("a nil set is not enumerable")
	}
	// Intersection across sets.
	a, _ := parseFieldPatterns([]string{"id", "name", "email"})
	b, _ := parseFieldPatterns([]string{"name", "phone"})
	sets := fieldSets{a, nil, b}
	if !sets.allows("name") || sets.allows("email") || sets.allows("phone") {
		t.Error("intersection must allow only name")
	}
	if names, ok := sets.enumerable(); !ok || !reflect.DeepEqual(names, []string{"name"}) {
		t.Errorf("intersection enumerable = %v, %v", names, ok)
	}
	if !strings.Contains(sets.sources(), "[id, name, email]") || !strings.Contains(sets.sources(), " ∩ ") {
		t.Errorf("sources = %q", sets.sources())
	}
	wild, _ := parseFieldPatterns([]string{"*_id"})
	if _, ok := (fieldSets{a, wild}).enumerable(); ok {
		t.Error("a wildcard set in the intersection is not enumerable")
	}
	if _, ok := (fieldSets{nil}).enumerable(); ok {
		t.Error("no restrictive set means nothing to enumerate")
	}
}

func TestFieldRedaction(t *testing.T) {
	set, _ := parseFieldPatterns([]string{"name", "address.city"})
	sets := fieldSets{set}
	data := map[string]any{"name": "Ann", "email": "a@x", "address": map[string]any{"city": "Cork", "zip": "T12"}, "meta": map[string]any{"secret": 1}}
	sets.redactMap("", data)
	if !reflect.DeepEqual(data, map[string]any{"name": "Ann", "address": map[string]any{"city": "Cork"}}) {
		t.Errorf("redacted map = %v", data)
	}
	if refused := sets.disallowedPaths(map[string]any{"name": "x", "email": "y", "address": map[string]any{"zip": "z"}, "empty": map[string]any{}}); !reflect.DeepEqual(refused, []string{"address.zip", "email", "empty"}) {
		t.Errorf("disallowedPaths = %v", refused)
	}
	if refused := sets.disallowedUpdates([]update.Update{update.ByFieldName("name", "x"), update.ByFieldPath(update.FieldPath{"address", "zip"}, "z"), update.DeleteByFieldName("email")}); !reflect.DeepEqual(refused, []string{"address.zip", "email"}) {
		t.Errorf("disallowedUpdates = %v", refused)
	}
	type user struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		PasswordHash string `json:"passwordHash"`
	}
	rec := record.NewRecordWithData(record.NewKeyWithID("users", "u1"), &user{Name: "Ann", Email: "a@x", PasswordHash: "h"})
	rec.SetError(nil)
	if err := sets.redactRecord(rec); err != nil || *rec.Data().(*user) != (user{Name: "Ann"}) {
		t.Errorf("struct redaction: err=%v data=%+v", err, rec.Data())
	}
	m := map[string]any{"name": "Ann", "email": "a@x"}
	mapRec := record.NewRecordWithData(record.NewKeyWithID("users", "u2"), m)
	mapRec.SetError(nil)
	if err := sets.redactRecord(mapRec); err != nil || len(m) != 1 {
		t.Errorf("map redaction: err=%v data=%v", err, m)
	}
	pm := map[string]any{"name": "Ann", "email": "a@x"}
	pointerRec := record.NewRecordWithData(record.NewKeyWithID("users", "u3"), &pm)
	pointerRec.SetError(nil)
	if err := sets.redactRecord(pointerRec); err != nil || len(pm) != 1 {
		t.Errorf("*map redaction: err=%v data=%v", err, pm)
	}
	missing := record.NewRecordWithData(record.NewKeyWithID("users", "u4"), &user{})
	missing.SetError(record.ErrRecordNotFound)
	if err := sets.redactRecord(missing); err != nil {
		t.Errorf("missing record: %v", err)
	}
	if err := (fieldSets{nil}).redactRecord(rec); err != nil {
		t.Errorf("non-restrictive: %v", err)
	}
	other := record.NewRecordWithData(record.NewKeyWithID("users", "u5"), map[int]any{1: "x"})
	other.SetError(nil)
	if err := sets.redactRecord(other); err == nil {
		t.Error("a non-string-keyed map cannot be redacted")
	}
	chanRec := record.NewRecordWithData(record.NewKeyWithID("users", "u6"), &struct{ C chan int }{C: make(chan int)})
	chanRec.SetError(nil)
	if err := sets.redactRecord(chanRec); err == nil {
		t.Error("unserialisable data must fail redaction")
	}
	scalar := record.NewRecordWithData(record.NewKeyWithID("users", "u7"), 42)
	scalar.SetError(nil)
	if err := sets.redactRecord(scalar); err != nil {
		t.Errorf("scalar data is left alone: %v", err)
	}
}

func TestFieldRuleCompilation(t *testing.T) {
	for name, rules := range map[string][]Rule{
		"fields on deny":   {Root(Deny(Get, "d").Fields("name"))},
		"fields on scope":  {Under(Path("x"), Allow(Get)).Fields("name")},
		"fields on opaque": {OpaqueQueryScope(Allow(Query, "q").Fields("name"))},
		"bad pattern":      {Root(Allow(Get, "g").Fields("a..b"))},
	} {
		if _, err := NewPolicy("p", rules...); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
	policy := MustPolicy("p", Scope("users", AnyID, Allow(Read).Fields("id", "name")))
	decision := policy.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("users", "u1"))}})
	mustDecideAllowed(t, decision)
	if !strings.Contains(decision.Rule, "fields [id, name]") || decision.Residuals != nil || decision.Writes == nil || decision.Writes[0].Terminal == nil || !reflect.DeepEqual(decision.Writes[0].Terminal.Fields, []string{"id", "name"}) {
		t.Errorf("fields-only decision = %+v", decision)
	}
}

func TestFieldsThroughStub(t *testing.T) {
	ctx := WithCurrentUser(context.Background(), "u1")
	stub := &stubReadwriteSession{rows: map[string]map[string]any{
		"u1": {"id": "u1", "name": "Ann", "email": "a@x", "passwordHash": "h1", "ownerID": "u1"},
		"u2": {"id": "u2", "name": "Bob", "email": "b@x", "passwordHash": "h2", "ownerID": "u2"},
	}}
	stub.load = nil
	// List users without passwordHash; see own row fully.
	policy := MustPolicy("users", Scope("users", AnyID,
		Allow(Get, "own-full").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))),
		Allow(Read, "public").Fields("id", "name"),
	), Collection("users", Allow(Query, "list").Fields("id", "name")))
	session := SecureReadwriteSession(stub, policy)
	key := func(id string) *record.Key { return record.NewKeyWithID("users", id) }
	own := map[string]any{}
	if err := session.Get(ctx, record.NewRecordWithData(key("u1"), &own)); err != nil || own["passwordHash"] != "h1" {
		t.Errorf("own row must be complete: err=%v data=%v", err, own)
	}
	other := map[string]any{}
	if err := session.Get(ctx, record.NewRecordWithData(key("u2"), &other)); err != nil || !reflect.DeepEqual(other, map[string]any{"id": "u2", "name": "Bob"}) {
		t.Errorf("other row must be redacted: err=%v data=%v", err, other)
	}
	batch := []record.Record{record.NewRecordWithData(key("u1"), &map[string]any{}), record.NewRecordWithData(key("u2"), &map[string]any{})}
	if err := session.GetMulti(ctx, batch); err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if second := *batch[1].Data().(*map[string]any); second["passwordHash"] != nil || second["name"] != "Bob" {
		t.Errorf("batch redaction: %v", second)
	}
	// Queries are projected and their records redacted.
	stub.queries = nil
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithData(record.NewKeyWithID("users", ""), &map[string]any{})
	})
	if _, err := session.ExecuteQueryToRecordsReader(ctx, query); err != nil {
		t.Fatalf("query: %v", err)
	}
	columns := stub.queries[0].(dal.StructuredQuery).Columns()
	if len(columns) != 2 || columns[0].Expression.(dal.FieldRef).Name() != "id" || columns[1].Expression.(dal.FieldRef).Name() != "name" {
		t.Errorf("projected columns = %v", columns)
	}
	// Caller columns are intersected with the allow-list.
	stub.queries = nil
	selected := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectColumns(dal.Column{Expression: dal.Field("name")}, dal.Column{Expression: dal.Field("passwordHash")})
	if _, err := session.ExecuteQueryToRecordsetReader(ctx, selected); err != nil {
		t.Fatalf("selected query: %v", err)
	}
	if columns := stub.queries[0].(dal.StructuredQuery).Columns(); len(columns) != 1 || columns[0].Expression.(dal.FieldRef).Name() != "name" {
		t.Errorf("intersected columns = %v", columns)
	}
	// A wildcard allow-list cannot be projected: records are redacted, recordsets refused.
	wild := SecureReadwriteSession(stub, MustPolicy("wild", Collection("users", Allow(Query, "list").Fields("public_*", "name"))))
	stub.queries = nil
	if _, err := wild.ExecuteQueryToRecordsReader(ctx, query); err != nil || len(stub.queries[0].(dal.StructuredQuery).Columns()) != 0 {
		t.Errorf("wildcard records query: err=%v columns=%v", err, stub.queries[0].(dal.StructuredQuery).Columns())
	}
	if _, err := wild.ExecuteQueryToRecordsetReader(ctx, query); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "cannot be projected") {
		t.Errorf("wildcard recordset query = %v", err)
	}
	// A non-field column selection cannot be projected either.
	computed := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectColumns(dal.Column{Expression: dal.Constant{Value: 1}, Alias: "one"})
	if _, err := session.ExecuteQueryToRecordsetReader(ctx, computed); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("computed column recordset = %v", err)
	}
	// Field rules on a joined source are refused.
	joined := MustPolicy("join", Collection("orders", Allow(Query, "all")), Collection("users", Allow(Query, "list").Fields("id")))
	joinQuery := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", "o")).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("users", "u"), dal.JoinInner))).SelectKeysOnly(reflect.String)
	if _, err := SecureReadwriteSession(stub, joined).ExecuteQueryToRecordsReader(ctx, joinQuery); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "joined source") {
		t.Errorf("joined fields = %v", err)
	}
	// Field rules never authorise an opaque query, so a redacting path with a
	// non-structured query is unreachable through policies; the guard itself is covered by rewriteQuery.
	// Adapter errors propagate through the redacting path.
	stub.queryErr = errors.New("boom")
	if _, err := session.ExecuteQueryToRecordsReader(ctx, query); err == nil || err.Error() != "boom" {
		t.Errorf("adapter error = %v", err)
	}
	stub.queryErr = nil

	// Writes: only allowed fields may be set or touched.
	writer := SecureReadwriteSession(stub, MustPolicy("edit", Scope("users", AnyID, Allow(Write, "profile").Fields("name", "phones.*"))))
	if err := writer.Update(ctx, key("u1"), []update.Update{update.ByFieldName("name", "Ann B"), update.ByFieldPath(update.FieldPath{"phones", "home"}, "1")}); err != nil {
		t.Errorf("allowed update: %v", err)
	}
	if err := writer.Update(ctx, key("u1"), []update.Update{update.ByFieldName("ownerID", "u2")}); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), `"ownerID"`) {
		t.Errorf("update outside fields = %v", err)
	}
	if err := writer.Insert(ctx, record.NewRecordWithData(key("u3"), map[string]any{"name": "Cy", "email": "c@x"})); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), `"email"`) {
		t.Errorf("insert outside fields = %v", err)
	}
	if err := writer.Set(ctx, record.NewRecordWithData(key("u1"), map[string]any{"name": "Ann"})); err != nil {
		t.Errorf("allowed set: %v", err)
	}
	// A conditional alternative with fields checks them too; the terminal's fields apply otherwise.
	mixed := SecureReadwriteSession(stub, MustPolicy("mixed", Root(Allow(Write, "any").Fields("name")), Scope("users", AnyID,
		Allow(Write, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("name", "email", "ownerID"))))
	if err := mixed.Update(ctx, key("u1"), []update.Update{update.ByFieldName("email", "new@x")}); err != nil {
		t.Errorf("own alternative allows email: %v", err)
	}
	if err := mixed.Update(ctx, key("u2"), []update.Update{update.ByFieldName("email", "new@x")}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("terminal fields must refuse email on another's row: %v", err)
	}
	if err := mixed.Insert(ctx, record.NewRecordWithData(key("u9"), map[string]any{"ownerID": "u1", "email": "e"})); err != nil {
		t.Errorf("new own row via alternative check: %v", err)
	}
	if err := mixed.Insert(ctx, record.NewRecordWithData(key("u9"), map[string]any{"ownerID": "u2", "email": "e"})); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("new other's row falls to terminal fields: %v", err)
	}
}

func TestFieldDocuments(t *testing.T) {
	source := `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: users
default: deny
scopes:
  - path: /users/*
    rules:
      - id: public
        effect: allow
        operations: [read]
        fields: [id, name, address.*]
`
	policy, err := UnmarshalAccessPolicyYAML([]byte(source))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	decision := policy.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.NewKeyWithID("users", "u1"))}})
	mustDecideAllowed(t, decision)
	if !reflect.DeepEqual(decision.Writes[0].Terminal.Fields, []string{"id", "name", "address.*"}) {
		t.Errorf("decoded fields = %v", decision.Writes[0].Terminal.Fields)
	}
	for name, marshal := range map[string]func(*AccessPolicy) ([]byte, error){"yaml": MarshalAccessPolicyYAML, "json": MarshalAccessPolicyJSON} {
		encoded, err := marshal(policy)
		if err != nil || !strings.Contains(string(encoded), "address.*") {
			t.Errorf("%s: err=%v encoded=%s", name, err, encoded)
		}
	}
	bad := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny",
		Scopes: []DocumentScope{{Path: "/x/*", Rules: []DocumentRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Fields: []string{"a..b"}}}}}}
	if _, err := DecodeAccessPolicy(strings.NewReader(""), staticCodec{d: bad}); err == nil {
		t.Error("invalid fields pattern must fail")
	}
}

func TestFieldsOnMemoryDB(t *testing.T) {
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	type user struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		PasswordHash string `json:"passwordHash"`
	}
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for id, u := range map[string]user{"u1": {"Ann", "a@x", "h1"}, "u2": {"Bob", "b@x", "h2"}} {
			u := u
			if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", id), &u)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db := MustSecureDB(raw, WithDatabasePolicies(MustPolicy("users",
		Scope("users", AnyID, Allow(Get, "profile").Fields("name", "email")),
		Collection("users", Allow(Query, "list").Fields("name")),
	)))
	got := &user{}
	if err := db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "u1"), got)); err != nil || got.PasswordHash != "" || got.Name != "Ann" || got.Email != "a@x" {
		t.Errorf("get: err=%v data=%+v", err, *got)
	}
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithData(record.NewKeyWithID("users", ""), &user{})
	})
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, db)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("rows = %d", len(records))
	}
	for _, rec := range records {
		switch data := rec.Data().(type) {
		case *user:
			if data.PasswordHash != "" || data.Email != "" || data.Name == "" {
				t.Errorf("row not limited to name: %+v", *data)
			}
		case map[string]any:
			if _, leaked := data["passwordHash"]; leaked || data["name"] == nil {
				t.Errorf("projected row not limited to name: %v", data)
			}
		default:
			t.Errorf("unexpected row data %T", data)
		}
	}
}

type fakeRecordsReader struct {
	dal.RecordsReader
	records []record.Record
}

func (r *fakeRecordsReader) Next() (record.Record, error) {
	if len(r.records) == 0 {
		return nil, dal.ErrNoMoreRecords
	}
	rec := r.records[0]
	r.records = r.records[1:]
	return rec, nil
}

func TestFieldEdgeBranches(t *testing.T) {
	ctx := WithCurrentUser(context.Background(), "u1")
	set, _ := parseFieldPatterns([]string{"name"})
	key := record.NewKeyWithID("users", "u1")
	// The redacting reader surfaces redaction errors and end-of-stream.
	odd := record.NewRecordWithData(key, map[int]any{1: "x"})
	odd.SetError(nil)
	fine := record.NewRecordWithData(key, map[string]any{"name": "Ann", "email": "e"})
	fine.SetError(nil)
	reader := redactingReader{RecordsReader: &fakeRecordsReader{records: []record.Record{fine, odd}}, sets: fieldSets{set}}
	if rec, err := reader.Next(); err != nil || len(rec.Data().(map[string]any)) != 1 {
		t.Errorf("redacted record: err=%v data=%v", err, rec)
	}
	if _, err := reader.Next(); err == nil {
		t.Error("unredactable record must fail")
	}
	if _, err := reader.Next(); !errors.Is(err, dal.ErrNoMoreRecords) {
		t.Errorf("end of stream = %v", err)
	}
	// projectQuery leaves unrestricted queries alone.
	bare := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectKeysOnly(reflect.String)
	if q, ok := projectQuery(bare, fieldSets{nil}); !ok || len(q.Columns()) != 0 {
		t.Errorf("unrestricted projection = %v, %v", q, ok)
	}
	// decidingFields reports an unevaluable alternative.
	bad := writeResidual{policy: "p", resource: RecordResourceForKey(key), residual: &WriteResidual{Alternatives: []WriteAlternative{{Rule: "bad", Where: fakeCond{}}}}}
	if _, err := decidingFields(Get, map[string]any{}, []writeResidual{bad}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("unevaluable alternative = %v", err)
	}
	loaded := record.NewRecordWithData(key, map[string]any{"name": "Ann"})
	loaded.SetError(nil)
	if err := enforceRead(Get, loaded, nil, []writeResidual{bad}); !errors.Is(err, ErrAccessDenied) || !errors.Is(loaded.Error(), ErrAccessDenied) {
		t.Errorf("unevaluable alternative on read = %v (record %v)", err, loaded.Error())
	}
	// A conditional alternative with fields bounds a query too.
	stub := &stubReadwriteSession{rows: map[string]map[string]any{}}
	session := SecureReadwriteSession(stub, MustPolicy("q", Collection("users", Allow(Query, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("id"))))
	query := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("users", ""))).SelectKeysOnly(reflect.String)
	if _, err := session.ExecuteQueryToRecordsReader(ctx, query); err != nil {
		t.Fatalf("conditional fields query: %v", err)
	}
	if columns := stub.queries[0].(dal.StructuredQuery).Columns(); len(columns) != 1 {
		t.Errorf("conditional alternative fields must project: %v", columns)
	}
	// A joined source under a conditional rule is refused by the rewrite.
	joined := MustPolicy("join", Collection("orders", Allow(Query, "all")), Collection("users", Allow(Query, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser")))))
	joinQuery := dal.NewQueryBuilder(dal.From(dal.NewRootCollectionRef("orders", "o")).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("users", "u"), dal.JoinInner))).SelectKeysOnly(reflect.String)
	if _, err := SecureReadwriteSession(stub, joined).ExecuteQueryToRecordsReader(ctx, joinQuery); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "conditional rules on joins") {
		t.Errorf("joined conditional = %v", err)
	}
	// Point reads: data that cannot be evaluated is refused under row rules and under field rules alike;
	// data that can be evaluated but not redacted is refused too.
	stubs := &stubReadSession{exists: true}
	rowRule := SecureReadSession(stubs, MustPolicy("rows", Scope("users", AnyID, Allow(Get, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))))))
	if err := rowRule.Get(ctx, record.NewRecordWithData(key, &struct{ C chan int }{C: make(chan int)})); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "could not be evaluated") {
		t.Errorf("unserialisable under row rule = %v", err)
	}
	fieldRule := SecureReadSession(stubs, MustPolicy("fields", Scope("users", AnyID, Allow(Get, "public").Fields("name"))))
	if err := fieldRule.Get(ctx, record.NewRecordWithData(key, &struct{ C chan int }{C: make(chan int)})); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "fields could not be evaluated") {
		t.Errorf("unserialisable under field rule = %v", err)
	}
	if err := fieldRule.Get(ctx, record.NewRecordWithData(key, map[int]any{1: "x"})); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "could not be redacted") {
		t.Errorf("unredactable under field rule = %v", err)
	}
	// A batch mixing an unconditional record whose data cannot be evaluated with a conditional one.
	mixed := SecureReadwriteSession(&stubReadwriteSession{rows: map[string]map[string]any{"u1": {"ownerID": "u1"}, "shared": {"x": 1}}},
		MustPolicy("mixed", Scope("users", "shared", Allow(Get, "shared")), Scope("users", AnyID, Allow(Get, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))))))
	if err := mixed.GetMulti(ctx, []record.Record{record.NewRecordWithData(record.NewKeyWithID("users", "shared"), &struct{ C chan int }{C: make(chan int)}), record.NewRecordWithData(key, &map[string]any{})}); err != nil {
		t.Errorf("unconditional record needs no evaluation: %v", err)
	}
	// A conditional alternative refuses a field outside its list on update.
	own := SecureReadwriteSession(&stubReadwriteSession{rows: map[string]map[string]any{"u1": {"ownerID": "u1"}}},
		MustPolicy("own", Scope("users", AnyID, Allow(Write, "own").Where(dal.WhereField("ownerID", dal.Equal, dal.NewParam("currentUser"))).Fields("name", "ownerID"))))
	if err := own.Update(ctx, key, []update.Update{update.ByFieldName("passwordHash", "x")}); !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), `"passwordHash"`) {
		t.Errorf("alternative field refusal = %v", err)
	}
}
