# Feature: ddl AlterOp (Composable Alteration Operations)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fddl%2Falter-ops) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

Defines the sealed `AlterOp` interface and its six MVP constructors — four field-level (`AddField`, `DropField`, `ModifyField`, `RenameField`) and two index-level (`AddIndex`, `DropIndex`). All six constructors accept `opts ...Option` for opt-in idempotency (reusing the same `Option` type that `CreateCollection`/`DropCollection` use). These AlterOp values are passed as a variadic list to `SchemaModifier.AlterCollection(ctx, name, ops...)` and the corresponding top-level helper. The unified composable model means BOTH field changes and index changes flow through `AlterCollection` — there are no standalone `CreateIndex` or `DropIndex` methods on `SchemaModifier`.

The interface is **sealed** (unexported marker method) so the set of valid alteration kinds is closed at the package boundary. Drivers know which `AlterOp` cases exist and translate accordingly. New alteration kinds require adding to this package, not arbitrary external types.

## Behavior

### REQ: alter-op-interface

The `ddl` package MUST export `type AlterOp interface { … }` with an UNEXPORTED marker method that prevents external packages from satisfying the interface.

#### AC-1: interface-exists

**Given** a Go program importing `ddl`
**When** the program references `ddl.AlterOp`
**Then** the program compiles.

#### AC-2: sealed-marker-present

**Given** the `ddl.AlterOp` interface declaration
**When** inspected via Go reflection in a test (`reflect.TypeOf((*ddl.AlterOp)(nil)).Elem()`) and iterated over its methods
**Then** the method set includes exactly one method whose name begins with a lowercase letter (the unexported marker), proving the interface cannot be satisfied from outside the `ddl` package.

#### AC-3: sealed-external-rejection

**Given** a `testdata/external_sealing/` sub-directory under `ddl/` containing a separate Go package that imports `ddl` and attempts `var _ ddl.AlterOp = externalStub{}` where `externalStub` has no marker method
**When** an in-package test runs `go build ./testdata/external_sealing/...` as a subprocess
**Then** the build reports a compile error containing the substring "missing method"

### REQ: add-field-constructor

The `ddl` package MUST export:

```go
func AddField(f dbschema.FieldDef, opts ...Option) AlterOp
```

The returned value satisfies `AlterOp` and carries `f` plus the resolved `Options` as immutable state. Drivers translate to engine-specific `ADD COLUMN` (SQL) or per-engine equivalent. When `IfNotExists()` is among `opts`, the driver MUST treat an existing field of the same name as a no-op rather than an error. `IfExists()` is meaningless for `AddField` and MUST be silently ignored.

#### AC-1: add-field-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.AddField(dbschema.FieldDef{Name: "email", Type: dbschema.String})`
**Then** the program compiles and `op != nil`.

#### AC-2: add-field-preserves-field

**Given** an `op := ddl.AddField(f)` for some `FieldDef f`
**When** the driver inspects `op` (via a package-internal accessor or type assertion to the concrete struct)
**Then** the carried `FieldDef` is byte-equal to `f`.

### REQ: drop-field-constructor

The `ddl` package MUST export:

```go
func DropField(name dal.FieldName, opts ...Option) AlterOp
```

When `IfExists()` is among `opts`, the driver MUST treat a missing field of the given name as a no-op rather than an error. `IfNotExists()` is meaningless for `DropField` and MUST be silently ignored.

#### AC-1: drop-field-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.DropField("legacy_user_code", ddl.IfExists())`
**Then** the program compiles and `op != nil`.

### REQ: modify-field-constructor

The `ddl` package MUST export:

```go
func ModifyField(name dal.FieldName, newDef dbschema.FieldDef, opts ...Option) AlterOp
```

The signature is **full-replacement** — caller supplies the complete new `FieldDef`. The driver diffs the new definition against the existing field's structure (which it knows from prior CREATE/ALTER history or from introspection) and emits the minimal engine-specific change. Drivers that cannot perform the implied change (e.g. SQLite changing a column type without a table rebuild) return `*dbschema.NotSupportedError`.

