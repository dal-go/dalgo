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

Two additive changes to the schema API, so that a consumer can create its whole
schema through DALgo when the driver needs more than fields and indexes:

1. **Subcollection declaration.** `dbschema.CollectionDef` gains a `Parent`
   field of the new type `dbschema.CollectionPath` (ancestor collection names,
   root first). A nested collection is created with one `CreateCollection`
   call per collection, after its parent exists. Drivers advertise nesting
   through a new `ddl.SubCollectionsAware` capability; the `ddl.CreateCollection`
   helper refuses a non-empty `Parent` with `*dbschema.NotSupportedError` when
   the driver does not advertise it.
2. **Driver extensions.** `ddl.Options` gains `Extensions []ddl.Extension`, set
   through `ddl.WithExtension(ext)`. An `Extension` is a driver-defined value
   that names the adapter it is addressed to. Extensions addressed to another
   adapter are ignored; an extension addressed to this driver that the driver
   does not recognise is refused with `*dbschema.NotSupportedError`.

DALgo core stays driver-agnostic: it defines only the slot, the addressing
rule and the refusal; the concrete extension types (for example an inGitDB
`record_file`) live in driver modules. Existing drivers and callers compile
unchanged.

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
(`ingitdb-go/ingitdb@v0.6.1/record_file_def.go:22-52`). The driver does accept a
path-form name (`"spaces/ext"`, `schema_modifier.go:48-50` and
`createSubCollection`, `schema_modifier.go:349`), but that convention is
undocumented in DALgo core, and a driver without nesting has no way to tell a
nested request from a collection whose name contains a slash.

The end user is DataTug's project store (`datatug/datatug` Feature
`dalgo-project-store`, REQ `canonical-project-layout` and REQ
`driver-prerequisites-are-recorded`). It needs `projects/{id}` with
subcollections `queries`, `entities`, `environments/{id}/servers` and others,
each with `record_file` `name: '{key}/{key}.<suffix>.json'`, `format: json`,
`type: map[string]any`, `records_dir: '.'`. The founder's rule is that DataTug
writes, schema creation included, go through DALgo and are fixed at source
with no bootstrap workaround ([ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16)).

## Design Principles

- **Core names the slot, drivers own the vocabulary.** No inGitDB, SQL or
  Firestore term enters `ddl` or `dbschema`.
- **No silent loss.** A declaration that the target driver cannot honour
  (a parent on a flat driver, an extension addressed to a driver that does not
  understand it) is an error, never a quiet default. That silent default is the
  exact defect behind dalgo2ingitdb#16.
- **Portable schema code.** One schema-creation routine can carry extensions
  for several backends and run against any of them.
- **Guard in the helper, mirror in the driver.** The `ddl` helpers refuse
  before dispatch, so drivers written before this Feature cannot silently
  ignore the new declarations when called through the helpers.

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
contains `/`, or has leading or trailing whitespace, and `nil` otherwise. An
empty (`nil` or zero-length) path is valid and denotes the root level.

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
`Path().String()`, is `projects/environments/servers`, and that string is the
`name` argument `DropCollection` and `AlterCollection` receive for the nested
collection. The string form matches the path-form name dalgo2ingitdb already
accepts. (`dbschema.DescribeCollection` takes a `*dal.CollectionRef`,
`dbschema/reader_helpers.go:44`; addressing a nested collection there is out of
scope.)

#### REQ: one-collection-per-create

Each `CreateCollection` call MUST create exactly one collection, the one named
by `c.Path()`. A driver that supports nesting MUST return a non-nil error, and
MUST NOT create any ancestor implicitly, when the collection at `c.Parent` does
not exist. `IfNotExists` applies to the collection at `c.Path()` only.

