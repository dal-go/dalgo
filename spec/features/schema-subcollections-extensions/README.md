---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Schema API: subcollection declaration and driver extensions for CreateCollection

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=request-change) |
**Status:** Draft
**Date:** 2026-09-17
**Owner:** alex
**Source Ideas:** —
**Tracking:** [dal-go/dalgo#163](https://github.com/dal-go/dalgo/issues/163)
**Related Features:** [`ddl`](../ddl/README.md), [`ddl/options`](../ddl/options/README.md), [`dbschema/collection-def`](../dbschema/collection-def/README.md)

## Summary

Two additive changes to the schema API, so that a consumer can create nested
collections, and pass driver-specific storage options for them, through DALgo:

1. **Subcollection declaration.** `dbschema.CollectionDef` gains a `Parent`
   field of the new type `dbschema.CollectionPath` (ancestor collection names,
   root first). A nested collection is created with one `CreateCollection` call
   per collection, after every ancestor exists. New helpers
   `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` address a nested
   collection by `CollectionPath`. Drivers advertise nesting through the new
   `ddl.SubCollectionsAware` capability, and the `ddl` helpers refuse nesting
   with `*dbschema.NotSupportedError` when the driver does not advertise it.
2. **Driver extensions.** `ddl.Options` gains `Extensions []ddl.Extension`, set
   through `ddl.WithExtension(ext)`. Each extension names a **target ID**, a
   constant exported by the driver module that defines the extension type (for
   example `dalgo2ingitdb.ExtensionTarget`). A driver declares its own target
   ID through the new `ddl.ExtensionAware` capability. By default an extension
   for another target is ignored, and one addressed to the driver that the
   driver does not recognise is refused. Under the new `ddl.StrictExtensions()`
   option, a foreign extension is refused too.

The guarantees hold when a consumer calls through the `ddl` helpers. Drivers
released before this Feature do not comply on direct calls (see
REQ:helpers-are-the-guarantee).

The concrete scope this Feature unblocks is the DataTug project store's
collection schema on the local inGitDB path (see REQ:collection-def-parent for
the list). It does not cover nested database roots or OpenVaultDB-backed stores
(see Not Doing).

DALgo core stays driver-agnostic: it defines only the slot, the addressing rule
and the refusals. Concrete extension types, such as an inGitDB `record_file`,
live in driver modules. Existing drivers and callers compile unchanged.

## Problem

The schema API cannot create a nested collection or pass driver-specific
storage options. In `dal-go/dalgo` v0.80.5:

- `ddl/options.go:17-20` declares a closed struct with only two flags:

  ```go
  type Options struct {
      IfNotExists bool
      IfExists    bool
  }
  ```

  and `ddl/options.go:27` `type Option func(*Options)` can only set those two
  flags, so a driver has no typed way to receive its own creation options.
- `ddl/modifier.go:38` `CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error`
  is the only creation entry point, and `ddl/operations.go:37-43` dispatches to
  it without inspecting the definition or the options.
- `dbschema/collection_def.go:24-35` `CollectionDef{Name, Fields, PrimaryKey, Indexes}`
  has no parent collection and no subcollections.
- `ddl/options.go:10-16` fixes the rule that drivers "MUST silently ignore
  semantically-mismatched options"; nothing says what a driver does with an
  option it cannot honour at all.

The consumer that needs both is `ingitdb/dalgo2ingitdb` (v0.6.1). Its
`CreateCollection` (`schema_modifier.go:42`) always builds the definition through
`buildIngitdbCollectionDef` (`schema_modifier.go:388-414`), which sets
`RecordFile: defaultRecordFile()` (`schema_modifier.go:409`), and
`defaultRecordFile()` (`schema_modifier.go:416-421`) is fixed to
`{key}.yaml`, `yaml`, `map[string]any`. A caller cannot choose `name`,
`format`, `type` or `records_dir` of inGitDB's `RecordFileDef`
(`ingitdb-go/ingitdb@v0.6.1/record_file_def.go:22-52`).

The driver does accept a path-form name such as `"spaces/ext"` on create
(`schema_modifier.go:48-50`, `createSubCollection` at `:349`), but:

- that convention is undocumented in DALgo core, and a driver without nesting
  cannot tell a nested request from a collection whose name contains a slash;
- `createSubCollection` checks that only the root exists
  (`schema_modifier.go:350-353`), not intermediate ancestors;
- `DropCollection` (`schema_modifier.go:107-110`) and `AlterCollection`
  (`schema_modifier.go:141`) look for `<name>/.collection/definition.yaml`,
  while nested definitions live under
  `<root>/.collection/subcollections/<sub>/definition.yaml`, so a nested
  collection cannot be dropped or altered by its path-form name.

The end user is DataTug's project store (`datatug/datatug` Feature
`dalgo-project-store`, REQ `canonical-project-layout` and REQ
`driver-prerequisites-are-recorded`). It needs `projects/{id}` with nested
collections, each with `record_file` `name: '{key}/{key}.<suffix>.json'`,
`format: json`, `type: map[string]any`, `records_dir: '.'`. The founder's rule
is that DataTug writes, schema creation included, go through DALgo and are
fixed at source with no bootstrap workaround
([ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16)).

