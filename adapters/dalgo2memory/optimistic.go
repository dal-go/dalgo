package dalgo2memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// ErrTransactionConflict is returned by RunReadwriteTransaction, for a
// database created with WithOptimisticConcurrency, when another transaction
// committed a write to a key this transaction read or wrote before this
// transaction reached its own commit. A caller that wants to retry should
// test for it with IsTransactionConflict rather than a direct comparison,
// since the error returned always wraps additional diagnostic context.
//
// Before adding this, both the dal and record packages (dalgo's own error
// vocabulary) were searched for an existing conflict / retryable / aborted
// error type or predicate to reuse instead — dalgo2mysql, dalgo2sql,
// dalgo2postgres and dalgo2sqlite were checked too, in case a convention
// already existed for a backend that has real serialization failures. None of
// them define one. This sentinel-plus-predicate pair follows the same idiom
// this package already uses for ErrReadAfterWriteInTransaction (a checked
// sentinel) and that the record package uses for
// ErrRecordNotFound/IsNotFound (a predicate function) — see IsTransactionConflict.
var ErrTransactionConflict = errors.New("dalgo2memory: transaction conflict: another transaction committed a write to a key this transaction read or wrote")

// IsTransactionConflict reports whether err is, or wraps, ErrTransactionConflict.
// A caller of a WithOptimisticConcurrency database's RunReadwriteTransaction
// should use this to decide whether a failed transaction is safe to retry.
func IsTransactionConflict(err error) bool {
	return err != nil && errors.Is(err, ErrTransactionConflict)
}

// optimisticState buffers one read-write transaction's reads and writes when
// the owning database runs with WithOptimisticConcurrency, instead of the
// whole-database lock RunReadwriteTransaction otherwise holds for the
// callback's entire duration.
//
// Nothing here touches a storage engine until commit (see the commit method),
// and that is not merely an optimization: it is what makes the failure mode
// correct. Consider two transactions racing to claim the same slug with
// Get-then-Insert. If Insert instead wrote through to the engine immediately
// (as the default mode's session.save does), the loser would either get
// whatever "duplicate" error the engine happens to raise — which depends on
// write ordering, not on what either transaction actually observed — or, if
// it happened to write first and got overtaken afterwards, its write would be
// visible to any other reader for real before anyone had detected the
// conflict, and undoing it would need its own undo log that the storage
// engines don't have. Deferring every write to a single, lock-guarded commit
// step sidesteps both problems: nothing is visible until it is valid, and the
// side that loses the race always gets ErrTransactionConflict specifically,
// which is the error a caller can actually retry on.
//
// The trade-off this buys: within one transaction, the immediate return value
// of Get/Exists/Set/Insert/Update/Delete only ever reflects what that same
// transaction has itself observed or written (see touch) — never a
// concurrent transaction's uncommitted work, and, for a key this transaction
// has not yet touched, never storage-fidelity validation a write's data would
// eventually fail (see validateForCommit's doc comment for the narrow gap
// that remains there). Existence-driven outcomes (Insert's duplicate check,
// Update's not-found check) are still resolved eagerly and correctly against
// this transaction's own view, exactly as they are in the default mode — the
// only thing that moves to commit time is telling this transaction whether
// its view was still valid when it finished.
//
// Reads are SNAPSHOT reads, matching Firestore: every value a transaction
// observes belongs to the single committed database state that existed at the
// transaction's first observation (startSeq). A key committed after that
// moment cannot be served without fracturing the view — handing the callback
// values from two different committed states, a mix that never existed and
// that Firestore never shows — so touching such a key fails with
// ErrTransactionConflict from the read itself, and poisons the commit in case
// the callback swallows the error (see observeAtSnapshot). No data is ever
// cloned to achieve this: as long as nothing observed has been overwritten,
// the live store IS the snapshot, and the moment that stops being true the
// transaction aborts and is retried instead of being served history.
type optimisticState struct {
	db *database

	// reads is this transaction's read set: the commit version observed the
	// first time each key was touched — by a read OR a write, see touch.
	// commit fails the transaction if any of these versions has advanced by
	// the time it runs.
	reads map[string]uint64

	// pending is this transaction's local, mutable view of every key it has
	// touched so far, keyed by conflictKey. See pendingEntry and touch.
	pending map[string]*pendingEntry

	// ownerHoldsLock is true when the transaction owning this state already
	// holds db.mu for its whole duration — the default (whole-database lock)
	// mode, see runLockedReadwriteTransaction. Every method below reaches the
	// shared store through lock/unlock rather than db.mu directly, because
	// sync.Mutex is not reentrant: taking db.mu again from inside a
	// default-mode transaction would deadlock instantly. In optimistic mode
	// this stays false and lock/unlock take the mutex for real, one short
	// critical section per operation, exactly as before this field existed.
	ownerHoldsLock bool

	// validateVersions is true only in optimistic mode, where commit checks
	// this transaction's read set against the live commit-version table and
	// stamps the keys it writes with a fresh commit sequence. The default
	// locked mode leaves it false: it holds db.mu for the callback's entire
	// duration, so no other transaction can interleave and there is nothing to
	// validate — and skipping the bookkeeping keeps db.versions empty rather
	// than growing it with entries nothing ever reads.
	validateVersions bool

	// startSeq is the database commit sequence this transaction reads at:
	// every value it observes belongs to the committed state that existed at
	// startSeq. It is pinned LAZILY, at the transaction's first observing
	// touch (a Get, Exists, Insert's duplicate check, or Update's merge —
	// never a blind Set/Delete, which observes nothing), rather than when the
	// transaction starts: Firestore's effective read time is likewise its
	// first read's, and pinning earlier would abort transactions that merely
	// began before an unrelated commit. Zero and meaningless until
	// startSeqPinned; guarded by db.mu like everything else here.
	startSeq       uint64
	startSeqPinned bool

	// conflict records the first snapshot violation an observing touch
	// detected (see observeAtSnapshot). Once set, the transaction is
	// poisoned: every later touch returns it and commit refuses, so a
	// callback that swallows the read error cannot commit work derived from a
	// fractured view. The real Firestore client gives the same protection by
	// failing the commit of a transaction whose read came back ABORTED.
	conflict error
}