Rationale (why `Parent`, not `SubCollections []CollectionDef` or a key-path
parent): a recursive `SubCollections` tree would force one options set on a
whole tree, while DataTug needs a different `record_file` per subcollection
(`.query.json`, `.entity.json`, `.server.json`); it would make `IfNotExists`
ambiguous on a partly existing tree and make partial failure likely on
non-transactional drivers; and `DropCollection`, `AlterCollection` and
`DescribeCollection` already address one collection by name. A key-path parent
such as `projects/{id}` confuses schema with instances: a schema is declared
once per collection shape, not per parent record.

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

#### REQ: parent-refused-without-nesting

When `c.Parent` is non-empty, the `ddl.CreateCollection` helper MUST, before
dispatching to the driver:

1. return an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)`
   if `c.Path().Validate()` fails (this also rejects a `Name` that contains `/`
   when `Parent` is set, so the two nesting forms cannot be mixed);
2. return `*dbschema.NotSupportedError{Op: "CreateCollection", Backend: <adapter name>, Reason: "driver does not support subcollections"}`
   if `ddl.SupportsSubCollections(db)` is `false`.

In both cases the driver's `CreateCollection` MUST NOT be invoked. A driver
that implements `SchemaModifier` and receives a non-empty `Parent` directly
(bypassing the helper) while not supporting nesting MUST return the same
`*dbschema.NotSupportedError`. When `c.Parent` is empty the helper MUST behave
exactly as today and perform neither check.

### Driver extensions

#### REQ: extension-type

The `ddl` package MUST export:

```go
// Extension is a driver-defined creation or modification option.
// Concrete types live in driver modules, never in dalgo core.
type Extension interface {
    // ExtensionTarget returns the dal.Adapter.Name() of the driver the
    // extension is addressed to, e.g. "dalgo2ingitdb".
    ExtensionTarget() string
}

