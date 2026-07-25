package dalgotest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2fs"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dalgotest"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// errStorage stands for any failure that is not a validation failure.
var errStorage = errors.New("dalgotest test: storage exploded")

// brokenDB is a database that misbehaves in exactly one configurable way, so
// every diagnostic the conformance suite can emit is produced by something. The
// suite is itself a test; these are its tests.
type brokenDB struct {
	dal.DB
	writeErr  error
	existsErr error
	exists    bool
}

func newBrokenDB(writeErr, existsErr error, exists bool) brokenDB {
	return brokenDB{DB: dalgo2memory.NewDB(), writeErr: writeErr, existsErr: existsErr, exists: exists}
}

func (db brokenDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return f(ctx, brokenTx{err: db.writeErr})
}

func (db brokenDB) Exists(context.Context, *record.Key) (bool, error) {
	return db.exists, db.existsErr
}

// brokenWriteDB additionally exposes the database-level write surface.
type brokenWriteDB struct {
	brokenDB
}

func (db brokenWriteDB) Insert(context.Context, record.Record, ...dal.InsertOption) error {
	return db.writeErr
}
func (db brokenWriteDB) InsertMulti(context.Context, []record.Record, ...dal.InsertOption) error {
	return db.writeErr
}
func (db brokenWriteDB) Set(context.Context, record.Record) error        { return db.writeErr }
func (db brokenWriteDB) SetMulti(context.Context, []record.Record) error { return db.writeErr }
func (db brokenWriteDB) Update(context.Context, *record.Key, []update.Update, ...dal.Precondition) error {
	return db.writeErr
}
func (db brokenWriteDB) UpdateRecord(context.Context, record.Record, []update.Update, ...dal.Precondition) error {
	return db.writeErr
}
func (db brokenWriteDB) UpdateMulti(context.Context, []*record.Key, []update.Update, ...dal.Precondition) error {
	return db.writeErr
}
func (db brokenWriteDB) Delete(context.Context, *record.Key) error        { return db.writeErr }
func (db brokenWriteDB) DeleteMulti(context.Context, []*record.Key) error { return db.writeErr }

var _ dal.WriteSession = brokenWriteDB{}

// brokenTx answers every operation with the configured error.
type brokenTx struct {
	err error
}

func (tx brokenTx) ID() string                      { return "broken" }
func (tx brokenTx) Options() dal.TransactionOptions { return dal.NewTransactionOptions() }
func (tx brokenTx) Get(context.Context, record.Record) error {
	return dal.ErrNotSupported
}
func (tx brokenTx) Exists(context.Context, *record.Key) (bool, error) {
	return false, dal.ErrNotSupported
}
func (tx brokenTx) GetMulti(context.Context, []record.Record) error { return dal.ErrNotSupported }
func (tx brokenTx) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (tx brokenTx) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}
func (tx brokenTx) Insert(context.Context, record.Record, ...dal.InsertOption) error { return tx.err }
func (tx brokenTx) InsertMulti(context.Context, []record.Record, ...dal.InsertOption) error {
	return tx.err
}
func (tx brokenTx) Set(context.Context, record.Record) error        { return tx.err }
func (tx brokenTx) SetMulti(context.Context, []record.Record) error { return tx.err }
func (tx brokenTx) Update(context.Context, *record.Key, []update.Update, ...dal.Precondition) error {
	return tx.err
}
func (tx brokenTx) UpdateRecord(context.Context, record.Record, []update.Update, ...dal.Precondition) error {
	return tx.err
}
func (tx brokenTx) UpdateMulti(context.Context, []*record.Key, []update.Update, ...dal.Precondition) error {
	return tx.err
}
func (tx brokenTx) Delete(context.Context, *record.Key) error        { return tx.err }
func (tx brokenTx) DeleteMulti(context.Context, []*record.Key) error { return tx.err }

var _ dal.ReadwriteTransaction = brokenTx{}

// failures maps check name to the error it reported.
func failures(db dal.DB) map[string]string {
	out := map[string]string{}
	for _, check := range dalgotest.Checks() {
		if err := check.Run(context.Background(), db); err != nil {
			out[check.Name] = err.Error()
		}
	}
	return out
}

