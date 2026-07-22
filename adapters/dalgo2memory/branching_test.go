package dalgo2memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/branchingtest"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type branchingRecord struct {
	Title string         `json:"title"`
	Done  bool           `json:"done"`
	Meta  map[string]any `json:"meta,omitempty"`
}

func TestBranchingConformanceSerialized(t *testing.T) {
	branchingtest.RunConformance(t, branchingtest.Callbacks{
		New: func(testing.TB) (dal.DB, branching.Provider) {
			return NewDB(), NewBranchingProvider()
		},
		Seed:   seedBranchingRecords,
		Mutate: mutateBranchingRecords,
		Digest: digestBranchingRecords,
	})
}

func seedBranchingRecords(ctx context.Context, db dal.DB) error {
	writer := db.(branchingWriter)
	for _, fixture := range []struct {
		key  *record.Key
		data *branchingRecord
	}{
		{branchingRootKey("milk"), &branchingRecord{Title: "milk", Meta: map[string]any{"version": 1}}},
		{branchingRootKey("bread"), &branchingRecord{Title: "bread"}},
		{branchingNestedKey("family", "groceries", "apples"), &branchingRecord{Title: "apples"}},
	} {
		if err := writer.Set(ctx, record.NewRecordWithData(fixture.key, fixture.data)); err != nil {
			return err
		}
	}
	return nil
}

func mutateBranchingRecords(ctx context.Context, db dal.DB) error {
	writer := db.(branchingWriter)
	if err := writer.Insert(ctx, record.NewRecordWithData(branchingRootKey("bananas"), &branchingRecord{Title: "bananas"})); err != nil {
		return err
	}
	if err := writer.Update(ctx, branchingRootKey("milk"), []update.Update{
		update.ByFieldName("done", true),
		update.ByFieldPath(update.FieldPath{"meta", "version"}, 2),
	}); err != nil {
		return err
	}
	return writer.Delete(ctx, branchingRootKey("bread"))
}

type branchingWriter interface {
	Set(context.Context, record.Record) error
	Insert(context.Context, record.Record, ...dal.InsertOption) error
	Update(context.Context, *record.Key, []update.Update, ...dal.Precondition) error
	Delete(context.Context, *record.Key) error
}

func digestBranchingRecords(ctx context.Context, db dal.DB) (string, error) {
	keys := []*record.Key{
		branchingRootKey("bananas"),
		branchingRootKey("bread"),
		branchingRootKey("milk"),
		branchingNestedKey("family", "groceries", "apples"),
	}
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		var data map[string]any
		err := db.Get(ctx, record.NewRecordWithData(key, &data))
		if record.IsNotFound(err) {
			entries = append(entries, keyPath(key)+"=<missing>")
			continue
		}
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		entries = append(entries, keyPath(key)+"="+string(encoded))
	}
	return fmt.Sprint(entries), nil
}

func branchingRootKey(id string) *record.Key {
	return record.NewKeyWithID("items", id)
}

func branchingNestedKey(spaceID, listID, itemID string) *record.Key {
	space := record.NewKeyWithID("spaces", spaceID)
	list := record.NewKeyWithParentAndID(space, "lists", listID)
	return record.NewKeyWithParentAndID(list, "items", itemID)
}

func TestBranchingPreservesNestedKeyChains(t *testing.T) {
	ctx := context.Background()
	source := NewDB().(*database)
	sourceKey := branchingNestedKey("family", "groceries", "apples")
	wantPath := sourceKey.String()
	if err := source.Set(ctx, record.NewRecordWithData(sourceKey, &branchingRecord{Title: "apples"})); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := NewBranchingProvider().Capture(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := checkpoint.Release(ctx); err != nil {
			t.Error(err)
		}
	}()

	// serializedEngine stores the caller's key pointer. Mutating it after
	// Capture must not alter the immutable checkpoint's copied parent chain.
	sourceKey.ID = "changed-item"
	sourceKey.Parent().ID = "changed-list"
	sourceKey.Parent().Parent().ID = "changed-space"

	first := mustBranch(t, checkpoint)
	defer closeTestBranch(t, first)
	second := mustBranch(t, checkpoint)
	defer closeTestBranch(t, second)

	firstKey := onlyQueriedItemKey(t, first.DB())
	secondKey := onlyQueriedItemKey(t, second.DB())
	if got := firstKey.String(); got != wantPath {
		t.Fatalf("first branch key = %q, want %q", got, wantPath)
	}
	if got := secondKey.String(); got != wantPath {
		t.Fatalf("second branch key = %q, want %q", got, wantPath)
	}
	for left, right := firstKey, secondKey; left != nil || right != nil; left, right = left.Parent(), right.Parent() {
		if left == right {
			t.Fatal("sibling branches share a record-key node")
		}
	}
}