var ErrInvalidExtension = errors.New("ddl: invalid extension")
```

`Extension` is an interface rather than `any` so that an arbitrary value cannot
be passed by mistake, and so that every extension carries its addressee, which
the unknown-extension rule depends on.

#### REQ: with-extension-option

`ddl.Options` MUST gain the field `Extensions []Extension`, and the package
MUST export `func WithExtension(ext Extension) Option`. Each `WithExtension`
MUST append `ext` to `Options.Extensions`, preserving call order across all
options passed to `ResolveOptions`. A `nil` `ext` MUST be ignored, the same as
a `nil` `Option` in `ResolveOptions` (`ddl/options.go:46-54`). `WithExtension`
MUST NOT change `IfNotExists` or `IfExists`. When several extensions of one
concrete type reach a driver, which one wins is the driver's documented choice.

#### REQ: extension-addressing

Extensions follow one rule, applied by the `ddl.CreateCollection` and
`ddl.DropCollection` helpers before dispatch and by every driver on every
operation that receives `Options`:

- **Addressed to another adapter** (`ext.ExtensionTarget()` is non-empty and
  differs from `db.Adapter().Name()`): ignored. The extension is still passed
  to the driver unchanged, and the driver MUST ignore it.
- **Addressed to this adapter and recognised** by the driver: honoured.
- **Addressed to this adapter and not recognised**: refused with
  `*dbschema.NotSupportedError{Op, Backend, Reason}`, where `Reason` names the
  extension's Go type. The operation MUST NOT be performed.
- **No addressee** (`ExtensionTarget()` returns `""`): refused with an error
  satisfying `errors.Is(err, ddl.ErrInvalidExtension)`.

Justification for refusing rather than ignoring an unknown extension addressed
to the driver: the existing ignore rule (`ddl/options.go:10-16`) covers hints
whose meaning is unambiguous, where there is nothing to do. An addressed
extension the driver does not understand is the opposite: the caller asked
this driver for a storage property it will not get, and ignoring it produces a
wrong on-disk layout that is found only later. Foreign extensions stay ignored,
consistently with that rule, so portable schema code can carry extensions for
several backends at once.

#### REQ: extension-recognition-capability

So the helpers can apply REQ:extension-addressing without knowing driver types,
the `ddl` package MUST export:

```go
type ExtensionAware interface {
    SupportsExtension(op string, ext Extension) bool
}
```

`op` is `"CreateCollection"` or `"DropCollection"`. For each extension whose
target equals `db.Adapter().Name()`, the helper MUST refuse with
`*dbschema.NotSupportedError` when the driver, resolved with `dal.As`, does not
implement `ExtensionAware` or returns `false`. A driver written before this
Feature therefore cannot silently drop an addressed extension when called
through the helpers. When `db.Adapter()` is `nil`, no extension can match an
adapter name, so every non-empty-target extension counts as foreign.

`AlterCollection` takes `AlterOp` values whose resolved `Options` the helper
cannot inspect. For `AlterOp` extensions, the driver alone applies
REQ:extension-addressing, on the `Options` its `Applier` receives.

### Compatibility

#### REQ: additive-compatibility

The change MUST be purely additive:

- `SchemaModifier`, `Applier`, `AlterOp`, `Option`, `IfNotExists`, `IfExists`,
  `ResolveOptions` and the three helpers keep their signatures.
- A driver that implements `SchemaModifier` today compiles unchanged and needs
  neither `SubCollectionsAware` nor `ExtensionAware`.
- A caller that passes no `Parent` and no extensions gets byte-for-byte the
  current helper behavior: no capability checks run and the call is dispatched
  as today.
- New fields are appended to `Options` and `CollectionDef`. Keyed composite
  literals keep compiling. A search of `dal-go`, `ingitdb`, `datatug` and
  `dalgo2ingitdb@v0.6.1` found no unkeyed literal of either struct.
- A path-form `Name` (`"spaces/ext"`) with empty `Parent` stays
  driver-interpreted as today; this Feature does not change or deprecate it
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

### AC: invalid-path-rejected (verifies REQ:collection-path-type, REQ:parent-refused-without-nesting)

**Given** a stub driver implementing `SchemaModifier` and `SubCollectionsAware` (returning `true`) that records every `CreateCollection` call
**When** `ddl.CreateCollection` is called with, in turn, `Parent{"projects"}` and `Name: "a/b"`, `Parent{""}` and `Name: "queries"`, and `Parent{" projects"}` and `Name: "queries"`
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: parent-refused-by-flat-driver (verifies REQ:parent-refused-without-nesting, REQ:sub-collections-capability)

**Given** a stub driver named `"flatdb"` that implements `SchemaModifier` but not `SubCollectionsAware`, and a second stub that implements it returning `false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}})` is called on each
**Then** each returns `*dbschema.NotSupportedError` with `Op == "CreateCollection"`, `Backend` equal to the stub's adapter name and `errors.Is(err, dal.ErrNotSupported)` true; neither stub's `CreateCollection` is invoked; and `ddl.SupportsSubCollections(db)` is `false` for both.

### AC: parent-dispatched-to-nesting-driver (verifies REQ:parent-refused-without-nesting, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` and `SubCollectionsAware` returning `true`
**When** `ddl.CreateCollection` is called with `Name: "queries", Parent: CollectionPath{"projects"}`
**Then** it returns the stub's result, the stub receives the `CollectionDef` with the same `Name` and `Parent`, and `ddl.SupportsSubCollections(db)` is `true`.

### AC: with-extension-accumulates (verifies REQ:with-extension-option, REQ:extension-type)

**Given** two values `e1`, `e2` of test types implementing `ddl.Extension`
**When** `ddl.ResolveOptions(ddl.WithExtension(e1), ddl.IfNotExists(), ddl.WithExtension(nil), ddl.WithExtension(e2))` is called
**Then** the result has `Extensions == []ddl.Extension{e1, e2}` in that order, `IfNotExists == true` and `IfExists == false`; and `ddl.ResolveOptions()` has `Extensions == nil`.

