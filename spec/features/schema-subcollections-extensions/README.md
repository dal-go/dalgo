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

Two changes to the schema API, so that a consumer can create nested collections,
and pass driver-specific storage options for them, through DALgo:

1. **Subcollection declaration.** `dbschema.CollectionDef` gains a `Parent`
   field of the new type `dbschema.CollectionPath` (ancestor collection names,
   root first). `Parent` is the **only** nesting form: a collection name that
   contains a path separator is invalid. A nested collection is created with
   one `CreateCollection` call per collection, after every ancestor exists.
   `SchemaModifier.DropCollection` and `AlterCollection` address a collection
   by `CollectionPath`; new helpers `ddl.DropCollectionAt` and
   `ddl.AlterCollectionAt` take a path, and the string helpers
   `ddl.DropCollection` / `ddl.AlterCollection` address root collections only.
   Every driver states whether it supports nesting through
   `SupportsSubCollections()`, which becomes part of `SchemaModifier`.
2. **Driver extensions.** `ddl.Options` gains `Extensions []ddl.Extension`, set
   through `ddl.WithExtension(ext)`. Each extension carries a stable target ID,
   a constant exported by the driver module that defines it (for example
   `dalgo2ingitdb.ExtensionTarget`). Extensions are strict: an extension is
   honoured only when the driver's `SupportsExtension(op, ext)` returns `true`,
   and is otherwise refused with `*dbschema.NotSupportedError`. There is no
   mode that ignores an extension.

The `ddl` helpers guard every call before dispatch, and every in-org driver
implementing `SchemaModifier` MUST enforce the same rules on direct calls
(REQ:in-org-drivers-comply).

