package dalgotest_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dalgotest"
	"github.com/dal-go/record"
)

// runChecks runs every conformance check against db and returns the names of
// the checks that failed. Checks is exported precisely so this is possible: a
// conformance suite that cannot fail proves nothing.
func runChecks(t *testing.T, db dal.DB) []string {
	t.Helper()
	var failed []string
	for _, check := range dalgotest.Checks() {
		if err := check.Run(context.Background(), db); err != nil {
			t.Logf("%s: %v", check.Name, err)
			failed = append(failed, check.Name)
		}
	}
	return failed
}

// TestSuitePassesAConformingAdapter guards against the opposite failure: a suite
// so strict that no real adapter can pass it is also useless.
func TestSuitePassesAConformingAdapter(t *testing.T) {
	if failed := runChecks(t, dalgo2memory.New(dalgo2memory.FirestoreProfile())); len(failed) != 0 {
		t.Fatalf("a conforming adapter failed %d checks: %v", len(failed), failed)
	}
}

// skippingDB is a deliberately non-conforming adapter: it hands the worker the
// raw backend transaction, bypassing the framework write pipeline entirely.
// This is exactly the shape dalgo2memory had before this change.
type skippingDB struct {
	dal.DB
}

func (db skippingDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	return dal.BackendOf(db.DB).RunReadwriteTransaction(ctx, f, options...)
}

// TestSuiteFailsAnAdapterThatSkipsValidation is the non-vacuity proof. If it
// goes red, the conformance suite is decorative and a non-validating adapter
// would ship green.
func TestSuiteFailsAnAdapterThatSkipsValidation(t *testing.T) {
	failed := runChecks(t, skippingDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())})
	if len(failed) == 0 {
		t.Fatal("the conformance suite passed an adapter that performs no validation at all")
	}
	// Every "rejects an invalid record on X" check must be among the failures:
	// each names one write operation, and an adapter that skips validation
	// fails all of them.
	for _, want := range []string{
		"rejects an invalid record on Insert",
		"rejects an invalid record on Set",
		"rejects an invalid record on UpdateRecord",
		"rejects an invalid record on InsertMulti",
		"rejects an invalid record on SetMulti",
		"rejects a batch when only the last record is invalid on InsertMulti",
		"rejects a batch when only the last record is invalid on SetMulti",
		"writes nothing when a record is rejected",
	} {
		if !contains(failed, want) {
			t.Errorf("check %q passed against a non-validating adapter", want)
		}
	}
}

// insertOnlyValidatingDB validates Insert but not Set or UpdateRecord — the
// exact within-adapter inconsistency dalgo2firestore had. A suite that only
// tested Insert would have passed it.
type insertOnlyValidatingDB struct {
	dal.DB
}

func (db insertOnlyValidatingDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	return dal.BackendOf(db.DB).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, insertOnlyTx{ReadwriteTransaction: tx})
	}, options...)
}

type insertOnlyTx struct {
	dal.ReadwriteTransaction
}

func (tx insertOnlyTx) Insert(ctx context.Context, r record.Record, opts ...dal.InsertOption) error {
	if err := validateInline(r); err != nil {
		return err
	}
	return tx.ReadwriteTransaction.Insert(ctx, r, opts...)
}

func validateInline(r record.Record) error {
	r.SetError(nil)
	if v, ok := r.Data().(dal.ValidatableRecord); ok {
		return v.Validate()
	}
	return nil
}

// TestSuiteFailsAnAdapterThatValidatesOnlyInsert is the second non-vacuity
// proof, aimed at the failure mode a naive suite misses: partial coverage
// inside one adapter.
func TestSuiteFailsAnAdapterThatValidatesOnlyInsert(t *testing.T) {
	failed := runChecks(t, insertOnlyValidatingDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())})

	if contains(failed, "rejects an invalid record on Insert") {
		t.Error("the Insert check failed against an adapter that does validate Insert")
	}
	for _, want := range []string{
		"rejects an invalid record on Set",
		"rejects an invalid record on UpdateRecord",
		"rejects an invalid record on SetMulti",
	} {
		if !contains(failed, want) {
			t.Errorf("check %q passed against an adapter that validates only Insert", want)
		}
	}
}

// duplicateAcceptingDB is a deliberately non-conforming adapter: its Insert
// never rejects a duplicate key at all — it silently behaves like Set. This is
// the most basic way an adapter can fail the new "rejects an Insert over an
// existing key" invariant.
type duplicateAcceptingDB struct {
	dal.DB
}

func (db duplicateAcceptingDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	return dal.BackendOf(db.DB).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, duplicateAcceptingTx{ReadwriteTransaction: tx})
	}, options...)
}

type duplicateAcceptingTx struct {
	dal.ReadwriteTransaction
}

