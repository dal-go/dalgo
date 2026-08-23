package dalgo2memory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	dalrecord "github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

// TestOptimisticConcurrencyOption checks WithOptimisticConcurrency's own
// bookkeeping: off by default, on when requested. Everything else in this
// file exercises the behavior that flag switches on.
func TestOptimisticConcurrencyOption(t *testing.T) {
	require.False(t, newDatabase().optimisticConcurrency)
	require.True(t, newDatabase(WithOptimisticConcurrency()).optimisticConcurrency)
}

// TestIsTransactionConflict is the direct unit test for the predicate the
// package doc promises callers can retry on — separate from the concurrency
// tests below, which prove the error actually gets produced under real
// contention.
func TestIsTransactionConflict(t *testing.T) {
	require.False(t, IsTransactionConflict(nil))
	require.False(t, IsTransactionConflict(errors.New("unrelated")))
	require.True(t, IsTransactionConflict(ErrTransactionConflict))
	require.True(t, IsTransactionConflict(fmt.Errorf("wrapped: %w", ErrTransactionConflict)))
}

// TestDefaultTransactionsRemainSerialized is the regression guard for the
// behavior this whole feature must not touch: without
// WithOptimisticConcurrency, RunReadwriteTransaction still takes a
// whole-database lock for a transaction's entire duration, so a second
// read-write transaction cannot even START running until the first one's
// callback returns. This is exactly what makes a "two concurrent claims,
// exactly one wins" test meaningless against the default mode (see
// WithOptimisticConcurrency's doc comment) — proving it still holds is what
// justifies every other test in this file bothering to exist.
func TestDefaultTransactionsRemainSerialized(t *testing.T) {
	ctx := context.Background()
	db := newDatabase() // no WithOptimisticConcurrency: the pre-existing default

	firstEntered := make(chan struct{})
	release := make(chan struct{})
	secondEntered := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		firstDone <- db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
			close(firstEntered)
			<-release
			return nil
		})
	}()
	<-firstEntered

	go func() {
		secondDone <- db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("a second read-write transaction started before the first one finished: the default whole-database lock was not held")
	case <-time.After(20 * time.Millisecond):
		// Expected: the second transaction is still blocked trying to acquire
		// db.mu, because the first one is still holding it inside its callback.
	}

	close(release)
	require.NoError(t, <-firstDone)
	<-secondEntered // now unblocks
	require.NoError(t, <-secondDone)
}

// slugThing is this file's stand-in for the sneat-co/bookius pattern the
// WithOptimisticConcurrency doc comment cites: a Get-then-Insert used to
// claim a unique key.
type slugThing struct {
	Owner string
}

// TestOptimisticConcurrencyGetThenInsertSameKeyExactlyOneWins is the
// headline required test: two transactions run the same Get-then-Insert
// slug claim concurrently for the same key. Exactly one must succeed, and
// the other must fail specifically with ErrTransactionConflict — not a
// "duplicate" error, since from that transaction's own point of view (it
// read the slug before either side had claimed it) nothing it did was
// wrong; it lost a race after the fact.
//
// bothRead/proceed force both transactions' Get to complete — recording the
// same commit-version baseline for the key — before either is allowed to
// reach its own Insert and commit. Without that, one transaction's entire
// Get-Insert-commit could occasionally finish before the other's Get even
// ran, which would make the second transaction's Get correctly find the key
// already claimed and take a different, though equally valid, code path —
// turning this into a flaky test of the wrong thing. See
// TestOptimisticConcurrencyReadWriteConflict's doc comment for why ordinary
// channels are enough to force this deterministically, with no dedicated
// test hook needed.
func TestOptimisticConcurrencyGetThenInsertSameKeyExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Slugs", "shared-slug")

	bothRead := make(chan struct{}, 2)
	proceed := make(chan struct{})
	results := make(chan error, 2)

	claim := func(owner string) {
		results <- db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			var existing slugThing
			err := tx.Get(ctx, dalrecord.NewRecordWithData(key, &existing))
			if err != nil && !dalrecord.IsNotFound(err) {
				return err
			}
			if err == nil {
				return fmt.Errorf("test setup: %s found the slug already claimed by %q before either side wrote", owner, existing.Owner)
			}
			bothRead <- struct{}{}
			<-proceed
			return tx.Insert(ctx, dalrecord.NewRecordWithData(key, &slugThing{Owner: owner}))
		})
	}

	go claim("alice")
	go claim("bob")

	<-bothRead
	<-bothRead
	close(proceed)

	err1, err2 := <-results, <-results

	successes, conflicts := 0, 0
	for _, err := range []error{err1, err2} {
		switch {
		case err == nil:
			successes++
		case IsTransactionConflict(err):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent claim of the same slug should succeed")
	require.Equal(t, 1, conflicts, "the losing claim should fail with ErrTransactionConflict, not some other error")

	// The stored value must be whichever claim actually won — not corrupted,
	// not both, not neither.
	var stored slugThing
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.Get(ctx, dalrecord.NewRecordWithData(key, &stored))
	}))
	require.Contains(t, []string{"alice", "bob"}, stored.Owner)
}

