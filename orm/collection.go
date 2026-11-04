package orm

import "github.com/dal-go/dalgo/dal"

// Collection exposes a dal collection reference and its fields.
type Collection interface {
	// CollectionRef returns the dal collection reference.
	CollectionRef() dal.CollectionRef
	// Fields returns the collection's fields.
	Fields() []Field
}
