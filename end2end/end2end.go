package end2end

import (
	"context"
	"github.com/dal-go/dalgo/dal"
	"sync"
	"testing"
)

// TestDalgoDB tests a dalgo DB implementation
func TestDalgoDB(t *testing.T, db dal.Database) {
	if t == nil {
		panic("t == nil")
	}
	if db == nil {
		panic("db == nil")
	}

	ctx := context.Background()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		t.Run("single", func(t *testing.T) {
			testSingleOperations(ctx, t, db)
		})
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		t.Run("multi", func(t *testing.T) {
			testMultiOperations(ctx, t, db)
		})
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		t.Run("query", func(t *testing.T) {
			testQueryOperations(ctx, t, db)
		})
		wg.Done()
	}()

	wg.Wait()
}
