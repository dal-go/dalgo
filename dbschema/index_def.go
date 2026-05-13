package dbschema

import "github.com/dal-go/dalgo/dal"

// IndexDef is the portable description of one index on a
// [CollectionDef]. The Fields slice is ordered — order matters for
// composite indexes (field ordinality affects which queries the
// index can serve).
//
// Collection is the simple name of the collection the index belongs
// to. Richer collection addressing (catalog/schema/parent-key) lives
// in [dal.CollectionRef] and is the argument type passed to reader
// and writer methods; this stored value is a plain name. Tier-2
// engine extensions MAY add a richer reference field if needed.
type IndexDef struct {
	// Name is the index name.
	Name string
	// Collection is the simple name of the collection this index belongs to.
	Collection string
	// Fields is the ordered list of fields the index covers.
	Fields []dal.FieldName
	// Unique is true if the index enforces uniqueness.
	Unique bool
}
