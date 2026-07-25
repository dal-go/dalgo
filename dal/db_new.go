package dal

import (
	"context"
)

// NewDB wraps a storage adapter's Backend in the framework-owned write pipeline
// and returns the sealed, caller-facing DB.
//
// It is the only way to obtain a DB, and it is the one line an adapter's public
// constructor changes:
//
//	func NewDB(options ...Option) dal.DB {
//		return dal.NewDB(newDatabase(options...))
//	}
//
// Every read-write transaction the returned DB starts hands the worker a
// transaction whose writes run BeforeSave first, so record validation and
// before-save hooks happen before the adapter's code is entered. When the
// backend also supports writes outside a transaction (it implements
// WriteSession), the returned DB exposes those through the same pipeline.
//
// NewDB is idempotent: a value that already satisfies DB has already been
// through the pipeline and is returned unchanged, so wrapping twice cannot run
// hooks twice.
//
// It is named NewDB rather than New to match this package's other constructors
// (NewAdapter, NewInsertOptions, NewTransactionOptions) and to read
// unambiguously at an adapter call site, where the adapter has a NewDB of
// its own.
func NewDB(backend Backend) DB {
	if backend == nil {
		panic("dal.NewDB: backend is nil")
	}
	if db, ok := backend.(DB); ok {
		return db
	}
	base := validatedDB{Backend: backend}
	if writes, ok := backend.(WriteSession); ok {
		db := &validatedWriteDB{validatedDB: base}
		db.writePipeline = writePipeline{ws: writes, db: db, validate: true}
		return db
	}
	return base
}

// BackendOf returns the Backend that db delegates to, or db itself when it is
// not a DB produced by NewDB (a decorator, for example).
//
// It exists for adapter internals that need their own concrete type back — the
// dalgo2memory branching provider recovers its *database this way. Writing
// through the returned Backend bypasses the framework write pipeline, so a call
// to BackendOf is exactly as visible and greppable as one to WithoutValidation,
// and should be just as rare.
func BackendOf(db DB) Backend {
	if u, ok := db.(interface{ dalgoBackend() Backend }); ok {
		return u.dalgoBackend()
	}
	return db
}

// As reports whether db — or the Backend it delegates to — implements the
// optional capability interface T, and returns it.
//
// DALgo advertises optional capabilities by type assertion
// (dbschema.SchemaReader, ddl.SchemaModifier, ddl.TransactionalDDL, …). A DB
// returned by NewDB decorates its Backend, so a plain assertion on the DB would
// stop seeing capabilities the adapter does implement. As is what consumers
// should use instead.
func As[T any](db DB) (value T, ok bool) {
	if value, ok = any(db).(T); ok {
		return value, true
	}
	if backend := BackendOf(db); backend != nil {
		if value, ok = any(backend).(T); ok {
			return value, true
		}
	}
	var zero T
	return zero, false
}

// validatedDB is the framework's DB over a Backend. It embeds the Backend, so
// reads, schema, adapter metadata and readonly transactions pass straight
// through; only the read-write transaction is decorated.
type validatedDB struct {
	Backend
}

var _ DB = validatedDB{}

func (validatedDB) dalgoDB() {}

func (db validatedDB) dalgoBackend() Backend { return db.Backend }

// RunReadwriteTransaction hands the worker a transaction whose writes run
// through the framework pipeline before reaching the adapter's transaction.
func (db validatedDB) RunReadwriteTransaction(ctx context.Context, f RWTxWorker, options ...TransactionOption) error {
	return db.Backend.RunReadwriteTransaction(ctx, func(ctx context.Context, tx ReadwriteTransaction) error {
		return f(ctx, newValidatedTx(tx, db))
	}, options...)
}

// validatedWriteDB is the framework's DB over a Backend that also writes outside
// a transaction. Those writes go through the same pipeline as transactional
// ones, so `db.(dal.WriteSession)` is not a way around validation.
type validatedWriteDB struct {
	validatedDB
	writePipeline
}

var (
	_ DB           = (*validatedWriteDB)(nil)
	_ WriteSession = (*validatedWriteDB)(nil)
)

func (db *validatedWriteDB) dalgoWithoutValidation() WriteSession {
	unvalidated := *db
	unvalidated.validate = false
	return &unvalidated
}

// newValidatedTx decorates a storage adapter's read-write transaction with the
// framework write pipeline. Reads and transaction metadata are forwarded
// unchanged.
func newValidatedTx(tx ReadwriteTransaction, db DB) validatedTx {
	return validatedTx{
		ReadTransaction: tx,
		writePipeline:   writePipeline{ws: tx, db: db, validate: true},
		rw:              tx,
	}
}

// validatedTx embeds ReadTransaction for reads and transaction options, and
// writePipeline for writes. The two method sets are disjoint, so nothing is
// ambiguous and nothing is forwarded by hand.
type validatedTx struct {
	ReadTransaction
	writePipeline
	rw ReadwriteTransaction
}

var _ ReadwriteTransaction = validatedTx{}

func (tx validatedTx) ID() string { return tx.rw.ID() }

func (tx validatedTx) dalgoWithoutValidation() WriteSession {
	tx.validate = false
	return tx
}