// observeAtSnapshot enforces this transaction's read snapshot for one key
// about to be observed for the first time. On the transaction's very first
// observation it pins startSeq to the database's current commit sequence —
// the moment that defines which committed state this transaction reads. On
// every later first observation it checks whether the key was committed
// AFTER startSeq: if so, no value can be served without fracturing the view
// (mixing values from two different committed states — the read-committed
// anomaly this mechanism exists to kill), so it poisons the transaction and
// returns ErrTransactionConflict from the read itself. The real Firestore
// client behaves the same way: a read inside a transaction can come back
// ABORTED, and the SDK's retry loop re-runs the whole callback.
//
// Blind writes (Set, Delete) never come through here: they observe nothing,
// and Firestore's blind writes likewise do not conflict on the written key's
// prior state. Their write-write races are still caught by commit's
// validation against the baseline firstTouchEntry records at touch time.
//
// The caller must already hold db.mu (it is touch's own precondition), so
// reading db.commitSeq and db.versions here is synchronized with every
// commit's stamping.
func (tx *optimisticState) observeAtSnapshot(ck string) error {
	if !tx.startSeqPinned {
		tx.startSeqPinned = true
		tx.startSeq = tx.db.commitSeq
		return nil
	}
	if seq := tx.db.versions[ck]; seq > tx.startSeq {
		tx.conflict = fmt.Errorf(
			"%w: key %q was committed at sequence %d, after this transaction's read snapshot (sequence %d)",
			ErrTransactionConflict, ck, seq, tx.startSeq)
		return tx.conflict
	}
	return nil
}

// lock acquires db.mu unless the owning transaction already holds it for its
// whole duration (the default locked mode). See ownerHoldsLock.
func (tx *optimisticState) lock() {
	if !tx.ownerHoldsLock {
		tx.db.mu.Lock()
	}
}