The `name` parameter is REDUNDANT with `newDef.Name` only when no rename is intended. When `name != newDef.Name`, the operation also renames the field. (Equivalent to a `RenameField` + `ModifyField` pair; using both forms gives callers a choice.)

`opts ...Option` is accepted for surface symmetry with the other AlterOps, but both `IfNotExists()` and `IfExists()` are semantically meaningless on `ModifyField` (you generally want a real error if the target field is gone). Drivers MUST silently ignore them.

#### AC-1: modify-field-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.ModifyField("created_at", dbschema.FieldDef{Name: "created_at", Type: dbschema.Time, Nullable: false})`
**Then** the program compiles and `op != nil`.

### REQ: rename-field-constructor

The `ddl` package MUST export:

```go
func RenameField(oldName, newName dal.FieldName, opts ...Option) AlterOp
```

`opts ...Option` is accepted for surface symmetry but both `IfNotExists()` and `IfExists()` are semantically meaningless on `RenameField`. Drivers MUST silently ignore them.

#### AC-1: rename-field-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.RenameField("user_name", "username")`
**Then** the program compiles and `op != nil`.

### REQ: add-index-constructor

The `ddl` package MUST export:

```go
func AddIndex(idx dbschema.IndexDef, opts ...Option) AlterOp
```

The returned value satisfies `AlterOp` and carries `idx` plus the resolved `Options` as immutable state. Drivers translate to engine-specific `CREATE INDEX` (SQL) or per-engine equivalent. On SQL engines that support combined `ALTER TABLE ... ADD INDEX` syntax, the driver MAY fold this into a single statement alongside other AlterOps in the same batch.

When `IfNotExists()` is among `opts`, the driver MUST treat an existing index of the same name as a no-op. `IfExists()` is meaningless for `AddIndex` and MUST be silently ignored.

#### AC-1: add-index-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.AddIndex(dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true})`
**Then** the program compiles and `op != nil`.

#### AC-2: add-index-preserves-idx

**Given** an `op := ddl.AddIndex(idx)` for some `IndexDef idx`
**When** the driver inspects `op` (via a package-internal accessor or type assertion to the concrete struct)
**Then** the carried `IndexDef` is byte-equal to `idx`.

### REQ: drop-index-constructor

The `ddl` package MUST export:

```go
func DropIndex(name string, opts ...Option) AlterOp
```

Drops an existing index by name. Drivers translate to engine-specific `DROP INDEX` (SQL) or per-engine equivalent. When `IfExists()` is among `opts`, the driver MUST treat a missing index of the given name as a no-op. `IfNotExists()` is meaningless for `DropIndex` and MUST be silently ignored.

#### AC-1: drop-index-constructs

**Given** a Go program importing `ddl`
**When** the program declares `var op ddl.AlterOp = ddl.DropIndex("ix_users_legacy", ddl.IfExists())`
**Then** the program compiles and `op != nil`.

## Architecture

| File | Contents |
|---|---|
| `ddl/alter_op.go` | `AlterOp` sealed interface, the six concrete constructor types, the six constructor functions. |
| `ddl/alter_op_test.go` | Tests covering the ACs above. |
| `ddl/testdata/external_sealing/` | A standalone Go package used by AC-3 to verify sealing via subprocess `go build`. |

## Outstanding Questions

- **Granular per-attribute ops** (`ChangeFieldType`, `SetFieldDefault`, `DropFieldDefault`) — explicitly deferred per Q20 lock. If real consumers want them, they're additive (new constructor functions; interface unchanged).
- **Per-`AlterOp` idempotency mismatched options.** `IfNotExists()` on `DropField`/`DropIndex` and `IfExists()` on `AddField`/`AddIndex` are semantically meaningless. Drivers MUST silently ignore them per the REQs above. On `ModifyField`/`RenameField`, BOTH `IfNotExists()` and `IfExists()` are no-ops by the same rule.
- **Constraint-level ops** (`AddConstraint`, `DropConstraint`, foreign-key declarations) — deferred per source Idea's Non-Goals. Constraints will slot in as additional AlterOp constructors when scoped, without growing the `SchemaModifier` interface.

---
*This document follows the https://specscore.md/feature-specification*
