package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
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

// ListCollections resolves db to a SchemaReader (see dal.As) and delegates;
// returns *NotSupportedError if the assertion fails.
func ListCollections(ctx context.Context, db dal.DB, parent *record.Key) ([]dal.CollectionRef, error) {
	r, ok := dal.As[SchemaReader](db)
	if !ok {
		return nil, notSupportedReader("ListCollections", db)
	}
	return r.ListCollections(ctx, parent)
}

// DescribeCollection resolves db to a SchemaReader (see dal.As) and delegates.
func DescribeCollection(ctx context.Context, db dal.DB, ref *dal.CollectionRef) (*CollectionDef, error) {
	r, ok := dal.As[SchemaReader](db)
	if !ok {
		return nil, notSupportedReader("DescribeCollection", db)
	}
	return r.DescribeCollection(ctx, ref)
}

// ListIndexes resolves db to a SchemaReader (see dal.As) and delegates.
func ListIndexes(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]IndexDef, error) {
	r, ok := dal.As[SchemaReader](db)
	if !ok {
		return nil, notSupportedReader("ListIndexes", db)
	}
	return r.ListIndexes(ctx, ref)
}

// ListConstraints resolves db to a SchemaReader (see dal.As) and delegates.
func ListConstraints(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]ConstraintDef, error) {
	r, ok := dal.As[SchemaReader](db)
	if !ok {
		return nil, notSupportedReader("ListConstraints", db)
	}
	return r.ListConstraints(ctx, ref)
}

// ListReferrers resolves db to a SchemaReader (see dal.As) and delegates.
func ListReferrers(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]Referrer, error) {
	r, ok := dal.As[SchemaReader](db)
	if !ok {
		return nil, notSupportedReader("ListReferrers", db)
	}
	return r.ListReferrers(ctx, ref)
}
