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

Three changes to the schema API, so that a consumer can create nested collections,
and pass driver-specific storage options for them, through DALgo:

1. **Subcollection declaration.** A nested collection is addressed by a
   `dbschema.SchemaPath`, written in either of two equivalent forms that
   normalise to the same structured value:
   - **structured:** `CollectionDef{Name: "queries", Parent: []dbschema.ParentStep{{Collection: "ext", ID: dbschema.ID("datatug")}, {Collection: "projects", ID: dbschema.Placeholder("projectID")}}}`;
   - **string:** `"ext/datatug/projects/{projectID}/queries"` (a leading `/` is
     optional), in the Firestore-style alternating grammar
     `collection/id/collection/id/…` that DALgo key paths already use
     (`record.Key.String()`). An **odd** segment count addresses a collection;
     an **even** count addresses a record and is rejected wherever a collection
     is expected.

   Each id position in a schema path is either a **concrete id**, which scopes
   the definition to that one parent record (`ext/datatug/…`, the namespacing
   DataTug uses in Firestore), or a **`{placeholder}`**, which applies it to
   every parent record. `{field=value,…}` is reserved for future multi-field
   keys and rejected today with a typed not-supported error. Data paths carry
   concrete ids only. A nested collection is created with one
   `CreateCollection` call per collection, after its ancestor collection
   definitions exist; a scoping parent record need not exist.
   `SchemaModifier.DropCollection` and `AlterCollection` take a `SchemaPath`.
   Every driver states whether it supports nesting through
   `SupportsSubCollections()`, which becomes part of `SchemaModifier`.
2. **Driver extensions.** `ddl.Options` gains `Extensions []ddl.Extension`, set
   through `ddl.WithExtension(ext)`. Each extension carries a stable target ID,
   a constant exported by the driver module that defines it (for example
   `dalgo2ingitdb.ExtensionTarget`). Extensions are strict: one is honoured only
   when the driver's `SupportsExtension(op, ext)` returns `true`, and is
   otherwise refused with `*dbschema.NotSupportedError`. There is no mode that
   ignores an extension.

