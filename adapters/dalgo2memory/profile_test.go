package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the explicit-profile constructor: dalgo is platform-
// independent, so New requires every call site to NAME the backend the
// in-memory database emulates instead of inheriting a vendor default.

// TestNew_RequiresProfile: nil is refused loudly — a silent fallback to some
// vendor's semantics is exactly what the required parameter abolishes.
func TestNew_RequiresProfile(t *testing.T) {
	require.PanicsWithValue(t,
		"dalgo2memory.New: profile must not be nil — name the backend this database emulates (e.g. FirestoreProfile())",
		func() { New(nil) })
}

// TestNew_ProfilesSelectTheirBundles: each profile selects its backend's
// transaction bundle; behavior is proven in depth elsewhere (flip_test.go,
// snapshot_test.go, optimistic_test.go), so this asserts the wiring.
func TestNew_ProfilesSelectTheirBundles(t *testing.T) {
	fs := newDatabase(Option(FirestoreProfile()))
	require.True(t, fs.optimisticConcurrency)
	require.True(t, fs.noReadsAfterWritesInTransaction)

	sw := newDatabase(Option(SingleWriterProfile()))
	require.False(t, sw.optimisticConcurrency)
	require.True(t, sw.noReadsAfterWritesInTransaction)
}

// TestNew_OptionsModifyAfterProfile: options apply after the profile,
// last-wins, so intent-named modifiers still compose on top of a named
// backend bundle.
func TestNew_OptionsModifyAfterProfile(t *testing.T) {
	db := newDatabase(Option(FirestoreProfile()), WithSingleWriterTransactions())
	require.False(t, db.optimisticConcurrency,
		"an option after the profile wins, in declaration order")
}

// TestNew_EndToEnd: the exported constructor produces a working database
// under each profile.
func TestNew_EndToEnd(t *testing.T) {
	ctx := context.Background()
	for name, profile := range map[string]Profile{
		"firestore":     FirestoreProfile(),
		"single-writer": SingleWriterProfile(),
	} {
		t.Run(name, func(t *testing.T) {
			db := New(profile)
			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tx.Set(ctx, thingRecord("t1", 1))
			})
			require.NoError(t, err)
			exists, err := db.Exists(ctx, thingKey("t1"))
			require.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

// TestNewDB_DeprecatedAliasIsFirestore: the compatibility constructor is
// exactly New(FirestoreProfile()) until its scheduled removal.
func TestNewDB_DeprecatedAliasIsFirestore(t *testing.T) {
	db := newDatabase()
	require.True(t, db.optimisticConcurrency)
	require.True(t, db.noReadsAfterWritesInTransaction)
}
