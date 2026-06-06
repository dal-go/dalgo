package dalgo2memory

import (
	"github.com/dal-go/dalgo/dal"
)

// storageEngine is the internal, per-collection storage seam. It owns how one
// collection's records are stored and read back, abstracting the concrete
// representation (JSON bytes for the Serialized engine, typed slices for a
// future Columnar engine) behind a representation-neutral interface: no member
// accepts or returns a serialized byte representation.
//
// An engine instance backs exactly one collection. Engine selection is per
// collection (see CollectionOption); a single database can host different
// engines for different collections.
type storageEngine interface {
	// exists reports whether a record with the given id is stored.
	exists(id string) bool

	// store writes the record's decoded data under id. When overwrite is false
	// and a record with that id already exists, it returns a duplicate error.
	store(id string, record dal.Record, overwrite bool) error

	// load reads the record stored under id into the caller-provided record.
	// It returns a not-found error when no record with that id is stored.
	load(id string, record dal.Record) error

	// delete removes the record stored under id, if any.
	delete(id string)

	// update applies decoded field updates (name -> value) to the record
	// stored under id, re-validating against the engine's own representation.
	// It returns a not-found error when no record with that id is stored.
	update(id string, updates map[string]any) error

	// rows enumerates the collection's stored rows for query execution. Each
	// row exposes a decoded map[string]any field view (for WHERE/GROUP
	// BY/ORDER BY/projection) and a materialize callback that decodes the row
	// into a caller-provided typed target.
	rows() ([]engineRow, error)
}

// engineRow is one enumerated row: its id, a decoded field view, and a
// materialize callback that loads the row into a caller-provided typed target.
type engineRow struct {
	id          string
	data        map[string]any
	materialize func(target any) error
}