DALgo is in private beta, so this Feature changes the `SchemaModifier`
interface rather than working around it (founder direction, 2026-09-17: "Don't
worry too much about what already exists, we are still in private beta stage
and can do changes.").

The concrete scope this Feature unblocks is the DataTug project store's
collection schema on the local inGitDB path (see REQ:collection-def-parent for
the list). It does not cover nested database roots or OpenVaultDB-backed stores
(see Not Doing).

DALgo core stays driver-agnostic: it defines only the slot, the addressing
rule and the refusals. Concrete extension types, such as an inGitDB
`record_file`, live in driver modules.

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
- `SchemaModifier.DropCollection` and `AlterCollection` take a bare
  `name string` (`ddl/modifier.go`), so a nested collection has no typed
  address.
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

The driver improvises nesting with a slash in the name (`"spaces/ext"`,
`schema_modifier.go:48-50`, `createSubCollection` at `:349`), which shows why a
typed form is needed:

- a driver without nesting cannot tell a nested request from a collection whose
  name contains a slash;
- `createSubCollection` checks that only the root exists
  (`schema_modifier.go:350-353`), not intermediate ancestors;
- `DropCollection` (`schema_modifier.go:107-110`) and `AlterCollection`
  (`schema_modifier.go:141`) look for `<name>/.collection/definition.yaml`,
  while nested definitions live under
  `<root>/.collection/subcollections/<sub>/definition.yaml`, so a nested
  collection cannot be dropped or altered by its slash name.

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
- **No silent loss.** A declaration that the target driver cannot honour
  (a parent on a flat driver, an extension the driver does not honour) is an
  error, never a quiet default. That silent default is the exact defect behind
  dalgo2ingitdb#16.
- **One way to say it.** Nesting is `Parent` / `CollectionPath` only; a name is
  always a single segment.
- **Addressing by compiled constant, not by display name.** Extension targets
  are constants exported by the defining module, never `dal.Adapter.Name()`.
- **Guard in the helper and in the driver.** The `ddl` helpers refuse before
  dispatch, and every in-org driver enforces the same rules on direct calls.

## Behavior

### Subcollection declaration

#### REQ: collection-path-type

The `dbschema` package MUST export:

```go
// CollectionPath is an ordered list of collection names, root first.
// It names a collection in the schema, not a record: it carries no record IDs.
type CollectionPath []string

func (p CollectionPath) String() string // segments joined with "/", for messages only
func (p CollectionPath) Validate() error

var ErrInvalidCollectionPath = errors.New("dbschema: invalid collection path")
```

`Validate` MUST return an error that satisfies
`errors.Is(err, dbschema.ErrInvalidCollectionPath)` when any segment is empty,
is `.` or `..`, contains `/` or `\`, contains a control character, or has
leading or trailing whitespace. It MUST return `nil` otherwise. An empty
(`nil` or zero-length) path is valid only as a `Parent`, where it denotes the
root level; as the address of a collection it is invalid.

`String()` is for display and error messages. It MUST NOT be used as an
address: no API in `ddl` accepts a `/`-joined string as a nested collection.

#### REQ: collection-def-parent

`dbschema.CollectionDef` MUST gain a field `Parent CollectionPath` and a method
`Path() CollectionPath` returning a new slice equal to `Parent` followed by
`Name`. A zero `Parent` means a root collection.

A non-empty `Parent` declares that the collection is a subcollection nested
under records of the collection at `Parent`. The declaration is per collection
shape: every record of the parent collection has the same subcollection schema.
`Parent` carries collection names only, never record IDs (`projects`, not
`projects/{id}`).

`CollectionDef{Name: "servers", Parent: CollectionPath{"projects", "environments"}}`
declares the collection whose records live at
`projects/{id}/environments/{id}/servers/{id}`.

The DataTug collections this makes expressible, all at most three segments
deep, are: `projects`; `projects/credentials`, `projects/queries`,
`projects/entities`, `projects/environments`, `projects/dbmodels`,
`projects/boards`, `projects/recordsets`, `projects/folders`,
`projects/dbdrivers`; and `projects/environments/servers`,
`projects/environments/catalogs`, `projects/dbdrivers/dbservers`.

#### REQ: parent-is-the-only-nesting-form

`Parent` MUST be the only way to declare or address a subcollection. A
`CollectionDef.Name`, or a `name` passed to `ddl.DropCollection` /
`ddl.AlterCollection`, that is not a single valid path segment (in particular,
one containing `/`) MUST be rejected with an error satisfying
`errors.Is(err, dbschema.ErrInvalidCollectionPath)`. This applies whether or
not `Parent` is set.

The slash-name handling in `dalgo2ingitdb` (`schema_modifier.go:48-50` and
`createSubCollection`) is removed as part of ingitdb/dalgo2ingitdb#16, replaced
by `Parent`.

#### REQ: one-collection-per-create

Each `CreateCollection` call MUST create exactly one collection, the one named
by `c.Path()`. A driver whose `SupportsSubCollections()` is `true` MUST return
a non-nil error, and MUST NOT create anything, when **any** ancestor path
(`c.Parent[:1]`, `c.Parent[:2]`, and so on up to `c.Parent`) does not exist. It
MUST NOT create an ancestor implicitly. `IfNotExists` applies to the collection
at `c.Path()` only.

Rationale (why `Parent`, not `SubCollections []CollectionDef` or a key-path
parent): a recursive `SubCollections` tree would force one options set on a
whole tree, while DataTug needs a different `record_file` per subcollection
(`.query.json`, `.entity.json`, `.server.json`). It would make `IfNotExists`
ambiguous on a partly existing tree and make partial failure likely on
non-transactional drivers. `DropCollection` and `AlterCollection` already
address one collection at a time. A key-path parent such as `projects/{id}`
confuses schema with instances: a schema is declared once per collection
shape, not per parent record.

#### REQ: path-addressed-drop-and-alter

The `SchemaModifier` interface MUST change to address collections by path:

```go
type SchemaModifier interface {
    SubCollectionsAware
    ExtensionAware
    CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error
    DropCollection(ctx context.Context, path dbschema.CollectionPath, opts ...Option) error
    AlterCollection(ctx context.Context, path dbschema.CollectionPath, ops ...AlterOp) error
}
```

The `ddl` package MUST export:

```go
func DropCollectionAt(ctx context.Context, db dal.DB, path dbschema.CollectionPath, opts ...Option) error
func AlterCollectionAt(ctx context.Context, db dal.DB, path dbschema.CollectionPath, ops ...AlterOp) error
```

and keep `ddl.DropCollection(ctx, db, name string, opts...)` and
`ddl.AlterCollection(ctx, db, name string, ops...)` for **root collections
only**: each MUST be exactly `DropCollectionAt` / `AlterCollectionAt` with
`CollectionPath{name}`, so a `name` that is not a single valid segment is
rejected per REQ:parent-is-the-only-nesting-form. Nested collections are
addressed only through the `*At` helpers.

A driver whose `SupportsSubCollections()` is `true` MUST resolve a multi-segment
`path` to the nested collection that `CreateCollection` created for it.
Dropping a collection that has subcollections MUST either remove their
definitions too or return an error; it MUST NOT leave orphaned subcollection
definitions. What a nested drop does with the dropped collection's **record
data** under each parent record is not decided by this Feature (see Open
Questions).

#### REQ: sub-collections-capability

The `ddl` package MUST export:

```go
type SubCollectionsAware interface {
    SupportsSubCollections() bool
}

func SupportsSubCollections(db dal.DB) bool
```

`SubCollectionsAware` is embedded in `SchemaModifier`, so every driver answers
it. The helper MUST resolve `SchemaModifier` with `dal.As` and MUST return
`false` when the DB does not implement it. The value MUST stay constant for the
lifetime of a DB value, mirroring `TransactionalDDL` (`ddl/transactional.go:28-42`).

#### REQ: nesting-refused-without-capability

When `ddl.CreateCollection` receives a non-empty `c.Parent`, or
`ddl.DropCollectionAt` / `ddl.AlterCollectionAt` receives a path with more than
one segment, and `SupportsSubCollections()` is `false`, the helper MUST return
`*dbschema.NotSupportedError{Op, Backend: <adapter name>, Reason: "driver does not support subcollections"}`
and MUST NOT invoke the driver. `Op` is `"CreateCollection"`,
`"DropCollection"` or `"AlterCollection"`.

A driver whose `SupportsSubCollections()` is `false` MUST return the same error
when it receives a non-empty `Parent` or a multi-segment path directly.

### Driver extensions

#### REQ: extension-type

The `ddl` package MUST export:

```go
// Extension is a driver-defined creation or modification option.
// Concrete types live in driver modules, never in dalgo core.
type Extension interface {
    // ExtensionTarget returns the stable target ID of the module that
    // defines the extension type (e.g. dalgo2ingitdb.ExtensionTarget).
    ExtensionTarget() string
}

var ErrInvalidExtension = errors.New("ddl: invalid extension")
```

`Extension` is an interface rather than `any`, so an arbitrary value cannot be
passed by mistake. The target ID identifies the extension's origin in error
messages and gives drivers a stable key for recognising types.

A target ID is an opaque string that the defining module fixes. It MUST NOT be
derived from `dal.Adapter.Name()`, and it MUST NOT change across the module's
major versions: a `/vN` import-path suffix MUST NOT appear in it. The
recommended value is the module's import path **without** any major-version
suffix, documented next to the constant as never changing. For example
`dalgo2ingitdb.ExtensionTarget = "github.com/ingitdb/dalgo2ingitdb"`, which stays
the same in a future `github.com/ingitdb/dalgo2ingitdb/v2`.

#### REQ: extension-aware-capability

The `ddl` package MUST export:

```go
type ExtensionAware interface {
    // SupportsExtension reports whether the driver honours ext for op.
    // It is consulted for every extension, whatever its target, so a
    // driver can honour extension types defined by other modules.
    SupportsExtension(op string, ext Extension) bool
}
```

`ExtensionAware` is embedded in `SchemaModifier`. `op` is `"CreateCollection"`,
`"DropCollection"` or `"AlterCollection"`. The result for a given `op` and
extension type MUST stay constant for the lifetime of a DB value. A driver that
honours no extensions returns `false` for every call.

A driver MAY honour another module's extension type (for example, a future
OpenVaultDB driver honouring the inGitDB `record_file` extension) by returning
`true` for it.

#### REQ: with-extension-option

`ddl.Options` MUST gain the field `Extensions []Extension`, and the package MUST
export `func WithExtension(ext Extension) Option`.

- Each `WithExtension` MUST append `ext` to `Options.Extensions`, preserving
  call order across all options passed to `ResolveOptions`. A `nil` `ext` MUST
  be ignored, the same as a `nil` `Option` in `ResolveOptions`
  (`ddl/options.go:46-54`).
- `WithExtension` does not change `IfNotExists` or `IfExists`.
- When several extensions of one concrete type reach a driver, the driver
  documents which one wins.

#### REQ: extension-addressing

For each extension `ext` in the resolved options, the first matching row
applies:

| # | Case | Result |
|---|---|---|
| 1 | `ext.ExtensionTarget() == ""` | refused: `errors.Is(err, ddl.ErrInvalidExtension)` |
| 2 | `SupportsExtension(op, ext)` is `true` | honoured |
| 3 | otherwise (unrecognised, whatever its target) | refused: `*dbschema.NotSupportedError` |

Every `*dbschema.NotSupportedError` for an extension MUST have `Reason` naming
the extension's Go type and its target ID. A refused call MUST NOT perform the
operation.

Every helper (`ddl.CreateCollection`, `ddl.DropCollection`,
`ddl.DropCollectionAt`, `ddl.AlterCollection` and `ddl.AlterCollectionAt`) MUST
apply this table before dispatch. For the two alter helpers, the options are
the resolved `Options` of each `AlterOp` (REQ:alter-op-options-visible) with
`op = "AlterCollection"`, and a refused extension on any one op refuses the
whole call before any op is applied. Every driver MUST apply the same table on
direct calls, including to the `Options` its `Applier` receives for each
`AlterOp`.

Justification for having no ignore mode: the existing ignore rule
(`ddl/options.go:10-16`) covers hints whose meaning is unambiguous, where there
is nothing to do. An extension the driver does not honour is the opposite: the
caller asked for a storage property it will not get, and ignoring it produces a
wrong on-disk layout that is found only later. A consumer that runs one schema
routine against several kinds of backend already knows which driver it opened,
so it attaches only the extensions that driver honours. It can check with
`SupportsExtension` first. An `IgnoreForeignExtensions()` opt-out would buy
that convenience at the price of reintroducing silent loss, so this Feature
does not add one.

#### REQ: alter-op-options-visible

The sealed `AlterOp` interface (`ddl/alter_op.go:24-33`) MUST gain an
unexported method `opts() Options` that returns the op's resolved options.
Every concrete op already stores them in its `options` field
(`ddl/alter_op.go:37-40`, `:55-58`, and the four other op types), so each
implementation returns that field. The method is named `opts`, not `options`,
because Go forbids a field and a method with the same name on one type.
`AlterOp` is already sealed by the unexported `alterOp()` marker, so no code
outside `ddl` implements it.

#### REQ: guard-precedence

Every `ddl` helper MUST check in this order and return the first failure:

1. **Path validity.** `c.Path().Validate()` for `CreateCollection`, and
   `path.Validate()` plus a non-empty `path` for Drop and Alter (the string
   helpers validate `CollectionPath{name}`). On failure:
   `dbschema.ErrInvalidCollectionPath`.
2. **Extension validity.** Row 1 of REQ:extension-addressing
   (`ErrInvalidExtension`), for `opts` or, for Alter, for every `AlterOp` in op
   order.
3. **Capability.** The DB implements `SchemaModifier`, otherwise the existing
   `*dbschema.NotSupportedError` ("driver does not implement
   ddl.SchemaModifier", `ddl/operations.go:25-31`). Then nesting, per
   REQ:nesting-refused-without-capability. Then rows 2-3 of
   REQ:extension-addressing, which need the driver's `SupportsExtension`.

### Driver obligations

#### REQ: helpers-are-the-guarantee

The `ddl` helpers are the designed entry point. Every refusal in this Feature
is enforced by them before dispatch, independent of driver quality, so a
consumer that calls through them never loses a `Parent` or an extension
silently. A DB that does not implement `SchemaModifier` gets the existing typed
`*dbschema.NotSupportedError` from every helper, so nothing is skipped
silently.

#### REQ: in-org-drivers-comply

Every driver in the `dal-go` and `ingitdb` organisations that implements
`SchemaModifier` MUST implement the changed interface and enforce, on direct
calls, the driver-side MUSTs of REQ:parent-is-the-only-nesting-form,
REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter,
REQ:nesting-refused-without-capability and REQ:extension-addressing. There is no
legacy exemption. As of 2026-09-17 these are:

| Driver | `SchemaModifier` today (origin/main) | Nesting |
|---|---|---|
| `dal-go/dalgo2sqlite` | `schema_modifier.go:16`, `:89`, `:106` | `SupportsSubCollections() == false` |
| `dal-go/dalgo2postgres` | `schema_modifier.go:15`, `:62`, `:75` | `false` |
| `dal-go/dalgo2mysql` | `schema_modifier.go:27`, `:58`, `:69` | `false` |
| `ingitdb/dalgo2ingitdb` | `schema_modifier.go:42`, `:102`, `:137` | `true` (ingitdb/dalgo2ingitdb#16) |
| `dal-go/dalgo` `mocks/mock_ddl` | generated by MockGen | regenerated with this change |

The per-driver work is tracked in one comment on dal-go/dalgo#163.

## Acceptance Criteria

### AC: root-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "users"}` with no `Parent`
**When** `c.Path()` is called
**Then** `c.Path()` equals `CollectionPath{"users"}`, `c.Parent == nil`, and `c.Path().Validate()` returns `nil`.

### AC: nested-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "servers", Parent: dbschema.CollectionPath{"projects", "environments"}}`
**When** `c.Path()` is called and the returned slice is then modified
**Then** `c.Path()` equals `CollectionPath{"projects", "environments", "servers"}`, and modifying the returned slice does not change `c.Parent`.

### AC: invalid-path-segments (verifies REQ:collection-path-type)

**Given** the paths `{"projects", ""}`, `{"projects", "."}`, `{"projects", ".."}`, `{"a/b"}`, `{"a\\b"}`, `{"a\tb"}` and `{" projects"}`
**When** `Validate()` is called on each
**Then** each returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)`, and `CollectionPath{"projects", "queries"}.Validate()` returns `nil`.

### AC: slash-name-rejected (verifies REQ:parent-is-the-only-nesting-form)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true` that records every call
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/queries"})`, `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "a/b", Parent: dbschema.CollectionPath{"projects"}})`, `ddl.DropCollection(ctx, db, "projects/queries")` and `ddl.AlterCollection(ctx, db, "projects/queries", op)` are called
**Then** each returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: invalid-path-rejected-before-dispatch (verifies REQ:guard-precedence)