### AC: foreign-extension-ignored (verifies REQ:extension-addressing)

**Given** a stub driver with adapter name `"sqlstub"` implementing `SchemaModifier` but not `ExtensionAware`, and an extension whose `ExtensionTarget()` is `"dalgo2ingitdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` and `ddl.DropCollection(ctx, db, "users", ddl.WithExtension(ext))` are called
**Then** both dispatch to the stub, which receives the extension unchanged in its resolved `Options.Extensions`, and neither helper returns an error of its own.

### AC: addressed-unrecognised-extension-refused (verifies REQ:extension-addressing, REQ:extension-recognition-capability)

**Given** an extension `ext` whose `ExtensionTarget()` is `"stubdb"`, a stub driver named `"stubdb"` implementing `SchemaModifier` but not `ExtensionAware`, and a second stub named `"stubdb"` whose `SupportsExtension` returns `false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` and `ddl.DropCollection(ctx, db, "users", ddl.WithExtension(ext))` are called on each
**Then** every call returns `*dbschema.NotSupportedError` whose `Op` is the helper's operation, whose `Backend` is `"stubdb"` and whose `Reason` contains the extension's Go type name, and no stub method other than `SupportsExtension` is invoked.

### AC: addressed-recognised-extension-dispatched (verifies REQ:extension-addressing, REQ:extension-recognition-capability)

**Given** a stub driver named `"stubdb"` whose `SupportsExtension("CreateCollection", ext)` returns `true` for `ext` addressed to `"stubdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` is called
**Then** the stub's `CreateCollection` is invoked once and `ddl.ResolveOptions` over the options it receives yields `Extensions == []ddl.Extension{ext}`.

### AC: extension-without-target-rejected (verifies REQ:extension-addressing)

**Given** an extension whose `ExtensionTarget()` returns `""` and any stub driver implementing `SchemaModifier`
**When** `ddl.CreateCollection` or `ddl.DropCollection` is called with it
**Then** the helper returns an error satisfying `errors.Is(err, ddl.ErrInvalidExtension)` and the stub's method is not invoked.

### AC: legacy-driver-and-caller-unchanged (verifies REQ:additive-compatibility)

**Given** a driver type and call sites written against v0.80.5 (a `SchemaModifier` with no capability methods; calls with keyed `CollectionDef` literals and only `IfNotExists`/`IfExists`), kept as a test fixture
**When** the fixture is built against this change and `ddl.CreateCollection`, `ddl.DropCollection` and `ddl.AlterCollection` are called with no `Parent` and no extensions
**Then** it compiles, each call reaches the driver exactly as before with identical arguments, and the existing `go test ./ddl/... ./dbschema/...` suite passes without modification.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:extension-addressing)

Consumer conformance statement. The implementation, and the test that runs
this AC, belong to [ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16).
This Feature only guarantees that the API makes it expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, which reports `ddl.SupportsSubCollections(db) == true`, and an extension type exported by `dalgo2ingitdb` that carries an `ingitdb.RecordFileDef`, whose `ExtensionTarget()` is `"dalgo2ingitdb"` and which the driver's `SupportsExtension("CreateCollection", ext)` accepts
**When** the caller runs
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>))`
and then
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}, Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>))`
**Then** both calls return `nil`; `projects/.collection/definition.yaml` has `record_file` equal to the first extension's values; `projects/.collection/subcollections/queries/definition.yaml` exists and its `record_file` is exactly `name: '{key}/{key}.query.json'`, `format: json`, `type: map[string]any`, `records_dir: '.'`; no `{key}.yaml` default record file is written for either collection; and creating `queries` before `projects` exists returns a non-nil error and writes nothing.

## Architecture

