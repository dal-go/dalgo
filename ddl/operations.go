package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// backendName returns db.Adapter().Name() if Adapter() returns a
// non-nil dal.Adapter, otherwise the empty string.
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

// notSupportedModifier builds the *dbschema.NotSupportedError returned
// by the helpers when db does not implement SchemaModifier.
func notSupportedModifier(op string, db dal.DB) error {
	return &dbschema.NotSupportedError{
		Op:      op,
		Backend: backendName(db),
		Reason:  "driver does not implement ddl.SchemaModifier",
	}
}

// CreateCollection creates a new collection (and any inline indexes
// declared on c.Indexes) via the driver's SchemaModifier. Returns
// *dbschema.NotSupportedError if db does not implement
// SchemaModifier.
func CreateCollection(ctx context.Context, db dal.DB, c dbschema.CollectionDef, opts ...Option) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("CreateCollection", db)
	}
	return m.CreateCollection(ctx, c, opts...)
}

// DropCollection drops a collection and its indexes via the driver's
// SchemaModifier. Returns *dbschema.NotSupportedError if db does not
// implement SchemaModifier.
func DropCollection(ctx context.Context, db dal.DB, name string, opts ...Option) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("DropCollection", db)
	}
	return m.DropCollection(ctx, name, opts...)
}

// AlterCollection applies a batch of AlterOp values to an existing
// collection via the driver's SchemaModifier. Returns
// *dbschema.NotSupportedError if db does not implement
// SchemaModifier.
//
// Transactional drivers (advertised via TransactionalDDL) roll back
// on partial failure. Non-transactional drivers MAY return
// *PartialSuccessError. Consumers wanting strict atomicity should
// check SupportsTransactionalDDL(db) before calling.
func AlterCollection(ctx context.Context, db dal.DB, name string, ops ...AlterOp) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("AlterCollection", db)
	}
	return m.AlterCollection(ctx, name, ops...)
}
