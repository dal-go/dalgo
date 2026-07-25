package dalgo2fs_test

import (
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2fs"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dalgotest"
)

// TestConformance runs the shared suite against dalgo2fs.
//
// dalgo2fs is a read-only stub: every write method returns ErrNotSupported or
// ErrNotImplementedYet. That makes it the sharpest available proof that
// validation runs BEFORE the adapter — a validation error from a database that
// cannot write at all can only have come from the framework write pipeline.
// The suite's "accepts a valid record" checks tolerate the unsupported answers.
func TestConformance(t *testing.T) {
	dalgotest.RunConformance(t, func(t *testing.T) (dal.DB, func()) {
		db, err := dalgo2fs.NewDB(t.TempDir())
		if err != nil {
			t.Fatalf("dalgo2fs.NewDB: %v", err)
		}
		return db, nil
	})
}