// unlock releases db.mu unless the owning transaction holds it for its whole
// duration (the default locked mode). See ownerHoldsLock.
func (tx *optimisticState) unlock() {
	if !tx.ownerHoldsLock {
		tx.db.mu.Unlock()
	}
}

// hasBufferedWrites reports whether this transaction has staged a write that
// has not yet reached the shared store. A query scans committed storage
// directly, so it cannot see such writes; ExecuteQueryToRecordsReader consults
// this to refuse the query rather than silently return a stale result.
//
// The caller must already hold db.mu (every caller runs inside a transaction
// method that does), so this reads tx.pending directly rather than locking.
func (tx *optimisticState) hasBufferedWrites() bool {
	for _, entry := range tx.pending {
		if entry.commit != nil {
			return true
		}
	}
	return false
}

// pendingEntry is one key's transaction-local view: the outcome of every
// Get/Exists/Set/Insert/Update/Delete this transaction has issued for the
// key, kept current so a later operation on the same key in the same
// transaction sees this transaction's own uncommitted writes (Firestore
// transactions behave the same way: a read after a write in the same
// transaction sees the write).
type pendingEntry struct {
	// key is the full key (with its parent chain) this entry was first
	// touched through.
	key *record.Key
	// present and data together are this transaction's current view of the
	// key's value: present is false for a key this transaction has observed
	// as absent (never stored, or deleted within this transaction); when
	// present, data is the fully decoded row, JSON-normalized the same way
	// normalizeData produces it.
	present bool
	data    map[string]any
	// commit describes how to apply this entry to the real storage engine at
	// commit time, reflecting only the LAST write this transaction made to
	// the key — an Update layered on an earlier Set in the same transaction
	// collapses to just the net resolved value, since only the final state is
	// ever visible to anyone once this transaction commits. It is nil for a
	// key this transaction has only read.
	commit *pendingWrite
}

// pendingWriteKind distinguishes the two things commit can do to a key.
type pendingWriteKind int

const (
	pendingWriteStore pendingWriteKind = iota
	pendingWriteDelete
)

// pendingWrite is how commit replays one key's net write against the real
// storage engine. rec/overwrite are only meaningful for pendingWriteStore.
type pendingWrite struct {
	kind      pendingWriteKind
	rec       record.Record
	overwrite bool
}

// conflictKey identifies a stored record for optimistic-concurrency version
// tracking. It is collection-prefixed because keyID's own doc comment already
// notes that a root-level id is not namespaced by collection on its own (two
// different collections can share a leaf id); a nested record's keyID already
// carries its full collection/id chain, so the prefix is redundant there but
// harmless.
func conflictKey(collection, id string) string {
	return collection + "\x00" + id
}

// runOptimisticReadwriteTransaction is RunReadwriteTransaction's
// WithOptimisticConcurrency path — see optimisticState's doc comment for the
// design. It never holds db.mu for the callback's duration, only briefly, once
// per read/write operation, and once more (also briefly) for the final
// commit, so unrelated transactions' callbacks run fully concurrently: a
// transaction can even pause indefinitely between operations (as a test
// driving a specific interleaving does) without blocking anyone else, since
// it is not holding any lock while paused.
func (db *database) runOptimisticReadwriteTransaction(ctx context.Context, f dal.RWTxWorker) error {
	tx := &optimisticState{
		db:               db,
		reads:            make(map[string]uint64),
		pending:          make(map[string]*pendingEntry),
		validateVersions: true,
	}
	txState := &transactionState{
		noReadsAfterWrites: db.noReadsAfterWritesInTransaction,
		optimistic:         tx,
	}
	s := session{db: db, txState: txState}
	// Stamp transactionInProgressKey on the ctx handed to f — not the ctx this
	// function received, which RunReadwriteTransaction already checked for that
	// key before dispatching here — so a nested RunReadonlyTransaction or
	// RunReadwriteTransaction call made from inside this callback is rejected by
	// ErrNestedTransaction instead of silently running as an independent,
	// concurrently-committing transaction (see that error's doc comment).
	if err := f(context.WithValue(ctx, transactionInProgressKey{}, true), s); err != nil {
		return err
	}
	if txState.readAfterWriteRejected {
		// The callback swallowed the read's ErrReadAfterWriteInTransaction and
		// returned nil, but the violation still poisons the transaction — see
		// transactionState.readAfterWriteRejected's doc comment for the
		// Firestore-client parity this matches. Refusing the commit here, rather
		// than calling tx.commit(), is what discards the buffered writes.
		return fmt.Errorf("%w: commit refused because a read-after-write was rejected earlier in this transaction", ErrReadAfterWriteInTransaction)
	}
	return tx.commit()
}