**Given** the recording stub of AC:slash-name-rejected
**When** `ddl.CreateCollection` is called with `Parent{".."}, Name: "queries"`, and `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` are called with `CollectionPath{"projects", ".."}` and with an empty path
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: parent-refused-by-flat-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}})`, `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"})` and `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"})` are called
**Then** each returns `*dbschema.NotSupportedError` with `Op` equal to `"CreateCollection"`, `"DropCollection"` or `"AlterCollection"` respectively and `errors.Is(err, dal.ErrNotSupported)` true; the stub's `CreateCollection`, `DropCollection` and `AlterCollection` are not invoked; and `ddl.SupportsSubCollections(db)` is `false`.

### AC: parent-dispatched-to-nesting-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`
**When** `ddl.CreateCollection` is called with `Name: "queries", Parent: CollectionPath{"projects"}`
**Then** it returns the stub's result, the stub receives the `CollectionDef` with the same `Name` and `Parent`, and `ddl.SupportsSubCollections(db)` is `true`.

### AC: drop-and-alter-dispatch-paths (verifies REQ:path-addressed-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`, which records the `path` each method receives
**When** `ddl.DropCollectionAt(ctx, db, CollectionPath{"projects", "environments", "servers"}, ddl.IfExists())`, `ddl.AlterCollectionAt(ctx, db, CollectionPath{"projects", "queries"}, op)`, `ddl.DropCollection(ctx, db, "users")` and `ddl.AlterCollection(ctx, db, "users", op)` are called
**Then** the stub's `DropCollection` receives `CollectionPath{"projects", "environments", "servers"}` with `IfExists` set and then `CollectionPath{"users"}`, and its `AlterCollection` receives `CollectionPath{"projects", "queries"}` and then `CollectionPath{"users"}`, each with `op`.