// TestOptimisticConcurrencyReadWriteConflict drives the exact interleaving
// the Feature calls for: tx A reads key K, then tx B reads and writes K and
// commits, then tx A writes K and commits — and A's commit must fail with
// ErrTransactionConflict, even though a plain, lock-free write of K by A
// would not itself have errored (Set has no duplicate check the way Insert
// does). This is the case that proves the conflict detection is genuine
// optimistic concurrency control and not just Insert's pre-existing
// duplicate check wearing a new name.
//
// The interleaving is deterministic with nothing beyond ordinary channels:
// tx A's callback blocks on <-releaseA after its Get, holding no lock while
// it waits (RunReadwriteTransaction never holds db.mu across a paused
// callback in optimistic mode — see optimisticState's doc comment), so tx
// B's own RunReadwriteTransaction call — run synchronously on the test
// goroutine — is free to read, write and fully commit before releaseA is
// closed. That gives a real happens-before edge from B's commit to A's
// resumption, which is all the determinism this test needs; a bespoke
// test-only hook would buy nothing beyond what the callback itself already
// provides as a synchronization point.
func TestOptimisticConcurrencyReadWriteConflict(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "contended")
	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "initial", Count: 0})))

	aReached := make(chan struct{})
	releaseA := make(chan struct{})
	resultA := make(chan error, 1)

	go func() {
		resultA <- db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			var got thing
			if err := tx.Get(ctx, dalrecord.NewRecordWithData(key, &got)); err != nil {
				return err
			}
			close(aReached)
			<-releaseA
			return tx.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "from-a", Count: got.Count + 1}))
		})
	}()

	<-aReached // A has read K; it is now paused, holding no lock

	// B reads and writes K and commits, fully, before A is allowed to continue.
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var got thing
		if err := tx.Get(ctx, dalrecord.NewRecordWithData(key, &got)); err != nil {
			return err
		}
		return tx.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "from-b", Count: got.Count + 1}))
	})
	require.NoError(t, err, "B's own transaction should commit cleanly")

	close(releaseA)
	err = <-resultA
	require.Error(t, err, "A must not be allowed to commit over B's committed write to a key A read before B ran")
	require.True(t, IsTransactionConflict(err), "A's failure must be classifiable as a conflict so a caller knows to retry: got %v", err)

	// B's write must be what is actually stored — A's conflicting write must
	// never have reached storage.
	var stored thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(key, &stored)))
	require.Equal(t, "from-b", stored.Name)
}

// TestOptimisticConcurrencyDisjointKeysDoNotConflict is the required
// negative case: two transactions that read and write different keys must
// both commit cleanly, even when forced into the same worst-case overlap in
// timing as the conflicting cases above use to prove a conflict. Optimistic
// concurrency that conflicted on unrelated keys would be worthless — it
// would recreate the whole-database lock's serialization by a different
// name.
func TestOptimisticConcurrencyDisjointKeysDoNotConflict(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	keyA := dalrecord.NewKeyWithID("Things", "alpha")
	keyB := dalrecord.NewKeyWithID("Things", "beta")

	bothRead := make(chan struct{}, 2)
	proceed := make(chan struct{})
	results := make(chan error, 2)

	run := func(key *dalrecord.Key, name string) {
		results <- db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			var existing thing
			err := tx.Get(ctx, dalrecord.NewRecordWithData(key, &existing))
			if err != nil && !dalrecord.IsNotFound(err) {
				return err
			}
			bothRead <- struct{}{}
			<-proceed
			return tx.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: name}))
		})
	}

	go run(keyA, "alpha-value")
	go run(keyB, "beta-value")

	<-bothRead
	<-bothRead
	close(proceed)

	require.NoError(t, <-results)
	require.NoError(t, <-results)

	var gotA, gotB thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(keyA, &gotA)))
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(keyB, &gotB)))
	require.Equal(t, "alpha-value", gotA.Name)
	require.Equal(t, "beta-value", gotB.Name)
}