## Design Principles

- **Core names the slot, drivers own the vocabulary.** No inGitDB, SQL or
  Firestore term enters `ddl` or `dbschema`.
- **No silent loss through the helpers.** A declaration that the target driver
  cannot honour (a parent on a flat driver, an extension addressed to a driver
  that does not understand it) is an error, never a quiet default. That silent
  default is the exact defect behind dalgo2ingitdb#16. A consumer that must not
  lose any extension opts into `StrictExtensions()`.
- **Addressing by compiled constant, not by display name.** Extensions match on
  a target ID that the defining driver module exports, never on
  `dal.Adapter.Name()`. A driver rename or a wrapper with its own adapter name
  cannot turn an extension foreign.
- **Portable schema code.** One schema-creation routine can carry extensions
  for several backends and run against any of them.
- **Guard in the helper.** The `ddl` helpers refuse before dispatch, so drivers
  written before this Feature cannot silently ignore the new declarations when
  called through the helpers.

## Behavior

### Subcollection declaration

#### REQ: collection-path-type

The `dbschema` package MUST export:

```go
// CollectionPath is an ordered list of collection names, root first.
// It names a collection in the schema, not a record: it carries no record IDs.
type CollectionPath []string

func (p CollectionPath) String() string // segments joined with "/"
func (p CollectionPath) Validate() error

var ErrInvalidCollectionPath = errors.New("dbschema: invalid collection path")
```

`Validate` MUST return an error that satisfies
`errors.Is(err, dbschema.ErrInvalidCollectionPath)` when any segment is empty,
is `.` or `..`, contains `/` or `\`, contains a control character, or has
leading or trailing whitespace. It MUST return `nil` otherwise. An empty
(`nil` or zero-length) path is valid and denotes the root level.

#### REQ: collection-def-parent

`dbschema.CollectionDef` MUST gain a field `Parent CollectionPath` and a method
`Path() CollectionPath` returning a new slice equal to `Parent` followed by
`Name`. A zero `Parent` means a root collection, which is exactly the meaning
every `CollectionDef` has today.

A non-empty `Parent` declares that the collection is a subcollection nested
under records of the collection at `Parent`. The declaration is per collection
shape: every record of the parent collection has the same subcollection schema.
`Parent` carries collection names only, never record IDs (`projects`, not
`projects/{id}`).

`CollectionDef{Name: "servers", Parent: CollectionPath{"projects", "environments"}}`
declares the collection whose records live at
`projects/{id}/environments/{id}/servers/{id}`. Its canonical string form,
`Path().String()`, is `projects/environments/servers`.

The DataTug collections this makes expressible, all at most three segments
deep, are: `projects`; `projects/credentials`, `projects/queries`,
`projects/entities`, `projects/environments`, `projects/dbmodels`,
`projects/boards`, `projects/recordsets`, `projects/folders`,
`projects/dbdrivers`; and `projects/environments/servers`,
`projects/environments/catalogs`, `projects/dbdrivers/dbservers`.

#### REQ: one-collection-per-create

Each `CreateCollection` call MUST create exactly one collection, the one named
by `c.Path()`. A driver that advertises `SupportsSubCollections() == true` MUST
return a non-nil error, and MUST NOT create anything, when **any** ancestor
path (`c.Parent[:1]`, `c.Parent[:2]`, and so on up to `c.Parent`) does not
exist. It MUST NOT create an ancestor implicitly. `IfNotExists` applies to the
collection at `c.Path()` only.

Rationale (why `Parent`, not `SubCollections []CollectionDef` or a key-path
parent): a recursive `SubCollections` tree would force one options set on a
whole tree, while DataTug needs a different `record_file` per subcollection
(`.query.json`, `.entity.json`, `.server.json`). It would make `IfNotExists`
ambiguous on a partly existing tree and make partial failure likely on
non-transactional drivers. `DropCollection` and `AlterCollection` already
address one collection at a time. A key-path parent such as `projects/{id}`
confuses schema with instances: a schema is declared once per collection
shape, not per parent record.

#### REQ: nested-drop-and-alter

The `ddl` package MUST export:

```go
func DropCollectionAt(ctx context.Context, db dal.DB, path dbschema.CollectionPath, opts ...Option) error
func AlterCollectionAt(ctx context.Context, db dal.DB, path dbschema.CollectionPath, ops ...AlterOp) error
```

Each helper MUST apply the checks of REQ:guard-precedence and then:

- when `len(path) == 1`, dispatch exactly as `DropCollection` / `AlterCollection`
  with `name = path[0]`;
- when `len(path) > 1`, dispatch to the driver's `DropCollection` /
  `AlterCollection` with `name = path.String()`.

A driver that advertises `SupportsSubCollections() == true` MUST resolve a
`/`-separated `name` received by `DropCollection` and `AlterCollection` as a
`CollectionPath` to the nested collection that `CreateCollection` created for
it. Dropping a collection that has subcollections MUST either remove their
definitions too or return an error; it MUST NOT leave orphaned subcollection
definitions. For `dalgo2ingitdb`, this resolution is part of the scope of
ingitdb/dalgo2ingitdb#16.

The existing `DropCollection` and `AlterCollection` helpers keep their current
behavior. They perform no nesting guard, so a slash in their `name` stays
driver-interpreted. Consumers addressing nested collections MUST use the `*At`
helpers.

#### REQ: sub-collections-capability

The `ddl` package MUST export an optional capability interface and helper that
mirror `TransactionalDDL` (`ddl/transactional.go:28-42`):

```go
type SubCollectionsAware interface {
    SupportsSubCollections() bool
}