### AC: schema-modifier-shape (verifies REQ:path-addressed-drop-and-alter, REQ:sub-collections-capability, REQ:extension-aware-capability)

**Given** a Go test file declaring `var _ ddl.SchemaModifier = (*stub)(nil)`
**When** `stub` omits `SupportsSubCollections`, or omits `SupportsExtension`, or declares `DropCollection(ctx, name string, ...)`
**Then** the file does not compile; with all methods in the shape of REQ:path-addressed-drop-and-alter it compiles.

### AC: no-schema-modifier-is-typed-error (verifies REQ:helpers-are-the-guarantee, REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier` (as for a store whose schema is defined elsewhere), and an extension `ext` with a non-empty target
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}}, ddl.WithExtension(ext))` is called
**Then** it returns `*dbschema.NotSupportedError` with `Reason` `"driver does not implement ddl.SchemaModifier"`.

### AC: with-extension-accumulates (verifies REQ:with-extension-option, REQ:extension-type)

**Given** two values `e1`, `e2` of test types implementing `ddl.Extension`
**When** `ddl.ResolveOptions(ddl.WithExtension(e1), ddl.IfNotExists(), ddl.WithExtension(nil), ddl.WithExtension(e2))` is called
**Then** the result has `Extensions == []ddl.Extension{e1, e2}` in that order, `IfNotExists == true` and `IfExists == false`; and `ddl.ResolveOptions()` has `Extensions == nil`.

### AC: unhonoured-extension-refused (verifies REQ:extension-addressing, REQ:extension-aware-capability)

**Given** a stub driver implementing `SchemaModifier` whose `SupportsExtension` returns `false` for every extension, an extension `own` with target `"example.com/stubdb"` (the stub module's own constant), and an extension `foreign` with target `"github.com/ingitdb/dalgo2ingitdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(x))` and `ddl.DropCollection(ctx, db, "users", ddl.WithExtension(x))` are called for `x` = `own` and for `x` = `foreign`
**Then** every call returns `*dbschema.NotSupportedError` whose `Op` is the helper's operation and whose `Reason` names `x`'s Go type and target, and the stub's `CreateCollection` and `DropCollection` are never invoked.

