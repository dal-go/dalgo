package dal

// Backend is the interface a DALgo storage adapter implements. It is the shape
// dal.DB used to have, unchanged.
//
// A Backend is not handed to callers directly: an adapter's constructor passes
// it to NewDB, which returns a DB that owns the write pipeline. That is what
// makes the record invariants DALgo declares (see ValidatableRecord and
// BeforeSave) impossible for an adapter to skip — they run before the adapter's
// code is entered.
type Backend interface {

	// ID is an identifier provided at time of DB creation
	ID() string

	// Adapter provides information about underlying name to access data
	Adapter() Adapter

	// Schema provides schema for the DB - for example, how keys are mapped to columns
	Schema() Schema

	// TransactionCoordinator provides shortcut methods to work with transactions
	// without opening a connection explicitly.
	TransactionCoordinator

	// ReadSession implements a virtual read session that opens connection/session for each read call on DB level
	// TODO: consider to sacrifice some simplicity for the sake of interoperability?
	ReadSession

	// ConcurrencyAware reports whether this backend supports concurrent
	// open connections. Drivers should embed NoConcurrency or
	// ConcurrencyAvailable in their concrete type to satisfy this.
	ConcurrencyAware

	// Removed members:
	// ===================================================================================
	// Close() error - is part of a connection.
	// Connect(ctx context.Context) (connection, error) - considered unneeded
}

// DB is the caller-facing database handle. It has the same shape as Backend and
// is additionally sealed: the unexported marker method means only this package
// can produce a value satisfying DB, so every DB a caller holds has been through
// NewDB and therefore through the framework write pipeline.
//
// Sealing enforces provenance, not behaviour — the pipeline enforces the
// behaviour. Together they mean a caller cannot be handed a database that
// silently skips the invariants.
//
// A decorator (a caching layer, an access-policy layer, a tracing layer) still
// works: embed DB in the decorating type and the marker method is promoted
// along with everything else the decorator does not override. See
// access.SecureDB in this repository, or dalgo-memcache-appengine, for worked
// examples.
type DB interface {
	Backend

	// dalgoDB seals this interface. It is deliberately unexported and has no
	// behaviour: a type outside package dal can only acquire it by embedding a
	// DB, which is precisely the "went through NewDB" guarantee.
	dalgoDB()
}