func SupportsSubCollections(db dal.DB) bool
```

The helper MUST resolve the interface with `dal.As` and MUST return `false`
when the driver does not implement it. The value MUST stay constant for the
lifetime of a DB value.

#### REQ: nesting-refused-without-capability

When `ddl.CreateCollection` receives a non-empty `c.Parent`, or
`ddl.DropCollectionAt` / `ddl.AlterCollectionAt` receives a path with more than
one segment, and `ddl.SupportsSubCollections(db)` is `false`, the helper MUST
return `*dbschema.NotSupportedError{Op, Backend: <adapter name>, Reason: "driver does not support subcollections"}`
and MUST NOT invoke the driver. `Op` is `"CreateCollection"`,
`"DropCollection"` or `"AlterCollection"`.

A driver that advertises `SupportsSubCollections() == false` and receives a
non-empty `Parent` directly MUST return the same error. A driver that does not
implement `SubCollectionsAware` is not bound by this; see
REQ:helpers-are-the-guarantee.

When `c.Parent` is empty and no extensions are passed, `ddl.CreateCollection`
MUST behave exactly as today.

### Driver extensions

#### REQ: extension-type

The `ddl` package MUST export:

```go
// Extension is a driver-defined creation or modification option.
// Concrete types live in driver modules, never in dalgo core.
type Extension interface {
    // ExtensionTarget returns the target ID of the driver the extension is
    // addressed to. It MUST return a constant exported by the module that
    // defines the extension type (e.g. dalgo2ingitdb.ExtensionTarget).
    ExtensionTarget() string
}