### AC: honoured-extension-dispatched (verifies REQ:extension-addressing, REQ:extension-aware-capability)

**Given** a stub driver whose `SupportsExtension("CreateCollection", ext)` returns `true` for `ext` with target `"example.com/stubdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` is called
**Then** the stub's `CreateCollection` is invoked once, and `ddl.ResolveOptions` over the options it receives yields `Extensions == []ddl.Extension{ext}`.

### AC: cross-module-extension-honoured (verifies REQ:extension-aware-capability, REQ:extension-addressing)

**Given** a stub driver from module `"example.com/vaultstub"` whose `SupportsExtension("CreateCollection", ext)` returns `true` for an extension `ext` with target `"github.com/ingitdb/dalgo2ingitdb"`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.WithExtension(ext))` is called
**Then** it dispatches to the stub's `CreateCollection`, which receives `ext` in its resolved `Options.Extensions`, and the helper returns no error of its own.

### AC: extension-without-target-rejected (verifies REQ:extension-addressing)

**Given** an extension whose `ExtensionTarget()` returns `""`, and a stub driver implementing `SchemaModifier` whose `SupportsExtension` returns `true` for everything
**When** `ddl.CreateCollection` or `ddl.DropCollection` is called with it
**Then** the helper returns an error satisfying `errors.Is(err, ddl.ErrInvalidExtension)`, and neither `SupportsExtension` nor the stub's operation is invoked.

### AC: alter-op-unhonoured-extension-refused (verifies REQ:alter-op-options-visible, REQ:extension-addressing, REQ:helpers-are-the-guarantee)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true` and `SupportsExtension` returning `false` for every extension, which records every call, and an extension `ext` with target `"github.com/ingitdb/dalgo2ingitdb"`
**When** `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"}, ddl.AddField(f1), ddl.AddField(f2, ddl.WithExtension(ext)))` and `ddl.AlterCollection(ctx, db, "users", ddl.AddField(f2, ddl.WithExtension(ext)))` are called
**Then** each returns `*dbschema.NotSupportedError` with `Op == "AlterCollection"` and a `Reason` naming `ext`'s Go type and target, and the stub's `AlterCollection` is never invoked, so `f1` is not applied either.

