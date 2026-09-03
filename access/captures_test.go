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

func spaceOfPath() dal.Condition {
	return dal.WhereField("spaceID", dal.Equal, dal.NewParam("path.spaceID"))
}

func trackusKeyIn(space string) *record.Key {
	return record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", space), "ext", "trackus")
}

func itemKeyIn(space, id string) *record.Key {
	return record.NewKeyWithParentAndID(trackusKeyIn(space), "items", id)
}

func TestCapturePatterns(t *testing.T) {
	pattern := Path("spaces", Capture("spaceID"), "ext", "trackus", "items", AnyID)
	if got := pattern.String(); got != "/spaces/{spaceID}/ext/trackus/items/*" {
		t.Errorf("String() = %q", got)
	}
	if got := pattern.captures(); !reflect.DeepEqual(got, []string{"spaceID"}) {
		t.Errorf("captures() = %v", got)
	}
	for name, parts := range map[string][]any{
		"invalid name":   {"spaces", Capture("1x")},
		"dotted name":    {"spaces", Capture("a.b")},
		"duplicate name": {"spaces", Capture("id"), "ext", Capture("id")},
	} {
		if _, err := NewPath(parts...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// A record and a collection resource both bind the capture.
	item := RecordResourceForKey(itemKeyIn("s1", "i1"))
	if !patternsMatch(pattern, item) {
		t.Fatalf("pattern must match %s", item)
	}
	if got := captureValues(pattern, item); !reflect.DeepEqual(got, map[string]any{"path.spaceID": "s1"}) {
		t.Errorf("captureValues(record) = %v", got)
	}
	shorter := CollectionResourceFor(nil, "spaces")
	if got := captureValues(pattern, shorter); got != nil {
		t.Errorf("captureValues(too short) = %v, want nil", got)
	}
	if got := captureValues(Path("spaces", AnyID), item); got != nil {
		t.Errorf("captureValues(no captures) = %v, want nil", got)
	}
}

func TestCaptureDocuments(t *testing.T) {
	policy := MustPolicy("spaces", Under(Path("spaces", Capture("spaceID"), "ext", "trackus"),
		Allow(Read, "own-space").Where(spaceOfPath())))
	encoded, err := MarshalAccessPolicyYAML(policy)
	if err != nil || !strings.Contains(string(encoded), "path: /spaces/{spaceID}/ext/trackus") || !strings.Contains(string(encoded), "param: path.spaceID") {
		t.Fatalf("encode: err=%v\n%s", err, encoded)
	}
	again, err := UnmarshalAccessPolicyYAML(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if again.compiled[0].pattern.String() != "/spaces/{spaceID}/ext/trackus" {
		t.Errorf("decoded pattern = %q", again.compiled[0].pattern)
	}
	for name, path := range map[string]string{
		"capture as collection": "/{spaces}/*",
		"bad capture name":      "/spaces/{1x}",
	} {
		document := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny",
			Scopes: []DocumentScope{{Path: path, Rules: []DocumentRule{{ID: "r", Effect: "allow", Operations: []string{"get"}}}}}}
		if _, err := DecodeAccessPolicy(strings.NewReader(""), staticCodec{d: document}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCaptureCompilation(t *testing.T) {
	if _, err := NewPolicy("p", Scope("spaces", AnyID, Allow(Get, "no-capture").Where(spaceOfPath()))); err == nil || !strings.Contains(err.Error(), "captures no such segment") {
		t.Errorf("unknown capture must fail compilation: %v", err)
	}
	if _, err := NewPolicy("p", Scope("spaces", Capture("spaceID"), Allow(Get, "ok").Where(spaceOfPath()))); err != nil {
		t.Errorf("declared capture must compile: %v", err)
	}
	// A capture declared by a parent scope is visible to nested scopes.
	if _, err := NewPolicy("p", Scope("spaces", Capture("spaceID"), Scope("ext", "trackus", Allow(Get, "nested").Where(spaceOfPath())))); err != nil {
		t.Errorf("parent capture must be visible: %v", err)
	}
}

func TestCaptureDecisions(t *testing.T) {
	policy := MustPolicy("spaces", Under(Path("spaces", Capture("spaceID"), "ext", "trackus"),
		Allow(Read|Write, "own-space").Where(spaceOfPath())))
	ctx := context.Background()
	item := RecordResourceForKey(itemKeyIn("s1", "i1"))
	decision := policy.Decide(ctx, Request{Operation: Get, Resources: []Resource{item}})
	mustDecideAllowed(t, decision)
	if got := decision.Residuals[0].String(); got != "spaceID = 's1'" {
		t.Errorf("residual = %q", got)
	}
	if decision.Condition != "spaceID = $path.spaceID" || strings.Contains(decision.Explanation, "s1") {
		t.Errorf("decision leaks or mislabels: %+v", decision)
	}
	collection := CollectionResourceFor(trackusKeyIn("s2"), "items")
	decision = policy.Decide(ctx, Request{Operation: Query, Resources: []Resource{collection}})
	mustDecideAllowed(t, decision)
	if got := decision.Residuals[0].String(); got != "spaceID = 's2'" {
		t.Errorf("collection residual = %q", got)
	}
	if got := decision.Writes[0].Alternatives[0].Where.String(); got != "spaceID = 's2'" {
		t.Errorf("write alternative = %q", got)
	}
	// A capture also feeds a terminal allow's check.
	checked := MustPolicy("checked", Under(Path("spaces", Capture("spaceID")), Allow(Write, "stay-in-space").Check(spaceOfPath())))
	decision = checked.Decide(ctx, Request{Operation: Set, Resources: []Resource{item}})
	mustDecideAllowed(t, decision)
	if got := decision.Writes[0].Terminal.Check.String(); got != "spaceID = 's1'" {
		t.Errorf("terminal check = %q", got)
	}
	// A capture takes precedence over a same-named variable.
	decision = policy.Decide(WithVariables(ctx, map[string]any{"path.spaceID": "other"}), Request{Operation: Get, Resources: []Resource{item}})
	mustDecideAllowed(t, decision)
	if got := decision.Residuals[0].String(); got != "spaceID = 's1'" {
		t.Errorf("capture precedence: %q", got)
	}
}

func TestCapturesOnMemoryDB(t *testing.T) {
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	itemKey := itemKeyIn
	type item struct {
		SpaceID string `json:"spaceID"`
		Title   string `json:"title"`
	}
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for _, seed := range []struct {
			space, id string
			data      item
		}{
			{"s1", "i1", item{SpaceID: "s1", Title: "own"}},
			{"s1", "i2", item{SpaceID: "s2", Title: "misfiled"}},
			{"s2", "i3", item{SpaceID: "s2", Title: "other"}},
		} {
			seed := seed
			if err := tx.Set(ctx, record.NewRecordWithData(itemKey(seed.space, seed.id), &seed.data)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db := MustSecureDB(raw, WithDatabasePolicies(MustPolicy("spaces", Under(Path("spaces", Capture("spaceID"), "ext", "trackus"),
		Allow(Read|Write, "own-space").Where(spaceOfPath())))))

	own := &item{}
	if err := db.Get(ctx, record.NewRecordWithData(itemKey("s1", "i1"), own)); err != nil || own.Title != "own" {
		t.Errorf("own item: err=%v data=%+v", err, *own)
	}
	misfiled := &item{}
	if err := db.Get(ctx, record.NewRecordWithData(itemKey("s1", "i2"), misfiled)); !errors.Is(err, ErrAccessDenied) || *misfiled != (item{}) {
		t.Errorf("misfiled item must be denied: err=%v data=%+v", err, *misfiled)
	}
	// A query under space s1 returns only rows whose spaceID is s1.
	query := dal.NewQueryBuilder(dal.From(dal.NewCollectionRef("items", "", trackusKeyIn("s1")))).
		SelectIntoRecord(func() record.Record { return record.NewRecordWithData(itemKey("s1", ""), &item{}) })
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, db)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(records) != 1 || records[0].Data().(*item).Title != "own" {
		t.Errorf("query rows = %d", len(records))
	}
	// Writes: an update that moves the row out of its space is refused.
	if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, itemKey("s1", "i1"), []update.Update{update.ByFieldName("spaceID", "s2")})
	}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("moving out of the space = %v", err)
	}
	if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, itemKey("s1", "i1"), []update.Update{update.ByFieldName("title", "renamed")})
	}); err != nil {
		t.Errorf("edit within the space: %v", err)
	}
}
