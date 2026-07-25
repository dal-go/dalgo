package dal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// recordBearingWrite is one write operation that carries record data and so
// must be validated. Cases are named after the operation because the table's
// whole point is that no operation is left out — within-adapter inconsistency
// (validating Insert but not Set) is the exact bug this work exists to remove.
type recordBearingWrite struct {
	name  string
	write func(ctx context.Context, s dal.WriteSession, records ...record.Record) error
}

func recordBearingWrites() []recordBearingWrite {
	return []recordBearingWrite{
		{"Insert", func(ctx context.Context, s dal.WriteSession, r ...record.Record) error {
			return s.Insert(ctx, r[0])
		}},
		{"Set", func(ctx context.Context, s dal.WriteSession, r ...record.Record) error {
			return s.Set(ctx, r[0])
		}},
		{"UpdateRecord", func(ctx context.Context, s dal.WriteSession, r ...record.Record) error {
			return s.UpdateRecord(ctx, r[0], []update.Update{update.ByFieldName("Name", "x")})
		}},
		{"InsertMulti", func(ctx context.Context, s dal.WriteSession, r ...record.Record) error {
			return s.InsertMulti(ctx, r)
		}},
		{"SetMulti", func(ctx context.Context, s dal.WriteSession, r ...record.Record) error {
			return s.SetMulti(ctx, r)
		}},
	}
}

// inTransaction runs f against the transaction a framework DB hands its worker,
// which is the write path every caller actually uses.
func inTransaction(t *testing.T, backend *spyBackend, f func(ctx context.Context, tx dal.ReadwriteTransaction) error) error {
	t.Helper()
	return dal.NewDB(backend).RunReadwriteTransaction(context.Background(), f)
}

// TestPipelineRejectsInvalidRecordBeforeReachingTheBackend is the core claim of
// this work. If it goes red, adapters accept records their own data type says
// are illegal — which is how five bugs survived in a downstream framework.
func TestPipelineRejectsInvalidRecordBeforeReachingTheBackend(t *testing.T) {
	for _, tt := range recordBearingWrites() {
		t.Run(tt.name, func(t *testing.T) {
			backend := newSpyBackend()
			err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tt.write(ctx, tx, invalidRecord("t1"))
			})
			if !errors.Is(err, errInvalidTestRecord) {
				t.Fatalf("%s of an invalid record: err = %v, want the validation error", tt.name, err)
			}
			if len(backend.writes) != 0 {
				t.Fatalf("%s reached the backend despite failing validation: %v", tt.name, backend.writes)
			}
		})
	}
}

// TestPipelineWritesValidRecordThrough guards the other half: enforcement must
// not reject records that are legal. If it goes red, every write breaks.
func TestPipelineWritesValidRecordThrough(t *testing.T) {
	for _, tt := range recordBearingWrites() {
		t.Run(tt.name, func(t *testing.T) {
			backend := newSpyBackend()
			err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tt.write(ctx, tx, validRecord("t1"))
			})
			if err != nil {
				t.Fatalf("%s of a valid record: err = %v, want nil", tt.name, err)
			}
			if len(backend.writes) != 1 {
				t.Fatalf("%s reached the backend %d times, want exactly 1: %v", tt.name, len(backend.writes), backend.writes)
			}
		})
	}
}

// TestMultiRejectsWholeBatchWhenOnlyTheLastRecordIsInvalid is where partial
// failure hides: validating as it goes would write the first two records and
// then fail. If it goes red, a rejected batch is half-applied.
func TestMultiRejectsWholeBatchWhenOnlyTheLastRecordIsInvalid(t *testing.T) {
	multi := map[string]func(ctx context.Context, tx dal.ReadwriteTransaction, records []record.Record) error{
		"InsertMulti": func(ctx context.Context, tx dal.ReadwriteTransaction, r []record.Record) error {
			return tx.InsertMulti(ctx, r)
		},
		"SetMulti": func(ctx context.Context, tx dal.ReadwriteTransaction, r []record.Record) error {
			return tx.SetMulti(ctx, r)
		},
	}
	for name, write := range multi {
		t.Run(name, func(t *testing.T) {
			backend := newSpyBackend()
			batch := []record.Record{validRecord("t1"), validRecord("t2"), invalidRecord("t3")}
			err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return write(ctx, tx, batch)
			})
			if !errors.Is(err, errInvalidTestRecord) {
				t.Fatalf("%s: err = %v, want the validation error", name, err)
			}
			if len(backend.writes) != 0 {
				t.Fatalf("%s wrote part of a rejected batch: %v", name, backend.writes)
			}
		})
	}
}

