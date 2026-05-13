package ddl

import (
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
}

// ---- Field-level AlterOps ----

type addFieldOp struct {
	field   dbschema.FieldDef
	options Options
}

func (addFieldOp) alterOp() {}

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

// DropIndex returns an AlterOp that drops an index by name.
// IfExists makes it idempotent (missing index = no-op). IfNotExists
// is meaningless and silently ignored.
func DropIndex(name string, opts ...Option) AlterOp {
	return dropIndexOp{name: name, options: ResolveOptions(opts...)}
}
