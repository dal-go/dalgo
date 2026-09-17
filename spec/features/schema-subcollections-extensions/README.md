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
**Related Features:** [`ddl`](../ddl/README.md), [`ddl/options`](../ddl/options/README.md), [`dbschema/collection-def`](../dbschema/collection-def/README.md), [`typed-collection`](../typed-collection/README.md)

## Summary

Two changes to the schema API, so that a consumer can create nested collections,
and pass driver-specific storage options for them, through DALgo:

1. **Subcollection declaration.** A nested collection is written in either of
   two equivalent forms that normalise to one internal, structured
   representation (`Parent` collection names plus their id placeholder names):
   - **structured:** `CollectionDef{Name: "queries", Parent: CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}}`;
   - **string:** `"projects/{projectID}/queries"` (or with a leading `/`), in the
     Firestore-style alternating grammar `collection/id/collection/id/…` that
     DALgo key paths already use (`record.Key.String()`). An **odd** segment
     count addresses a collection; an **even** count addresses a record and is
     rejected wherever a collection is expected.

   Id segments are told apart by their shape: `{name}` is a **placeholder**,
   valid only in schema paths (a schema declaration is about every parent
   record); `{field=value,…}` is a **reserved multi-field key**, rejected today
   with a typed not-supported error; a bare segment is a **concrete id**, valid
   only in data paths. A nested collection is created with one
   `CreateCollection` call per collection, after every ancestor exists.
   `SchemaModifier.DropCollection` and `AlterCollection` take a
   `CollectionPath`. Every driver states whether it supports nesting through
   `SupportsSubCollections()`, which becomes part of `SchemaModifier`.
2. **Driver extensions.** `ddl.Options` gains `Extensions []ddl.Extension`, set
   through `ddl.WithExtension(ext)`. Each extension carries a stable target ID,
   a constant exported by the driver module that defines it (for example
   `dalgo2ingitdb.ExtensionTarget`). Extensions are strict: one is honoured only
   when the driver's `SupportsExtension(op, ext)` returns `true`, and is
   otherwise refused with `*dbschema.NotSupportedError`. There is no mode that
   ignores an extension.

The `ddl` helpers guard every call before dispatch, and every in-org driver
implementing `SchemaModifier` MUST enforce the same rules on direct calls
(REQ:in-org-drivers-comply).