var ErrInvalidExtension = errors.New("ddl: invalid extension")
```

`Extension` is an interface rather than `any`, so an arbitrary value cannot be
passed by mistake and every extension carries its addressee. A target ID is an
opaque string that the defining module fixes. It MUST NOT be derived from
`dal.Adapter.Name()`. The module's Go import path is the recommended value,
for example `"github.com/ingitdb/dalgo2ingitdb"`.

#### REQ: extension-aware-capability

The `ddl` package MUST export:

```go
type ExtensionAware interface {
    // DDLExtensionTarget returns the driver's target ID, the same constant
    // its own extension types return from ExtensionTarget.
    DDLExtensionTarget() string
    // SupportsExtension reports whether the driver honours ext for op.
    SupportsExtension(op string, ext Extension) bool
}
```

`op` is `"CreateCollection"`, `"DropCollection"` or `"AlterCollection"`. Both
return values MUST stay constant for the lifetime of a DB value. A driver that
does not implement `ExtensionAware` has no target ID, so every extension is
foreign to it.

#### REQ: with-extension-option

`ddl.Options` MUST gain the fields `Extensions []Extension` and
`StrictExtensions bool`. The package MUST export
`func WithExtension(ext Extension) Option` and `func StrictExtensions() Option`.

- Each `WithExtension` MUST append `ext` to `Options.Extensions`, preserving
  call order across all options passed to `ResolveOptions`. A `nil` `ext` MUST
  be ignored, the same as a `nil` `Option` in `ResolveOptions`
  (`ddl/options.go:46-54`).
- `StrictExtensions` MUST set `Options.StrictExtensions = true`.
- Neither option changes `IfNotExists` or `IfExists`.
- When several extensions of one concrete type reach a driver, the driver
  documents which one wins.

#### REQ: extension-addressing

Let `T` be the driver's target ID (from `ExtensionAware`), or no target if the
driver, resolved with `dal.As`, does not implement `ExtensionAware`. For each
extension `ext` in the resolved options:

| Case | Default | Under `StrictExtensions()` |
|---|---|---|
| `ext.ExtensionTarget() == ""` | refused: `errors.Is(err, ddl.ErrInvalidExtension)` | same |
| target differs from `T`, or the driver has no target (foreign) | ignored; passed to the driver unchanged, and the driver MUST ignore it | refused: `*dbschema.NotSupportedError` |
| target equals `T` and `SupportsExtension(op, ext)` is `true` | honoured | honoured |
| target equals `T` and `SupportsExtension(op, ext)` is `false` | refused: `*dbschema.NotSupportedError` | same |

Every `*dbschema.NotSupportedError` for an extension MUST have `Reason` naming
the extension's Go type and its target ID. A refused call MUST NOT invoke the
driver's operation.

The `ddl.CreateCollection`, `ddl.DropCollection` and `ddl.DropCollectionAt`
helpers MUST apply this table before dispatch. A driver that implements
`ExtensionAware` MUST apply the same table on direct calls, including to the
`Options` its `Applier` receives for each `AlterOp`.

Justification: the existing ignore rule (`ddl/options.go:10-16`) covers hints
whose meaning is unambiguous, where there is nothing to do. An addressed
extension the driver does not understand is the opposite: the caller asked this
driver for a storage property it will not get, and ignoring it produces a wrong
on-disk layout that is found only later. Foreign extensions stay ignored by
default, consistently with that rule, so portable schema code can carry
extensions for several backends at once. A consumer that targets one backend
and must never lose an extension, such as DataTug, passes `StrictExtensions()`.

#### REQ: guard-precedence

`ddl.CreateCollection`, `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` MUST
check in this order and return the first failure:

1. **Path validity.** `c.Path().Validate()` when `c.Parent` is non-empty, and
   `path.Validate()` for the `*At` helpers, plus a non-empty `path`. On failure:
   `dbschema.ErrInvalidCollectionPath`. With a non-empty `Parent` this also
   rejects a `Name` containing `/`, so the two nesting forms cannot be mixed.
2. **Extension validity**, per REQ:extension-addressing, including strict mode.
   It does not apply to `AlterCollectionAt`, whose options live inside
   `AlterOp` values.
3. **Capability.** The driver implements `SchemaModifier`, otherwise the
   existing `*dbschema.NotSupportedError` ("driver does not implement
   ddl.SchemaModifier", `ddl/operations.go:25-31`). Then nesting, per
   REQ:nesting-refused-without-capability.

`ddl.DropCollection` applies steps 2 and 3 (without nesting); `ddl.AlterCollection`
applies step 3 only.

### Compatibility

#### REQ: helpers-are-the-guarantee

The refusals in this Feature are guaranteed only for calls made through the
`ddl` helper functions. Consumers that require them (no silent loss of `Parent`
or of an extension) MUST call `ddl.CreateCollection`, `ddl.DropCollectionAt`
and `ddl.AlterCollectionAt`, not driver methods directly.

Drivers released before this Feature do not comply on direct calls. That
includes dalgo2sql's SQLite, PostgreSQL and MySQL drivers and `dalgo2ingitdb`
v0.6.1, whose direct `CreateCollection` ignores `Parent` and creates a root
collection named `c.Name` (`schema_modifier.go:42-99`). The driver-side MUSTs
of REQ:nesting-refused-without-capability, REQ:one-collection-per-create,
REQ:nested-drop-and-alter and REQ:extension-addressing bind only drivers that
implement the corresponding capability interface.

A driver without `SchemaModifier` at all gets the existing typed error from
every helper, so nothing is skipped silently. `AlterOp` extensions are the one
gap the helpers cannot close (see Error Handling).

#### REQ: additive-compatibility

The change MUST be purely additive:

- `SchemaModifier`, `Applier`, `AlterOp`, `Option`, `IfNotExists`, `IfExists`,
  `ResolveOptions` and the existing three helpers keep their signatures.
- A driver that implements `SchemaModifier` today compiles unchanged and needs
  neither `SubCollectionsAware` nor `ExtensionAware` to compile.
- A caller that passes no `Parent` and no extensions gets exactly the current
  helper behavior.
- New fields are appended to `Options` and `CollectionDef`. Keyed composite
  literals keep compiling. A search of `dal-go`, `ingitdb`, `datatug` and
  `dalgo2ingitdb@v0.6.1` found no unkeyed literal of either struct.
- A path-form `Name` (`"spaces/ext"`) with empty `Parent` stays
  driver-interpreted as today; this Feature neither changes nor deprecates it
  (see Open Questions).

## Acceptance Criteria

### AC: root-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "users"}` with no `Parent`
**When** `c.Path()` and `c.Path().String()` are called
**Then** `c.Path()` equals `CollectionPath{"users"}`, the string is `"users"`, `c.Parent == nil`, and `c.Path().Validate()` returns `nil`.

