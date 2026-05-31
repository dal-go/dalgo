package end2end

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

var runSingleAndMulti = true // test hook to optionally skip single/multi in specialized tests

// TestDalgoDB tests a dalgo DB implementation
func TestDalgoDB(t *testing.T, db dal.DB, errQuerySupport error, eventuallyConsistent bool) {
	if t == nil {
		panic("t == nil")
	}
	if db == nil {
		panic("db == nil")
	}

	ctx := context.Background()

	if runSingleAndMulti {
		t.Run("single", func(t *testing.T) {
			singleOperationsTest(ctx, t, db)
		})
		t.Run("multi", func(t *testing.T) {
			multiOperationsTest(ctx, t, db)
		})
	}

	t.Run("query", func(t *testing.T) {
		if errQuerySupport == nil {
			queryOperationsTest(ctx, t, db, eventuallyConsistent)
		} else {
			t.Skip("query not supported by dalgo driver or underlying DB:", errQuerySupport)
		}
	})
}