// runLockedReadwriteTransaction is RunReadwriteTransaction's default path: it
// holds db.mu for the callback's entire duration, so read-write transactions
// are fully serialized and cannot contend (see WithOptimisticConcurrency for
// the mode where they can).
//
// Writes are nonetheless buffered through the same optimisticState machinery
// rather than applied to the storage engines as they are made, because that is
// what makes a transaction ATOMIC: Firestore discards every write of a
// transaction whose callback returns an error, and before this the in-memory
// adapter left them behind, so a failed transaction committed its partial work
// for real. Returning early below — without calling commit — is the whole
// rollback mechanism: nothing was ever written, so there is nothing to undo.
//
// No read-overlay logic is needed to make the buffer invisible, because the
// default noReadsAfterWritesInTransaction rule already rejects any read that
// follows a write in the same transaction: a read can only happen while the
// buffer is still empty. The one configuration that can observe the buffer is
// WithInterleavedReadsAndWritesInTransaction, and there reads DO consult it —
// every read goes through optimisticState.touch, which returns this
// transaction's own pending view of a key it has written. The single case
// buffering cannot serve is a QUERY issued after a write, since a query scans
// committed storage rather than resolving keys through touch; that is refused
// explicitly rather than answered with a stale result (see
// session.ExecuteQueryToRecordsReader).
func (db *database) runLockedReadwriteTransaction(ctx context.Context, f dal.RWTxWorker) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx := &optimisticState{
		db:      db,
		reads:   make(map[string]uint64),
		pending: make(map[string]*pendingEntry),
		// The callback runs with db.mu held for its whole duration, so the
		// per-operation locking inside optimisticState must be suppressed
		// (sync.Mutex is not reentrant), and there is no concurrent commit to
		// validate a read set against.
		ownerHoldsLock:   true,
		validateVersions: false,
	}
	txState := &transactionState{
		noReadsAfterWrites: db.noReadsAfterWritesInTransaction,
		optimistic:         tx,
	}
	s := session{db: db, txState: txState}
	// See the identical comment in runOptimisticReadwriteTransaction: this
	// stamps the ctx handed to f, not the ctx RunReadwriteTransaction already
	// checked, so a nested transaction call from inside this callback is
	// rejected by ErrNestedTransaction instead of deadlocking on db.mu, which
	// this function holds for the callback's entire duration (sync.RWMutex is
	// not reentrant).
	if err := f(context.WithValue(ctx, transactionInProgressKey{}, true), s); err != nil {
		return err // buffered writes are discarded: this IS the rollback
	}
	if txState.readAfterWriteRejected {
		// The callback swallowed the read's ErrReadAfterWriteInTransaction and
		// returned nil, but the violation still poisons the transaction — see
		// transactionState.readAfterWriteRejected's doc comment for the
		// Firestore-client parity this matches. Refusing the commit here, rather
		// than calling tx.commit(), is what discards the buffered writes.
		return fmt.Errorf("%w: commit refused because a read-after-write was rejected earlier in this transaction", ErrReadAfterWriteInTransaction)
	}
	return tx.commit()
}