// TestOptimisticConcurrencyReadYourOwnWrites is not one of the Feature's
// required tests, but it is the load-bearing property that makes the
// buffered design usable at all: within a single optimistic transaction, a
// Get after a Set/Insert/Update/Delete of the same key must see that
// transaction's own pending write, even though nothing has reached the
// shared engine yet.
func TestOptimisticConcurrencyReadYourOwnWrites(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "rmw")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "first", Count: 1})); err != nil {
			return err
		}
		var got thing
		if err := tx.Get(ctx, dalrecord.NewRecordWithData(key, &got)); err != nil {
			return err
		}
		if got.Name != "first" || got.Count != 1 {
			return fmt.Errorf("read-your-own-write: got %+v, want {first 1}", got)
		}
		exists, err := tx.Exists(ctx, key)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("Exists after Set within the same transaction reported false")
		}
		return tx.Delete(ctx, key)
	})
	require.NoError(t, err)

	// The Set must never have reached the shared engine (it was superseded by
	// the Delete before commit), so nothing should be stored afterwards.
	exists, err := db.Exists(ctx, key)
	require.NoError(t, err)
	require.False(t, exists)
}

// TestOptimisticConcurrencyUpdateCompoundsAndCommits checks that Update
// inside an optimistic transaction (a) requires the key to already exist,
// eagerly, like the default mode does, (b) compounds correctly when called
// twice on the same key in the same transaction, and (c) actually reaches
// the storage engine once the transaction commits.
func TestOptimisticConcurrencyUpdateCompoundsAndCommits(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "update-me")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Count", 1)})
	})
	require.Error(t, err, "Update of a key that does not exist yet must fail immediately, like the default mode")
	require.True(t, dalrecord.IsNotFound(err))

	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "base", Count: 1})))

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Update(ctx, key, []update.Update{update.ByFieldName("Count", 2)}); err != nil {
			return err
		}
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Name", "updated")})
	})
	require.NoError(t, err)

	var got thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(key, &got)))
	require.Equal(t, thing{Name: "updated", Count: 2}, got)
}

// TestOptimisticConcurrencyQueryUnsupported documents and locks in the one
// deliberate gap in this mode: a query inside an optimistic transaction reads
// committed storage directly, so it would see neither the transaction's own
// buffered writes nor participate in its conflict detection. Rather than
// silently returning a semantically inconsistent result, it fails clearly.
func TestOptimisticConcurrencyQueryUnsupported(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		q := dal.From(dal.NewRootCollectionRef("Things", "")).NewQuery().SelectKeysOnly(reflect.String)
		_, err := tx.ExecuteQueryToRecordsReader(ctx, q)
		return err
	})
	require.ErrorIs(t, err, dal.ErrNotSupported)
}

