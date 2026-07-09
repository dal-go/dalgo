package dalgo2memory

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
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

	// update applies a list of field updates to the record stored under id,
	// re-validating against the engine's own representation. Each Update carries
	// either a FieldName (top-level key) or a FieldPath (nested key sequence).
	// A value of update.DeleteField removes the key rather than setting it.
	// It returns a not-found error when no record with that id is stored.
	update(id string, updates []update.Update) error

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

// applyUpdatesToMap applies a slice of Update values to a flat-or-nested
// map[string]any. Each Update has either a FieldName (a single top-level key)
// or a FieldPath (a sequence of nested keys). A value of update.DeleteField
// removes the leaf key; any other value sets it. Intermediate nodes are created
// as map[string]any when they are missing; if an intermediate node exists but is
// not a map[string]any, applyUpdatesToMap returns a descriptive error rather
// than panicking.
func applyUpdatesToMap(data map[string]any, updates []update.Update) error {
	for _, upd := range updates {
		var path []string
		if fp := upd.FieldPath(); len(fp) > 0 {
			path = fp
		} else if fn := upd.FieldName(); fn != "" {
			path = []string{fn}
		} else {
			continue // neither set — skip (should not happen after Validate)
		}
		if err := applyPathUpdate(data, path, upd.Value()); err != nil {
			return err
		}
	}
	return nil
}

// applyPathUpdate walks path in data, creating intermediate map[string]any
// nodes as needed, then sets or deletes the leaf. Returns an error when an
// intermediate value exists but is not a map[string]any.
func applyPathUpdate(data map[string]any, path []string, value any) error {
	// Navigate to the parent map of the leaf.
	current := data
	for _, key := range path[:len(path)-1] {
		switch v := current[key].(type) {
		case map[string]any:
			current = v
		case nil:
			next := make(map[string]any)
			current[key] = next
			current = next
		default:
			return fmt.Errorf("field path segment %q is not a map (got %T)", key, current[key])
		}
	}
	leaf := path[len(path)-1]
	if value == update.DeleteField {
		delete(current, leaf)
	} else {
		current[leaf] = value
	}
	return nil
}
