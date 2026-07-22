// Package branchingtest provides reusable conformance tests for DALgo branching providers.
package branchingtest

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
)

// Factory returns a fresh source database and a provider configured for it.
// Each conformance subtest receives a new pair.
type Factory func(testing.TB) (dal.DB, branching.Provider)

// Callbacks describe a provider-specific semantic fixture. Generic dal.DB does
// not promise enumeration, so conformance never guesses collections or records.
type Callbacks struct {
	New    Factory
	Seed   func(context.Context, dal.DB) error
	Mutate func(context.Context, dal.DB) error
	Digest func(context.Context, dal.DB) (string, error)
}

// RunConformance proves immutable checkpoints, source/sibling isolation, fresh
// handles, semantic fidelity and idempotent cleanup.
func RunConformance(t *testing.T, callbacks Callbacks) {
	t.Helper()
	validate(t, callbacks)

	t.Run("empty database", func(t *testing.T) {
		source, provider := callbacks.New(t)
		checkpoint := capture(t, provider, source)
		defer release(t, checkpoint)

		baseline := digest(t, callbacks, source)
		branch := startBranch(t, checkpoint, source)
		if got := digest(t, callbacks, branch.DB()); got != baseline {
			t.Fatalf("empty branch digest = %q, want %q", got, baseline)
		}
		closeBranch(t, branch)
		closeBranch(t, branch)
	})

	t.Run("seed fidelity and fresh handle", func(t *testing.T) {
		source, provider := callbacks.New(t)
		seed(t, callbacks, source)
		baseline := digest(t, callbacks, source)
		checkpoint := capture(t, provider, source)
		defer release(t, checkpoint)

		branch := startBranch(t, checkpoint, source)
		defer closeBranch(t, branch)
		if got := digest(t, callbacks, branch.DB()); got != baseline {
			t.Fatalf("seeded branch digest = %q, want %q", got, baseline)
		}
	})

	t.Run("source mutation does not change checkpoint", func(t *testing.T) {
		source, provider := callbacks.New(t)
		seed(t, callbacks, source)
		baseline := digest(t, callbacks, source)
		checkpoint := capture(t, provider, source)
		defer release(t, checkpoint)

		mutate(t, callbacks, source)
		if got := digest(t, callbacks, source); got == baseline {
			t.Fatalf("mutation did not change source digest %q", baseline)
		}
		branch := startBranch(t, checkpoint, source)
		defer closeBranch(t, branch)
		if got := digest(t, callbacks, branch.DB()); got != baseline {
			t.Fatalf("branch after source mutation digest = %q, want checkpoint %q", got, baseline)
		}
	})

	t.Run("sibling isolation", func(t *testing.T) {
		source, provider := callbacks.New(t)
		seed(t, callbacks, source)
		baseline := digest(t, callbacks, source)
		checkpoint := capture(t, provider, source)
		defer release(t, checkpoint)

		first := startBranch(t, checkpoint, source)
		mutate(t, callbacks, first.DB())
		firstDigest := digest(t, callbacks, first.DB())
		if firstDigest == baseline {
			t.Fatalf("first sibling mutation did not change digest %q", baseline)
		}
		closeBranch(t, first)

		second := startBranch(t, checkpoint, source)
		if second.DB() == first.DB() {
			t.Fatal("siblings received the same dal.DB handle")
		}
		if got := digest(t, callbacks, second.DB()); got != baseline {
			t.Fatalf("second sibling digest = %q, want %q", got, baseline)
		}
		closeBranch(t, second)

		third := startBranch(t, checkpoint, source)
		if got := digest(t, callbacks, third.DB()); got != baseline {
			t.Fatalf("checkpoint changed after sibling mutation: got %q, want %q", got, baseline)
		}
		closeBranch(t, third)
	})

	t.Run("release is idempotent", func(t *testing.T) {
		source, provider := callbacks.New(t)
		checkpoint := capture(t, provider, source)
		if err := checkpoint.Release(context.Background()); err != nil {
			t.Fatalf("first Release(): %v", err)
		}
		if err := checkpoint.Release(context.Background()); err != nil {
			t.Fatalf("second Release(): %v", err)
		}
		if _, err := checkpoint.Branch(context.Background()); !errors.Is(err, branching.ErrReleased) {
			t.Fatalf("Branch() after Release() error = %v, want ErrReleased", err)
		}
	})
}

func validate(t testing.TB, callbacks Callbacks) {
	t.Helper()
	if callbacks.New == nil || callbacks.Seed == nil || callbacks.Mutate == nil || callbacks.Digest == nil {
		t.Fatal("branchingtest: New, Seed, Mutate and Digest callbacks are required")
	}
}

func capture(t testing.TB, provider branching.Provider, source dal.DB) branching.Checkpoint {
	t.Helper()
	if source == nil || provider == nil {
		t.Fatal("branchingtest: factory returned nil source or provider")
	}
	checkpoint, err := provider.Capture(context.Background(), source)
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	if checkpoint == nil {
		t.Fatal("Capture() returned nil checkpoint")
	}
	if checkpoint.Generation() == "" {
		t.Fatal("checkpoint generation is empty")
	}
	return checkpoint
}

func startBranch(t testing.TB, checkpoint branching.Checkpoint, source dal.DB) branching.Branch {
	t.Helper()
	branch, err := checkpoint.Branch(context.Background())
	if err != nil {
		t.Fatalf("Branch(): %v", err)
	}
	if branch == nil || branch.DB() == nil {
		t.Fatal("Branch() returned nil branch or database")
	}
	if branch.DB() == source {
		t.Fatal("Branch() returned the live source dal.DB handle")
	}
	return branch
}

func seed(t testing.TB, callbacks Callbacks, db dal.DB) {
	t.Helper()
	if err := callbacks.Seed(context.Background(), db); err != nil {
		t.Fatalf("Seed(): %v", err)
	}
}

func mutate(t testing.TB, callbacks Callbacks, db dal.DB) {
	t.Helper()
	if err := callbacks.Mutate(context.Background(), db); err != nil {
		t.Fatalf("Mutate(): %v", err)
	}
}

func digest(t testing.TB, callbacks Callbacks, db dal.DB) string {
	t.Helper()
	digest, err := callbacks.Digest(context.Background(), db)
	if err != nil {
		t.Fatalf("Digest(): %v", err)
	}
	return digest
}

func closeBranch(t testing.TB, branch branching.Branch) {
	t.Helper()
	if err := branch.Close(context.Background()); err != nil {
		t.Fatalf("Close(): %v", err)
	}
}

func release(t testing.TB, checkpoint branching.Checkpoint) {
	t.Helper()
	if err := checkpoint.Release(context.Background()); err != nil {
		t.Fatalf("Release(): %v", err)
	}
}