### AC: guard-order (verifies REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier`, and an extension with target `""`
**When** `ddl.CreateCollection` is called with `Parent{".."}, Name: "q"` and that extension; then with `Parent{"projects"}, Name: "q"` and that extension; then with `Parent{"projects"}, Name: "q"` and no extension
**Then** the calls fail, in order, with `dbschema.ErrInvalidCollectionPath`, then `ddl.ErrInvalidExtension`, then `*dbschema.NotSupportedError` (driver does not implement `ddl.SchemaModifier`).

### AC: flat-in-org-drivers-refuse-directly (verifies REQ:in-org-drivers-comply, REQ:nesting-refused-without-capability, REQ:extension-addressing)

Driver conformance statement, verified in each driver's own suite
(`dalgo2sqlite`, `dalgo2postgres`, `dalgo2mysql`) once it adopts this Feature.

**Given** a database value from the driver, with no collections
**When** its `CreateCollection` method is called directly (not through `ddl`) with `dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}}`, and again with `dbschema.CollectionDef{Name: "users"}` plus `ddl.WithExtension(ext)` for an extension `ext` the driver does not honour
**Then** `SupportsSubCollections()` is `false`; both calls return `*dbschema.NotSupportedError`; and no `queries` or `users` table exists afterwards.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:extension-addressing, REQ:in-org-drivers-comply)