3. **Safe drops.** A drop, root or nested, removes only the definition by
   default and refuses with `ddl.ErrCollectionNotEmpty` when records,
   descendant definitions or referencing records would go too. Explicit
   `ddl.DeleteRecords()` and `ddl.DeleteNested()` authorise those losses;
   `ddl.DeleteDependents()` is reserved.

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
collections, namespaced under the DataTug extension record as in Firestore
(`ext/datatug/projects/<id>/…`; founder, 2026-09-17: "That's how we keep them in
Firestore and we should do the same in git storage for consistency. This would
also allow us to store data from multiple extensions without conflicts -
namespacing."), each with `record_file` `name: '{key}/{key}.<suffix>.json'`,
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
  schema path normalise to the same `SchemaPath` before any driver sees them.
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

A DALgo path string MUST follow this grammar. It is the grammar
`record.Key.String()` already emits for keys with scalar ids, extended only as
stated here:

```
path          = [ "/" ] segment *( "/" segment )   ; no trailing "/", no empty segment
                                                   ; odd positions (1st, 3rd, …) are collections,
                                                   ; even positions are ids
collection    = name
id            = composite-key / placeholder / concrete-id
composite-key = "{" <any text containing "="> "}"  ; reserved, not supported yet
placeholder   = "{" identifier "}"                 ; schema paths only
identifier    = ( ALPHA / "_" ) *( ALPHA / DIGIT / "_" )  ; = dalgo/access capture name
concrete-id   = <record.EscapeID output>           ; never begins with "{"
```

- **Leading `/`: optional on input, never emitted.** Parsers MUST accept both
  `ext/datatug/projects` and `/ext/datatug/projects` as the same path. Every
  `String()` in this Feature MUST emit the canonical form without the leading
  `/`, matching `record.Key.String()` and `dal.CollectionRef.Path()` today.
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
- **Schema paths** MAY mix concrete ids and placeholders, one per id position
  (REQ:schema-path-scopes). **Data paths** (record keys, `dal.CollectionRef`
  under a concrete parent) MUST contain concrete ids only; any data-path parser
  added later MUST reject a placeholder with a typed error. Keys built with the
  `record` constructors cannot contain one, because `record.EscapeID` escapes
  `{` (see REQ:key-constructors-validate for why this holds for every key).
- **Placeholder names within one path MUST be unique**
  (`projects/{id}/environments/{id}/servers` is malformed).
- **One definition shared with `dalgo/access`.** The placeholder is the same
  construct as an access-policy capture: `access.Capture(name)`
  (`access/resource.go:134`) renders as `{name}` (`:63`), requires the identifier
  regular expression `^[A-Za-z_][A-Za-z0-9_]*$` (`:227`) and unique names within
  a pattern. The identifier rule MUST move to `github.com/dal-go/record`
  (e.g. `record.ValidPlaceholderName`), and both `access` and `dbschema` MUST use
  it, so `ext/datatug/projects/{projectID}/queries` means the same text in a
  schema path and in an access pattern. The access `*` (`access.AnyID`) has no
  schema-path equivalent; a schema path always names its placeholders.
- **Escaping.** `record.EscapeID` MUST be extended so that, besides its current
  characters, `{` `}` `,` `=` become `%7B` `%7D` `%2C` `%3D`. Every escaped id
  is unambiguous, and no concrete id can look like a placeholder or a composite
  key, **provided** its raw value passed `record.ValidateStringID`, which
  reserves a literal `%` (REQ:key-constructors-validate). Ids containing
  `{ } , =`, such as base64 values ending in `=`, MUST round-trip through
  `EscapeID` and `UnescapeID`.
- **Composite `record.FieldVal` ids are outside this grammar.** Today
  `record.Key.String()` prints them with `%v` (`key.go:54-70`), which is not the
  reserved `{field=value,…}` form. Until the multi-field keys Feature defines
  that form, their string output is unspecified and MUST NOT be parsed.

The grammar, the segment classification, the placeholder identifier rule and
the escape table MUST be defined once, in `github.com/dal-go/record` next to
`EscapeID`. `dbschema` MUST implement schema-path parsing on top of it rather
than with its own splitting or escaping rules.

#### REQ: key-constructors-validate

Today `record.NewKeyWithID` and `record.NewKeyWithParentAndID`
(`key.go:151-163`) accept any string id without calling
`record.ValidateStringID`, so ids `a/b` and `a%2Fb` both print as `a%2Fb`. In
line with the private-beta direction, the `record` constructors MUST validate
the characters of an id, but MUST keep the empty-id incomplete-key pattern:

- `NewKeyWithID`, `NewKeyWithParentAndID` and `WithKeyID` (`key.go:174`) MUST
  reject a **non-empty** string id that contains the reserved `%`. The two
  panicking constructors panic, as they already do for an empty collection name
  (`key.go:159-161`). `WithKeyID` returns an error satisfying
  `errors.Is(err, record.ErrInvalidStringID)` through `NewKeyWithOptions`.
- An **empty** string id stays legal in every constructor. It denotes an
  incomplete key whose id a driver or generator assigns before the write, the
  same state `NewIncompleteKey` (`key.go:166`) produces. Record factories across
  the fleet build keys this way (for example `dal/schema_test.go:184`,
  `access/conditions_test.go:368` and `dalgo2ingitdb` `crud_test.go:293`), so no
  caller migration is needed.
- `Key.String()` MUST NOT panic. It MUST stop calling `Key.Validate()`
  (`key.go:54-58`), so logging any key, incomplete or invalid, is safe. For a
  key with an empty id, `String()` emits an empty id segment (`users/`), which
  is not a valid path under REQ:path-grammar and MUST NOT be parsed.
- Completeness and id validity are checked at **write time**: `Key.Validate()`
  (`key.go:130`) MUST apply the `%` check to non-empty string ids at every
  level, and the write path (driver or id generator) MUST reject a key that is
  still incomplete when the record is persisted.

Recommendation, over both limiting the claims to schema paths and migrating
every empty-id caller to `NewIncompleteKey`: one grammar for keys and schema
paths is only true if every key string is unambiguous, and rejecting `%` at
construction is the cheapest place to guarantee that. Keeping empty ids legal
avoids migrating 65 `NewKeyWithID(collection, "")` call sites (all in tests, in
`dalgo`, `dalgo2ingitdb` and `dalgo2ingitdb4local`, found by searching
`/home/ai/projects` on 2026-09-17) for no gain, because an empty id never
reaches a written path.

#### REQ: schema-path-type

The `dbschema` package MUST export the structured form of a schema path:

```go
// PathID is one id position of a schema path. Exactly one field is non-empty.
type PathID struct {
    ID          string // concrete id (unescaped): scopes to that one parent record
    Placeholder string // placeholder name: applies to every parent record
}

func ID(id string) PathID
func Placeholder(name string) PathID

// ParentStep is one ancestor level: a collection and the id position below it.
type ParentStep struct {
    Collection string
    ID         PathID
}

// SchemaPath addresses a collection definition: its ancestors, root first,
// and its own name.
type SchemaPath struct {
    Parent []ParentStep
    Name   string
}

func ParseSchemaPath(s string) (SchemaPath, error)
func (p SchemaPath) String() string            // e.g. "ext/datatug/projects/{projectID}/queries"
func (p SchemaPath) Validate() error
func (p SchemaPath) SameCollection(q SchemaPath) bool
func (p SchemaPath) Overlaps(q SchemaPath) bool

var (
    ErrInvalidCollectionPath   = errors.New("dbschema: invalid collection path")
    ErrRecordPathNotCollection = errors.New("dbschema: path addresses a record, not a collection")
    ErrReservedKeySegment      = errors.New("dbschema: multi-field key segments are not supported yet")
)
```

- `Validate()` MUST check every name and every `PathID` against
  REQ:path-grammar (exactly one of `ID` / `Placeholder` set; `ID` non-empty and
  valid under `record.ValidateStringID`; `Placeholder` an identifier, unique in
  the path) and return an error satisfying `errors.Is(err, ErrInvalidCollectionPath)`
  on the first violation. `Name` MUST be non-empty.
- A nil and an empty `Parent` MUST mean the same (root). `ParseSchemaPath` and
  `CollectionDef.Normalize` MUST return `Parent == nil` for a root collection,
  so a root path round-trips to an equal value whichever way it was built.
- `String()` MUST interleave collection names and ids, escaping concrete ids with
  `record.EscapeID` and writing placeholders as `{name}`:
  `SchemaPath{Parent: []ParentStep{{"ext", ID("datatug")}, {"projects", Placeholder("projectID")}}, Name: "queries"}`
  gives `"ext/datatug/projects/{projectID}/queries"`, and `SchemaPath{Name: "users"}`
  gives `"users"`.
- `ParseSchemaPath` MUST apply REQ:path-grammar, and `ParseSchemaPath(p.String())`
  MUST equal `p` for every valid `p`.
- `ErrRecordPathNotCollection` MUST be returned wrapped so that
  `errors.Is(err, ErrInvalidCollectionPath)` is also `true`.
- `SameCollection` MUST return `true` when both paths have the same `Name`, the
  same collection names at every level, and, at every id position, either equal
  concrete ids or placeholders on both sides (placeholder names do not affect
  identity).
- `Overlaps` MUST return `true` when the paths are not `SameCollection` but have
  the same `Name` and collection names and, at every id position, equal concrete
  ids or a placeholder on at least one side (for example
  `ext/{extID}/projects` and `ext/datatug/projects`).

#### REQ: schema-path-scopes

A schema declaration (`CreateCollection`, `DropCollection`, `AlterCollection`
and their helpers) addresses a collection **definition**. Each id position in
its path is one of:

- a **placeholder** (`projects/{projectID}/queries`): the definition applies to
  the subcollection under **every** record of the parent collection;
- a **concrete id** (`ext/datatug/projects`): the definition is **scoped** to
  the subcollection under that **one** parent record. This is how extensions
  namespace their data (`ext/datatug/…`, `ext/<other>/…`) without conflicts,
  matching DataTug's Firestore layout.

Rules for drivers whose `SupportsSubCollections()` is `true`:

- A driver MUST keep the scope: a definition created at
  `ext/datatug/projects` MUST be found by `SameCollection` at that path, and
  MUST NOT apply to records under `ext/other/projects`. Where and how the driver
  stores the scope is the driver's choice (for `dalgo2ingitdb`,
  ingitdb/dalgo2ingitdb#16, which depends on REQ:ingitdb-format-dependency).
- Scoped definitions under different concrete ids are independent:
  `ext/datatug/projects` and `ext/sneat/projects` MAY both exist, with
  different fields and extensions.
- A driver MUST refuse to create a definition that `Overlaps` an existing one
  (for example `ext/{extID}/projects` when `ext/datatug/projects` exists, or the
  reverse), with a driver-specific non-nil error, because a record under
  `ext/datatug/projects` would then match two definitions.
- **The scoping parent record need not exist.** Creating `ext/datatug/projects`
  MUST NOT require an `ext` record with id `datatug`, and MUST NOT create one,
  matching Firestore, where a subcollection may exist under a parent document
  that does not. The ancestor collection **definitions** still must exist
  (REQ:one-collection-per-create).

**Placeholder names are part of the schema, not free-form.** A placeholder
names the id of the collection it follows, so it MUST be the same wherever that
collection definition appears: every path through `ext/datatug/projects` uses
`{projectID}`. The `ddl` helpers cannot see earlier calls, so they check only
uniqueness within one path. A nesting driver that persists placeholder names
MUST refuse a declaration whose name for an existing ancestor differs from the
stored one. Addressing ignores names (`SameCollection`). Rationale: typed-key
and code generation tooling maps `{projectID}` to one key field, so one name per
collection keeps generated code stable. How drivers without storage for names
enforce this is under Open Questions.

#### REQ: ingitdb-format-dependency

`dalgo2ingitdb` cannot meet REQ:schema-path-scopes with today's inGitDB definition
format:

- `ingitdb-go` `CollectionDef.SubCollections` is `map[string]*CollectionDef`
  keyed by collection name (`ingitdb/collection_def.go:42`), so
  `ext/datatug/projects` and `ext/sneat/projects` collide;
- `dalgo2ingitdb`'s `resolveScopedCollection` (`scoped_collection.go:39`) looks
  subcollections up by name only;
- there is no field for a placeholder name, and the strict `KnownFields` reader
  rejects unknown keys.

This Feature therefore has an explicit dependency on an `ingitdb-go`
definition-format change for id-scoped subcollection definitions and
placeholder names: [ingitdb/ingitdb-go#26](https://github.com/ingitdb/ingitdb-go/issues/26).

Alternative evaluated, which needs no format change: register a **root**
collection whose `DirPath` is `ext/datatug/projects`, with nested definitions
under it. The `ingitdb-go` validator accepts it: `RootConfig.Validate`
(`ingitdb/config/root_config.go:112-160`) rejects only empty ids, `*` and
duplicate paths, not a path inside another collection's directory, and
`readRootCollections` (`ingitdb/validator/def_validator.go:240-256`) reads each
entry independently. It is **not recommended**:

- the root id must pass `ValidateCollectionID` (`ingitdb/collection_id.go:11`),
  which forbids `/`, so `dalgo2ingitdb` would need a second mapping from key
  prefixes (`ext/datatug/projects`) to invented ids;
- it cannot express a placeholder scope above the scoped level
  (`ext/{extID}/projects`), cannot persist placeholder names, and cannot detect
  overlaps;
- an `ext` collection with `records_dir: '.'` would see `ext/datatug/` as a
  record directory.

Recommendation: the `ingitdb-go` format change, which models scopes directly
and fits the founder's fix-at-source rule.

### Subcollection declaration

#### REQ: collection-def-parent

`dbschema.CollectionDef` MUST gain the field `Parent []ParentStep` and the method
`SchemaPath() SchemaPath` (a copy of `Parent` plus `Name`). An empty `Parent`
means a root collection. A non-empty `Parent` declares that the collection is a
subcollection nested under records of the last `Parent` collection, scoped per
REQ:schema-path-scopes.

The DataTug collections this makes expressible, with the founder's `ext`
namespacing, are: `ext`, `ext/datatug/projects`;
`ext/datatug/projects/{projectID}/credentials`,
`ext/datatug/projects/{projectID}/queries`,
`ext/datatug/projects/{projectID}/entities`,
`ext/datatug/projects/{projectID}/environments`,
`ext/datatug/projects/{projectID}/dbmodels`,
`ext/datatug/projects/{projectID}/boards`,
`ext/datatug/projects/{projectID}/recordsets`,
`ext/datatug/projects/{projectID}/folders`,
`ext/datatug/projects/{projectID}/dbdrivers`; and
`ext/datatug/projects/{projectID}/environments/{envID}/servers`,
`ext/datatug/projects/{projectID}/environments/{envID}/catalogs`,
`ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers`.

#### REQ: string-and-structured-forms-round-trip

`CollectionDef.Name` MAY hold a schema path string instead of a single name.
`dbschema.CollectionDef` MUST gain `Normalize() (CollectionDef, error)`:

- If `Name` contains no `/` (after an optional leading `/`), `Normalize` returns
  the definition unchanged, after validating `SchemaPath()`.
- Otherwise, if `Parent` is non-empty, it MUST fail with
  `ErrInvalidCollectionPath`: the two forms MUST NOT be mixed.
- Otherwise it parses `Name` with `ParseSchemaPath` and returns a copy with
  `Parent` and `Name` taken from the result.

So `CollectionDef{Name: "/ext/datatug/projects/{projectID}/queries"}`,
`CollectionDef{Name: "ext/datatug/projects/{projectID}/queries"}` and
`CollectionDef{Name: "queries", Parent: []ParentStep{{"ext", ID("datatug")}, {"projects", Placeholder("projectID")}}}`
all normalise to the last of these, whose `SchemaPath().String()` is
`"ext/datatug/projects/{projectID}/queries"`. The `ddl` helpers MUST normalise
before dispatch, so a driver always receives the structured form. A driver
called directly MUST call `Normalize` itself.

#### REQ: one-collection-per-create

Each `CreateCollection` call MUST create exactly one collection definition, the
one addressed by the normalised `c.SchemaPath()`. A driver whose
`SupportsSubCollections()` is `true` MUST return a non-nil error, and MUST NOT
create anything, when the definition of **any** ancestor (the path truncated
after each `Parent` collection, keeping its ids) does not exist, matched with
`SameCollection` or, for a placeholder ancestor definition, one it covers. It
MUST NOT create an ancestor implicitly. Parent **records** are never required
(REQ:schema-path-scopes). `IfNotExists` applies to the addressed definition
only.

Rationale (why a `Parent` chain, not `SubCollections []CollectionDef` or a
parent record key): a recursive `SubCollections` tree would force one options
set on a whole tree, while DataTug needs a different `record_file` per
subcollection (`.query.json`, `.entity.json`, `.server.json`). It would make
`IfNotExists` ambiguous on a partly existing tree and make partial failure
likely on non-transactional drivers. A single parent record key could not say
"every project"; the per-position concrete-id-or-placeholder chain says both.

#### REQ: path-addressed-drop-and-alter

The `SchemaModifier` interface MUST change to address collections by schema
path:

```go
type SchemaModifier interface {
    SubCollectionsAware
    ExtensionAware
    DropCapable
    CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error
    DropCollection(ctx context.Context, path dbschema.SchemaPath, opts ...Option) error
    AlterCollection(ctx context.Context, path dbschema.SchemaPath, ops ...AlterOp) error
}
```

The `ddl` package MUST export:

```go
func DropCollectionAt(ctx context.Context, db dal.DB, path dbschema.SchemaPath, opts ...Option) error
func AlterCollectionAt(ctx context.Context, db dal.DB, path dbschema.SchemaPath, ops ...AlterOp) error
```

and keep `ddl.DropCollection(ctx, db, path string, opts...)` and
`ddl.AlterCollection(ctx, db, path string, ops...)`. Each string helper MUST
parse its argument with `dbschema.ParseSchemaPath` and then behave exactly as
the `*At` helper. `"users"`, `"ext/datatug/projects/{projectID}/queries"` and
`"/ext/datatug/projects/{pid}/queries"` are all valid, and the last two address
the same definition; `"ext/datatug"` is an even-count record path and is
rejected.

A driver whose `SupportsSubCollections()` is `true` MUST resolve `path` with
`SameCollection` to the definition that `CreateCollection` created, and MUST
treat a scoped path and a placeholder path as different definitions. Dropping a
collection that has subcollections is governed by REQ:drop-refuses-data-loss-by-default.

#### REQ: drop-refuses-data-loss-by-default

Founder decision, 2026-09-17: "1-A. We probably should have explicit flags like
delete nested and delete recursive dependents?"

A drop (`DropCollection`, root or nested, through any helper or directly) MUST
by default remove **only** the addressed definition. If anything else would be
deleted, it MUST remove nothing and return `*ddl.CollectionNotEmptyError`:

```go
var ErrCollectionNotEmpty = errors.New("ddl: collection is not empty")

type CollectionNotEmptyError struct {
    Path           dbschema.SchemaPath   // the collection asked to drop
    Records        int                   // records of Path found, under every matching parent
    MoreRecords    bool                  // true if the driver stopped counting early
    SubCollections []dbschema.SchemaPath // descendant definitions that would be dropped
    Referrers      []dbschema.Referrer   // collections whose records reference records of Path
}

func (e *CollectionNotEmptyError) Error() string
func (e *CollectionNotEmptyError) Is(target error) bool // true for ErrCollectionNotEmpty
```

Blockers are, and a non-empty field MUST be reported for each one present:

1. records of the addressed collection, under every parent record the path's
   scope matches (a placeholder matches every parent record, a concrete id only
   that one), unless `DeleteRecords()` is given;
2. descendant subcollection definitions, unless `DeleteNested()` is given;
3. records of descendant subcollections, unless both `DeleteNested()` and
   `DeleteRecords()` are given;
4. records in other collections that reference records to be deleted, where
   the driver knows such references (for example `dalgo2ingitdb`'s foreign keys,
   `foreign_keys.go:102`, or an SQL foreign key). `DeleteDependents()` is
   reserved (REQ:drop-flags), so this blocker cannot be lifted today.

A driver MAY stop counting records after finding one and set `MoreRecords`.

This supersedes the current root `DropCollection` in `dalgo2ingitdb`, which
removes the whole collection directory, records included (`os.RemoveAll`,
`schema_modifier.go:122`). That becomes the behaviour of
`DropCollection(…, DeleteRecords(), DeleteNested())` only, in the scope of
ingitdb/dalgo2ingitdb#16.

#### REQ: drop-flags

`ddl.Options` MUST gain `DeleteRecords`, `DeleteNested` and `DeleteDependents`
booleans, set by these options:

```go
func DeleteRecords() Option    // also delete the dropped collections' records
func DeleteNested() Option     // also drop descendant subcollection definitions
func DeleteDependents() Option // reserved: cascade to referencing records
```

- `DeleteRecords()` allows deleting the records of every collection the drop
  removes: the addressed collection's records under every parent record its
  scope matches, and, with `DeleteNested()`, the descendants' records.
- `DeleteNested()` allows dropping descendant definitions. It is
  **independent** of `DeleteRecords()`: on its own it drops empty descendants
  and still refuses if any of them has records. Recommendation for
  independence: each flag authorises exactly one kind of loss, so an empty tree
  can be dropped without also authorising record deletion, and the error names
  precisely which flag is missing.
- `DeleteDependents()` is **reserved**. DALgo has no driver-agnostic
  foreign-key declaration or cascade model today: `dbschema.ConstraintDef`
  carries only `Name` and `Type` and defers "foreign-key target + cascade
  actions" (`dbschema/constraint.go:3-9`), and `dbschema.Referrer`
  (`dbschema/referrer.go`) is read-side introspection through the optional
  `SchemaReader.ListReferrers`. Every drop helper (`ddl.DropCollection` and
  `ddl.DropCollectionAt`) MUST refuse `DeleteDependents()`
  with `*dbschema.NotSupportedError{Reason: "DeleteDependents is reserved"}`.
  A drop that would orphan referencing records refuses with blocker 4.
- The flags are meaningful on drops only. On `CreateCollection` and on
  `AlterOp`s they are mismatched options and are silently ignored, per
  `ddl/options.go:10-16`.

Drivers declare which flags they can execute through `SupportsDrop` (embedded
in `SchemaModifier`):

```go
type DropCapable interface {
    // SupportsDrop reports whether the driver can execute a drop with every
    // flag set in flags. It is constant for the lifetime of a DB value.
    SupportsDrop(flags DropFlags) bool
}

type DropFlags struct{ DeleteRecords, DeleteNested bool }
```

A driver whose `SupportsSubCollections()` is `false` MUST return `false` for
`DeleteNested`.

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

When a `ddl` helper receives, after normalisation, a path with a non-empty
`Parent`, and `SupportsSubCollections()` is `false`, the helper MUST return
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
   path errors of REQ:path-grammar and REQ:schema-path-type.
2. **Option validity.** Row 1 of REQ:extension-addressing
   (`ErrInvalidExtension`), for `opts` or, for Alter, for every `AlterOp` in op
   order. Then, on drops, a reserved `DeleteDependents()` (REQ:drop-flags).
3. **Capability.** The DB implements `SchemaModifier`, otherwise the existing
   `*dbschema.NotSupportedError` ("driver does not implement
   ddl.SchemaModifier", `ddl/operations.go:25-31`). Then nesting, per
   REQ:nesting-refused-without-capability. Then, on drops with `DeleteRecords()`
   or `DeleteNested()`, `SupportsDrop` with those flags, otherwise
   `*dbschema.NotSupportedError` naming the flag. Then rows 2-3 of
   REQ:extension-addressing, which need the driver's `SupportsExtension`.

Data-dependent refusals (`CollectionNotEmptyError`, missing ancestors,
overlaps) come from the driver after dispatch.

### Driver obligations

#### REQ: helpers-are-the-guarantee

The `ddl` helpers are the designed entry point. Every **declaration and
capability** refusal in this Feature (path validity, extensions, reserved or
unsupported flags, nesting) is enforced by them before dispatch, independent of
driver quality, so a consumer that calls through them never loses a nesting
declaration or an extension silently. **Data-dependent** refusals
(`CollectionNotEmptyError`, a missing ancestor definition, an overlapping
definition) can only be decided by the driver after dispatch; they bind drivers
through REQ:in-org-drivers-comply and are verified by each driver's
conformance ACs. A DB that does not implement `SchemaModifier` gets the
existing typed `*dbschema.NotSupportedError` from every helper, so nothing is
skipped silently.

#### REQ: in-org-drivers-comply

Every driver in the founder's organisations (`dal-go`, `ingitdb`, `openvaultdb`,
`datatug`, `sneat-dev`, `synchestra-io`) that implements `SchemaModifier` MUST
implement the changed interface and enforce, on direct calls, the driver-side
MUSTs of REQ:string-and-structured-forms-round-trip, REQ:drop-refuses-data-loss-by-default, REQ:drop-flags,
REQ:schema-path-scopes,
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
(`EscapeID` extension, new `UnescapeID`, placeholder identifier rule) of
REQ:path-grammar and REQ:key-constructors-validate. The per-module work is
tracked on dal-go/dalgo#163.

#### REQ: in-org-callers-comply

Every in-org **caller** of `SchemaModifier`, the `ddl` helpers or the path
grammar MUST conform in the same release wave: nested collections are addressed
by schema paths (odd segment count, `{placeholder}` or concrete ids), and ids are
unescaped with `record.UnescapeID` or a standard percent-decoder, never a
hard-coded table of the current six escapes. Found by searching every repository
under `/home/ai/projects` (local checkouts, 2026-09-17):

| Caller | Today | Required change |
|---|---|---|
| `openvaultdb/openvaultdb-go` | `collectionChains` (`pkg/core/key.go:110`) builds names-only chains like `spaces/ext`; `core.go:358` passes them to `modifier.CreateCollection` (`core.go:555`). Even count, so it breaks. | Derive `spaces/{spaceID}/ext` (or concrete ids for a scoped definition) from the key, or pass `CollectionDef.Parent`. Tracked in [openvaultdb/openvaultdb-go#28](https://github.com/openvaultdb/openvaultdb-go/issues/28). |
| `dal-go/dalgo2openvaultdb` | `unescapeSegment` (`query.go:293`) decodes only the six current `EscapeID` codes. | Use `record.UnescapeID`; ids with `{ } , =` must round-trip. |
| `openvaultdb/ovdb` | `internal/datapath/datapath.go:41` hard-codes the six codes and treats any other `%` as invalid. | Use `record.UnescapeID`; ids with `{ } , =` must round-trip. |
| `datatug/datatug-cli` | `pkg/dbcopy/engine.go:150` `ddl.DropCollection(…, ref.Name(), ddl.IfExists())`, `:259` `ddl.CreateCollection`. Root names only. | Compatible. Its drop now refuses non-empty targets; it must pass `DeleteRecords()` where it intends to replace data. Because `DeleteDependents()` is reserved, on relational targets it must drop referencing tables before the tables they reference (reverse foreign-key order), or the drop refuses with blocker 4. |
| `sneat-dev/wb` | `internal/hubstore/hubstore.go:98` `CreateCollection` with a root name. | None. |
| `synchestra-io/synchestra` | `pkg/state/replication/dal_journal.go:128` `ddl.CreateCollection` with a root definition. | None. |

Already safe, because they use `url.PathUnescape` or their own escaper:
`openvaultdb-go` `ParseKeyPath` (`pkg/core/key.go:93-97`) and `dalgo2ingitdb`
`record_io.go:27`.

## Acceptance Criteria

### AC: root-def-path (verifies REQ:collection-def-parent, REQ:schema-path-type)

**Given** `c := dbschema.CollectionDef{Name: "users"}` with no `Parent`
**When** `c.SchemaPath()` and `c.SchemaPath().String()` are called
**Then** `c.SchemaPath()` equals `SchemaPath{Name: "users"}`, the string is `"users"`, and `c.SchemaPath().Validate()` returns `nil`.

### AC: schema-path-round-trip (verifies REQ:schema-path-type, REQ:path-grammar)

**Given** the strings `"users"`, `"/users"`, `"ext/datatug/projects"`, `"/ext/datatug/projects/{projectID}/queries"`, `"ext/datatug/projects/{projectID}/environments/{envID}/servers"` and `"projects/a%2Fb/queries"`
**When** each is passed to `dbschema.ParseSchemaPath`, and the result's `String()` is parsed again
**Then** they yield, respectively: `{Name: "users"}` twice; `{Parent: [{ext, ID("datatug")}], Name: "projects"}`; `{Parent: [{ext, ID("datatug")}, {projects, Placeholder("projectID")}], Name: "queries"}`; `{Parent: [{ext, ID("datatug")}, {projects, Placeholder("projectID")}, {environments, Placeholder("envID")}], Name: "servers"}`; and `{Parent: [{projects, ID("a/b")}], Name: "queries"}` (unescaped id). The canonical strings have no leading `/`, and parsing each canonical string returns an equal `SchemaPath`.

### AC: forms-normalise-identically (verifies REQ:string-and-structured-forms-round-trip, REQ:collection-def-parent)

**Given** `a := CollectionDef{Name: "queries", Parent: []ParentStep{{"ext", ID("datatug")}, {"projects", Placeholder("projectID")}}}`, `b := CollectionDef{Name: "ext/datatug/projects/{projectID}/queries"}` and `c := CollectionDef{Name: "/ext/datatug/projects/{projectID}/queries"}`, each with the same `Fields`
**When** `Normalize()` is called on each, and on `d := CollectionDef{Name: "projects/{projectID}/queries", Parent: []ParentStep{{"ext", ID("datatug")}}}` and `e := CollectionDef{Name: "queries", Parent: []ParentStep{{"projects", PathID{}}}}`
**Then** `a`, `b` and `c` normalise to equal definitions whose `SchemaPath().String()` is `"ext/datatug/projects/{projectID}/queries"`; `d` (mixed forms) and `e` (empty id position) fail with `errors.Is(err, dbschema.ErrInvalidCollectionPath)`.

### AC: same-collection-and-overlap (verifies REQ:schema-path-type, REQ:schema-path-scopes)

**Given** the parsed paths `p1 = "ext/datatug/projects/{projectID}/queries"`, `p2 = "ext/datatug/projects/{pid}/queries"`, `p3 = "ext/sneat/projects/{projectID}/queries"` and `p4 = "ext/{extID}/projects/{projectID}/queries"`
**When** `SameCollection` and `Overlaps` are evaluated pairwise
**Then** `p1.SameCollection(p2)` is `true` and `p1.Overlaps(p2)` is `false`; `p1` and `p3` are neither the same nor overlapping; `p1.Overlaps(p4)` and `p3.Overlaps(p4)` are `true` and neither is `SameCollection` with `p4`.

### AC: malformed-and-record-paths-rejected (verifies REQ:path-grammar, REQ:schema-path-type, REQ:path-addressed-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true` that records every call
**When** `ddl.DropCollection(ctx, db, s)` and `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: s})` are called for each `s` in: `"ext/datatug"` and `"projects/{projectID}"` (even count); `""`, `"/"`, `"ext/datatug/"`, `"ext//projects"`, `"ext/{}/projects"`, `"ext/{ext-id}/projects"`, `"ext/{extID/projects"`, `"ext/{id}/projects/{id}/queries"`, `"ext/datatug/proj{ects"`, `"ext/data,tug/projects"` and `"ext/data%tug/projects"` (malformed)
**Then** every call fails with `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; the even-count cases also satisfy `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; and the stub records no call.

### AC: multi-field-key-segment-reserved (verifies REQ:path-grammar)

**Given** the strings `"countries/{country=us,state=ca}/cities"` and `"/countries/{country=us}/cities"`
**When** each is passed to `dbschema.ParseSchemaPath`
**Then** each fails with an error satisfying `errors.Is(err, dbschema.ErrReservedKeySegment)` and `errors.Is(err, dal.ErrNotSupported)`.

### AC: id-escaping-extended (verifies REQ:path-grammar)

**Given** `record.NewKeyWithID("projects", "{a=b,c}")` under the extended `record.EscapeID`
**When** `String()` is called and its id segment is passed to `record.UnescapeID`
**Then** the string is `"projects/%7Ba%3Db%2Cc%7D"`, so the id segment is neither a placeholder nor a composite key; `UnescapeID` returns `"{a=b,c}"`; and `record.NewKeyWithID("projects", "a.b").String()` is still `"projects/a%2Eb"`.

### AC: key-constructors-keep-incomplete-keys (verifies REQ:key-constructors-validate)

**Given** the `record` package with this change
**When** `k := record.NewKeyWithID("users", "")`, `record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "s1"), "ext", "")`, `record.NewKeyWithID("users", "a%2Fb")` and `record.NewKeyWithOptions("users", record.WithKeyID("a%b"))` are called, and `k.String()` and `k.Validate()` are called on the first key
**Then** the two empty-id constructors return keys without panicking; `k.String()` returns `"users/"` without panicking; `NewKeyWithID("users", "a%2Fb")` panics with a message naming `record.ErrInvalidStringID`; `NewKeyWithOptions` returns an error satisfying `errors.Is(err, record.ErrInvalidStringID)`; and `String()` on a key assembled with an invalid id also returns without panicking.

### AC: invalid-path-rejected-before-dispatch (verifies REQ:guard-precedence)

**Given** the recording stub of AC:malformed-and-record-paths-rejected
**When** `ddl.CreateCollection` is called with `Parent: []ParentStep{{"..", ID("x")}}, Name: "queries"`, and `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` are called with `SchemaPath{Parent: []ParentStep{{"projects", PathID{}}}, Name: "queries"}` and with the zero `SchemaPath{}`
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: parent-refused-by-flat-driver (verifies REQ:nesting-refused-without-capability, REQ:sub-collections-capability)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == false`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "queries", Parent: []dbschema.ParentStep{{"projects", dbschema.Placeholder("projectID")}}})`, `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects"})`, `ddl.DropCollection(ctx, db, "projects/{projectID}/queries")` and `ddl.AlterCollectionAt(ctx, db, dbschema.SchemaPath{Parent: []dbschema.ParentStep{{"ext", dbschema.ID("datatug")}}, Name: "projects"})` are called
**Then** each returns `*dbschema.NotSupportedError` with `Op` equal to `"CreateCollection"`, `"CreateCollection"`, `"DropCollection"` or `"AlterCollection"` respectively and `errors.Is(err, dal.ErrNotSupported)` true; the stub's `CreateCollection`, `DropCollection` and `AlterCollection` are not invoked; and `ddl.SupportsSubCollections(db)` is `false`.

### AC: parent-dispatched-to-nesting-driver (verifies REQ:nesting-refused-without-capability, REQ:string-and-structured-forms-round-trip)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`
**When** `ddl.CreateCollection` is called with `CollectionDef{Name: "/ext/datatug/projects/{projectID}/queries"}`
**Then** it returns the stub's result, and the stub receives a `CollectionDef` with `Name == "queries"` and `Parent == []ParentStep{{"ext", ID("datatug")}, {"projects", Placeholder("projectID")}}`.

### AC: drop-and-alter-dispatch-paths (verifies REQ:path-addressed-drop-and-alter)

**Given** a stub driver implementing `SchemaModifier` with `SupportsSubCollections() == true`, which records the `path` each method receives
**When** `ddl.DropCollection(ctx, db, "/ext/datatug/projects/{projectID}/environments/{envID}/servers", ddl.IfExists())`, `ddl.AlterCollection(ctx, db, "ext/datatug/projects/{pid}/queries", op)` and `ddl.DropCollection(ctx, db, "users")` are called
**Then** the stub's `DropCollection` receives `SchemaPath{Parent: [{ext, ID("datatug")}, {projects, Placeholder("projectID")}, {environments, Placeholder("envID")}], Name: "servers"}` with `IfExists` set, then `SchemaPath{Name: "users"}`; and its `AlterCollection` receives a path for which `SameCollection` with `ParseSchemaPath("ext/datatug/projects/{projectID}/queries")` is `true`, with `op`.

### AC: schema-modifier-shape (verifies REQ:path-addressed-drop-and-alter, REQ:sub-collections-capability, REQ:extension-aware-capability)

**Given** a Go test file declaring `var _ ddl.SchemaModifier = (*stub)(nil)`
**When** `stub` omits `SupportsSubCollections`, or omits `SupportsExtension`, or declares `DropCollection(ctx, name string, ...)` instead of taking a `dbschema.SchemaPath`
**Then** the file does not compile; with all methods in the shape of REQ:path-addressed-drop-and-alter it compiles.

### AC: no-schema-modifier-is-typed-error (verifies REQ:helpers-are-the-guarantee, REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier` (as for a store whose schema is defined elsewhere), and an extension `ext` with a non-empty target
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/queries"}, ddl.WithExtension(ext))` is called
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
**When** `ddl.AlterCollection(ctx, db, "ext/datatug/projects/{projectID}/queries", ddl.AddField(f1), ddl.AddField(f2, ddl.WithExtension(ext)))` and `ddl.AlterCollection(ctx, db, "users", ddl.AddField(f2, ddl.WithExtension(ext)))` are called
**Then** each returns `*dbschema.NotSupportedError` with `Op == "AlterCollection"` and a `Reason` naming `ext`'s Go type and target, and the stub's `AlterCollection` is never invoked, so `f1` is not applied either.

### AC: guard-order (verifies REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier`, and an extension with target `""`
**When** `ddl.CreateCollection` is called with `Name: "ext/datatug"` and that extension; then with `Name: "ext/datatug/projects"` and that extension; then with `Name: "ext/datatug/projects"` and no extension
**Then** the calls fail, in order, with `dbschema.ErrRecordPathNotCollection`, then `ddl.ErrInvalidExtension`, then `*dbschema.NotSupportedError` (driver does not implement `ddl.SchemaModifier`).

### AC: flat-in-org-drivers-refuse-directly (verifies REQ:in-org-drivers-comply, REQ:nesting-refused-without-capability, REQ:extension-addressing)

Driver conformance statement, verified in each driver's own suite
(`dalgo2sqlite`, `dalgo2postgres`, `dalgo2mysql`) once it adopts this Feature.

**Given** a database value from the driver, with no collections
**When** its `CreateCollection` method is called directly (not through `ddl`) with `dbschema.CollectionDef{Name: "ext/datatug/projects"}`, with `dbschema.CollectionDef{Name: "ext/datatug"}`, and with `dbschema.CollectionDef{Name: "users"}` plus `ddl.WithExtension(ext)` for an extension `ext` the driver does not honour
**Then** `SupportsSubCollections()` is `false`; the first and third calls return `*dbschema.NotSupportedError`; the second returns an error satisfying `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; and no `projects`, `ext/datatug` or `users` table exists afterwards.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:schema-path-scopes, REQ:string-and-structured-forms-round-trip, REQ:extension-addressing, REQ:in-org-drivers-comply)

Consumer conformance statement. The implementation, and the tests that run
this AC and the three below, belong to
[ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16),
including where the driver stores a scoped definition. This Feature only
guarantees that the API makes them expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, which reports `ddl.SupportsSubCollections(db) == true`, and an extension type exported by `dalgo2ingitdb` that carries an `ingitdb.RecordFileDef`, returns `dalgo2ingitdb.ExtensionTarget`, and is honoured by the driver's `SupportsExtension("CreateCollection", ext)`
**When** the caller runs, in order,
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext", Fields: fx})`,
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>))`
and
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/queries", Fields: f}, ddl.WithExtension(<record file: name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>))`
**Then** all three calls return `nil`; no `ext` record with id `datatug` is required or created; the `ext/datatug/projects` and `ext/datatug/projects/{projectID}/queries` definitions are persisted with exactly the `record_file` values of their extensions (`name`, `format: json`, `type: map[string]any`, `records_dir: '.'`) and with their scope `datatug`; no `{key}.yaml` default record file is written for either; a record written at key `ext/datatug/projects/p1/queries/q1` is stored under the `.query.json` layout; the same result is produced when the third call uses `Name: "queries", Parent: []dbschema.ParentStep{{"ext", dbschema.ID("datatug")}, {"projects", dbschema.Placeholder("projectID")}}` instead; and `db.CreateCollection(ctx, dbschema.CollectionDef{Name: "ext/datatug"})` called directly fails with `errors.Is(err, dbschema.ErrRecordPathNotCollection)`.

### AC: dalgo2ingitdb-scopes-are-independent (verifies REQ:schema-path-scopes)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/sneat/projects", Fields: g})` is called, and then `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/{extID}/projects", Fields: g})`
**Then** the first returns `nil` and does not change the `ext/datatug/projects` definition; the second returns a non-nil error because it overlaps both scoped definitions, and creates nothing.

### AC: dalgo2ingitdb-depth-two-ancestor-required (verifies REQ:one-collection-per-create)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with no `ext/datatug/projects/{projectID}/environments` definition
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/environments/{envID}/servers"}, ddl.WithExtension(<record file '{key}/{key}.server.json', json, map[string]any, '.'>))` is called
**Then** it returns a non-nil error, and no `servers` definition is persisted.

### AC: dalgo2ingitdb-depth-two-created-and-dropped (verifies REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries with `ext/datatug/projects/{projectID}/dbdrivers` also created (record file `'{key}/{key}.dbdriver.json'`), and no `dbservers` records
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers"}, ddl.WithExtension(<record file '{key}/{key}.dbserver.json', json, map[string]any, '.'>))` is called, followed by `ddl.AlterCollection(ctx, db, "ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers", ddl.AddField(fd))` and `ddl.DropCollection(ctx, db, "/ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers")`
**Then** the create returns `nil` and persists the `dbservers` definition with that `record_file`; the alter returns `nil` and adds the field to that same definition; the drop, given no flags, returns `nil` because `dbservers` has no records and no descendants, and removes that definition while the `dbdrivers` definition remains.

### AC: drop-flags-guarded-by-helpers (verifies REQ:drop-flags, REQ:guard-precedence)

**Given** a stub driver implementing `SchemaModifier` whose `SupportsDrop` returns `false` for `DropFlags{DeleteRecords: true}` and for `DropFlags{DeleteNested: true}`, which records every call
**When** `ddl.DropCollection(ctx, db, "users", ddl.DeleteRecords())`, `ddl.DropCollection(ctx, db, "users", ddl.DeleteNested())`, `ddl.DropCollection(ctx, db, "users", ddl.DeleteDependents())` and `ddl.DropCollection(ctx, db, "ext/data,tug/projects", ddl.DeleteDependents())` are called
**Then** the first two return `*dbschema.NotSupportedError` naming the flag; the third returns `*dbschema.NotSupportedError` with `Reason` `"DeleteDependents is reserved"`, even though the stub supports nothing else either; the fourth returns `dbschema.ErrInvalidCollectionPath` (path first); and the stub's `DropCollection` is never invoked. `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "users"}, ddl.DeleteRecords())` dispatches normally.

### AC: collection-not-empty-error-shape (verifies REQ:drop-refuses-data-loss-by-default)

**Given** `err := &ddl.CollectionNotEmptyError{Path: p, Records: 3, SubCollections: []dbschema.SchemaPath{q}}`
**When** it is checked with `errors.Is(err, ddl.ErrCollectionNotEmpty)` and `err.Error()` is read
**Then** `errors.Is` is `true`, and the message contains `p.String()`, `3` and `q.String()`.

### AC: dalgo2ingitdb-drop-empty-without-flags (verifies REQ:drop-refuses-data-loss-by-default)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16, as are the three ACs below.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with no records
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/{projectID}/queries")` is called with no flags, and then `ddl.DropCollection(ctx, db, "users")` for an empty root collection `users` created beforehand
**Then** both return `nil` and remove only those definitions.

### AC: dalgo2ingitdb-drop-non-empty-refused (verifies REQ:drop-refuses-data-loss-by-default)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with records `ext/datatug/projects/p1` and `ext/datatug/projects/p1/queries/q1`
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects")` is called with no flags
**Then** it returns an error satisfying `errors.Is(err, ddl.ErrCollectionNotEmpty)` whose `*ddl.CollectionNotEmptyError` has `Records >= 1` (or `MoreRecords`) and `SubCollections` containing `ext/datatug/projects/{projectID}/queries`; and every definition and record is still present.

### AC: dalgo2ingitdb-delete-records-across-parents (verifies REQ:drop-flags, REQ:drop-refuses-data-loss-by-default)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with records `ext/datatug/projects/p1/queries/q1`, `ext/datatug/projects/p2/queries/q2` and the two project records
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/{projectID}/queries")` is called first with no flags and then with `ddl.DeleteRecords()`
**Then** the first returns `ErrCollectionNotEmpty` with `Records == 2` (or `MoreRecords`) and deletes nothing; the second returns `nil`, removes the `queries` definition and both `q1` and `q2`, and leaves the `p1` and `p2` project records untouched.

### AC: dalgo2ingitdb-delete-nested-removes-descendants (verifies REQ:drop-flags)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with `ext/datatug/projects/{projectID}/environments` and `ext/datatug/projects/{projectID}/environments/{envID}/servers` also created, and no records anywhere
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/{projectID}/environments")` is called first with no flags and then with `ddl.DeleteNested()`
**Then** the first returns `ErrCollectionNotEmpty` with `Records == 0` and `SubCollections` containing the `servers` path, and deletes nothing; the second returns `nil` and removes both the `environments` and `servers` definitions, while `queries` and `projects` remain.

## Architecture

| File | Change |
|---|---|
| `github.com/dal-go/record` `key.go` | `EscapeID` also escapes `{` `}` `,` `=`; new `UnescapeID`; constructors and `Key.Validate` apply `ValidateStringID`; the path grammar (segment splitting, parity, id-segment classification, placeholder identifier) is defined here once. |
| `access/resource.go` | Capture names use the `record` placeholder identifier rule. |
| `dbschema/schema_path.go` | New: `PathID`, `ID`, `Placeholder`, `ParentStep`, `SchemaPath` with `String`, `Validate`, `SameCollection`, `Overlaps`; `ParseSchemaPath` (built on the `record` grammar); the three path errors. |
| `dbschema/collection_def.go` | Add `Parent []ParentStep`, `SchemaPath()`, `Normalize()`; godoc for nesting, scopes and the two forms. |
| `ddl/modifier.go` | `SchemaModifier` embeds `SubCollectionsAware` and `ExtensionAware`; `DropCapable`; `DropCollection` and `AlterCollection` take `dbschema.SchemaPath`. |
| `ddl/options.go` | Add `Options.Extensions`, `WithExtension`, the drop flags and their options; godoc states the strict rule next to the mismatched-option rule. |
| `ddl/drop.go` | New: `DropCapable`, `DropFlags`, `ErrCollectionNotEmpty`, `CollectionNotEmptyError`. |
| `ddl/extension.go` | New: `Extension`, `ExtensionAware`, `ErrInvalidExtension`, and the internal addressing check. |
| `ddl/subcollections.go` | New: `SubCollectionsAware`, `SupportsSubCollections`. |
| `ddl/alter_op.go` | Sealed `AlterOp` gains unexported `opts() Options`; each of the six op types returns its stored field. |
| `ddl/operations.go` | All helpers apply REQ:guard-precedence and normalise paths; new `DropCollectionAt` and `AlterCollectionAt`; `DropCollection` / `AlterCollection` parse a schema path string. |
| `mocks/mock_ddl/schema_modifier.go` | Regenerated for the changed interface. |
| `spec/features/ddl/`, `spec/features/ddl/schema-modifier/`, `spec/features/ddl/options/`, `spec/features/dbschema/collection-def/` | Updated to the changed interface once this Feature is Approved. |

## Error Handling and Failure Modes

| Failure mode | Result |
|---|---|
| Malformed path (empty segment, trailing `/`, forbidden character in a name, bad escape in an id, malformed or duplicate `{placeholder}`, empty id position in structured form) | error, `errors.Is(err, dbschema.ErrInvalidCollectionPath)`; no dispatch |
| Even segment count where a collection is expected | also `errors.Is(err, dbschema.ErrRecordPathNotCollection)`; no dispatch |
| Composite key segment `{field=value,…}` | `errors.Is(err, dbschema.ErrReservedKeySegment)` and `errors.Is(err, dal.ErrNotSupported)`; no dispatch |
| Both a slash-path `Name` and a non-empty `Parent` | `dbschema.ErrInvalidCollectionPath`; no dispatch |
| New definition `Overlaps` an existing one (placeholder versus concrete id at the same position) | driver-specific non-nil error; nothing created |
| Placeholder name for an existing ancestor differs from the one a nesting driver stored | driver-specific non-nil error; nothing created |
| Extension with empty target | error, `errors.Is(err, ddl.ErrInvalidExtension)`; no dispatch |
| Extension the driver does not honour, on any operation or `AlterOp`, whatever its target | `*dbschema.NotSupportedError` naming type and target; no operation performed |
| DB does not implement `SchemaModifier` (for example a store whose schema is defined elsewhere) | existing `*dbschema.NotSupportedError`; no dispatch |
| Nested path or `Parent`, driver lacks nesting | `*dbschema.NotSupportedError`; no dispatch |
| Drop would delete records, descendant definitions or orphan referencing records without the matching flag | `*ddl.CollectionNotEmptyError` (`errors.Is(err, ddl.ErrCollectionNotEmpty)`) listing counts and paths; nothing removed |
| `DeleteDependents()` on any drop | `*dbschema.NotSupportedError` (reserved); no dispatch |
| `DeleteRecords()` / `DeleteNested()` on a driver whose `SupportsDrop` is `false` for it | `*dbschema.NotSupportedError` naming the flag; no dispatch |
| An ancestor collection definition does not exist (nesting driver) | driver-specific non-nil error; nothing created. A missing scoping parent **record** is not an error. |
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
- **Record data under a scoping parent.** A scoped definition does not create,
  require or validate the scoping parent record (`ext/datatug`).
- **Changing `record.Key.CollectionPath()`**, which emits collection names
  joined by `/` for other callers. Schema paths use `SchemaPath.String()`.
- **A `CollectionPath` names-only type.** Superseded by `SchemaPath`, because
  ids in a schema path can be concrete.
- **Concrete extension types in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Implementing module changes in this repo.** The work in
  REQ:in-org-drivers-comply is tracked on dal-go/dalgo#163. dalgo2ingitdb's
  extension type, target constant, nesting, depth-N ancestor check, path-based
  Drop/Alter and conforming its slash parsing to the grammar are
  ingitdb/dalgo2ingitdb#16.
- **A foreign-key and cascade model.** A driver-agnostic declaration of
  references with cascade actions, which `DeleteDependents()` needs, is a later
  Feature. Until then the flag is reserved and refused.
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

- **Enforcing one placeholder name per collection.** REQ:schema-path-scopes
  requires the same name wherever a collection definition appears (e.g. always
  `{projectID}` under `ext/datatug/projects`). A nesting driver that persists names refuses
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

---
*This document follows the https://specscore.md/feature-specification*