func onlyQueriedItemKey(t testing.TB, db dal.DB) *record.Key {
	t.Helper()
	query := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().SelectKeysOnly(reflect.String)
	records, err := dal.ExecuteQueryAndReadAllToRecords(context.Background(), query, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("query returned %d records, want 1", len(records))
	}
	return records[0].Key()
}

func TestBranchingDeepCopiesSerializedBytes(t *testing.T) {
	ctx := context.Background()
	source := NewDB().(*database)
	key := branchingRootKey("milk")
	if err := source.Set(ctx, record.NewRecordWithData(key, &branchingRecord{Title: "milk"})); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := NewBranchingProvider().Capture(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = checkpoint.Release(ctx) }()

	storedID := keyID(key)
	sourceBytes := source.collections["items"].(*serializedEngine).records[storedID]
	sourceBytes[0] = '['
	assertBranchingTitle(t, mustBranch(t, checkpoint), key, "milk")

	first := mustBranch(t, checkpoint)
	firstDB := first.DB().(*database)
	firstDB.collections["items"].(*serializedEngine).records[storedID][0] = '['
	closeTestBranch(t, first)
	assertBranchingTitle(t, mustBranch(t, checkpoint), key, "milk")
}

func assertBranchingTitle(t testing.TB, branch branching.Branch, key *record.Key, want string) {
	t.Helper()
	defer closeTestBranch(t, branch)
	var got branchingRecord
	if err := branch.DB().Get(context.Background(), record.NewRecordWithData(key, &got)); err != nil {
		t.Fatal(err)
	}
	if got.Title != want {
		t.Fatalf("title = %q, want %q", got.Title, want)
	}
}

func TestBranchingRejectsColumnarEngine(t *testing.T) {
	tests := []struct {
		name       string
		collection CollectionOption
		initialize bool
		wantMode   string
	}{
		{name: "configured columnar", collection: WithColumnarStorage(), wantMode: "columnar"},
		{name: "initialized columnar", collection: WithColumnarStorage(), initialize: true, wantMode: "columnar"},
		{name: "configured custom", collection: withBranchingCustomStorage(), wantMode: "custom"},
		{name: "configured custom returning serialized", collection: withBranchingCustomSerializedStorage(), wantMode: "custom"},
		{name: "initialized custom", collection: withBranchingCustomStorage(), initialize: true, wantMode: "custom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := NewDB(WithSchema(false,
				WithCollection[branchingRecord]("items", nil, tc.collection),
			)).(*database)
			if tc.initialize {
				if err := db.Set(context.Background(), record.NewRecordWithData(branchingRootKey("milk"), &branchingRecord{Title: "milk"})); err != nil {
					t.Fatal(err)
				}
			}
			checkpoint, err := NewBranchingProvider().Capture(context.Background(), db)
			if checkpoint != nil {
				t.Fatal("unsupported engine published a checkpoint")
			}
			var unsupported *branching.UnsupportedError
			if !errors.As(err, &unsupported) {
				t.Fatalf("Capture() error = %v, want *branching.UnsupportedError", err)
			}
			if !errors.Is(err, branching.ErrUnsupportedCapability) {
				t.Fatalf("Capture() error = %v, want ErrUnsupportedCapability", err)
			}
			if unsupported.Mode != tc.wantMode {
				t.Fatalf("unsupported mode = %q, want %q", unsupported.Mode, tc.wantMode)
			}
		})
	}
}

