package dal

import (
	"context"
	"fmt"
	"sync"

	"github.com/dal-go/record"
)

// ValidatableRecord is implemented by record data that can check its own
// invariants. The framework write pipeline (see NewDB) calls Validate before a
// record reaches the storage adapter, so an adapter never gets the chance to
// skip it.
type ValidatableRecord interface {
	Validate() error
}

var (
	hooksMu sync.RWMutex

	// beforeSaveHooks run before a record-bearing write (Insert, Set,
	// UpdateRecord and their Multi variants).
	beforeSaveHooks []RecordHook

	// beforeDeleteHooks run before a delete. A delete carries no record data,
	// so it has nothing to validate, but it still participates in the pipeline.
	beforeDeleteHooks []RecordHook
)

// AddBeforeSaveHook registers hooks that the write pipeline calls before a
// record-bearing write reaches the storage adapter. Hooks run in registration
// order and the first error aborts the write, wrapped in ErrHookFailed.
//
// The registry is process-wide, which is why it is append-only: a package
// registers its hooks once at init and they apply to every DB built by NewDB.
func AddBeforeSaveHook(hooks ...RecordHook) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	beforeSaveHooks = append(beforeSaveHooks, hooks...)
}

// AddBeforeDeleteHook registers hooks that the write pipeline calls before a
// delete reaches the storage adapter. The hook receives a record carrying only
// the key being deleted; there is no data to validate.
func AddBeforeDeleteHook(hooks ...RecordHook) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	beforeDeleteHooks = append(beforeDeleteHooks, hooks...)
}

func registeredHooks(hooks *[]RecordHook) []RecordHook {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	return *hooks
}

// BeforeSave enforces the record invariants DALgo declares for a write that
// carries record data: the data is validated (when it implements
// ValidatableRecord) and then the registered before-save hooks run.
//
// It is the single enforcement point for those invariants. Callers do not
// normally invoke it: a DB built by NewDB runs it on every record-bearing write
// before delegating to the storage adapter.
func BeforeSave(ctx context.Context, db DB, r record.Record) error {
	if err := beforeSave(ctx, db, r); err != nil {
		return err
	}
	return callRecordHooks(ctx, r, registeredHooks(&beforeSaveHooks))
}

// BeforeDelete runs the registered before-delete hooks for key. A delete has no
// record data, so nothing is validated; the operation still participates in the
// pipeline so that the write path is uniform.
func BeforeDelete(ctx context.Context, _ DB, key *record.Key) error {
	hooks := registeredHooks(&beforeDeleteHooks)
	if len(hooks) == 0 {
		return nil
	}
	return callRecordHooks(ctx, record.NewRecord(key), hooks)
}

func callRecordHooks(ctx context.Context, r record.Record, hooks []RecordHook) error {
	for _, hook := range hooks {
		if err := hook(ctx, r); err != nil {
			return fmt.Errorf("%w: %v", ErrHookFailed, err)
		}
	}
	return nil
}

// beforeSave validates the record's data. It is spelled beforeSave, not
// beforeSafe: the typo it used to carry is the clearest evidence that this path
// was never executed.
func beforeSave(_ context.Context, _ DB, r record.Record) error {
	data := recordDataToValidate(r)
	if validatable, ok := data.(ValidatableRecord); ok {
		if err := validatable.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// recordDataToValidate returns the data carried by r.
//
// This used to recover from a panic. record.Record.Data() panics when the
// record's error has never been set — a deliberate guard against reading data
// before anyone has checked how the load went — and a record about to be
// written was landing in that state, so the pipeline caught the panic and
// cleared the error itself. That was working around the guard rather than
// respecting it.
//
// Fixed properly upstream in github.com/dal-go/record v0.1.1: a record built
// with caller-supplied data (NewRecordWithData, NewRecordWithoutKey) now sets
// ErrNoError at construction, because the caller supplied the data and there is
// no retrieval to wait for. NewRecord — an empty envelope awaiting a load —
// still guards its data, which is the half of the invariant worth keeping.
func recordDataToValidate(r record.Record) any {
	if r == nil {
		return nil
	}
	return r.Data()
}
