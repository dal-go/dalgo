package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func TestBranchingCaptureRejectsInvalidSources(t *testing.T) {
	provider := NewBranchingProvider()

	t.Run("nil interface", func(t *testing.T) {
		checkpoint, err := provider.Capture(context.Background(), nil)
		if checkpoint != nil || !errors.Is(err, branching.ErrNilSourceDB) {
			t.Fatalf("Capture() = (%v, %v), want (nil, ErrNilSourceDB)", checkpoint, err)
		}
	})

	t.Run("typed nil", func(t *testing.T) {
		var memoryDB *database
		var source dal.DB = memoryDB
		checkpoint, err := provider.Capture(context.Background(), source)
		if checkpoint != nil || !errors.Is(err, branching.ErrNilSourceDB) {
			t.Fatalf("Capture() = (%v, %v), want (nil, ErrNilSourceDB)", checkpoint, err)
		}
	})

	t.Run("other adapter", func(t *testing.T) {
		source := struct{ dal.DB }{DB: NewDB()}
		checkpoint, err := provider.Capture(context.Background(), source)
		if checkpoint != nil {
			t.Fatal("Capture() published a checkpoint for a non-dalgo2memory adapter")
		}
		var unsupported *branching.UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("Capture() error = %v, want *branching.UnsupportedError", err)
		}
		if unsupported.Provider != branchingProviderName || unsupported.Mode != "source:struct { dal.DB }" {
			t.Fatalf("unsupported capability = %#v", unsupported)
		}
	})
}

func TestBranchingCaptureHonorsContextAtBothBoundaries(t *testing.T) {
	provider := NewBranchingProvider()

	t.Run("before capture", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		checkpoint, err := provider.Capture(ctx, NewDB())
		if checkpoint != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("Capture() = (%v, %v), want (nil, context.Canceled)", checkpoint, err)
		}
	})

	t.Run("after database lock", func(t *testing.T) {
		ctx := &errSequenceContext{errs: []error{nil, context.Canceled}}
		checkpoint, err := provider.Capture(ctx, NewDB())
		if checkpoint != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("Capture() = (%v, %v), want (nil, context.Canceled)", checkpoint, err)
		}
		if ctx.calls != 2 {
			t.Fatalf("context Err() calls = %d, want 2", ctx.calls)
		}
	})
}

func TestBranchingPreservesSerializedSchemaConfiguration(t *testing.T) {
	ctx := context.Background()
	source := NewDB(
		WithNoReadsAfterWritesInTransaction(),
		WithoutSchemaRefBreaking(),
		WithSchema(true,
			WithCollection[branchingRecord]("items", nil, WithSerializedStorage()),
		),
	).(*database)
	if err := source.Set(ctx, record.NewRecordWithData(branchingRootKey("milk"), &branchingRecord{Title: "milk"})); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := NewBranchingProvider().Capture(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = checkpoint.Release(ctx) }()

	// Mutating source configuration after Capture must not mutate the checkpoint.
	delete(source.schema.collections, "items")
	delete(source.schema.engines, "items")
	source.schema.allowUndefined = false

	first := mustBranch(t, checkpoint)
	defer closeTestBranch(t, first)
	firstDB := first.DB().(*database)
	assertSerializedSchemaClone(t, firstDB)

	// A branch also owns its schema maps; sibling creation must clone them again.
	delete(firstDB.schema.collections, "items")
	delete(firstDB.schema.engines, "items")
	second := mustBranch(t, checkpoint)
	defer closeTestBranch(t, second)
	assertSerializedSchemaClone(t, second.DB().(*database))
}

func assertSerializedSchemaClone(t testing.TB, db *database) {
	t.Helper()
	if db.schema == nil || !db.schema.allowUndefined {
		t.Fatalf("branched schema = %#v, want allowUndefined schema", db.schema)
	}
	if db.schema.collections["items"] == nil || db.schema.engines["items"] == nil {
		t.Fatalf("branched schema lost items factories: %#v", db.schema)
	}
	if !db.noReadsAfterWritesInTransaction {
		t.Fatal("branch lost no-reads-after-writes setting")
	}
	if db.schemaRefBreaking {
		t.Fatal("branch lost schema reference-breaking setting")
	}
}

func TestCloneRecordKeyHandlesIncompleteAndCompositeIDs(t *testing.T) {
	t.Run("incomplete parent chain", func(t *testing.T) {
		parent := record.NewIncompleteKey("spaces", reflect.String, nil)
		key := record.NewIncompleteKey("items", reflect.Int64, parent)
		cloned := cloneRecordKey(key)
		if cloned == key || cloned.Parent() == parent {
			t.Fatal("clone reused a key node")
		}
		if cloned.Collection() != "items" || cloned.ID != nil || cloned.IDKind != reflect.Int64 {
			t.Fatalf("cloned key = %#v", cloned)
		}
		if got := cloned.Parent(); got.Collection() != "spaces" || got.ID != nil || got.IDKind != reflect.String {
			t.Fatalf("cloned parent = %#v", got)
		}
	})

	t.Run("composite ID slice", func(t *testing.T) {
		key := record.NewKeyWithFields("items",
			record.FieldVal{Name: "list", Value: "groceries"},
			record.FieldVal{Name: "title", Value: "milk"},
		)
		cloned := cloneRecordKey(key)
		originalFields := key.ID.([]record.FieldVal)
		clonedFields := cloned.ID.([]record.FieldVal)
		originalFields[0] = record.FieldVal{Name: "changed", Value: "changed"}
		if clonedFields[0].Name != "list" || clonedFields[0].Value != "groceries" {
			t.Fatalf("cloned composite fields changed with source: %#v", clonedFields)
		}
	})
}

func TestBranchingBranchHonorsContextAtBothBoundaries(t *testing.T) {
	checkpoint, err := NewBranchingProvider().Capture(context.Background(), NewDB())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = checkpoint.Release(context.Background()) }()

	t.Run("before branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		branch, err := checkpoint.Branch(ctx)
		if branch != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("Branch() = (%v, %v), want (nil, context.Canceled)", branch, err)
		}
	})

	t.Run("after checkpoint lock", func(t *testing.T) {
		ctx := &errSequenceContext{errs: []error{nil, context.Canceled}}
		branch, err := checkpoint.Branch(ctx)
		if branch != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("Branch() = (%v, %v), want (nil, context.Canceled)", branch, err)
		}
		if ctx.calls != 2 {
			t.Fatalf("context Err() calls = %d, want 2", ctx.calls)
		}
	})
}

type errSequenceContext struct {
	errs  []error
	calls int
}

func (c *errSequenceContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *errSequenceContext) Done() <-chan struct{}       { return nil }
func (c *errSequenceContext) Value(any) any               { return nil }
func (c *errSequenceContext) Err() error {
	index := c.calls
	c.calls++
	if index >= len(c.errs) {
		return c.errs[len(c.errs)-1]
	}
	return c.errs[index]
}
