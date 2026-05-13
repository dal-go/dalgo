package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// backendName extracts the driver name from db.Adapter() if non-nil,
// otherwise returns the empty string.
func backendName(db dal.DB) string {
	if db == nil {
		return ""
	}
	a := db.Adapter()
	if a == nil {
		return ""
	}
	return a.Name()
}

// notSupportedReader returns a *NotSupportedError for the given op
// when db does not implement SchemaReader.
func notSupportedReader(op string, db dal.DB) error {
	return &NotSupportedError{
		Op:      op,
		Backend: backendName(db),
		Reason:  "driver does not implement dbschema.SchemaReader",
	}
}

// ListCollections type-asserts db to SchemaReader and delegates;
// returns *NotSupportedError if the assertion fails.
func ListCollections(ctx context.Context, db dal.DB, parent *dal.Key) ([]dal.CollectionRef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListCollections", db)
	}
	return r.ListCollections(ctx, parent)
}

// DescribeCollection type-asserts db to SchemaReader and delegates.
func DescribeCollection(ctx context.Context, db dal.DB, ref *dal.CollectionRef) (*CollectionDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("DescribeCollection", db)
	}
	return r.DescribeCollection(ctx, ref)
}

// ListIndexes type-asserts db to SchemaReader and delegates.
func ListIndexes(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]IndexDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListIndexes", db)
	}
	return r.ListIndexes(ctx, ref)
}

// ListConstraints type-asserts db to SchemaReader and delegates.
func ListConstraints(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]ConstraintDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListConstraints", db)
	}
	return r.ListConstraints(ctx, ref)
}

// ListReferrers type-asserts db to SchemaReader and delegates.
func ListReferrers(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]Referrer, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListReferrers", db)
	}
	return r.ListReferrers(ctx, ref)
}
