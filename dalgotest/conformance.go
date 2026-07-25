// Package dalgotest provides a reusable conformance suite that asserts the
// record invariants DALgo enforces on the write path.
//
// A storage adapter runs it from its own tests:
//
//	func TestConformance(t *testing.T) {
//		dalgotest.RunConformance(t, func(t *testing.T) (dal.DB, func()) {
//			return myadapter.NewDB(), nil
//		})
//	}
//
// The suite is behavioural: it writes through a live database and asserts what
// a caller observes. It is the backstop for invariants the framework write
// pipeline cannot express by construction, and the thing that turns a
// divergence between two adapters from a silent difference into a failing
// build.
//
// An adapter that cannot perform a write at all (a read-only or stub adapter)
// still conforms: the suite accepts dal.ErrNotSupported and
// dal.ErrNotImplementedYet where it expects a write to succeed. What it never
// accepts is a storage error where a validation error was due — which is
// exactly what a non-validating adapter produces.
package dalgotest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// ErrInvalidRecord is returned by Record.Validate for a record the suite means
// to be rejected. Checks assert on it with errors.Is, so an adapter that
// happens to fail for an unrelated reason does not pass by accident.
var ErrInvalidRecord = errors.New("dalgotest: conformance record is invalid")

// Record is the record data the suite writes. It is invalid when Name is empty,
// so the validity of a fixture is visible in its literal.
type Record struct {
	Name string `json:"name"`
}

// Validate implements dal.ValidatableRecord.
func (r Record) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: Name is required", ErrInvalidRecord)
	}
	return nil
}

// Plain is record data that deliberately does not implement
// dal.ValidatableRecord, used to prove enforcement does not affect models that
// declare no invariants.
type Plain struct {
	Name string `json:"name"`
}

// Factory creates a database for a single conformance check. It returns the
// database and an optional cleanup function.
//
// An adapter that needs an external resource (a Firestore emulator, a SQL
// server) calls t.Skip from inside the factory when the resource is absent, so
// the suite is runnable everywhere and meaningful where the resource exists.
type Factory func(t *testing.T) (db dal.DB, cleanup func())

// DefaultCollection is where conformance records are written unless
// WithCollection says otherwise.
const DefaultCollection = "dalgotest_conformance"

type options struct {
	collection string
}

// Option configures the conformance suite.
type Option func(*options)

// WithCollection sets the collection conformance records are written to, for
// adapters whose schema restricts collection names.
func WithCollection(name string) Option {
	return func(o *options) { o.collection = name }
}