DALgo is in private beta, so this Feature changes the `SchemaModifier` interface
and `record.EscapeID` rather than working around them (founder direction,
2026-09-17: "Don't worry too much about what already exists, we are still in
private beta stage and can do changes.").

The concrete scope this Feature unblocks is the DataTug project store's
collection schema on the local inGitDB path (see REQ:collection-def-parent for
the list). It does not cover nested database roots or OpenVaultDB-backed stores
(see Not Doing).

DALgo core stays driver-agnostic: it defines only the path grammar, the slot,
the addressing rule and the refusals. Concrete extension types, such as an
inGitDB `record_file`, live in driver modules.

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
  `name string` (`ddl/modifier.go`), with no defined meaning for a nested
  collection.
- `ddl/options.go:10-16` fixes the rule that drivers "MUST silently ignore
  semantically-mismatched options"; nothing says what a driver does with an
  option it cannot honour at all.

DALgo already has a string grammar for **data** paths, but no parser and no
schema-level form:

- `record.Key.String()` (`github.com/dal-go/record` `key.go:54-70`, v0.1.3 and
  origin/main) emits `collection/id/collection/id`, root first, with no leading
  `/`, escaping each id with `record.EscapeID` (`key.go:25-37`: `.` `$` `#` `[`
  `]` `/` become `%2E` `%24` `%23` `%5B` `%5D` `%2F`). `record.ValidateStringID`
  (`key.go:43-52`) reserves a literal `%` in string ids.
- `dal.CollectionRef.Path()` (`dal/q_collection_ref.go:85-90`) emits
  `parent.String() + "/" + name`: a collection under a concrete parent record,
  which has an odd segment count.
- `record.Key.CollectionPath()` (`key.go:72-86`) emits collection names only
  (`projects/queries`), a different shape that cannot be told apart from a
  record path.
- Neither module has a function that parses any of these strings.

The consumer that needs this is `ingitdb/dalgo2ingitdb` (v0.6.1). Its
`CreateCollection` (`schema_modifier.go:42`) always builds the definition through
`buildIngitdbCollectionDef` (`schema_modifier.go:388-414`), which sets
`RecordFile: defaultRecordFile()` (`schema_modifier.go:409`), and
`defaultRecordFile()` (`schema_modifier.go:416-421`) is fixed to
`{key}.yaml`, `yaml`, `map[string]any`. A caller cannot choose `name`,
`format`, `type` or `records_dir` of inGitDB's `RecordFileDef`
(`ingitdb-go/ingitdb@v0.6.1/record_file_def.go:22-52`).

The driver improvises nesting with names such as `"spaces/ext"`
(`schema_modifier.go:48-50`, `createSubCollection` at `:349`). That form has an
even segment count, so under DALgo's key grammar it reads as a record path. It
also has these defects:

- `createSubCollection` checks that only the root exists
  (`schema_modifier.go:350-353`), not intermediate ancestors;
- `DropCollection` (`schema_modifier.go:107-110`) and `AlterCollection`
  (`schema_modifier.go:141`) look for `<name>/.collection/definition.yaml`,
  while nested definitions live under
  `<root>/.collection/subcollections/<sub>/definition.yaml`, so a nested
  collection cannot be dropped or altered.

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
- **One path grammar.** Schema paths, collection references and record keys
  share the alternating `collection/id/…` grammar and escaping of
  `record.Key.String()`. Segment parity tells a collection from a record.
- **Two spellings, one representation.** The structured and string forms of a
  schema path normalise to the same `CollectionPath` before any driver sees
  them.
- **No silent loss.** A declaration that the target driver cannot honour
  (a parent on a flat driver, an extension the driver does not honour) is an
  error, never a quiet default. That silent default is the exact defect behind
  dalgo2ingitdb#16.
- **Addressing by compiled constant, not by display name.** Extension targets
  are constants exported by the defining module, never `dal.Adapter.Name()`.
- **Guard in the helper and in the driver.** The `ddl` helpers refuse before
  dispatch, and every in-org driver enforces the same rules on direct calls.

## Behavior

### Path grammar

#### REQ: path-grammar

A DALgo path string MUST follow this grammar, which is the grammar
`record.Key.String()` already emits, extended only as stated here:

```
path          = [ "/" ] segment *( "/" segment )   ; no trailing "/", no empty segment
                                                   ; odd positions (1st, 3rd, …) are collections,
                                                   ; even positions are ids
collection    = name
id            = placeholder / composite-key / concrete-id
placeholder   = "{" identifier "}"                 ; schema paths only
identifier    = ( ALPHA / "_" ) *( ALPHA / DIGIT / "_" )
composite-key = "{" <any text containing "="> "}"  ; reserved, not supported yet
concrete-id   = <record.EscapeID output>           ; data paths only; never begins with "{"
```

- **Leading `/`: optional on input, never emitted.** Parsers MUST accept both
  `projects/{projectID}/queries` and `/projects/{projectID}/queries` as the same
  path. Every `String()` in this Feature MUST emit the canonical form without
  the leading `/`, matching `record.Key.String()` and `dal.CollectionRef.Path()`
  today.
- **Parity.** A path with an odd segment count addresses a collection; a path
  with an even count addresses a record. Wherever a collection is expected, an
  even count MUST fail with an error satisfying both
  `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and
  `errors.Is(err, dbschema.ErrRecordPathNotCollection)`.
- **Collection names** MUST be non-empty, MUST NOT be `.` or `..`, MUST NOT
  have leading or trailing whitespace, and MUST NOT contain a control character
  or any of `/` `\` `{` `}` `,` `=` `%`. Names are not escaped.
- **Id segments are classified by shape, in this order:**
  1. begins with `{`, ends with `}` and contains `=`: **composite key**. It MUST
     fail with an error satisfying both
     `errors.Is(err, dbschema.ErrReservedKeySegment)` and
     `errors.Is(err, dal.ErrNotSupported)`. This reserves
     `{field=value,field=value}` (e.g. `/countries/{country=us,state=ca}/cities`)
     for a future Feature without accepting it today.
  2. `{identifier}`: **placeholder**.
  3. any other segment beginning with `{`: malformed, `ErrInvalidCollectionPath`.
  4. otherwise: **concrete id**, unescaped with `record.UnescapeID` (new). A
     concrete id containing a raw `{` `}` `,` or `=`, or a `%` not part of a
     known escape, is malformed.
- **No mixing.** A schema path MUST NOT contain a concrete id
  (`dbschema.ErrConcreteIDInSchemaPath`). A data path MUST NOT contain a
  placeholder; any data-path parser added later MUST reject one with a typed
  error. Keys built with the `record` constructors cannot contain one, because
  `record.EscapeID` escapes `{`.
- **Placeholder names within one path MUST be unique**
  (`projects/{id}/environments/{id}/servers` is malformed), so every placeholder
  names exactly one id position.
- **Escaping.** `record.EscapeID` MUST be extended so that, besides its current
  characters, `{` `}` `,` `=` become `%7B` `%7D` `%2C` `%3D`. Because
  `record.ValidateStringID` already reserves a literal `%` in string ids, every
  escaped id stays unambiguous, and no concrete id can look like a placeholder
  or a composite key.

The grammar, the segment classification and the escape table MUST be defined
once, in `github.com/dal-go/record` next to `EscapeID`. `dbschema` MUST
implement schema-path parsing on top of it rather than with its own splitting
or escaping rules.

#### REQ: collection-path-type

The `dbschema` package MUST export:

```go
// CollectionPath is the schema address of a collection: its collection names,
// root first. It carries no record IDs and no placeholder names.
type CollectionPath []string

func (p CollectionPath) Validate() error

// SchemaPath is a parsed schema path: collection names plus the placeholder
// name of every id position. len(IDNames) == len(Collections)-1.
type SchemaPath struct {
    Collections CollectionPath
    IDNames     []string
}

func ParseSchemaPath(s string) (SchemaPath, error)
func (p SchemaPath) String() string // e.g. "projects/{projectID}/queries"
func (p SchemaPath) Validate() error

var (
    ErrInvalidCollectionPath   = errors.New("dbschema: invalid collection path")
    ErrRecordPathNotCollection = errors.New("dbschema: path addresses a record, not a collection")
    ErrConcreteIDInSchemaPath  = errors.New("dbschema: schema path must use a {placeholder} for every id")
    ErrReservedKeySegment      = errors.New("dbschema: multi-field key segments are not supported yet")
)
```

- `CollectionPath.Validate()` MUST check each name against REQ:path-grammar and
  return an error satisfying `errors.Is(err, ErrInvalidCollectionPath)` on the
  first violation. An empty path is invalid as a collection address and valid
  only as a `CollectionDef.Parent`, where it means the root level.
- `SchemaPath.Validate()` MUST also check `len(IDNames) == len(Collections)-1`,
  that every id name is an identifier, and that id names are unique.
- `SchemaPath.String()` MUST interleave names and `{idName}` placeholders:
  `SchemaPath{Collections: {"projects", "queries"}, IDNames: {"projectID"}}`
  gives `"projects/{projectID}/queries"`, and a one-collection path gives just
  the name.
- `ParseSchemaPath` MUST apply REQ:path-grammar and REQ:schema-paths-use-placeholders,
  and `ParseSchemaPath(p.String())` MUST equal `p` for every valid `p`.
- `ErrRecordPathNotCollection` and `ErrConcreteIDInSchemaPath` MUST be returned
  wrapped so that `errors.Is(err, ErrInvalidCollectionPath)` is also `true`.

#### REQ: schema-paths-use-placeholders

A schema declaration (`CreateCollection`, `DropCollection`, `AlterCollection`
and their helpers) is about every parent record, not one. In a schema path
string, every id segment MUST be a `{placeholder}`. A concrete id
(`projects/p1/queries`) MUST be rejected with `ErrConcreteIDInSchemaPath`, and a
composite key with `ErrReservedKeySegment`. A data-level path (a record key, or
a `dal.CollectionRef` under a concrete parent) carries concrete ids.

This follows the founder decision of 2026-09-17: "DataTug projects keys should
prefixed by project parent key as we can have multiple projects per store. So
yes, projects/{projectID}/…".

**Placeholder names are part of the schema, not free-form.** A placeholder
names the id of the collection it follows, so it MUST be the same wherever that
collection appears: every path through `projects` uses `{projectID}`. The `ddl`
helpers cannot see earlier calls, so they check only uniqueness within one
path. A nesting driver that persists placeholder names MUST refuse a
declaration whose name for an existing ancestor differs from the stored one.
Addressing ignores names: `DropCollection` and `AlterCollection` resolve by
collection names only. Recommendation and rationale: typed-key and code
generation tooling maps `{projectID}` to one key field of `projects`, so one
name per collection keeps generated code stable. How drivers without storage
for names enforce this is under Open Questions.

### Subcollection declaration

#### REQ: collection-def-parent

`dbschema.CollectionDef` MUST gain the fields `Parent CollectionPath` and
`ParentIDNames []string`, and the methods `Path() CollectionPath` (a new slice
equal to `Parent` followed by `Name`) and `SchemaPath() SchemaPath`. A zero
`Parent` means a root collection, with `ParentIDNames` empty. When `Parent` is
non-empty, `len(ParentIDNames)` MUST equal `len(Parent)`; each entry names the
id position under the `Parent` collection at the same index.

A non-empty `Parent` declares that the collection is a subcollection nested
under records of the collection at `Parent`. The declaration is per collection
shape: every record of the parent collection has the same subcollection schema.

`CollectionDef{Name: "servers", Parent: CollectionPath{"projects", "environments"}, ParentIDNames: []string{"projectID", "envID"}}`
declares the collection whose records live at
`projects/<project id>/environments/<env id>/servers/<server id>`. Its schema
path is `projects/{projectID}/environments/{envID}/servers`.

The DataTug collections this makes expressible, all at most three collections
deep, are: `projects`; `projects/{projectID}/credentials`,
`projects/{projectID}/queries`, `projects/{projectID}/entities`,
`projects/{projectID}/environments`, `projects/{projectID}/dbmodels`,
`projects/{projectID}/boards`, `projects/{projectID}/recordsets`,
`projects/{projectID}/folders`, `projects/{projectID}/dbdrivers`; and
`projects/{projectID}/environments/{envID}/servers`,
`projects/{projectID}/environments/{envID}/catalogs`,
`projects/{projectID}/dbdrivers/{dbdriverID}/dbservers`.

#### REQ: string-and-structured-forms-round-trip

`CollectionDef.Name` MAY hold a schema path string instead of a single name.
`dbschema.CollectionDef` MUST gain `Normalize() (CollectionDef, error)`:

- If `Name` contains no `/` (after an optional leading `/`), `Normalize` returns
  the definition unchanged, after validating `SchemaPath()`.
- Otherwise, if `Parent` or `ParentIDNames` is non-empty, it MUST fail with
  `ErrInvalidCollectionPath`: the two forms MUST NOT be mixed.
- Otherwise it parses `Name` with `ParseSchemaPath` and returns a copy with
  `Parent = Collections[:len-1]`, `ParentIDNames = IDNames` and
  `Name = Collections[len-1]`.

So `CollectionDef{Name: "/projects/{projectID}/queries"}`,
`CollectionDef{Name: "projects/{projectID}/queries"}` and
`CollectionDef{Name: "queries", Parent: CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}}`
all normalise to the last of these, and its `SchemaPath().String()` is
`"projects/{projectID}/queries"`. The `ddl` helpers MUST normalise before
dispatch, so a driver always receives the structured form. A driver called
directly MUST call `Normalize` itself.

#### REQ: one-collection-per-create

Each `CreateCollection` call MUST create exactly one collection, the one named
by the normalised `c.Path()`. A driver whose `SupportsSubCollections()` is
`true` MUST return a non-nil error, and MUST NOT create anything, when **any**
ancestor path (`c.Parent[:1]`, `c.Parent[:2]`, and so on up to `c.Parent`) does
not exist. It MUST NOT create an ancestor implicitly. `IfNotExists` applies to
the collection at `c.Path()` only.

Rationale (why a names-only `Parent`, not `SubCollections []CollectionDef` or a
parent record key): a recursive `SubCollections` tree would force one options
set on a whole tree, while DataTug needs a different `record_file` per
subcollection (`.query.json`, `.entity.json`, `.server.json`). It would make
`IfNotExists` ambiguous on a partly existing tree and make partial failure
likely on non-transactional drivers. A parent record key confuses schema with
instances: a schema is declared once per collection shape, not per parent
record.

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

and keep `ddl.DropCollection(ctx, db, path string, opts...)` and
`ddl.AlterCollection(ctx, db, path string, ops...)`. Each string helper MUST
parse its argument with `dbschema.ParseSchemaPath` and then behave exactly as
the `*At` helper with its `Collections`; placeholder names do not affect
addressing. `"users"`, `"projects/{projectID}/queries"` and
`"/projects/{projectID}/queries"` are all valid; `"projects/queries"` is an
even-count record path and `"projects/p1/queries"` has a concrete id, and both
are rejected.

A driver whose `SupportsSubCollections()` is `true` MUST resolve a
multi-segment `path` to the nested collection that `CreateCollection` created
for it. Dropping a collection that has subcollections MUST either remove their
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

When a `ddl` helper receives, after normalisation, a non-empty `Parent` or a
path with more than one collection, and `SupportsSubCollections()` is `false`,
the helper MUST return
`*dbschema.NotSupportedError{Op, Backend: <adapter name>, Reason: "driver does not support subcollections"}`
and MUST NOT invoke the driver. `Op` is `"CreateCollection"`,
`"DropCollection"` or `"AlterCollection"`.

A driver whose `SupportsSubCollections()` is `false` MUST return the same error
when it receives a nested definition or path directly.

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

1. **Path validity.** `CreateCollection`: `c.Normalize()`. String helpers:
   `ParseSchemaPath`. `*At` helpers: `path.Validate()`. Failures are the
   path errors of REQ:path-grammar, REQ:collection-path-type and
   REQ:schema-paths-use-placeholders.
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
consumer that calls through them never loses a nesting declaration or an
extension silently. A DB that does not implement `SchemaModifier` gets the
existing typed `*dbschema.NotSupportedError` from every helper, so nothing is
skipped silently.

#### REQ: in-org-drivers-comply

Every driver in the `dal-go` and `ingitdb` organisations that implements
`SchemaModifier` MUST implement the changed interface and enforce, on direct
calls, the driver-side MUSTs of REQ:string-and-structured-forms-round-trip,
REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter,
REQ:nesting-refused-without-capability and REQ:extension-addressing. There is no
legacy exemption. As of 2026-09-17 these are:

| Module | `SchemaModifier` today (origin/main) | Nesting |
|---|---|---|
| `dal-go/dalgo2sqlite` | `schema_modifier.go:16`, `:89`, `:106` | `SupportsSubCollections() == false` |
| `dal-go/dalgo2postgres` | `schema_modifier.go:15`, `:62`, `:75` | `false` |
| `dal-go/dalgo2mysql` | `schema_modifier.go:27`, `:58`, `:69` | `false` |
| `ingitdb/dalgo2ingitdb` | `schema_modifier.go:42`, `:102`, `:137` | `true`; its slash parsing is kept but conformed to REQ:path-grammar via `Normalize` (ingitdb/dalgo2ingitdb#16) |
| `dal-go/dalgo` `mocks/mock_ddl` | generated by MockGen | regenerated with this change |

`dal-go/record` is not a driver, but it carries the grammar change
(`EscapeID` extension, new `UnescapeID`) of REQ:path-grammar. The per-module
work is tracked on dal-go/dalgo#163.

## Acceptance Criteria

### AC: root-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "users"}` with no `Parent`
**When** `c.Path()` and `c.SchemaPath().String()` are called
**Then** `c.Path()` equals `CollectionPath{"users"}`, the string is `"users"`, `c.Parent == nil`, and `c.SchemaPath().Validate()` returns `nil`.

### AC: nested-def-path (verifies REQ:collection-def-parent, REQ:collection-path-type)

**Given** `c := dbschema.CollectionDef{Name: "servers", Parent: dbschema.CollectionPath{"projects", "environments"}, ParentIDNames: []string{"projectID", "envID"}}`
**When** `c.Path()` and `c.SchemaPath().String()` are called and the returned slice is then modified
**Then** `c.Path()` equals `CollectionPath{"projects", "environments", "servers"}`, the string is `"projects/{projectID}/environments/{envID}/servers"`, and modifying the returned slice does not change `c.Parent`.

### AC: schema-path-round-trip (verifies REQ:collection-path-type, REQ:path-grammar)

**Given** the strings `"users"`, `"/users"`, `"projects/{projectID}/queries"`, `"/projects/{projectID}/queries"` and `"projects/{projectID}/dbdrivers/{dbdriverID}/dbservers"`
**When** each is passed to `dbschema.ParseSchemaPath`, and the result's `String()` is parsed again
**Then** they yield `Collections` `{"users"}`, `{"users"}`, `{"projects","queries"}`, `{"projects","queries"}` and `{"projects","dbdrivers","dbservers"}` with `IDNames` `nil`, `nil`, `{"projectID"}`, `{"projectID"}` and `{"projectID","dbdriverID"}`; the canonical strings have no leading `/`; and parsing each canonical string returns an equal `SchemaPath`.

### AC: forms-normalise-identically (verifies REQ:string-and-structured-forms-round-trip, REQ:collection-def-parent)

**Given** `a := CollectionDef{Name: "queries", Parent: CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}}`, `b := CollectionDef{Name: "projects/{projectID}/queries"}` and `c := CollectionDef{Name: "/projects/{projectID}/queries"}`, each with the same `Fields`
**When** `Normalize()` is called on each, and on `d := CollectionDef{Name: "{projectID}/queries", Parent: CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}}`, `e := CollectionDef{Name: "projects/{projectID}/queries", Parent: CollectionPath{"x"}, ParentIDNames: []string{"xID"}}` and `f := CollectionDef{Name: "queries", Parent: CollectionPath{"projects"}}`
**Then** `a`, `b` and `c` normalise to equal definitions with `Name == "queries"`, `Parent == CollectionPath{"projects"}` and `ParentIDNames == []string{"projectID"}`, whose `SchemaPath().String()` is `"projects/{projectID}/queries"`; `d`, `e` and `f` (missing id name) fail with `errors.Is(err, dbschema.ErrInvalidCollectionPath)`.

### AC: malformed-and-record-paths-rejected (verifies REQ:path-grammar, REQ:schema-paths-use-placeholders, REQ:path-addressed-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true` that records every call
**When** `ddl.DropCollection(ctx, db, s)` and `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: s})` are called for each `s` in: `"projects/queries"` and `"projects/{projectID}"` (even count); `"projects/p1/queries"` (concrete id); `""`, `"/"`, `"projects/{projectID}/"`, `"projects//queries"`, `"projects/{}/queries"`, `"projects/{project-id}/queries"`, `"projects/{projectID/queries"`, `"projects/{id}/environments/{id}/servers"`, `"projects/{projectID}/que{ries"` and `"proj%ects/{projectID}/queries"` (malformed)
**Then** every call fails with `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; the even-count cases also satisfy `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; the concrete-id case also satisfies `errors.Is(err, dbschema.ErrConcreteIDInSchemaPath)`; and the stub records no call.

### AC: multi-field-key-segment-reserved (verifies REQ:path-grammar)

**Given** the strings `"countries/{country=us,state=ca}/cities"` and `"/countries/{country=us}/cities"`
**When** each is passed to `dbschema.ParseSchemaPath`
**Then** each fails with an error satisfying `errors.Is(err, dbschema.ErrReservedKeySegment)` and `errors.Is(err, dal.ErrNotSupported)`.

### AC: id-escaping-extended (verifies REQ:path-grammar)

**Given** `record.NewKeyWithID("projects", "{a=b,c}")` under the extended `record.EscapeID`
**When** `String()` is called and its id segment is passed to `record.UnescapeID`
**Then** the string is `"projects/%7Ba%3Db%2Cc%7D"`, so the id segment is neither a placeholder nor a composite key; `UnescapeID` returns `"{a=b,c}"`; and `record.NewKeyWithID("projects", "a.b").String()` is still `"projects/a%2Eb"`.

### AC: invalid-path-rejected-before-dispatch (verifies REQ:guard-precedence)

**Given** the recording stub of AC:malformed-and-record-paths-rejected
**When** `ddl.CreateCollection` is called with `Parent{".."}, Name: "queries"`, and `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` are called with `CollectionPath{"projects", ".."}` and with an empty path
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: parent-refused-by-flat-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: dbschema.CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}})`, `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/{projectID}/queries"})`, `ddl.DropCollection(ctx, db, "projects/{projectID}/queries")` and `ddl.AlterCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "queries"})` are called
**Then** each returns `*dbschema.NotSupportedError` with `Op` equal to `"CreateCollection"`, `"CreateCollection"`, `"DropCollection"` or `"AlterCollection"` respectively and `errors.Is(err, dal.ErrNotSupported)` true; the stub's `CreateCollection`, `DropCollection` and `AlterCollection` are not invoked; and `ddl.SupportsSubCollections(db)` is `false`.