// bumpConflictVersion stamps one key with a fresh commit sequence when this
// database runs with WithOptimisticConcurrency, so an in-flight optimistic
// transaction correctly treats a write made outside any transaction as a
// conflicting external commit — both at its own commit (baseline validation)
// and at read time (observeAtSnapshot sees the key committed after its
// snapshot). Each top-level write is its own single-key commit, so it
// advances db.commitSeq by one and stamps the key with the new value. (A
// default-mode transaction never reaches this: RunReadwriteTransaction routes
// every transaction through runOptimisticReadwriteTransaction instead, once
// optimisticConcurrency is set, so the only caller left here is a top-level,
// non-transactional write.) It is a no-op — adding no locking or bookkeeping
// cost — for a database created without WithOptimisticConcurrency, keeping
// the default path exactly as it was before this mode existed.
//
// Callers must already hold db.mu: every call site (session.save, the
// non-optimistic session.Delete and session.UpdateRecord) does, since all
// three only ever run inside a top-level method or a default-mode
// transaction, both of which hold it for the call's duration.
func (db *database) bumpConflictVersion(collection, id string) {
	if !db.optimisticConcurrency {
		return
	}
	db.commitSeq++
	db.versions[conflictKey(collection, id)] = db.commitSeq
}

// firstTouchEntry returns key's pendingEntry, creating it — and recording its
// commit-version baseline in reads — on first contact. isNew reports whether
// this call created it, which touch uses to decide whether the entry still
// needs to be populated from the live engine. The caller must already hold
// db.mu, so that creating the entry and snapshotting the live commit version
// happen atomically with respect to any other transaction's commit.
//
// A key's first contact establishes this transaction's baseline for it
// whether that contact is a read or a write, matching the rule that a
// transaction conflicts on a key it wrote just as much as one it only read.
func (tx *optimisticState) firstTouchEntry(collection, id string, key *record.Key) (entry *pendingEntry, isNew bool) {
	ck := conflictKey(collection, id)
	if existing, ok := tx.pending[ck]; ok {
		return existing, false
	}
	if _, ok := tx.reads[ck]; !ok {
		tx.reads[ck] = tx.db.versions[ck] // implicit 0 for a key with no committed write yet
	}
	entry = &pendingEntry{key: key}
	tx.pending[ck] = entry
	return entry, true
}

// touch is the read-side entry point into a key's transaction-local view: it
// returns the pendingEntry, loading it from the live engine on first contact
// so this and later reads in the same transaction see a stable,
// JSON-normalized value without going back to the engine again. Get, Exists
// (via read), Insert's duplicate check, and Update's merge all go through it.
// A later call for the same key, from any operation, always reuses the
// existing entry instead — that is both what gives read-your-own-writes
// within one transaction even though nothing has reached the shared store,
// and what keeps a re-read of the same key snapshot-stable after another
// transaction commits over it (commit's baseline validation then reports the
// conflict). A write that does not need the prior value (Set, Delete) uses
// firstTouchEntry directly instead, since it can never fail.
//
// A key touched for the first time is checked against the transaction's read
// snapshot BEFORE any entry is created (see observeAtSnapshot): a key
// committed after startSeq fails right here, leaving no half-initialized
// entry behind for a later same-transaction read to mistake for "absent".
// The caller must already hold db.mu.
func (tx *optimisticState) touch(collection, id string, key *record.Key) (*pendingEntry, error) {
	if tx.conflict != nil {
		// The transaction is already poisoned by an earlier snapshot
		// violation; no further observation can produce a usable result, and
		// commit will refuse regardless.
		return nil, tx.conflict
	}
	ck := conflictKey(collection, id)
	if existing, ok := tx.pending[ck]; ok {
		return existing, nil
	}
	if err := tx.observeAtSnapshot(ck); err != nil {
		return nil, err
	}
	entry, _ := tx.firstTouchEntry(collection, id, key)
	eng := tx.db.engine(collection)
	var data map[string]any
	target := record.NewRecordWithData(key, &data)
	switch err := eng.load(id, target); {
	case err == nil:
		entry.present = true
		entry.data = data
	case record.IsNotFound(err):
		// leave entry.present at its zero value (false)
	default:
		return nil, err
	}
	return entry, nil
}

// read performs one read-side touch: it takes db.mu, resolves the key's
// pendingEntry, and releases db.mu again before returning. Both session.Get
// and session.Exists go through it.
func (tx *optimisticState) read(collection, id string, key *record.Key) (*pendingEntry, error) {
	tx.lock()
	defer tx.unlock()
	return tx.touch(collection, id, key)
}

