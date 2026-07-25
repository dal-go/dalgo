package dal_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// TestSealIsEnforcedAtCompileTime runs the negative-compile files and requires
// them to fail. Without this, a build-tagged "proof" file is only a comment:
// nothing would notice if the seal were removed and it started compiling.
func TestSealIsEnforcedAtCompileTime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	for _, tt := range []struct {
		tag  string
		want string
	}{
		{
			tag:  "dalgo_seal_nocompile",
			want: "dalgoDB",
		},
		{
			tag:  "dalgo_collection_nocompile",
			want: "WriteSession",
		},
	} {
		t.Run(tt.tag, func(t *testing.T) {
			out, err := exec.Command("go", "vet", "-tags", tt.tag, ".").CombinedOutput()
			if err == nil {
				t.Fatalf("go vet -tags %s compiled cleanly; the negative-compile proof no longer proves anything\n%s", tt.tag, out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Fatalf("go vet -tags %s failed, but not for the expected reason (%q):\n%s", tt.tag, tt.want, out)
			}
		})
	}
}

// cachingDB is the shape a legitimate decorator must take: it EMBEDS dal.DB
// rather than naming it in a field, which is how the unexported seal method is
// promoted. It lives in package dal_test, outside package dal, so it proves the
// promotion works across a package boundary — the same visibility rule that
// applies across a module boundary.
type cachingDB struct {
	dal.DB
	gets int
}

func (db *cachingDB) Get(ctx context.Context, r record.Record) error {
	db.gets++
	return db.DB.Get(ctx, r)
}

var _ dal.DB = (*cachingDB)(nil)

// TestDecoratorEmbeddingDBSatisfiesDB is the positive half of the seal: sealing
// must not make decoration impossible. dalgo-memcache-appengine and
// access.SecureDB are the real cases.
func TestDecoratorEmbeddingDBSatisfiesDB(t *testing.T) {
	backend := newSpyBackend()
	var db dal.DB = &cachingDB{DB: dal.NewDB(backend)}

	if err := db.Get(context.Background(), validRecord("t1")); !errors.Is(err, dal.ErrNotSupported) {
		t.Fatalf("decorated Get: err = %v, want the inner DB's error", err)
	}
	if got := db.(*cachingDB).gets; got != 1 {
		t.Fatalf("decorator Get count = %d, want 1", got)
	}

	// The decorator inherits the pipeline it does not override.
	err := db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, invalidRecord("t1"))
	})
	if !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("write through a decorator: err = %v, want the validation error", err)
	}
	if len(backend.writes) != 0 {
		t.Fatalf("write through a decorator reached the backend: %v", backend.writes)
	}
}

// TestNewDBIsIdempotent: an already-sealed DB has already been through the
// pipeline. Wrapping it again would run hooks twice.
func TestNewDBIsIdempotent(t *testing.T) {
	db := dal.NewDB(newSpyBackend())
	if again := dal.NewDB(db); again != db {
		t.Fatalf("NewDB(NewDB(b)) returned a new value %T, want the same DB", again)
	}
}

// TestNewDBPanicsOnNilBackend: silently returning a DB over nothing would
// surface much later as a nil dereference inside a write.
func TestNewDBPanicsOnNilBackend(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewDB(nil) did not panic")
		}
	}()
	_ = dal.NewDB(nil)
}

// TestBackendOfReturnsTheAdaptersOwnType: adapter internals (the dalgo2memory
// branching provider, for one) must be able to recover their concrete type
// through the wrapper, or wrapping silently breaks those capabilities.
func TestBackendOfReturnsTheAdaptersOwnType(t *testing.T) {
	backend := newSpyBackend()
	if got := dal.BackendOf(dal.NewDB(backend)); got != dal.Backend(backend) {
		t.Fatalf("BackendOf returned %T, want the original *spyBackend", got)
	}
}

// capability is an optional interface of the kind dbschema/ddl advertise.
type capability interface{ Capability() string }

func (b *spyBackend) Capability() string { return "spy-capability" }

// TestAsFindsOptionalCapabilitiesThroughTheWrapper: DALgo advertises optional
// features by type assertion. If it goes red, wrapping an adapter silently
// disables its schema introspection and DDL support.
func TestAsFindsOptionalCapabilitiesThroughTheWrapper(t *testing.T) {
	db := dal.NewDB(newSpyBackend())

	if _, ok := db.(capability); ok {
		t.Fatal("a plain type assertion on the wrapper should not see the backend's capability")
	}
	c, ok := dal.As[capability](db)
	if !ok {
		t.Fatal("dal.As did not find the backend's capability through the wrapper")
	}
	if got := c.Capability(); got != "spy-capability" {
		t.Fatalf("Capability() = %q, want the backend's", got)
	}
}

// capableDecorator advertises a capability of its own, on top of a DB.
type capableDecorator struct {
	dal.DB
}

func (capableDecorator) Capability() string { return "decorator-capability" }

// TestAsPrefersTheOutermostImplementation: a decorator that adds or overrides a
// capability must win over the backend it wraps, or a caching or policy layer
// would be silently bypassed.
func TestAsPrefersTheOutermostImplementation(t *testing.T) {
	db := capableDecorator{DB: dal.NewDB(newSpyBackend())}
	c, ok := dal.As[capability](db)
	if !ok {
		t.Fatal("dal.As did not find the decorator's own capability")
	}
	if got := c.Capability(); got != "decorator-capability" {
		t.Fatalf("Capability() = %q, want the decorator's", got)
	}
}

// TestBackendOfLeavesADecoratorAlone: a decorator is not a framework wrapper and
// has no backend to unwrap, so BackendOf must return it unchanged rather than
// reaching past it.
func TestBackendOfLeavesADecoratorAlone(t *testing.T) {
	db := &cachingDB{DB: dal.NewDB(newSpyBackend())}
	if got := dal.BackendOf(db); got != dal.Backend(db) {
		t.Fatalf("BackendOf(decorator) = %T, want the decorator itself", got)
	}
}

// TestAsReportsMissingCapability keeps dal.As from being a blanket "yes".
func TestAsReportsMissingCapability(t *testing.T) {
	if _, ok := dal.As[capability](dal.NewDB(readOnlyBackend{})); ok {
		t.Fatal("dal.As found a capability the backend does not implement")
	}
}