type branchingCustomEngine struct {
	inner *serializedEngine
}

func withBranchingCustomStorage() CollectionOption {
	return func(def *collectionDef) {
		def.newEngine = func(collection string, factory func() any, _ bool) storageEngine {
			return &branchingCustomEngine{inner: newSerializedEngine(collection, factory)}
		}
	}
}

func withBranchingCustomSerializedStorage() CollectionOption {
	return func(def *collectionDef) {
		def.newEngine = func(collection string, factory func() any, _ bool) storageEngine {
			return newSerializedEngine(collection, factory)
		}
	}
}

func (e *branchingCustomEngine) exists(id string) bool { return e.inner.exists(id) }
func (e *branchingCustomEngine) store(id string, rec record.Record, overwrite bool) error {
	return e.inner.store(id, rec, overwrite)
}
func (e *branchingCustomEngine) load(id string, rec record.Record) error {
	return e.inner.load(id, rec)
}
func (e *branchingCustomEngine) delete(id string) { e.inner.delete(id) }
func (e *branchingCustomEngine) update(id string, updates []update.Update) error {
	return e.inner.update(id, updates)
}
func (e *branchingCustomEngine) rows() ([]engineRow, error) { return e.inner.rows() }

func TestTwoMemoryDatabasesConformAsOneGroup(t *testing.T) {
	t.Run("fresh handles and sibling isolation", func(t *testing.T) {
		ctx := context.Background()
		primary := NewDB()
		audit := NewDB()
		mustSetBranchingTitle(t, primary, "primary", "baseline-primary")
		mustSetBranchingTitle(t, audit, "audit", "baseline-audit")

		group := testMemoryGroup{
			{name: "primary", source: primary, provider: NewBranchingProvider()},
			{name: "audit", source: audit, provider: NewBranchingProvider()},
		}
		checkpoint, err := group.Capture(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := checkpoint.Release(ctx); err != nil {
				t.Error(err)
			}
		}()

		mustSetBranchingTitle(t, primary, "primary", "changed-source-primary")
		mustSetBranchingTitle(t, audit, "audit", "changed-source-audit")

		first, err := checkpoint.Branch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertGroupFresh(t, first, map[string]dal.DB{"primary": primary, "audit": audit})
		assertBranchingDBTitle(t, first.DB("primary"), "primary", "baseline-primary")
		assertBranchingDBTitle(t, first.DB("audit"), "audit", "baseline-audit")
		mustSetBranchingTitle(t, first.DB("primary"), "primary", "changed-first-primary")
		mustSetBranchingTitle(t, first.DB("audit"), "audit", "changed-first-audit")
		if err := first.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err := first.Close(ctx); err != nil {
			t.Fatalf("second Close(): %v", err)
		}

		second, err := checkpoint.Branch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = second.Close(ctx) }()
		assertGroupFresh(t, second, map[string]dal.DB{
			"primary": primary,
			"audit":   audit,
		})
		if second.DB("primary") == first.DB("primary") || second.DB("audit") == first.DB("audit") {
			t.Fatal("sibling groups share a database handle")
		}
		assertBranchingDBTitle(t, second.DB("primary"), "primary", "baseline-primary")
		assertBranchingDBTitle(t, second.DB("audit"), "audit", "baseline-audit")
	})

	t.Run("checkpoint failure releases partial captures", func(t *testing.T) {
		firstProvider := &trackingProvider{inner: NewBranchingProvider()}
		group := testMemoryGroup{
			{name: "primary", source: NewDB(), provider: firstProvider},
			{name: "audit", source: NewDB(), provider: failingProvider{err: errTestCapture}},
		}
		checkpoint, err := group.Capture(context.Background())
		if !errors.Is(err, errTestCapture) || checkpoint != nil {
			t.Fatalf("Capture() = (%v, %v), want (nil, errTestCapture)", checkpoint, err)
		}
		if firstProvider.checkpoint == nil || firstProvider.checkpoint.releases != 1 {
			t.Fatalf("partial checkpoint releases = %v, want 1", firstProvider.releaseCount())
		}
	})

	t.Run("branch failure closes partial branches", func(t *testing.T) {
		firstProvider := &trackingProvider{inner: NewBranchingProvider()}
		secondProvider := branchFailingProvider{inner: NewBranchingProvider(), err: errTestBranch}
		group := testMemoryGroup{
			{name: "primary", source: NewDB(), provider: firstProvider},
			{name: "audit", source: NewDB(), provider: secondProvider},
		}
		checkpoint, err := group.Capture(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = checkpoint.Release(context.Background()) }()
		branch, err := checkpoint.Branch(context.Background())
		if !errors.Is(err, errTestBranch) || branch != nil {
			t.Fatalf("Branch() = (%v, %v), want (nil, errTestBranch)", branch, err)
		}
		if firstProvider.checkpoint == nil || firstProvider.checkpoint.lastBranch == nil || firstProvider.checkpoint.lastBranch.closes != 1 {
			t.Fatalf("partial branch closes = %v, want 1", firstProvider.closeCount())
		}
	})
}