func newOptions(opts ...Option) options {
	o := options{collection: DefaultCollection}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Check is one named conformance assertion. Run returns nil when the database
// conforms and a descriptive error when it does not.
//
// Checks is exported alongside RunConformance so that a harness can assert a
// deliberately non-conforming database FAILS the suite. A conformance suite
// that cannot detect a non-conforming adapter does nothing, which is the
// failure mode this package exists to remove one layer down.
type Check struct {
	Name string
	Run  func(ctx context.Context, db dal.DB) error
}

// RunConformance runs every check against a fresh database from newDB, one
// subtest per check.
func RunConformance(t *testing.T, newDB Factory, opts ...Option) {
	t.Helper()
	if newDB == nil {
		panic("dalgotest: RunConformance requires a Factory")
	}
	for _, check := range Checks(opts...) {
		t.Run(check.Name, func(t *testing.T) {
			db, cleanup := newDB(t)
			if cleanup != nil {
				defer cleanup()
			}
			if err := check.Run(context.Background(), db); err != nil {
				t.Error(err)
			}
		})
	}
}

// Checks returns the conformance checks.
func Checks(opts ...Option) []Check {
	o := newOptions(opts...)
	s := suite{options: o}

	checks := []Check{
		{"leaves records without validatable data alone", s.plainRecordIsUnaffected},
		{"rejects an invalid record on Insert", s.rejects("insert-invalid", s.insert)},
		{"rejects an invalid record on Set", s.rejects("set-invalid", s.set)},
		{"rejects an invalid record on UpdateRecord", s.rejects("update-record-invalid", s.updateRecord)},
		{"rejects an invalid record on InsertMulti", s.rejects("insert-multi-invalid", s.insertMultiSingle)},
		{"rejects an invalid record on SetMulti", s.rejects("set-multi-invalid", s.setMultiSingle)},
		{"accepts a valid record on Insert", s.accepts("insert-valid", s.insert)},
		{"accepts a valid record on Set", s.accepts("set-valid", s.set)},
		{"accepts a valid record on UpdateRecord", s.accepts("update-record-valid", s.updateRecord)},
		{"accepts a valid record on InsertMulti", s.accepts("insert-multi-valid", s.insertMultiSingle)},
		{"accepts a valid record on SetMulti", s.accepts("set-multi-valid", s.setMultiSingle)},
		{"rejects a batch when only the last record is invalid on InsertMulti", s.rejectsBatch("insert-batch", s.insertMulti)},
		{"rejects a batch when only the last record is invalid on SetMulti", s.rejectsBatch("set-batch", s.setMulti)},
		{"writes nothing when a record is rejected", s.rejectedRecordIsNotPersisted},
		{"lets WithoutValidation bypass validation", s.withoutValidationBypasses},
		{"validates database-level writes too", s.databaseLevelWritesAreValidated},
	}
	return checks
}

type suite struct {
	options
}

func (s suite) key(id string) *record.Key {
	return record.NewKeyWithID(s.collection, id)
}

func (s suite) valid(id string) record.Record {
	return record.NewRecordWithData(s.key(id), &Record{Name: id})
}

func (s suite) invalid(id string) record.Record {
	return record.NewRecordWithData(s.key(id), &Record{})
}

// writeFunc performs one write operation on a session.
type writeFunc func(ctx context.Context, ws dal.WriteSession, records ...record.Record) error

func (s suite) insert(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.Insert(ctx, r[0])
}

func (s suite) set(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.Set(ctx, r[0])
}

// updateRecord seeds a valid record at the same key first, so the check
// exercises "update an existing record with this data" rather than tripping over
// a not-found. The seed is a write in its own right and goes through the
// pipeline like any other.
func (s suite) updateRecord(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	id, _ := r[0].Key().ID.(string)
	if err := ws.Set(ctx, s.valid(id)); err != nil && !unsupported(err) {
		return fmt.Errorf("seeding the record to update: %w", err)
	}
	return ws.UpdateRecord(ctx, r[0], []update.Update{update.ByFieldName("name", id)})
}

func (s suite) insertMulti(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.InsertMulti(ctx, r)
}

func (s suite) setMulti(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.SetMulti(ctx, r)
}

func (s suite) insertMultiSingle(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.InsertMulti(ctx, r[:1])
}

func (s suite) setMultiSingle(ctx context.Context, ws dal.WriteSession, r ...record.Record) error {
	return ws.SetMulti(ctx, r[:1])
}

// inTx runs write inside a read-write transaction, which is the write path
// every DALgo caller uses.
func inTx(ctx context.Context, db dal.DB, write func(ctx context.Context, tx dal.ReadwriteTransaction) error) error {
	return db.RunReadwriteTransaction(ctx, write, dal.TxWithMessage("dalgotest conformance"))
}

// unsupported reports whether err is an adapter saying "I cannot do this at
// all", which conformance tolerates where it expects a write to succeed.
func unsupported(err error) bool {
	return errors.Is(err, dal.ErrNotSupported) || errors.Is(err, dal.ErrNotImplementedYet)
}

func (s suite) rejects(id string, write writeFunc) func(context.Context, dal.DB) error {
	return func(ctx context.Context, db dal.DB) error {
		err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return write(ctx, tx, s.invalid(id))
		})
		if err == nil {
			return errors.New("an invalid record was accepted")
		}
		if !errors.Is(err, ErrInvalidRecord) {
			return fmt.Errorf("an invalid record failed with %v, want the validation error (dalgotest.ErrInvalidRecord)", err)
		}
		return nil
	}
}