// TestValidationErrorIsSurfacedNotAStorageError proves the caller can tell a
// rejected record from a failed write. If it goes red, callers retry writes
// that will never succeed (or give up on ones that would).
func TestValidationErrorIsSurfacedNotAStorageError(t *testing.T) {
	backend := newSpyBackend()
	backend.fail = errStorageFailed
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, invalidRecord("t1"))
	})
	if !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("err = %v, want the validation error", err)
	}
	if errors.Is(err, errStorageFailed) {
		t.Fatalf("err = %v, want the validation error and NOT the storage error", err)
	}
}

// TestRecordsWithoutValidatableDataAreUnaffected: enforcing an invariant on
// data that declares none would break every model that has no Validate method.
func TestRecordsWithoutValidatableDataAreUnaffected(t *testing.T) {
	backend := newSpyBackend()
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, plainRecord("t1"))
	})
	if err != nil {
		t.Fatalf("err = %v, want nil for data that does not implement dal.ValidatableRecord", err)
	}
	if len(backend.writes) != 1 {
		t.Fatalf("backend writes = %v, want exactly one Insert", backend.writes)
	}
}

// TestWithoutValidationBypassesValidation is the designed escape hatch. If it
// goes red, repair migrations and bulk imports have no sanctioned route and
// will reach for an unsanctioned one.
func TestWithoutValidationBypassesValidation(t *testing.T) {
	backend := newSpyBackend()
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return dal.WithoutValidation(tx).Insert(ctx, invalidRecord("t1"))
	})
	if err != nil {
		t.Fatalf("WithoutValidation Insert: err = %v, want nil", err)
	}
	if len(backend.writes) != 1 {
		t.Fatalf("backend writes = %v, want exactly one Insert", backend.writes)
	}
}

// TestWithoutValidationIsTheOnlyBypass: the same transaction, used normally,
// must still reject. If it goes red, WithoutValidation leaks into writes that
// did not ask for it.
func TestWithoutValidationIsTheOnlyBypass(t *testing.T) {
	backend := newSpyBackend()
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := dal.WithoutValidation(tx).Insert(ctx, invalidRecord("t1")); err != nil {
			return err
		}
		return tx.Insert(ctx, invalidRecord("t2"))
	})
	if !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("err = %v, want the validation error from the second, ordinary Insert", err)
	}
	if len(backend.writes) != 1 {
		t.Fatalf("backend writes = %v, want only the bypassed Insert", backend.writes)
	}
}

// TestWithoutValidationLeavesForeignSessionsUnchanged: a session the framework
// did not build has no validation to remove, and must not be silently replaced.
func TestWithoutValidationLeavesForeignSessionsUnchanged(t *testing.T) {
	foreign := &spyWriteSession{}
	if got := dal.WithoutValidation(foreign); got != dal.WriteSession(foreign) {
		t.Fatalf("WithoutValidation returned %T, want the same session back", got)
	}
}

// TestKeyOnlyWritesReachTheBackend: Update and Delete by key carry no data to
// validate. If it goes red, deletes and field updates stop working entirely.
func TestKeyOnlyWritesReachTheBackend(t *testing.T) {
	key := record.NewKeyWithID("things", "t1")
	for name, write := range map[string]func(ctx context.Context, tx dal.ReadwriteTransaction) error{
		"Update": func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.Update(ctx, key, []update.Update{update.ByFieldName("Name", "x")})
		},
		"UpdateMulti": func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.UpdateMulti(ctx, []*record.Key{key}, []update.Update{update.ByFieldName("Name", "x")})
		},
		"Delete": func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.Delete(ctx, key)
		},
		"DeleteMulti": func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.DeleteMulti(ctx, []*record.Key{key})
		},
	} {
		t.Run(name, func(t *testing.T) {
			backend := newSpyBackend()
			err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return write(ctx, tx)
			})
			if err != nil {
				t.Fatalf("%s: err = %v, want nil", name, err)
			}
			if len(backend.writes) != 1 {
				t.Fatalf("%s: backend writes = %v, want exactly one", name, backend.writes)
			}
		})
	}
}

