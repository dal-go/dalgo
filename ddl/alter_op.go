package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// AlterOp is the sealed interface for collection-altering operations
// passed to AlterCollection. Sealed via an unexported marker method
// (alterOp) so the set of valid alterations is closed at the package
// boundary — drivers know which cases exist and translate
// accordingly. New alteration kinds require adding to this package.
//
// MVP constructors:
//
//   - Field-level: [AddField], [DropField], [ModifyField], [RenameField]
//   - Index-level: [AddIndex], [DropIndex]
//
// All six constructors accept opts ...Option for opt-in idempotency
// (reusing the same Option type as CreateCollection / DropCollection).
// Drivers MUST silently ignore semantically-mismatched options.
type AlterOp interface {
	alterOp() // sealed marker
	// ApplyTo dispatches this AlterOp to the matching ApplyXxx method
	// on the given Applier, passing the op's stored fields and resolved
	// Options. Returns the Applier's error verbatim. Used by driver-side
	// SchemaModifier.AlterCollection implementations to handle a slice
	// of AlterOp values without type-switching on unexported concrete
	// types.
	ApplyTo(ctx context.Context, a Applier) error
}

// ---- Field-level AlterOps ----

type addFieldOp struct {
	field   dbschema.FieldDef
	options Options
}

func (addFieldOp) alterOp() {}

func (o addFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyAddField(ctx, o.field, o.options)
}

// AddField returns an AlterOp that adds a field to the collection.
// IfNotExists makes it idempotent (existing field of same name = no-op).
// IfExists is meaningless and silently ignored.
func AddField(f dbschema.FieldDef, opts ...Option) AlterOp {
	return addFieldOp{field: f, options: ResolveOptions(opts...)}
}

type dropFieldOp struct {
	name    dal.FieldName
	options Options
}

func (dropFieldOp) alterOp() {}

func (o dropFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyDropField(ctx, o.name, o.options)
}

// DropField returns an AlterOp that drops a field by name.
// IfExists makes it idempotent (missing field = no-op). IfNotExists
// is meaningless and silently ignored.
func DropField(name dal.FieldName, opts ...Option) AlterOp {
	return dropFieldOp{name: name, options: ResolveOptions(opts...)}
}

type modifyFieldOp struct {
	name    dal.FieldName
	newDef  dbschema.FieldDef
	options Options
}

func (modifyFieldOp) alterOp() {}

func (o modifyFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyModifyField(ctx, o.name, o.newDef, o.options)
}

// ModifyField returns an AlterOp that replaces an existing field's
// definition with newDef. The driver diffs old vs new and emits the
// minimal engine-specific change. When name != newDef.Name, the
// operation also renames the field.
//
// opts is accepted for surface symmetry but both IfNotExists and
// IfExists are semantically meaningless on ModifyField. Drivers MUST
// silently ignore them.
func ModifyField(name dal.FieldName, newDef dbschema.FieldDef, opts ...Option) AlterOp {
	return modifyFieldOp{name: name, newDef: newDef, options: ResolveOptions(opts...)}
}

type renameFieldOp struct {
	oldName dal.FieldName
	newName dal.FieldName
	options Options
}

func (renameFieldOp) alterOp() {}

func (o renameFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyRenameField(ctx, o.oldName, o.newName, o.options)
}

// RenameField returns an AlterOp that renames a field from oldName
// to newName.
//
// opts is accepted for surface symmetry but both IfNotExists and
// IfExists are semantically meaningless on RenameField. Drivers MUST
// silently ignore them.
func RenameField(oldName, newName dal.FieldName, opts ...Option) AlterOp {
	return renameFieldOp{oldName: oldName, newName: newName, options: ResolveOptions(opts...)}
}

// ---- Index-level AlterOps ----

type addIndexOp struct {
	index   dbschema.IndexDef
	options Options
}

func (addIndexOp) alterOp() {}

func (o addIndexOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyAddIndex(ctx, o.index, o.options)
}

// AddIndex returns an AlterOp that creates an index on the
// collection. On engines that support combined ALTER TABLE ... ADD
// INDEX syntax (MySQL), the driver MAY fold this into a single
// statement alongside other AlterOps in the same batch.
//
// IfNotExists makes it idempotent (existing index of same name =
// no-op). IfExists is meaningless and silently ignored.
func AddIndex(idx dbschema.IndexDef, opts ...Option) AlterOp {
	return addIndexOp{index: idx, options: ResolveOptions(opts...)}
}

type dropIndexOp struct {
	name    string
	options Options
}

func (dropIndexOp) alterOp() {}

func (o dropIndexOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyDropIndex(ctx, o.name, o.options)
}

// DropIndex returns an AlterOp that drops an index by name.
// IfExists makes it idempotent (missing index = no-op). IfNotExists
// is meaningless and silently ignored.
func DropIndex(name string, opts ...Option) AlterOp {
	return dropIndexOp{name: name, options: ResolveOptions(opts...)}
}