// TestOptimisticConcurrencyInsertWithGenerator checks that Insert's
// id-generator retry loop (dal.InsertWithIdGenerator) is wired correctly
// under optimistic mode: its "does this id already exist" probe must consult
// this transaction's own buffered/tracked view (via optimisticState.exists),
// not the live engine directly.
func TestOptimisticConcurrencyInsertWithGenerator(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, dalrecord.NewRecordWithIncompleteKey("Things", reflect.String, &thing{Name: "generated"}),
			dal.WithRandomStringKey(8, 3))
	})
	require.NoError(t, err)

	q := dal.From(dal.NewRootCollectionRef("Things", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	rec, err := reader.Next()
	require.NoError(t, err)
	require.NotNil(t, rec.Key().ID)
}

// TestOptimisticConcurrencyInsertWithGeneratorRetriesOnCollision checks the
// generator callback's "id is taken, retry" branch deterministically: every
// one of the 62 possible single-character ids random.ID(1) can produce is
// pre-occupied, so a WithRandomStringKey(1, ...) generator collides on every
// attempt, exhausting insertWithGeneratorMaxAttempts — exactly
// TestSession_Insert_GeneratorExhaustion's technique, adapted so the
// collision is guaranteed by exhausting the id space instead of relying on a
// day-granularity timestamp.
func TestOptimisticConcurrencyInsertWithGeneratorRetriesOnCollision(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for _, c := range charset {
			if err := tx.Insert(ctx, dalrecord.NewRecordWithData(dalrecord.NewKeyWithID("Slots", string(c)), &thing{Name: "taken"})); err != nil {
				return err
			}
		}
		return nil
	}))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		rec := dalrecord.NewRecordWithIncompleteKey("Slots", reflect.String, &thing{Name: "new"})
		return tx.Insert(ctx, rec, dal.WithRandomStringKey(1, 5))
	})
	require.Error(t, err)
	require.ErrorIs(t, err, dal.ErrExceedsMaxNumberOfAttempts)
}

// TestOptimisticConcurrencyInsertWithGeneratorPropagatesExistsError checks
// that a genuine engine error from the generator callback's existence probe
// (as opposed to an ordinary "does/doesn't exist" answer) is propagated
// rather than silently treated as either. A day-accuracy timestamp generator
// produces the same id deterministically within a run (see
// TestSession_Insert_GeneratorExhaustion), so the id to corrupt can be
// computed up front.
func TestOptimisticConcurrencyInsertWithGeneratorPropagatesExistsError(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	gen := dal.WithTimeStampStringID(dal.TimeStampAccuracyDay, 10, 5)
	generator := dal.NewInsertOptions(gen).IDGenerator()

	probe := dalrecord.NewRecordWithIncompleteKey("Days", reflect.String, &thing{})
	require.NoError(t, generator(ctx, probe))
	id, ok := probe.Key().ID.(string)
	require.True(t, ok)
	db.collections["Days"] = &serializedEngine{records: map[string][]byte{id: []byte("{")}}

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		rec := dalrecord.NewRecordWithIncompleteKey("Days", reflect.String, &thing{Name: "x"})
		return tx.Insert(ctx, rec, gen)
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to check if record exists")
}

// TestOptimisticConcurrencyGuardedCollectionErrors checks that the schema
// guard (guardCollection's error, surfaced via recordFactory) still applies
// to every optimistic-mode write/read path — Get, Set, Insert, Update — the
// same way it already does in the default mode.
func TestOptimisticConcurrencyGuardedCollectionErrors(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency(), WithSchema(false, WithCollection[thing]("Known", nil)))
	key := dalrecord.NewKeyWithID("Unknown", "x")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var got thing
		return tx.Get(ctx, dalrecord.NewRecordWithData(key, &got))
	})
	require.Error(t, err, "Get on an undeclared collection must be rejected")

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "x"}))
	})
	require.Error(t, err, "Set on an undeclared collection must be rejected")

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "x"}))
	})
	require.Error(t, err, "Insert on an undeclared collection must be rejected")

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Name", "y")})
	})
	require.Error(t, err, "Update on an undeclared collection must be rejected")
}

// TestOptimisticConcurrencyMalformedRecordDataErrors checks that Set and
// Insert both fail immediately — at the point of the call, not deferred to
// commit — for a record whose data cannot even be marshaled to JSON. This is
// the same class of failure TestBadRecordDataAndUnsupportedQuery already
// covers for the default mode.
func TestOptimisticConcurrencyMalformedRecordDataErrors(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	badRecord := func() dalrecord.Record {
		return dalrecord.NewRecordWithData(dalrecord.NewKeyWithID("Bad", "json"), func() {})
	}

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, badRecord())
	})
	require.Error(t, err)

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, badRecord())
	})
	require.Error(t, err)
}

// strictThing is a schema type with no field named "Extra", used to prove
// validateForCommit's eager unknown-field check runs for Set/Insert/Update
// under optimistic mode exactly as the live engines' own checkUnknownFields
// call already does in the default mode.
type strictThing struct {
	Name string
}