// get implements session.Get for an optimistic-mode transaction.
func (tx *optimisticState) get(rec record.Record) error {
	key := rec.Key()
	collection := key.Collection()
	if err := tx.db.guardCollection(collection); err != nil {
		rec.SetError(err)
		return err
	}
	entry, err := tx.read(collection, keyID(key), key)
	if err != nil {
		rec.SetError(err)
		return err
	}
	if !entry.present {
		notFoundErr := dal.NewErrNotFoundByKey(key, nil)
		rec.SetError(notFoundErr)
		return notFoundErr
	}
	rec.SetError(nil)
	if err := materializeInto(entry.data, rec.Data()); err != nil {
		rec.SetError(err)
		return err
	}
	return nil
}

// exists implements session.Exists for an optimistic-mode transaction. Like
// the default mode's session.Exists, it performs no schema-guard check.
func (tx *optimisticState) exists(key *record.Key) (bool, error) {
	entry, err := tx.read(key.Collection(), keyID(key), key)
	if err != nil {
		return false, err
	}
	return entry.present, nil
}

// stage implements session.Set for an optimistic-mode transaction: rec's data
// becomes this transaction's resolved value for its key, replacing whatever
// this transaction observed there before (an earlier read, or an earlier
// write of its own). Nothing reaches the shared storage engine until commit —
// see optimisticState's doc comment. Unlike insert, staging a Set can never
// fail once its data has validated, since Set has no duplicate check to
// consult the key's prior value for.
func (tx *optimisticState) stage(rec record.Record) error {
	key := rec.Key()
	collection := key.Collection()
	factory, err := tx.db.recordFactory(collection)
	if err != nil {
		rec.SetError(err)
		return err
	}
	data, err := normalizeData(rec.Data())
	if err != nil {
		rec.SetError(err)
		return err
	}
	if err := validateForCommit(collection, factory, data); err != nil {
		rec.SetError(err)
		return err
	}

	tx.lock()
	entry, _ := tx.firstTouchEntry(collection, keyID(key), key)
	entry.present = true
	entry.data = data
	entry.commit = &pendingWrite{kind: pendingWriteStore, rec: rec, overwrite: true}
	tx.unlock()

	rec.SetError(nil)
	return nil
}

// insert implements session.Insert for an optimistic-mode transaction.
// Unlike stage, it first consults this transaction's own view of the key
// (via touch, which loads from the live engine only on the key's first
// contact) and fails immediately, with the same "record already exists"
// shape the engines themselves use, if this transaction has already observed
// the key as present. That keeps a same-transaction duplicate (e.g. Insert
// after a Get that found the key) an immediate, ordinary error exactly as in
// the default mode (see TestInsertDuplicateAndMissingUpdate); a race with a
// *different*, concurrently-committing transaction is instead only caught
// later, at commit, as ErrTransactionConflict — see optimisticState's doc
// comment for why the two cases need different errors.
func (tx *optimisticState) insert(rec record.Record) error {
	key := rec.Key()
	collection := key.Collection()
	factory, err := tx.db.recordFactory(collection)
	if err != nil {
		rec.SetError(err)
		return err
	}
	data, err := normalizeData(rec.Data())
	if err != nil {
		rec.SetError(err)
		return err
	}
	if err := validateForCommit(collection, factory, data); err != nil {
		rec.SetError(err)
		return err
	}

	tx.lock()
	entry, touchErr := tx.touch(collection, keyID(key), key)
	var resultErr error
	switch {
	case touchErr != nil:
		resultErr = touchErr
	case entry.present:
		resultErr = fmt.Errorf("%w: %s", record.ErrRecordExists, key)
	default:
		entry.present = true
		entry.data = data
		entry.commit = &pendingWrite{kind: pendingWriteStore, rec: rec, overwrite: false}
	}
	tx.unlock()

	if resultErr != nil {
		rec.SetError(resultErr)
		return resultErr
	}
	rec.SetError(nil)
	return nil
}