### AC: nested-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "servers", Parent: dbschema.CollectionPath{"projects", "environments"}}`
**When** `c.Path().String()` is called, and `c.Path()` is then modified
**Then** the string is `"projects/environments/servers"`, and modifying the returned slice does not change `c.Parent`.

### AC: invalid-path-segments (verifies REQ:collection-path-type)

**Given** the paths `{"projects", ""}`, `{"projects", "."}`, `{"projects", ".."}`, `{"a/b"}`, `{"a\\b"}`, `{"a\tb"}` and `{" projects"}`
**When** `Validate()` is called on each
**Then** each returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)`, and `CollectionPath{"projects", "queries"}.Validate()` and `CollectionPath(nil).Validate()` return `nil`.

### AC: invalid-path-rejected-before-dispatch (verifies REQ:guard-precedence)

**Given** a stub driver implementing `SchemaModifier` and `SubCollectionsAware` (returning `true`) that records every call
**When** `ddl.CreateCollection` is called with `Parent{"projects"}, Name: "a/b"` and with `Parent{".."}, Name: "queries"`, and `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` are called with `CollectionPath{"projects", ".."}` and with an empty path
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: parent-refused-by-flat-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver that implements `SchemaModifier` but not `SubCollectionsAware`, and a second stub that implements it returning `false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}})`, `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"})` and `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"})` are called on each
**Then** each returns `*dbschema.NotSupportedError` with `Op` equal to `"CreateCollection"`, `"DropCollection"` or `"AlterCollection"` respectively and `errors.Is(err, dal.ErrNotSupported)` true; no stub method is invoked; and `ddl.SupportsSubCollections(db)` is `false` for both.

### AC: legacy-driver-guarded-by-helper (verifies REQ:helpers-are-the-guarantee, REQ:nesting-refused-without-capability)

**Given** a stub driver with the v0.80.5 shape of `dalgo2ingitdb` v0.6.1 (implements `SchemaModifier`; implements neither `SubCollectionsAware` nor `ExtensionAware`; its `CreateCollection` records the definition and would create a root collection named `c.Name`)
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}})` is called
**Then** it returns `*dbschema.NotSupportedError` with `Op == "CreateCollection"`, the stub's `CreateCollection` is not invoked, and so no root `queries` collection is recorded.

### AC: parent-dispatched-to-nesting-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` and `SubCollectionsAware` returning `true`
**When** `ddl.CreateCollection` is called with `Name: "queries", Parent: CollectionPath{"projects"}`
**Then** it returns the stub's result, the stub receives the `CollectionDef` with the same `Name` and `Parent`, and `ddl.SupportsSubCollections(db)` is `true`.

### AC: at-helpers-dispatch-path-form (verifies REQ:nested-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` and `SubCollectionsAware` returning `true`, which records the `name` each method receives
**When** `ddl.DropCollectionAt(ctx, db, CollectionPath{"projects", "environments", "servers"}, ddl.IfExists())`, `ddl.AlterCollectionAt(ctx, db, CollectionPath{"projects", "queries"}, op)` and `ddl.DropCollectionAt(ctx, db, CollectionPath{"users"})` are called
**Then** the stub's `DropCollection` receives `"projects/environments/servers"` with `IfExists` set, its `AlterCollection` receives `"projects/queries"` with `op`, and its `DropCollection` receives `"users"`.