### AC: parent-dispatched-to-nesting-driver (verifies REQ:nesting-refused-without-capability, REQ:string-and-structured-forms-round-trip)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`
**When** `ddl.CreateCollection` is called with `CollectionDef{Name: "/projects/{projectID}/queries"}`
**Then** it returns the stub's result, and the stub receives a `CollectionDef` with `Name == "queries"`, `Parent == CollectionPath{"projects"}` and `ParentIDNames == []string{"projectID"}`.

### AC: drop-and-alter-dispatch-paths (verifies REQ:path-addressed-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`, which records the `path` each method receives
**When** `ddl.DropCollectionAt(ctx, db, CollectionPath{"projects", "environments", "servers"}, ddl.IfExists())`, `ddl.DropCollection(ctx, db, "/projects/{projectID}/environments/{envID}/servers")`, `ddl.AlterCollection(ctx, db, "projects/{pid}/queries", op)` and `ddl.DropCollection(ctx, db, "users")` are called
**Then** the stub's `DropCollection` receives `CollectionPath{"projects", "environments", "servers"}` twice (the first with `IfExists` set) and then `CollectionPath{"users"}`, and its `AlterCollection` receives `CollectionPath{"projects", "queries"}` with `op` (placeholder names do not affect addressing).

### AC: schema-modifier-shape (verifies REQ:path-addressed-drop-and-alter, REQ:sub-collections-capability, REQ:extension-aware-capability)