// delete implements session.Delete for an optimistic-mode transaction. Like
// the default mode's session.Delete, deleting an already-absent key is a
// no-op (never an error), and no schema-guard check is performed.
func (tx *optimisticState) delete(key *record.Key) {
	collection := key.Collection()
	tx.lock()
	defer tx.unlock()
	// A delete never needs the prior value, so firstTouchEntry (which never
	// fails) is enough — no need for touch's live-engine load.
	entry, _ := tx.firstTouchEntry(collection, keyID(key), key)
	entry.present = false
	entry.data = nil
	entry.commit = &pendingWrite{kind: pendingWriteDelete}
}

// updateRecord implements session.UpdateRecord for an optimistic-mode
// transaction. Like the live engines' update, it requires the key to already
// exist — checked eagerly against this transaction's own view, matching the
// default mode's immediate not-found behavior — then applies updates to a
// copy of this transaction's local snapshot via the same applyUpdatesToMap
// the live engines use, so a second Update of the same key later in the same
// transaction compounds on top of the first rather than the value the
// transaction started with.
func (tx *optimisticState) updateRecord(rec record.Record, updates []update.Update) error {
	key := rec.Key()
	collection := key.Collection()
	factory, err := tx.db.recordFactory(collection)
	if err != nil {
		return err
	}

	tx.lock()
	defer tx.unlock()
	entry, err := tx.touch(collection, keyID(key), key)
	if err != nil {
		return err
	}
	if !entry.present {
		return dal.NewErrNotFoundByKey(key, nil)
	}
	// Work on a defensive copy so a failure below (an unknown field, say)
	// cannot leave entry.data half-updated for a later read in this same
	// transaction to observe. cloneEntryData, unlike normalizeData, cannot
	// itself fail: entry.data is already valid, previously round-tripped JSON
	// data (see cloneEntryData's doc comment).
	merged := cloneEntryData(entry.data)
	if err := applyUpdatesToMap(merged, updates); err != nil {
		return err
	}
	if err := validateForCommit(collection, factory, merged); err != nil {
		return err
	}
	entry.data = merged
	entry.commit = &pendingWrite{
		kind:      pendingWriteStore,
		rec:       record.NewRecordWithData(key, merged),
		overwrite: true,
	}
	return nil
}

// commit is called once, synchronously, when the transaction's callback
// returns nil (see runOptimisticReadwriteTransaction). It validates this
// transaction's full read set — every key it touched, by a read or a write,
// see touch — against the live commit-version table and, only if nothing has
// changed since this transaction first touched it, applies its buffered
// writes for real and advances their versions. Both steps run inside one
// critical section so that validating and applying are atomic with respect to
// every other transaction's own commit: that atomicity is the entire point,
// since two separate steps here is exactly what a concurrent commit could
// interleave between.
//
// A deferred validation failure — a buffered write whose data passed the
// eager validateForCommit check at the point of the call, but still somehow
// fails the target storage engine's own validation (a declared columnar
// column's type mismatch discovered only during the real engine.store, for
// instance) — can leave a multi-key commit partially applied: an earlier
// key's write in the same commit may already have been applied to the engine
// before a later key's write fails. This is a narrow, documented gap, not a
// silent one: it cannot happen for the Serialized engine, nor for any of the
// failure modes validateForCommit already screens for (unknown fields,
// non-marshalable data) — see WithOptimisticConcurrency's doc comment and
// this adapter's Feature report for the trade-off this accepts rather than
// building a full two-phase commit across the storageEngine interface.
func (tx *optimisticState) commit() error {
	tx.lock()
	defer tx.unlock()

	if tx.conflict != nil {
		// A read in this transaction already observed a snapshot violation;
		// the callback swallowed that error and returned nil, but its work is
		// derived from a fractured view and must not become visible. Refusing
		// here mirrors the real Firestore client, which fails the commit of a
		// transaction whose read came back ABORTED regardless of what the
		// callback returned.
		return fmt.Errorf("commit refused: an earlier read in this transaction observed a snapshot conflict: %w", tx.conflict)
	}

	if tx.validateVersions {
		for ck, baseline := range tx.reads {
			if tx.db.versions[ck] != baseline {
				return fmt.Errorf("%w: key %q changed after this transaction observed it", ErrTransactionConflict, ck)
			}
		}
	}

	// appliedSeq is this commit's entry in the global commit sequence,
	// allocated lazily at the first applied write so a read-only commit does
	// not advance the sequence (it changed nothing, so no snapshot needs to
	// know it happened). Every key this commit touches is stamped with the
	// same value: the batch is one atomic commit, and stamping its keys with
	// one sequence is what lets observeAtSnapshot ask "was this key committed
	// after my snapshot?" with a single comparison.
	var appliedSeq uint64
	for ck, entry := range tx.pending {
		w := entry.commit
		if w == nil {
			continue // touched only by a read; nothing to apply
		}
		collection := entry.key.Collection()
		id := keyID(entry.key)
		switch w.kind {
		case pendingWriteDelete:
			tx.db.engine(collection).delete(id)
		case pendingWriteStore:
			if err := tx.db.engine(collection).store(id, w.rec, w.overwrite); err != nil {
				return err
			}
		}
		if tx.validateVersions {
			if appliedSeq == 0 {
				tx.db.commitSeq++
				appliedSeq = tx.db.commitSeq
			}
			tx.db.versions[ck] = appliedSeq
		}
	}
	return nil
}