### AC: no-schema-modifier-is-typed-error (verifies REQ:helpers-are-the-guarantee, REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier` (as for a store whose schema comes from elsewhere), and an extension `ext` addressed to some other target
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}}, ddl.WithExtension(ext))` is called without strict mode, and again with `ddl.StrictExtensions()`
**Then** the first call returns `*dbschema.NotSupportedError` with `Reason` `"driver does not implement ddl.SchemaModifier"`; the second returns `*dbschema.NotSupportedError` whose `Reason` names `ext`'s type and target; and neither returns `nil`.

### AC: with-extension-accumulates (verifies REQ:with-extension-option, REQ:extension-type)

**Given** two values `e1`, `e2` of test types implementing `ddl.Extension`
**When** `ddl.ResolveOptions(ddl.WithExtension(e1), ddl.IfNotExists(), ddl.WithExtension(nil), ddl.StrictExtensions(), ddl.WithExtension(e2))` is called
**Then** the result has `Extensions == []ddl.Extension{e1, e2}` in that order, `StrictExtensions == true`, `IfNotExists == true` and `IfExists == false`; and `ddl.ResolveOptions()` has `Extensions == nil` and `StrictExtensions == false`.

### AC: foreign-extension-ignored-by-default (verifies REQ:extension-addressing)

**Given** a stub driver implementing `SchemaModifier` and `ExtensionAware` with target `"example.com/sqlstub"`, and an extension `ext` with target `"github.com/ingitdb/dalgo2ingitdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` and `ddl.DropCollection(ctx, db, "users", ddl.WithExtension(ext))` are called
**Then** both dispatch to the stub, which receives `ext` unchanged in its resolved `Options.Extensions`, `SupportsExtension` is not called, and neither helper returns an error of its own. The same holds for a stub that does not implement `ExtensionAware`.

### AC: foreign-extension-refused-when-strict (verifies REQ:extension-addressing, REQ:with-extension-option)

**Given** the stubs and `ext` of AC:foreign-extension-ignored-by-default
**When** the same two helper calls are made with `ddl.StrictExtensions()` added
**Then** each returns `*dbschema.NotSupportedError` whose `Reason` contains `"github.com/ingitdb/dalgo2ingitdb"` and `ext`'s Go type name, and the stubs' `CreateCollection` and `DropCollection` are not invoked.

### AC: addressed-unrecognised-extension-refused (verifies REQ:extension-addressing, REQ:extension-aware-capability)

**Given** a stub driver implementing `SchemaModifier` and `ExtensionAware` with target `"example.com/stubdb"`, whose `SupportsExtension` returns `false`, and an extension `ext` with target `"example.com/stubdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` and `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"users"}, ddl.WithExtension(ext))` are called, with and without `ddl.StrictExtensions()`
**Then** every call returns `*dbschema.NotSupportedError` whose `Op` is the helper's operation and whose `Reason` names `ext`'s Go type and target, and no stub method other than `DDLExtensionTarget` and `SupportsExtension` is invoked.

### AC: addressed-recognised-extension-dispatched (verifies REQ:extension-addressing, REQ:extension-aware-capability)