**Given** a Go test file declaring `var _ ddl.SchemaModifier = (*stub)(nil)`
**When** `stub` omits `SupportsSubCollections`, or omits `SupportsExtension`, or declares `DropCollection(ctx, name string, ...)`
**Then** the file does not compile; with all methods in the shape of REQ:path-addressed-drop-and-alter it compiles.

### AC: no-schema-modifier-is-typed-error (verifies REQ:helpers-are-the-guarantee, REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier` (as for a store whose schema is defined elsewhere), and an extension `ext` with a non-empty target
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/{projectID}/queries"}, ddl.WithExtension(ext))` is called
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
**When** `ddl.CreateCollection` is called with `Name: "projects/q"` and that extension; then with `Name: "projects/{projectID}/q"` and that extension; then with `Name: "projects/{projectID}/q"` and no extension
**Then** the calls fail, in order, with `dbschema.ErrRecordPathNotCollection`, then `ddl.ErrInvalidExtension`, then `*dbschema.NotSupportedError` (driver does not implement `ddl.SchemaModifier`).

### AC: flat-in-org-drivers-refuse-directly (verifies REQ:in-org-drivers-comply, REQ:nesting-refused-without-capability, REQ:extension-addressing)

Driver conformance statement, verified in each driver's own suite
(`dalgo2sqlite`, `dalgo2postgres`, `dalgo2mysql`) once it adopts this Feature.

