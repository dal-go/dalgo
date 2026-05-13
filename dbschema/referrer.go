package dbschema

import "github.com/dal-go/dalgo/dal"

// Referrer describes one collection that references the queried
// collection via a foreign key.
type Referrer struct {
	// Collection identifies the referencing collection.
	Collection dal.CollectionRef
	// Fields lists the fields in Collection that reference back to
	// the queried collection.
	Fields []dal.FieldName
}