// TestOptimisticConcurrencyUnknownFieldValidation checks that Set and Insert
// both fail immediately for data containing a field the schema does not
// declare, and that Update's merged result is validated the same way.
func TestOptimisticConcurrencyUnknownFieldValidation(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency(), WithSchema(false, WithCollection[strictThing]("Strict", nil)))
	key := dalrecord.NewKeyWithID("Strict", "x")
	withExtraField := map[string]any{"Name": "ok", "Extra": "not declared"}

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, dalrecord.NewRecordWithData(key, withExtraField))
	})
	require.Error(t, err, "Set of a record with an undeclared field must be rejected")

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, dalrecord.NewRecordWithData(key, withExtraField))
	})
	require.Error(t, err, "Insert of a record with an undeclared field must be rejected")

	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &strictThing{Name: "ok"})))
	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Extra", "not declared")})
	})
	require.Error(t, err, "Update that merges in an undeclared field must be rejected")
}

// TestOptimisticConcurrencyEngineLoadErrorPropagates checks that a genuine
// storage error — as opposed to a plain not-found — surfaces correctly from
// every optimistic-mode operation that has to consult the live engine on a
// key's first touch: Get, Exists, Insert's duplicate check, and Update. It
// mirrors TestMalformedStoredData's technique of corrupting the underlying
// Serialized engine's bytes directly.
func TestOptimisticConcurrencyEngineLoadErrorPropagates(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Bad", "json")
	db.collections[key.Collection()] = &serializedEngine{records: map[string][]byte{keyID(key): []byte("{")}}

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var got thing
		return tx.Get(ctx, dalrecord.NewRecordWithData(key, &got))
	})
	require.Error(t, err, "Get must propagate a genuine engine-load error rather than mistaking it for not-found")

	var existsErr error
	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, existsErr = tx.Exists(ctx, key)
		return nil
	})
	require.NoError(t, err)
	require.Error(t, existsErr)

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "x"}))
	})
	require.Error(t, err, "Insert's own existence probe must also surface the engine error rather than treating the key as absent")

	err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Name", "y")})
	})
	require.Error(t, err, "Update must also surface the engine error")
}

// TestOptimisticConcurrencyGetMaterializeError checks that Get reports an
// error — rather than silently zero-filling or panicking — when a
// transaction's own normalized snapshot cannot be decoded into the caller's
// target type.
func TestOptimisticConcurrencyGetMaterializeError(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Mismatched", "x")
	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "n", Count: 5})))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var mismatch struct{ Count string }
		return tx.Get(ctx, dalrecord.NewRecordWithData(key, &mismatch))
	})
	require.Error(t, err)
}

// TestOptimisticConcurrencyInsertDuplicateWithinTransaction checks the
// same-transaction duplicate case insert's doc comment calls out
// specifically: Insert of a key this same transaction already inserted must
// fail immediately with an ordinary error, never with ErrTransactionConflict
// — nothing else committed anything, so there is nothing for this
// transaction to have lost a race against.
func TestOptimisticConcurrencyInsertDuplicateWithinTransaction(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "dup")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Insert(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "first"})); err != nil {
			return err
		}
		dupErr := tx.Insert(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "second"}))
		if dupErr == nil {
			return errors.New("test setup: second insert of the same key in the same transaction unexpectedly succeeded")
		}
		if IsTransactionConflict(dupErr) {
			return fmt.Errorf("a same-transaction duplicate must not be reported as ErrTransactionConflict: %w", dupErr)
		}
		if !dalrecord.IsAlreadyExists(dupErr) {
			return fmt.Errorf("a same-transaction duplicate must satisfy record.IsAlreadyExists: %w", dupErr)
		}
		return nil // the duplicate was correctly rejected with record.ErrRecordExists
	})
	require.NoError(t, err)
}

// TestOptimisticConcurrencyUpdateWithNonMarshalableValueFails checks that an
// Update whose merged result cannot be marshaled fails immediately, and —
// since updateRecord works on a defensive copy of the transaction's own
// snapshot — leaves that snapshot (and the eventually-committed value)
// unaffected.
func TestOptimisticConcurrencyUpdateWithNonMarshalableValueFails(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "bad-update")
	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "ok"})))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldName("Bad", func() {})})
	})
	require.Error(t, err)

	var got thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(key, &got)))
	require.Equal(t, "ok", got.Name)
}