**Given** a stub driver with target `"example.com/stubdb"` whose `SupportsExtension("CreateCollection", ext)` returns `true` for `ext` with that target
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext), ddl.StrictExtensions())` is called
**Then** the stub's `CreateCollection` is invoked once, and `ddl.ResolveOptions` over the options it receives yields `Extensions == []ddl.Extension{ext}`.

### AC: extension-without-target-rejected (verifies REQ:extension-addressing)

**Given** an extension whose `ExtensionTarget()` returns `""`, and a stub driver implementing `SchemaModifier`
**When** `ddl.CreateCollection` or `ddl.DropCollection` is called with it
**Then** the helper returns an error satisfying `errors.Is(err, ddl.ErrInvalidExtension)` and the stub's method is not invoked.

### AC: guard-order (verifies REQ:guard-precedence)

**Given** a stub `dal.DB` that implements neither `SchemaModifier` nor any capability, and an extension with target `""`
**When** `ddl.CreateCollection` is called with `Parent{".."}, Name: "q"` and that extension; then with `Parent{"projects"}, Name: "q"` and that extension; then with `Parent{"projects"}, Name: "q"` and no extension
**Then** the calls fail, in order, with `dbschema.ErrInvalidCollectionPath`, then `ddl.ErrInvalidExtension`, then `*dbschema.NotSupportedError` (driver does not implement `ddl.SchemaModifier`).

### AC: legacy-driver-and-caller-unchanged (verifies REQ:additive-compatibility)

**Given** a driver type and call sites written against v0.80.5 (a `SchemaModifier` with no capability methods; calls with keyed `CollectionDef` literals and only `IfNotExists`/`IfExists`), kept as a test fixture
**When** the fixture is built against this change and `ddl.CreateCollection`, `ddl.DropCollection` and `ddl.AlterCollection` are called with no `Parent` and no extensions
**Then** it compiles, each call reaches the driver exactly as before with identical arguments, and the existing `go test ./ddl/... ./dbschema/...` suite passes without modification.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:extension-addressing)

Consumer conformance statement. The implementation, and the test that runs
this AC and the two below, belong to
[ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16).
This Feature only guarantees that the API makes them expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, which reports `ddl.SupportsSubCollections(db) == true` and implements `ddl.ExtensionAware` with target `dalgo2ingitdb.ExtensionTarget`, and an extension type exported by `dalgo2ingitdb` that carries an `ingitdb.RecordFileDef` and returns that target
**When** the caller runs, with `ddl.StrictExtensions()` on both calls,
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>), ddl.StrictExtensions())`
and then
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}, Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>), ddl.StrictExtensions())`
**Then** both calls return `nil`; `projects/.collection/definition.yaml` has `record_file` equal to the first extension's values; `projects/.collection/subcollections/queries/definition.yaml` exists and its `record_file` is exactly `name: '{key}/{key}.query.json'`, `format: json`, `type: map[string]any`, `records_dir: '.'`; no `{key}.yaml` default record file is written for either collection; and creating `queries` before `projects` exists returns a non-nil error and writes nothing.

### AC: dalgo2ingitdb-depth-two-ancestor-required (verifies REQ:one-collection-per-create)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries after `projects` is created, and no `projects/environments` collection
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "servers", Parent: dbschema.CollectionPath{"projects", "environments"}}, ddl.WithExtension(<record file '{key}/{key}.server.json', json, map[string]any, '.'>), ddl.StrictExtensions())` is called
**Then** it returns a non-nil error, and no `servers` definition exists anywhere under `projects/`.