**Given** a database value from the driver, with no collections
**When** its `CreateCollection` method is called directly (not through `ddl`) with `dbschema.CollectionDef{Name: "projects/{projectID}/queries"}`, with `dbschema.CollectionDef{Name: "projects/queries"}`, and with `dbschema.CollectionDef{Name: "users"}` plus `ddl.WithExtension(ext)` for an extension `ext` the driver does not honour
**Then** `SupportsSubCollections()` is `false`; the first and third calls return `*dbschema.NotSupportedError`; the second returns an error satisfying `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; and no `queries`, `projects/queries` or `users` table exists afterwards.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:string-and-structured-forms-round-trip, REQ:extension-addressing, REQ:in-org-drivers-comply)

Consumer conformance statement. The implementation, and the test that runs
this AC and the two below, belong to
[ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16).
This Feature only guarantees that the API makes them expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, which reports `ddl.SupportsSubCollections(db) == true`, and an extension type exported by `dalgo2ingitdb` that carries an `ingitdb.RecordFileDef`, returns `dalgo2ingitdb.ExtensionTarget`, and is honoured by the driver's `SupportsExtension("CreateCollection", ext)`
**When** the caller runs
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>))`
and then
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/{projectID}/queries", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>))`
**Then** both calls return `nil`; `projects/.collection/definition.yaml` has `record_file` equal to the first extension's values; `projects/.collection/subcollections/queries/definition.yaml` exists and its `record_file` is exactly `name: '{key}/{key}.query.json'`, `format: json`, `type: map[string]any`, `records_dir: '.'`; no `{key}.yaml` default record file is written for either collection; creating `queries` before `projects` exists returns a non-nil error and writes nothing; the same result is produced when the second call uses `Name: "/projects/{projectID}/queries"` or `Name: "queries", Parent: dbschema.CollectionPath{"projects"}, ParentIDNames: []string{"projectID"}` instead; and `db.CreateCollection(ctx, dbschema.CollectionDef{Name: "projects/queries"})` called directly fails with `errors.Is(err, dbschema.ErrRecordPathNotCollection)`.