// TestOptimisticConcurrencyCommitAppliesWritesAndSkipsReadOnlyKeys checks
// commit's handling of a transaction that touches more than one key with a
// mix of outcomes: a key it only read must be left alone (and must not
// somehow block or corrupt the commit), while a key it wrote must actually
// reach the storage engine.
func TestOptimisticConcurrencyCommitAppliesWritesAndSkipsReadOnlyKeys(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	readOnlyKey := dalrecord.NewKeyWithID("Things", "read-only")
	writeKey := dalrecord.NewKeyWithID("Things", "written")
	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(readOnlyKey, &thing{Name: "untouched"})))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var got thing
		if err := tx.Get(ctx, dalrecord.NewRecordWithData(readOnlyKey, &got)); err != nil {
			return err
		}
		return tx.Set(ctx, dalrecord.NewRecordWithData(writeKey, &thing{Name: "new"}))
	})
	require.NoError(t, err)

	var stillThere thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(readOnlyKey, &stillThere)))
	require.Equal(t, "untouched", stillThere.Name)

	var written thing
	require.NoError(t, db.Get(ctx, dalrecord.NewRecordWithData(writeKey, &written)))
	require.Equal(t, "new", written.Name)
}

// TestOptimisticConcurrencyCommitDeferredValidationFailure exercises the
// narrow gap documented on commit and validateForCommit: a buffered write
// whose data passes the eager, generic validateForCommit check (a JSON
// marshal plus, for a map-backed collection, an unknown-field check that a
// map target can never actually fail) can still fail the columnar engine's
// own per-column typed decode once commit actually calls engine.store. This
// is the same scenario TestMixed_DeclaredValueWrongTypeErrors proves for the
// default mode, replayed here to prove the deferral doesn't hide it, only
// moves it from the point of the call to the point of commit.
func TestOptimisticConcurrencyCommitDeferredValidationFailure(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency(), WithSchema(false,
		WithCollection[map[string]any]("m", nil, WithColumnarStorage(WithDeclaredColumn[int]("age"))),
	))
	key := dalrecord.NewKeyWithID("m", "u1")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		// This Set itself must succeed: validateForCommit's generic checks have
		// nothing to object to for a map-backed collection.
		return tx.Set(ctx, dalrecord.NewRecordWithData(key, map[string]any{"age": "old", "name": "Alice"}))
	})
	require.Error(t, err, "the type mismatch must still surface, just at commit instead of at the point of the call")
	require.ErrorContains(t, err, "age")
	require.False(t, IsTransactionConflict(err), "a deferred storage-fidelity failure is not an optimistic-concurrency conflict")

	exists, existsErr := db.Exists(ctx, key)
	require.NoError(t, existsErr)
	require.False(t, exists, "the rejected write must not have been persisted")
}

// TestOptimisticConcurrencyUpdatePathThroughNonMapFails checks updateRecord's
// applyUpdatesToMap error path: a FieldPath update that tries to descend
// through an existing, non-map value must fail with a descriptive error
// rather than panicking — the same behavior applyUpdatesToMap's own doc
// comment promises for the default mode.
func TestOptimisticConcurrencyUpdatePathThroughNonMapFails(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	key := dalrecord.NewKeyWithID("Things", "bad-path")
	require.NoError(t, db.Set(ctx, dalrecord.NewRecordWithData(key, &thing{Name: "text"})))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Update(ctx, key, []update.Update{update.ByFieldPath(update.FieldPath{"Name", "Sub"}, "x")})
	})
	require.Error(t, err)
}

// TestOptimisticConcurrencySetOfNonObjectDataFails checks normalizeData's own
// error path: data that marshals to a JSON array (or any other non-object
// JSON value) cannot decode into the generic map[string]any every
// optimistic-mode operation works with, and Set must report that rather than
// buffering something Get could never materialize back out.
func TestOptimisticConcurrencySetOfNonObjectDataFails(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, dalrecord.NewRecordWithData(dalrecord.NewKeyWithID("Things", "array-data"), []int{1, 2, 3}))
	})
	require.Error(t, err)
}
