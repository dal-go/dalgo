package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// SchemaReader is the capability interface for schema introspection.
// Drivers that support introspection (SQL backends via
// information_schema / SQLite pragmas, Firestore via the admin API,
// etc.) opt in by implementing SchemaReader on their dal.DB value or
// a related type reachable via type assertion. Drivers that don't
// implement SchemaReader simply don't satisfy it; the top-level
// helper functions return *NotSupportedError on the failed
// assertion.
//
// ListCollections, DescribeCollection, and ListIndexes are REQUIRED
// for drivers that implement SchemaReader at all. ListConstraints and
// ListReferrers are OPTIONAL: drivers whose backend lacks the
// concept (e.g. Firestore has no SQL-style constraints) MUST return
// *NotSupportedError from those methods. The interface satisfaction
// is structural — all five methods must be present for a driver to
// satisfy SchemaReader; runtime behavior decides which methods
// actually do work.
type SchemaReader interface {
	// ListCollections returns the collections (tables) accessible to
	// db. The optional parent key narrows scope when the backend
	// supports hierarchical addressing (e.g. SQL catalog/schema).
	// Pass nil for "everything visible."
	ListCollections(ctx context.Context, parent *record.Key) ([]dal.CollectionRef, error)

	// DescribeCollection returns the structural definition of one
	// collection, including its fields, primary key, and inline
	// indexes.
	DescribeCollection(ctx context.Context, ref *dal.CollectionRef) (*CollectionDef, error)

	// ListIndexes returns the indexes on a collection. The returned
	// slice MAY include indexes already reported inline via
	// DescribeCollection's Indexes field.
	ListIndexes(ctx context.Context, ref *dal.CollectionRef) ([]IndexDef, error)

	// ListConstraints is OPTIONAL. Drivers that do not support
	// constraint introspection MUST return *NotSupportedError.
	ListConstraints(ctx context.Context, ref *dal.CollectionRef) ([]ConstraintDef, error)

	// ListReferrers is OPTIONAL. Drivers MAY return
	// *NotSupportedError. Returns the collections that reference ref
	// via foreign keys.
	ListReferrers(ctx context.Context, ref *dal.CollectionRef) ([]Referrer, error)
}