| File | Change |
|---|---|
| `dbschema/collection_path.go` | New: `CollectionPath`, `String`, `Validate`, `ErrInvalidCollectionPath`. |
| `dbschema/collection_def.go` | Add `Parent CollectionPath` and `Path()`; godoc for the nesting meaning. |
| `ddl/options.go` | Add `Options.Extensions`, `WithExtension`; extend the godoc with the addressing rule next to the mismatched-option rule. |
| `ddl/extension.go` | New: `Extension`, `ExtensionAware`, `ErrInvalidExtension`. |
| `ddl/subcollections.go` | New: `SubCollectionsAware`, `SupportsSubCollections`. |
| `ddl/operations.go` | `CreateCollection` gains the parent guard; `CreateCollection` and `DropCollection` gain the extension guard. `AlterCollection` is unchanged. |
| `spec/features/ddl/options/`, `spec/features/dbschema/collection-def/` | Cross-link to this Feature once it is Approved. |

## Error Handling and Failure Modes

| Failure mode | Result |
|---|---|
| Invalid `Parent` or `Name` segment with `Parent` set | error, `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; no dispatch |
| Non-empty `Parent`, driver lacks nesting | `*dbschema.NotSupportedError{Op: "CreateCollection"}`; no dispatch |
| Parent collection does not exist | driver-specific non-nil error; nothing created |
| Extension with empty target | error, `errors.Is(err, ddl.ErrInvalidExtension)`; no dispatch |
| Extension addressed to this driver, not recognised | `*dbschema.NotSupportedError` naming the extension type; no dispatch |
| Extension addressed to another adapter | ignored |
| Recognised extension with an invalid value (e.g. an inGitDB `RecordFileDef` failing `Validate`) | driver-specific error; the surface is supported, the value is not |

## Testing Strategy

In-package Go tests in `dbschema` and `ddl` with stub `dal.DB` values, following
the existing `ddl/operations_test.go` pattern. The legacy-compatibility fixture
is a test-only driver and call-site file frozen at the v0.80.5 shape.
AC:dalgo2ingitdb-creates-datatug-queries is verified in `ingitdb/dalgo2ingitdb`'s
suite once #16 lands, against a DALgo release carrying this Feature. No Rehearse
stubs: every AC has a direct Go test surface.

## Not Doing / Out of Scope

- **Concrete extension types in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Implementing either driver change.** dalgo2ingitdb's extension type,
  `SubCollectionsAware` and `ExtensionAware` implementation are
  dalgo2ingitdb#16.
- **Mapping nesting onto SQL** (parent-key columns, foreign keys, table naming).
  SQL drivers keep returning `NotSupportedError` for `Parent` until a separate
  Feature designs it.
- **Declaring a subcollection tree in one call** (`SubCollections []CollectionDef`),
  rejected in REQ:one-collection-per-create.
- **Read-side round trip.** `dbschema.DescribeCollection` does not report
  `Parent` or extensions in this Feature.
- **Helper-level extension guard for `AlterOp`s.** Drivers apply
  REQ:extension-addressing to `AlterOp` options; the `AlterCollection` helper
  does not.
- **Changing or deprecating path-form names** (`Name: "spaces/ext"`).

## Open Questions

- **Path-form name deprecation.** dalgo2ingitdb accepts `Name: "projects/queries"`
  with no `Parent` today. Should DALgo deprecate that form in favour of
  `Parent`, so a slash in `Name` becomes invalid for every driver, or keep both
  forms indefinitely? Needs a founder decision; this Feature keeps both.
- **Addressing by adapter name.** Addressing uses `dal.Adapter.Name()`
  (`"dalgo2ingitdb"`). A DB wrapper that reports its own adapter name would make
  inner-driver extensions look foreign and be ignored. Should the wrappers in
  this module forward the inner adapter name, or should `Extension` match on a
  separate driver ID? Needs a founder decision.
- **Extension guard on `DescribeCollection`.** Should read-side schema calls
  accept extensions later, for example to ask inGitDB for the `record_file` back?
  Deferred to the read-side round-trip follow-up.

---
*This document follows the https://specscore.md/feature-specification*
