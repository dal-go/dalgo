package dalgotest_test

import (
	"context"
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
	if failed := runChecks(t, dalgo2memory.NewDB()); len(failed) != 0 {
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
	failed := runChecks(t, skippingDB{DB: dalgo2memory.NewDB()})
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
	failed := runChecks(t, insertOnlyValidatingDB{DB: dalgo2memory.NewDB()})

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

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle || strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}