func (s suite) accepts(id string, write writeFunc) func(context.Context, dal.DB) error {
	return func(ctx context.Context, db dal.DB) error {
		err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return write(ctx, tx, s.valid(id))
		})
		switch {
		case err == nil, unsupported(err):
			return nil
		case errors.Is(err, ErrInvalidRecord):
			return fmt.Errorf("a valid record was rejected as invalid: %v", err)
		default:
			return fmt.Errorf("a valid record failed with %v, want nil or an unsupported-operation error", err)
		}
	}
}

func (s suite) rejectsBatch(id string, write writeFunc) func(context.Context, dal.DB) error {
	return func(ctx context.Context, db dal.DB) error {
		first, second := s.valid(id+"-1"), s.valid(id+"-2")
		batch := []record.Record{first, second, s.invalid(id + "-3")}
		err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return write(ctx, tx, batch...)
		})
		if err == nil {
			return errors.New("a batch whose last record is invalid was accepted")
		}
		if !errors.Is(err, ErrInvalidRecord) {
			return fmt.Errorf("a batch with an invalid last record failed with %v, want the validation error", err)
		}
		// The valid records that preceded it must not have been written: the
		// batch is rejected whole, not half-applied.
		for _, r := range []record.Record{first, second} {
			written, ok, err := s.persisted(ctx, db, r.Key())
			if err != nil {
				return err
			}
			if ok && written {
				return fmt.Errorf("record %v from a rejected batch was written", r.Key())
			}
		}
		return nil
	}
}

// persisted reports whether key exists. The second result is false when the
// adapter cannot answer, in which case the caller skips the assertion.
func (s suite) persisted(ctx context.Context, db dal.DB, key *record.Key) (exists, known bool, err error) {
	exists, err = db.Exists(ctx, key)
	switch {
	case err == nil:
		return exists, true, nil
	case unsupported(err), record.IsNotFound(err):
		return false, false, nil
	default:
		return false, false, fmt.Errorf("Exists(%v) failed: %w", key, err)
	}
}

func (s suite) rejectedRecordIsNotPersisted(ctx context.Context, db dal.DB) error {
	key := s.key("not-persisted")
	err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, s.invalid("not-persisted"))
	})
	if !errors.Is(err, ErrInvalidRecord) {
		return fmt.Errorf("an invalid record was accepted by Insert with %v, want the validation error", err)
	}
	exists, known, err := s.persisted(ctx, db, key)
	if err != nil {
		return err
	}
	if known && exists {
		return errors.New("a record that failed validation was written to storage")
	}
	return nil
}

func (s suite) plainRecordIsUnaffected(ctx context.Context, db dal.DB) error {
	r := record.NewRecordWithData(s.key("plain"), &Plain{Name: "plain"})
	err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, r)
	})
	if err != nil && !unsupported(err) {
		return fmt.Errorf("a Set of data that does not implement dal.ValidatableRecord failed with %v", err)
	}
	return nil
}

func (s suite) withoutValidationBypasses(ctx context.Context, db dal.DB) error {
	err := inTx(ctx, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return dal.WithoutValidation(tx).Insert(ctx, s.invalid("without-validation"))
	})
	if errors.Is(err, ErrInvalidRecord) {
		return fmt.Errorf("dal.WithoutValidation did not bypass validation: %v", err)
	}
	if err != nil && !unsupported(err) {
		return fmt.Errorf("dal.WithoutValidation Insert failed with %v", err)
	}
	return nil
}

// databaseLevelWritesAreValidated covers adapters that also write outside a
// transaction. It passes trivially for adapters that do not.
func (s suite) databaseLevelWritesAreValidated(ctx context.Context, db dal.DB) error {
	ws, ok := db.(dal.WriteSession)
	if !ok {
		return nil
	}
	err := ws.Insert(ctx, s.invalid("db-level"))
	if err == nil {
		return errors.New("a database-level Insert accepted an invalid record")
	}
	if !errors.Is(err, ErrInvalidRecord) {
		return fmt.Errorf("a database-level Insert of an invalid record failed with %v, want the validation error", err)
	}
	return nil
}