### AC: dalgo2ingitdb-depth-two-ancestor-required (verifies REQ:one-collection-per-create)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries after `projects` is created, and no `projects/{projectID}/environments` collection
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/{projectID}/environments/{envID}/servers"}, ddl.WithExtension(<record file '{key}/{key}.server.json', json, map[string]any, '.'>))` is called
**Then** it returns a non-nil error, and no `servers` definition exists anywhere under `projects/`.

### AC: dalgo2ingitdb-depth-two-created-and-dropped (verifies REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries with `projects` and `projects/{projectID}/dbdrivers` created (record file `'{key}/{key}.dbdriver.json'`), and no `dbservers` records
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "projects/{projectID}/dbdrivers/{dbdriverID}/dbservers"}, ddl.WithExtension(<record file '{key}/{key}.dbserver.json', json, map[string]any, '.'>))` is called, followed by `ddl.AlterCollection(ctx, db, "projects/{projectID}/dbdrivers/{dbdriverID}/dbservers", ddl.AddField(fd))` and `ddl.DropCollectionAt(ctx, db, dbschema.CollectionPath{"projects", "dbdrivers", "dbservers"})`
**Then** the create returns `nil` and writes `projects/.collection/subcollections/dbdrivers/subcollections/dbservers/definition.yaml` with that `record_file`; the alter returns `nil` and adds the field to that same file; the drop returns `nil` and removes that definition while `projects/.collection/subcollections/dbdrivers/definition.yaml` remains. This AC deliberately has no `dbservers` records and makes no claim about record data, which waits on the nested-drop Open Question.

## Architecture

| File | Change |
|---|---|
| `github.com/dal-go/record` `key.go` | `EscapeID` also escapes `{` `}` `,` `=`; new `UnescapeID`; the path grammar (segment splitting, parity, id-segment classification) is defined here once. |
| `dbschema/collection_path.go` | New: `CollectionPath`, `SchemaPath`, `ParseSchemaPath` (built on the `record` grammar), and the four path errors. |
| `dbschema/collection_def.go` | Add `Parent`, `ParentIDNames`, `Path()`, `SchemaPath()`, `Normalize()`; godoc for the nesting meaning and the two forms. |
| `ddl/modifier.go` | `SchemaModifier` embeds `SubCollectionsAware` and `ExtensionAware`; `DropCollection` and `AlterCollection` take `dbschema.CollectionPath`. |
| `ddl/options.go` | Add `Options.Extensions` and `WithExtension`; godoc states the strict rule next to the mismatched-option rule. |
| `ddl/extension.go` | New: `Extension`, `ExtensionAware`, `ErrInvalidExtension`, and the internal addressing check. |
| `ddl/subcollections.go` | New: `SubCollectionsAware`, `SupportsSubCollections`. |
| `ddl/alter_op.go` | Sealed `AlterOp` gains unexported `opts() Options`; each of the six op types returns its stored field. |
| `ddl/operations.go` | All helpers apply REQ:guard-precedence and normalise paths; new `DropCollectionAt` and `AlterCollectionAt`; `DropCollection` / `AlterCollection` parse a schema path string. |
| `mocks/mock_ddl/schema_modifier.go` | Regenerated for the changed interface. |
| `spec/features/ddl/`, `spec/features/ddl/schema-modifier/`, `spec/features/ddl/options/`, `spec/features/dbschema/collection-def/` | Updated to the changed interface once this Feature is Approved. |

## Error Handling and Failure Modes

| Failure mode | Result |
|---|---|
| Malformed path (empty segment, trailing `/`, forbidden character in a name, bad escape in an id, malformed or duplicate `{placeholder}`, missing id name in structured form) | error, `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; no dispatch |
| Even segment count where a collection is expected | also `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; no dispatch |
| Concrete id in a schema path | also `errors.Is(err, dbschema.ErrConcreteIDInSchemaPath)`; no dispatch |
| Composite key segment `{field=value,…}` | `errors.Is(err, dbschema.ErrReservedKeySegment)` and `errors.Is(err, dal.ErrNotSupported)`; no dispatch |
| Both a slash-path `Name` and a non-empty `Parent` / `ParentIDNames` | `dbschema.ErrInvalidCollectionPath`; no dispatch |
| Placeholder name for an existing ancestor differs from the one a nesting driver stored | driver-specific non-nil error; nothing created |
| Extension with empty target | error, `errors.Is(err, ddl.ErrInvalidExtension)`; no dispatch |
| Extension the driver does not honour, on any operation or `AlterOp`, whatever its target | `*dbschema.NotSupportedError` naming type and target; no operation performed |
| DB does not implement `SchemaModifier` (for example a store whose schema is defined elsewhere) | existing `*dbschema.NotSupportedError`; no dispatch |
| Nested path or `Parent`, driver lacks nesting | `*dbschema.NotSupportedError`; no dispatch |
| An ancestor collection does not exist (nesting driver) | driver-specific non-nil error; nothing created |
| Honoured extension with an invalid value (e.g. an inGitDB `RecordFileDef` failing `Validate`) | driver-specific error; the surface is supported, the value is not |

## Testing Strategy

In-package Go tests in `record`, `dbschema` and `ddl` with stub `dal.DB` values,
following the existing `ddl/operations_test.go` pattern, plus a compile-shape
test for `SchemaModifier` and a round-trip test over `ParseSchemaPath` /
`String`. AC:flat-in-org-drivers-refuse-directly runs in each SQL driver's
suite, and the three `dalgo2ingitdb-*` ACs in `ingitdb/dalgo2ingitdb`'s suite,
against releases carrying this Feature. No Rehearse stubs: every AC has a
direct Go test surface.

## Not Doing / Out of Scope

- **Multi-field keys (future work).** The id-segment syntax
  `{field=value,field=value}` (e.g. `/countries/{country=us,state=ca}/…`) is
  reserved by REQ:path-grammar and rejected as not supported. Parsing it into
  `record.FieldVal` keys, and escaping inside braces, is a later Feature.
- **Parsing record keys from strings.** This Feature parses schema paths only.
  A `record.ParseKey` for data paths is a later addition. It MUST use the same
  grammar from REQ:path-grammar and reject placeholders.
- **Changing `record.Key.CollectionPath()`**, which emits collection names
  joined by `/` for other callers. Schema paths use `SchemaPath.String()`.
- **Concrete extension types in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Implementing module changes in this repo.** The work in
  REQ:in-org-drivers-comply is tracked on dal-go/dalgo#163. dalgo2ingitdb's
  extension type, target constant, nesting, depth-N ancestor check, path-based
  Drop/Alter and conforming its slash parsing to the grammar are
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
  datasets. A subcollection lives inside one database; a schema path cannot
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

- **Enforcing one placeholder name per collection.** REQ:schema-paths-use-placeholders
  requires the same name wherever a collection appears (e.g. always
  `{projectID}` under `projects`). A nesting driver that persists names refuses
  a conflict. Should a driver that has nowhere to store names (a future flat or
  remote driver) be required to add storage for them, or may it skip the
  cross-call check?
  - **A.** Required: every nesting driver persists placeholder names and
    refuses conflicts.
  - **B.** Optional: drivers without name storage skip the cross-call check;
    the `ddl` helpers still enforce uniqueness within one path.

  Recommendation: **A**, because code generation and typed keys depend on the
  names being stable per collection. For inGitDB this is one extra field in
  `definition.yaml`. Needs a founder decision.
- **Record data on a nested drop.** For a nesting driver, what does
  `ddl.DropCollection(ctx, db, "projects/{projectID}/queries")` do with existing `queries`
  records under the `projects` records?
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