### AC: dalgo2ingitdb-depth-two-created-and-dropped (verifies REQ:one-collection-per-create, REQ:nested-drop-and-alter)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries with `projects` and `projects/dbdrivers` created (record file `'{key}/{key}.dbdriver.json'`)
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "dbservers", Parent: dbschema.CollectionPath{"projects", "dbdrivers"}}, ddl.WithExtension(<record file '{key}/{key}.dbserver.json', json, map[string]any, '.'>), ddl.StrictExtensions())` is called, followed by `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "dbdrivers", "dbservers"}, ddl.AddField(fd))` and `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "dbdrivers", "dbservers"})`
**Then** the create returns `nil` and writes `projects/.collection/subcollections/dbdrivers/subcollections/dbservers/definition.yaml` with that `record_file`; the alter returns `nil` and adds the field to that same file; the drop returns `nil` and removes that definition while `projects/.collection/subcollections/dbdrivers/definition.yaml` remains.

## Architecture

| File | Change |
|---|---|
| `dbschema/collection_path.go` | New: `CollectionPath`, `String`, `Validate`, `ErrInvalidCollectionPath`. |
| `dbschema/collection_def.go` | Add `Parent CollectionPath` and `Path()`; godoc for the nesting meaning. |
| `ddl/options.go` | Add `Options.Extensions`, `Options.StrictExtensions`, `WithExtension`, `StrictExtensions`; extend the godoc with the addressing rule next to the mismatched-option rule. |
| `ddl/extension.go` | New: `Extension`, `ExtensionAware`, `ErrInvalidExtension`, and the internal addressing check. |
| `ddl/subcollections.go` | New: `SubCollectionsAware`, `SupportsSubCollections`. |
| `ddl/operations.go` | `CreateCollection` gains the precedence-ordered guards; `DropCollection` gains the extension guard; new `DropCollectionAt` and `AlterCollectionAt`. `AlterCollection` is unchanged. |
| `spec/features/ddl/options/`, `spec/features/dbschema/collection-def/` | Cross-link to this Feature once it is Approved. |

## Error Handling and Failure Modes

| Failure mode | Result |
|---|---|
| Invalid path segment (empty, `.`, `..`, `/`, `\`, control character, whitespace) | error, `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; no dispatch |
| Extension with empty target | error, `errors.Is(err, ddl.ErrInvalidExtension)`; no dispatch |
| Foreign extension, default mode | ignored |
| Foreign extension, `StrictExtensions()` | `*dbschema.NotSupportedError` naming type and target; no dispatch |
| Extension addressed to this driver, not recognised | `*dbschema.NotSupportedError` naming type and target; no dispatch |
| Driver does not implement `SchemaModifier` (for example a store whose schema is defined elsewhere) | existing `*dbschema.NotSupportedError`; no dispatch |
| Nested path or `Parent`, driver lacks nesting | `*dbschema.NotSupportedError`; no dispatch |
| An ancestor collection does not exist (nesting driver) | driver-specific non-nil error; nothing created |
| Recognised extension with an invalid value (e.g. an inGitDB `RecordFileDef` failing `Validate`) | driver-specific error; the surface is supported, the value is not |
| Extension set on an `AlterOp` (via its constructor's `opts`) | **not guarded by the helpers**, which cannot see `AlterOp` options. A driver implementing `ExtensionAware` applies REQ:extension-addressing. A legacy driver silently drops the extension, even under `StrictExtensions()`. |
| Direct driver method call on a legacy driver with `Parent` or extensions | not guarded; the legacy driver may ignore them (REQ:helpers-are-the-guarantee) |

## Testing Strategy

In-package Go tests in `dbschema` and `ddl` with stub `dal.DB` values, following
the existing `ddl/operations_test.go` pattern. The legacy fixtures are test-only
driver and call-site files frozen at the v0.80.5 shape. The three
`dalgo2ingitdb-*` ACs are verified in `ingitdb/dalgo2ingitdb`'s suite once #16
lands, against a DALgo release carrying this Feature. No Rehearse stubs: every
AC has a direct Go test surface.

## Not Doing / Out of Scope

- **Concrete extension types in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Implementing any driver change.** dalgo2ingitdb's extension type, target
  constant, `SubCollectionsAware` and `ExtensionAware` implementation, the
  depth-N ancestor check and nested Drop/Alter resolution are all
  ingitdb/dalgo2ingitdb#16.
- **OpenVaultDB-backed stores.** DataTug's remote path does not create its
  schema through DALgo DDL. An OpenVaultDB store's schema, including
  subcollections and per-collection `record_file`, comes from the OpenVaultDB
  manifest ([openvaultdb/openvaultdb-go#26](https://github.com/openvaultdb/openvaultdb-go/issues/26)).
  If a consumer calls the `ddl` helpers against a DALgo driver for such a store
  that does not implement `SchemaModifier`, it gets the existing typed
  `*dbschema.NotSupportedError` (AC:no-schema-modifier-is-typed-error), so
  nothing is skipped silently.
- **Nested database roots.** DataTug keeps each project's user data as a
  separate inGitDB database root at `projects/<id>/data/ingitdb/`, with `.ingr`
  datasets. `Parent` declares a subcollection inside one database; it cannot
  declare a database root inside a record. The consumer opens a separate DALgo
  database at that path and creates its collections there.
- **Mapping nesting onto SQL** (parent-key columns, foreign keys, table naming).
  SQL drivers are refused by the helpers until a separate Feature designs it.
- **Declaring a subcollection tree in one call** (`SubCollections []CollectionDef`),
  rejected in REQ:one-collection-per-create.
- **Read-side round trip.** `dbschema.DescribeCollection` takes a
  `*dal.CollectionRef` (`dbschema/reader_helpers.go:44`) and does not report
  `Parent` or extensions in this Feature.
- **Helper-level extension guard for `AlterOp`s** (see Error Handling).
- **Changing or deprecating path-form names** (`Name: "spaces/ext"`).

## Open Questions

- **Path-form name deprecation.** dalgo2ingitdb accepts `Name: "projects/queries"`
  with no `Parent` today. Should DALgo deprecate that form in favour of `Parent`
  and the `*At` helpers, so a slash in `Name` becomes invalid for every driver,
  or keep both forms indefinitely? Needs a founder decision; this Feature keeps
  both.
- **Strict extensions by default.** Should `StrictExtensions()` become the
  default in a future major version of DALgo (dal-go/dalgo v1 or later), with a
  `LenientExtensions()` opt-out for multi-backend schema code? Needs a founder
  decision; this Feature keeps lenient as the default so it stays additive.

---
*This document follows the https://specscore.md/feature-specification*