func assertGroupFresh(t testing.TB, branch *testMemoryGroupBranch, sources map[string]dal.DB) {
	t.Helper()
	for name, source := range sources {
		if branch.DB(name) == nil {
			t.Fatalf("group branch database %q is nil", name)
		}
		if branch.DB(name) == source {
			t.Fatalf("group branch reused source database %q", name)
		}
	}
}

func mustSetBranchingTitle(t testing.TB, db dal.DB, id, title string) {
	t.Helper()
	if err := db.(branchingWriter).Set(context.Background(), record.NewRecordWithData(branchingRootKey(id), &branchingRecord{Title: title})); err != nil {
		t.Fatal(err)
	}
}

func assertBranchingDBTitle(t testing.TB, db dal.DB, id, want string) {
	t.Helper()
	var got branchingRecord
	if err := db.Get(context.Background(), record.NewRecordWithData(branchingRootKey(id), &got)); err != nil {
		t.Fatal(err)
	}
	if got.Title != want {
		t.Fatalf("record %q title = %q, want %q", id, got.Title, want)
	}
}

type testMemoryGroupMember struct {
	name     string
	source   dal.DB
	provider branching.Provider
}

type testMemoryGroup []testMemoryGroupMember

func (g testMemoryGroup) Capture(ctx context.Context) (*testMemoryGroupCheckpoint, error) {
	members := append(testMemoryGroup(nil), g...)
	captured := make([]testMemoryGroupCheckpointMember, 0, len(members))
	for _, member := range members {
		checkpoint, err := member.provider.Capture(ctx, member.source)
		if err != nil {
			for i := len(captured) - 1; i >= 0; i-- {
				_ = captured[i].checkpoint.Release(context.Background())
			}
			return nil, err
		}
		captured = append(captured, testMemoryGroupCheckpointMember{name: member.name, checkpoint: checkpoint})
	}
	return &testMemoryGroupCheckpoint{members: captured}, nil
}

type testMemoryGroupCheckpointMember struct {
	name       string
	checkpoint branching.Checkpoint
}

type testMemoryGroupCheckpoint struct {
	members  []testMemoryGroupCheckpointMember
	released bool
}

func (c *testMemoryGroupCheckpoint) Branch(ctx context.Context) (*testMemoryGroupBranch, error) {
	if c.released {
		return nil, branching.ErrReleased
	}
	started := make([]testMemoryGroupBranchMember, 0, len(c.members))
	for _, member := range c.members {
		branch, err := member.checkpoint.Branch(ctx)
		if err != nil {
			for i := len(started) - 1; i >= 0; i-- {
				_ = started[i].branch.Close(context.Background())
			}
			return nil, err
		}
		started = append(started, testMemoryGroupBranchMember{name: member.name, branch: branch})
	}
	return &testMemoryGroupBranch{members: started}, nil
}