Consumer conformance statement. The implementation, and the test that runs
this AC and the two below, belong to
[ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16).
This Feature only guarantees that the API makes them expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, which reports `ddl.SupportsSubCollections(db) == true`, and an extension type exported by `dalgo2ingitdb` that carries an `ingitdb.RecordFileDef`, returns `dalgo2ingitdb.ExtensionTarget`, and is honoured by the driver's `SupportsExtension("CreateCollection", ext)`
**When** the caller runs
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>))`
and then
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}, Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>))`
**Then** both calls return `nil`; `projects/.collection/definition.yaml` has `record_file` equal to the first extension's values; `projects/.collection/subcollections/queries/definition.yaml` exists and its `record_file` is exactly `name: '{key}/{key}.query.json'`, `format: json`, `type: map[string]any`, `records_dir: '.'`; no `{key}.yaml` default record file is written for either collection; creating `queries` before `projects` exists returns a non-nil error and writes nothing; and `db.CreateCollection(ctx, dbschema.CollectionDef{Name: "projects/queries"})` called directly returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)`.

### AC: dalgo2ingitdb-depth-two-ancestor-required (verifies REQ:one-collection-per-create)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries after `projects` is created, and no `projects/environments` collection
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "servers", Parent: dbschema.CollectionPath{"projects", "environments"}}, ddl.WithExtension(<record file '{key}/{key}.server.json', json, map[string]any, '.'>))` is called
**Then** it returns a non-nil error, and no `servers` definition exists anywhere under `projects/`.

### AC: dalgo2ingitdb-depth-two-created-and-dropped (verifies REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries with `projects` and `projects/dbdrivers` created (record file `'{key}/{key}.dbdriver.json'`), and no `dbservers` records
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "dbservers", Parent: dbschema.CollectionPath{"projects", "dbdrivers"}}, ddl.WithExtension(<record file '{key}/{key}.dbserver.json', json, map[string]any, '.'>))` is called, followed by `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "dbdrivers", "dbservers"}, ddl.AddField(fd))` and `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "dbdrivers", "dbservers"})`
**Then** the create returns `nil` and writes `projects/.collection/subcollections/dbdrivers/subcollections/dbservers/definition.yaml` with that `record_file`; the alter returns `nil` and adds the field to that same file; the drop returns `nil` and removes that definition while `projects/.collection/subcollections/dbdrivers/definition.yaml` remains. This AC deliberately has no `dbservers` records and makes no claim about record data, which waits on the nested-drop Open Question.

## Architecture

| File | Change |
|---|---|
| `dbschema/collection_path.go` | New: `CollectionPath`, `String`, `Validate`, `ErrInvalidCollectionPath`. |
| `dbschema/collection_def.go` | Add `Parent CollectionPath` and `Path()`; godoc for the nesting meaning. |
| `ddl/modifier.go` | `SchemaModifier` embeds `SubCollectionsAware` and `ExtensionAware`; `DropCollection` and `AlterCollection` take `dbschema.CollectionPath`. |
| `ddl/options.go` | Add `Options.Extensions` and `WithExtension`; godoc states the strict rule next to the mismatched-option rule. |
| `ddl/extension.go` | New: `Extension`, `ExtensionAware`, `ErrInvalidExtension`, and the internal addressing check. |
| `ddl/subcollections.go` | New: `SubCollectionsAware`, `SupportsSubCollections`. |
| `ddl/alter_op.go` | Sealed `AlterOp` gains unexported `opts() Options`; each of the six op types returns its stored field. |
| `ddl/operations.go` | All helpers apply REQ:guard-precedence; new `DropCollectionAt` and `AlterCollectionAt`; `DropCollection` / `AlterCollection` become root-only wrappers over them. |
| `mocks/mock_ddl/schema_modifier.go` | Regenerated for the changed interface. |
| `spec/features/ddl/`, `spec/features/ddl/schema-modifier/`, `spec/features/ddl/options/`, `spec/features/dbschema/collection-def/` | Updated to the changed interface once this Feature is Approved. |