func (tx duplicateAcceptingTx) Insert(ctx context.Context, r record.Record, opts ...dal.InsertOption) error {
	return tx.Set(ctx, r)
}

// TestSuiteFailsAnAdapterThatAcceptsDuplicateInserts is the non-vacuity proof
// for the "record already exists" invariant's most basic violation: an Insert
// that never rejects a duplicate key at all.
func TestSuiteFailsAnAdapterThatAcceptsDuplicateInserts(t *testing.T) {
	failed := runChecks(t, duplicateAcceptingDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())})
	if !contains(failed, "rejects an Insert over an existing key with record.IsAlreadyExists") {
		t.Fatal("the conformance suite passed an adapter whose Insert never rejects a duplicate key")
	}
}

// errUnclassifiedDuplicate is what unclassifiedDuplicateTx.Insert reports for
// a duplicate key — an ordinary error that does not wrap record.ErrRecordExists,
// exactly what an adapter looks like before it adopts the sentinel.
var errUnclassifiedDuplicate = errors.New("dalgotest: duplicate key (unclassified)")

// unclassifiedDuplicateDB is a deliberately non-conforming adapter: it DOES
// reject a duplicate Insert, but with a plain error that does not satisfy
// record.IsAlreadyExists. This is the subtler violation the check exists to
// catch — the exact "isDuplicate treats everything as a duplicate, or nothing
// is classified at all" gap that motivated ErrRecordExists/IsAlreadyExists in
// the first place.
type unclassifiedDuplicateDB struct {
	dal.DB
}

func (db unclassifiedDuplicateDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	return dal.BackendOf(db.DB).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, unclassifiedDuplicateTx{ReadwriteTransaction: tx})
	}, options...)
}

type unclassifiedDuplicateTx struct {
	dal.ReadwriteTransaction
}

func (tx unclassifiedDuplicateTx) Insert(ctx context.Context, r record.Record, opts ...dal.InsertOption) error {
	exists, err := tx.Exists(ctx, r.Key())
	if err != nil {
		return err
	}
	if exists {
		return errUnclassifiedDuplicate
	}
	return tx.ReadwriteTransaction.Insert(ctx, r, opts...)
}

// TestSuiteFailsAnAdapterWithAnUnclassifiedDuplicateError is the second
// non-vacuity proof: an adapter that does reject a duplicate key, but with an
// error the caller cannot tell apart from any other insert failure, must still
// fail the check — anything less would let dalgo2memory's old isDuplicate
// heuristic (any error that isn't ErrRecordNotFound) pass by accident.
func TestSuiteFailsAnAdapterWithAnUnclassifiedDuplicateError(t *testing.T) {
	failed := runChecks(t, unclassifiedDuplicateDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())})
	if !contains(failed, "rejects an Insert over an existing key with record.IsAlreadyExists") {
		t.Fatal("the conformance suite passed an adapter whose duplicate-key error does not satisfy record.IsAlreadyExists")
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle || strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}

// TestRunConformanceReportsFailures proves the testing.T wrapper turns a
// failing check into a failed subtest. A failing subtest would fail this
// package's own run, so the helper below executes in a subprocess whose
// coverage directory is forwarded, exactly as branchingtest does.
func TestRunConformanceReportsFailures(t *testing.T) {
	cmd := exec.Command(os.Args[0], append([]string{
		"-test.run=^TestRunConformanceFailureHelper$",
		"-test.count=1",
	}, coverageArguments()...)...)
	cmd.Env = append(os.Environ(), "DALGO_DALGOTEST_FAILURE=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("failure helper unexpectedly passed:\n%s", output)
	}
	for _, want := range []string{
		"--- FAIL: TestRunConformanceFailureHelper/rejects_an_invalid_record_on_Insert",
		"--- FAIL: TestRunConformanceFailureHelper/rejects_an_invalid_record_on_Set",
		"--- FAIL: TestRunConformanceFailureHelper/rejects_an_invalid_record_on_Update",
	} {
		if !strings.Contains(string(output), want) {
			t.Errorf("failure helper output did not contain %q:\n%s", want, output)
		}
	}
}

// TestRunConformanceFailureHelper runs the suite against the validation-
// skipping adapter and is expected to fail; it only runs as a subprocess of
// TestRunConformanceReportsFailures.
func TestRunConformanceFailureHelper(t *testing.T) {
	if os.Getenv("DALGO_DALGOTEST_FAILURE") == "" {
		t.Skip("only run as a subprocess by TestRunConformanceReportsFailures")
	}
	dalgotest.RunConformance(t, func(*testing.T) (dal.DB, func()) {
		return skippingDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())}, nil
	})
}

func coverageArguments() []string {
	var args []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.gocoverdir=") {
			args = append(args, arg)
		}
	}
	return args
}
