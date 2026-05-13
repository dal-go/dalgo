package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// Applier is the visitor interface for AlterOp dispatch. Driver-side
// implementations of [SchemaModifier.AlterCollection] construct an
// Applier (typically a struct holding the in-flight transaction and
// the target table name), then call [AlterOp.ApplyTo] on each op
// in their batch. ApplyTo dispatches to the matching ApplyXxx method
// on the Applier, where the driver translates the operation into
// engine-specific SQL or other side effects.
//
// All six methods take ctx as the first parameter (forwarded by
// AlterOp.ApplyTo) and return error so driver-side failures surface
// to the AlterCollection caller.
//
// The resolved [Options] value (already processed by [ResolveOptions]
// at AlterOp-constructor time) is passed last. Drivers MAY consult
// opts.IfNotExists / opts.IfExists for the field/index-level
// AlterOps where those flags are semantically meaningful (see the
// alter-ops Feature for which combinations matter per op).
//
// Applier is NOT sealed — driver packages MUST implement it. Test
// stubs (e.g. a recording fakeApplier) MAY also implement it. New
// AlterOp variants added in future dalgo releases will extend this
// interface, which is a breaking change for existing implementers
// (compile-time error) — the same property the AlterOp sealed
// interface has on the producer side.
type Applier interface {
	// ApplyAddField is called by addFieldOp.ApplyTo.
	ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts Options) error

	// ApplyDropField is called by dropFieldOp.ApplyTo.
	ApplyDropField(ctx context.Context, name dal.FieldName, opts Options) error

	// ApplyModifyField is called by modifyFieldOp.ApplyTo. name is
	// the current field name; newDef is its replacement (which MAY
	// have a different Name, indicating a rename as part of modify).
	ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts Options) error

	// ApplyRenameField is called by renameFieldOp.ApplyTo.
	ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts Options) error

	// ApplyAddIndex is called by addIndexOp.ApplyTo.
	ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts Options) error

	// ApplyDropIndex is called by dropIndexOp.ApplyTo.
	ApplyDropIndex(ctx context.Context, name string, opts Options) error
}