## Error Handling and Failure Modes

| Failure mode | Result |
|---|---|
| Invalid path segment (empty, `.`, `..`, `/`, `\`, control character, whitespace), including a slash in `Name` | error, `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; no dispatch |
| Extension with empty target | error, `errors.Is(err, ddl.ErrInvalidExtension)`; no dispatch |
| Extension the driver does not honour, on any operation or `AlterOp`, whatever its target | `*dbschema.NotSupportedError` naming type and target; no operation performed |
| DB does not implement `SchemaModifier` (for example a store whose schema is defined elsewhere) | existing `*dbschema.NotSupportedError`; no dispatch |
| Nested path or `Parent`, driver lacks nesting | `*dbschema.NotSupportedError`; no dispatch |
| An ancestor collection does not exist (nesting driver) | driver-specific non-nil error; nothing created |
| Honoured extension with an invalid value (e.g. an inGitDB `RecordFileDef` failing `Validate`) | driver-specific error; the surface is supported, the value is not |

## Testing Strategy

In-package Go tests in `dbschema` and `ddl` with stub `dal.DB` values, following
the existing `ddl/operations_test.go` pattern, plus a compile-shape test for
`SchemaModifier`. AC:flat-in-org-drivers-refuse-directly runs in each SQL
driver's suite, and the three `dalgo2ingitdb-*` ACs in `ingitdb/dalgo2ingitdb`'s
suite, against a DALgo release carrying this Feature. No Rehearse stubs: every
AC has a direct Go test surface.

## Not Doing / Out of Scope

- **Concrete extension types in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Implementing driver changes in this repo.** The driver work in
  REQ:in-org-drivers-comply is tracked on dal-go/dalgo#163. dalgo2ingitdb's
  extension type, target constant, nesting, depth-N ancestor check, path-based
  Drop/Alter and removal of its slash-name handling are
  ingitdb/dalgo2ingitdb#16.
- **An ignore-foreign-extensions mode.** Rejected in REQ:extension-addressing.
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
  SQL drivers report `SupportsSubCollections() == false` until a separate
  Feature designs it.
- **Declaring a subcollection tree in one call** (`SubCollections []CollectionDef`),
  rejected in REQ:one-collection-per-create.
- **Read-side round trip.** `dbschema.DescribeCollection` takes a
  `*dal.CollectionRef` (`dbschema/reader_helpers.go:44`) and does not report
  `Parent` or extensions in this Feature.

## Open Questions

- **Record data on a nested drop.** For a nesting driver, what does
  `ddl.DropCollectionAt(ctx, db, CollectionPath{"projects", "queries"})` do with
  existing `queries` records under the `projects` records?
  - **A.** Delete the definition and the subcollection's records under every
    parent record (for inGitDB, every `projects/<id>/queries/` tree), the same
    as a root drop.
  - **B.** Refuse with an error while any record of that subcollection exists
    under any parent record, unless the caller passes an explicit force option
    (e.g. `ddl.DeleteRecords()`), which deletes them as in A.

  Recommendations:
  - **Author: A.** It matches the root `DropCollection` in dalgo2ingitdb, which
    removes the whole collection directory, records included
    (`schema_modifier.go:122-124`), and SQL `DROP TABLE`, so "drop" means the
    same at every depth.
  - **Coordinator: B.** A nested drop fans out across every parent record, so
    its blast radius is much larger than it looks at the call site. An explicit
    force option makes that deletion deliberate.

  Needs a founder decision. AC:dalgo2ingitdb-depth-two-created-and-dropped
  stays neutral until it is made.

---
*This document follows the https://specscore.md/feature-specification*