// TestSuiteDiagnostics pins what the suite says about each way an adapter can be
// wrong. If a case goes red the suite has either stopped detecting that fault or
// started describing it as a different one, and a maintainer reading the failure
// would be sent to the wrong place.
func TestSuiteDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name  string
		db    dal.DB
		check string
		want  string
	}{
		{
			name:  "a storage error where a validation error was due is not mistaken for validation",
			db:    newBrokenDB(errStorage, nil, false),
			check: "rejects an invalid record on Insert",
			want:  "want the validation error",
		},
		{
			name:  "a valid record rejected as invalid is called out as such",
			db:    newBrokenDB(dalgotest.ErrInvalidRecord, nil, true),
			check: "accepts a valid record on Insert",
			want:  "a valid record was rejected as invalid",
		},
		{
			name:  "a valid record failing for any other reason is reported with that reason",
			db:    newBrokenDB(errStorage, nil, false),
			check: "accepts a valid record on Set",
			want:  "storage exploded",
		},
		{
			name:  "a rejected batch whose earlier records were written is caught",
			db:    newBrokenDB(dalgotest.ErrInvalidRecord, nil, true),
			check: "rejects a batch when only the last record is invalid on InsertMulti",
			want:  "from a rejected batch was written",
		},
		{
			name:  "an Exists that fails is surfaced rather than swallowed",
			db:    newBrokenDB(dalgotest.ErrInvalidRecord, errStorage, false),
			check: "writes nothing when a record is rejected",
			want:  "Exists(",
		},
		{
			name:  "a record that failed validation but was stored anyway is caught",
			db:    newBrokenDB(dalgotest.ErrInvalidRecord, nil, true),
			check: "writes nothing when a record is rejected",
			want:  "was written to storage",
		},
		{
			name:  "breaking data that declares no invariants is caught",
			db:    newBrokenDB(errStorage, nil, false),
			check: "leaves records without validatable data alone",
			want:  "does not implement dal.ValidatableRecord",
		},
		{
			name:  "a WithoutValidation that still validates is caught",
			db:    newBrokenDB(dalgotest.ErrInvalidRecord, nil, true),
			check: "lets WithoutValidation bypass validation",
			want:  "did not bypass validation",
		},
		{
			name:  "a WithoutValidation that fails for another reason is reported with it",
			db:    newBrokenDB(errStorage, nil, false),
			check: "lets WithoutValidation bypass validation",
			want:  "storage exploded",
		},
		{
			name:  "a database-level write that accepts an invalid record is caught",
			db:    brokenWriteDB{brokenDB: newBrokenDB(nil, nil, false)},
			check: "validates database-level writes too",
			want:  "accepted an invalid record",
		},
		{
			name:  "a database-level write failing for a storage reason is not mistaken for validation",
			db:    brokenWriteDB{brokenDB: newBrokenDB(errStorage, nil, false)},
			check: "validates database-level writes too",
			want:  "want the validation error",
		},
		{
			name:  "a seed write that fails takes the UpdateRecord check with it",
			db:    newBrokenDB(errStorage, nil, false),
			check: "rejects an invalid record on UpdateRecord",
			want:  "want the validation error",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := failures(tt.db)[tt.check]
			if !ok {
				t.Fatalf("check %q passed; it should have failed", tt.check)
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("check %q reported %q, want it to mention %q", tt.check, got, tt.want)
			}
		})
	}
}

// TestSuiteToleratesAnAdapterThatCannotAnswerExists: an adapter whose Exists is
// unsupported must not fail a check that only uses it to strengthen an
// assertion. Here the rejection itself is correct, so the check must pass.
func TestSuiteToleratesAnAdapterThatCannotAnswerExists(t *testing.T) {
	db := newBrokenDB(dalgotest.ErrInvalidRecord, dal.ErrNotSupported, false)
	if got, ok := failures(db)["writes nothing when a record is rejected"]; ok {
		t.Fatalf("an unanswerable Exists was reported as a conformance failure: %s", got)
	}
}

// TestRunConformanceRejectsAMissingFactory: a nil factory is a programming
// error, and failing at the call site beats a nil dereference inside a subtest.
func TestRunConformanceRejectsAMissingFactory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RunConformance(t, nil) did not panic")
		}
	}()
	dalgotest.RunConformance(t, nil)
}

// TestSuitePassesAReadOnlyAdapter: an adapter that cannot write at all still
// conforms. dalgo2fs is the in-repo example, and it is the sharpest proof that
// validation runs before the adapter: a validation error from a database with no
// working write method can only have come from the framework.
func TestSuitePassesAReadOnlyAdapter(t *testing.T) {
	db, err := dalgo2fs.NewDB(t.TempDir())
	if err != nil {
		t.Fatalf("dalgo2fs.NewDB: %v", err)
	}
	if failed := failures(db); len(failed) != 0 {
		t.Fatalf("a read-only adapter failed %d checks: %v", len(failed), failed)
	}
}

// TestWithCollectionRedirectsWrites lets an adapter whose schema restricts
// collection names run the suite at all.
func TestWithCollectionRedirectsWrites(t *testing.T) {
	const collection = "custom_conformance"
	db := dalgo2memory.NewDB(dalgo2memory.WithSchema(false,
		dalgo2memory.WithCollection[dalgotest.Record](collection, nil),
	))
	for _, check := range dalgotest.Checks(dalgotest.WithCollection(collection)) {
		if err := check.Run(context.Background(), db); err != nil {
			t.Errorf("%s: %v", check.Name, err)
		}
	}
	// And the default collection is refused by that schema, which is what makes
	// the option load-bearing rather than decorative.
	var defaulted int
	for _, check := range dalgotest.Checks() {
		if err := check.Run(context.Background(), db); err != nil {
			defaulted++
		}
	}
	if defaulted == 0 {
		t.Fatal("the schema accepted the default collection; WithCollection proves nothing here")
	}
}

// TestRunConformanceDrivesTheChecks covers the testing.T wrapper itself,
// including a factory that returns a cleanup function and one that does not.
func TestRunConformanceDrivesTheChecks(t *testing.T) {
	t.Run("with cleanup", func(t *testing.T) {
		var cleaned int
		dalgotest.RunConformance(t, func(*testing.T) (dal.DB, func()) {
			return dalgo2memory.NewDB(), func() { cleaned++ }
		})
		if cleaned != len(dalgotest.Checks()) {
			t.Fatalf("cleanup ran %d times, want once per check (%d)", cleaned, len(dalgotest.Checks()))
		}
	})
	t.Run("without cleanup", func(t *testing.T) {
		dalgotest.RunConformance(t, func(*testing.T) (dal.DB, func()) {
			return dalgo2memory.NewDB(), nil
		})
	})
}