// TestDatabaseLevelWritesGoThroughThePipeline covers backends that also write
// outside a transaction (dalgo2memory, dalgo2firestore). If it goes red,
// `db.(dal.WriteSession).Insert(...)` is a hole straight past validation.
func TestDatabaseLevelWritesGoThroughThePipeline(t *testing.T) {
	backend := newSpyBackend()
	db := dal.NewDB(backend)

	writes, ok := db.(dal.WriteSession)
	if !ok {
		t.Fatal("a DB over a write-capable backend must expose dal.WriteSession")
	}
	if err := writes.Insert(context.Background(), invalidRecord("t1")); !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("database-level Insert: err = %v, want the validation error", err)
	}
	if len(backend.writes) != 0 {
		t.Fatalf("database-level Insert reached the backend: %v", backend.writes)
	}
	if err := writes.Insert(context.Background(), validRecord("t1")); err != nil {
		t.Fatalf("database-level Insert of a valid record: err = %v, want nil", err)
	}
	if len(backend.writes) != 1 {
		t.Fatalf("backend writes = %v, want exactly one Insert", backend.writes)
	}
}

// TestDatabaseLevelWithoutValidation proves the escape hatch also reaches the
// non-transactional write surface.
func TestDatabaseLevelWithoutValidation(t *testing.T) {
	backend := newSpyBackend()
	db := dal.NewDB(backend)
	if err := dal.WithoutValidation(db.(dal.WriteSession)).Insert(context.Background(), invalidRecord("t1")); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(backend.writes) != 1 {
		t.Fatalf("backend writes = %v, want exactly one Insert", backend.writes)
	}
}

// TestReadOnlyBackendGetsNoWriteSurface: NewDB must not advertise writes an
// adapter cannot perform.
func TestReadOnlyBackendGetsNoWriteSurface(t *testing.T) {
	db := dal.NewDB(readOnlyBackend{})
	if _, ok := db.(dal.WriteSession); ok {
		t.Fatal("a DB over a read-only backend must not satisfy dal.WriteSession")
	}
}

// TestInsertRecordWithDataAndIDIsValidated: the framework's own insert helper —
// the one downstream frameworks actually call — must be on the pipeline. It is
// by construction, because it writes through the session; this test is what
// keeps that true.
func TestInsertRecordWithDataAndIDIsValidated(t *testing.T) {
	backend := newSpyBackend()
	key := record.NewKeyWithID("things", "t1")
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		_, err := dal.InsertRecordWithDataAndID(ctx, tx, key, "t1", &validatableData{})
		return err
	})
	if !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("InsertRecordWithDataAndID: err = %v, want the validation error", err)
	}
	if len(backend.writes) != 0 {
		t.Fatalf("InsertRecordWithDataAndID reached the backend: %v", backend.writes)
	}
}

// TestCollectionWriteTerminalsAreValidated: Collection[K,T]'s write terminals
// delegate to the session, so they inherit the pipeline. This is the test that
// keeps that inheritance from being quietly broken.
func TestCollectionWriteTerminalsAreValidated(t *testing.T) {
	backend := newSpyBackend()
	things := dal.CollectionAt[string, validatableData]("things")
	err := inTransaction(t, backend, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return things.SetByID(ctx, tx, "t1", validatableData{})
	})
	if !errors.Is(err, errInvalidTestRecord) {
		t.Fatalf("Collection.SetByID: err = %v, want the validation error", err)
	}
	if len(backend.writes) != 0 {
		t.Fatalf("Collection.SetByID reached the backend: %v", backend.writes)
	}
}