func (c *testMemoryGroupCheckpoint) Release(ctx context.Context) error {
	if c.released {
		return nil
	}
	c.released = true
	var errs []error
	for i := len(c.members) - 1; i >= 0; i-- {
		if err := c.members[i].checkpoint.Release(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type testMemoryGroupBranchMember struct {
	name   string
	branch branching.Branch
}

type testMemoryGroupBranch struct {
	members []testMemoryGroupBranchMember
	closed  bool
}

func (b *testMemoryGroupBranch) DB(name string) dal.DB {
	for _, member := range b.members {
		if member.name == name {
			return member.branch.DB()
		}
	}
	return nil
}

func (b *testMemoryGroupBranch) Close(ctx context.Context) error {
	if b.closed {
		return nil
	}
	b.closed = true
	var errs []error
	for i := len(b.members) - 1; i >= 0; i-- {
		if err := b.members[i].branch.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var (
	errTestCapture = errors.New("test capture failure")
	errTestBranch  = errors.New("test branch failure")
)

type failingProvider struct {
	err error
}

func (f failingProvider) Capability() branching.Capability { return branching.Capability{} }
func (f failingProvider) Capture(context.Context, dal.DB) (branching.Checkpoint, error) {
	return nil, f.err
}

type branchFailingProvider struct {
	inner branching.Provider
	err   error
}

func (p branchFailingProvider) Capability() branching.Capability { return p.inner.Capability() }
func (p branchFailingProvider) Capture(ctx context.Context, db dal.DB) (branching.Checkpoint, error) {
	checkpoint, err := p.inner.Capture(ctx, db)
	if err != nil {
		return nil, err
	}
	return branchFailingCheckpoint{Checkpoint: checkpoint, err: p.err}, nil
}

type branchFailingCheckpoint struct {
	branching.Checkpoint
	err error
}

func (c branchFailingCheckpoint) Branch(context.Context) (branching.Branch, error) {
	return nil, c.err
}

type trackingProvider struct {
	inner      branching.Provider
	checkpoint *trackingCheckpoint
}

func (p *trackingProvider) Capability() branching.Capability { return p.inner.Capability() }
func (p *trackingProvider) Capture(ctx context.Context, db dal.DB) (branching.Checkpoint, error) {
	checkpoint, err := p.inner.Capture(ctx, db)
	if err != nil {
		return nil, err
	}
	p.checkpoint = &trackingCheckpoint{Checkpoint: checkpoint}
	return p.checkpoint, nil
}

func (p *trackingProvider) releaseCount() int {
	if p.checkpoint == nil {
		return 0
	}
	return p.checkpoint.releases
}

func (p *trackingProvider) closeCount() int {
	if p.checkpoint == nil || p.checkpoint.lastBranch == nil {
		return 0
	}
	return p.checkpoint.lastBranch.closes
}

type trackingCheckpoint struct {
	branching.Checkpoint
	releases   int
	lastBranch *trackingBranch
}

func (c *trackingCheckpoint) Branch(ctx context.Context) (branching.Branch, error) {
	branch, err := c.Checkpoint.Branch(ctx)
	if err != nil {
		return nil, err
	}
	c.lastBranch = &trackingBranch{Branch: branch}
	return c.lastBranch, nil
}

func (c *trackingCheckpoint) Release(ctx context.Context) error {
	c.releases++
	return c.Checkpoint.Release(ctx)
}

type trackingBranch struct {
	branching.Branch
	closes int
}

func (b *trackingBranch) Close(ctx context.Context) error {
	b.closes++
	return b.Branch.Close(ctx)
}

func mustBranch(t testing.TB, checkpoint branching.Checkpoint) branching.Branch {
	t.Helper()
	branch, err := checkpoint.Branch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if branch == nil || branch.DB() == nil {
		t.Fatal("Branch() returned nil branch or database")
	}
	return branch
}

func closeTestBranch(t testing.TB, branch branching.Branch) {
	t.Helper()
	if err := branch.Close(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestBranchingCapability(t *testing.T) {
	got := NewBranchingProvider().Capability()
	want := branching.Capability{Provider: "dalgo2memory", Version: "1", Mode: "serialized"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capability() = %#v, want %#v", got, want)
	}
}