// validateForCommit runs the same generic checks the real storage engines
// perform before persisting — the data marshals to JSON, and, when factory is
// non-nil (the collection has a schema), contains no field the schema does
// not declare — so a malformed write fails immediately at the point of the
// Set/Insert/Update call instead of being silently accepted into the buffer
// and only discovered at commit. It mirrors serializedEngine.store's own
// validation exactly (that engine performs no other checks); a storage
// engine with additional validation of its own — the columnar engine's
// per-column typed decode, for instance — can still fail at commit for a
// reason this eager check does not catch. See commit's doc comment for the
// resulting trade-off.
//
// factory is resolved once by the caller (stage/insert/updateRecord, via
// recordFactory) rather than re-resolved here, since re-resolving it would
// only ever repeat a check the caller already made successfully — by the
// time any of them calls this, recordFactory has already been asked about
// this exact collection and returned no error.
func validateForCommit(collection string, factory func() any, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if factory != nil {
		return checkUnknownFields(collection, factory, b)
	}
	return nil
}

// normalizeData JSON-round-trips a record's data into a generic
// map[string]any, matching the shape the Serialized engine actually persists
// (and the shape the columnar engine's own rowData/materializeSlot produce)
// so every later same-transaction operation on the key — materializeInto,
// Update's merge, a later Insert's duplicate check — has a stable,
// engine-independent snapshot to work from, isolated from the original data
// the caller passed in.
func normalizeData(data any) (map[string]any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// cloneEntryData deep-copies a pendingEntry's data via the same JSON round
// trip normalizeData uses, but — unlike normalizeData — cannot itself fail:
// its input is always already-valid, previously round-tripped JSON data (it
// came from normalizeData or from an engine's own generic decode in touch),
// and a generic map[string]any decoded from JSON contains only
// JSON-representable values, which always re-marshal, and re-marshaled JSON
// always re-decodes.
func cloneEntryData(data map[string]any) map[string]any {
	b, _ := json.Marshal(data)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// materializeInto decodes data into target via a JSON round-trip, so the
// result shares no references with the transaction's own snapshot or any
// other read — the same isolation columnarEngine.materializeSlot already
// gives query results. Only the decode into target can fail here (target is
// a caller-supplied Get destination, which may not match what was stored):
// the marshal side cannot, because every write path validates
// marshalability before a value ever reaches a pendingEntry (see
// normalizeData and cloneEntryData's doc comments), and get is
// materializeInto's only caller, always with a pendingEntry's data.
func materializeInto(data map[string]any, target any) error {
	b, _ := json.Marshal(data)
	return json.Unmarshal(b, target)
}
