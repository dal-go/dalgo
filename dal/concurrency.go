package dal

// ConcurrencyAware is implemented by [DB] values that can report whether
// the underlying backend supports multiple concurrent open connections
// from a single client process.
//
// The returned value is constant from the moment a DB value is returned
// by its constructor until it is discarded. Drivers MUST NOT change the
// answer in response to reconnects, failovers, transient errors, or
// runtime configuration reloads against the same DB handle. Callers are
// entitled to memoize the value once per DB value.
//
// The boolean intentionally does not distinguish read-vs-write
// concurrency. A driver like SQLite that supports concurrent readers
// but serializes writers collapses to false. Refining this surface (or
// adding a sibling) is a future change if a real consumer needs the
// distinction.
//
// ConcurrencyAware is embedded into [DB]; every DB implementation
// therefore answers the question. Drivers SHOULD embed one of the
// reusable structs [NoConcurrency] or [ConcurrencyAvailable] rather
// than hand-writing the method.
type ConcurrencyAware interface {
	// SupportsConcurrentConnections reports whether the underlying
	// backend tolerates more than one open connection from a single
	// client process at the same time. See [ConcurrencyAware] for the
	// stability and asymmetry contract.
	SupportsConcurrentConnections() bool
}
