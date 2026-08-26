package dalgo2memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dalgotest"
	"github.com/dal-go/record"
)

// TestConformance is the headline proof of framework-enforced record
// invariants: dalgo2memory never validated anything, and it now passes the
// shared suite without a line of validation code of its own.
func TestConformance(t *testing.T) {
	dalgotest.RunConformance(t, func(*testing.T) (dal.DB, func()) {
		return dalgo2memory.New(dalgo2memory.FirestoreProfile()), nil
	})
}

// errMemoryTestInvalid is what the fixture below reports.
var errMemoryTestInvalid = errors.New("dalgo2memory test: record is invalid")

type validatedThing struct {
	Name string `json:"name"`
}

func (t validatedThing) Validate() error {
	if t.Name == "" {
		return errMemoryTestInvalid
	}
	return nil
}

// TestInvalidRecordIsRejectedAndNotStored is the before/after regression test
// for this adapter specifically. Before framework-enforced invariants this
// exact code returned nil and the record was readable afterwards, which is why
// validation bugs in downstream frameworks were invisible in tests and live
// only in production.
func TestInvalidRecordIsRejectedAndNotStored(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	key := record.NewKeyWithID("things", "t1")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record.NewRecordWithData(key, &validatedThing{}))
	})
	if !errors.Is(err, errMemoryTestInvalid) {
		t.Fatalf("Insert of an invalid record: err = %v, want the data type's validation error", err)
	}

	exists, err := db.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("the invalid record was persisted")
	}
}

// TestValidRecordIsStored is the other half: enforcement must not break the
// ordinary path.
func TestValidRecordIsStored(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	key := record.NewKeyWithID("things", "t1")

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record.NewRecordWithData(key, &validatedThing{Name: "milk"}))
	})
	if err != nil {
		t.Fatalf("Insert of a valid record: err = %v, want nil", err)
	}
	exists, err := db.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("a valid record was not persisted")
	}
}
