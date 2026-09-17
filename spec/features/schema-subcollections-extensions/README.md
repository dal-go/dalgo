---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Schema API: subcollection declaration and driver options for CreateCollection

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/schema-subcollections-extensions?op=request-change) |
**Status:** Draft
**Date:** 2026-09-17 (reworked 2026-09-17: minimal interface, functional options)
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
   A driver that cannot nest refuses a nested path itself with an error
   matching `dal.ErrNotSupported`; there is no capability method.
2. **Driver-specific settings as functional options.** `ddl.Options` gains a
   generic, vocabulary-free value carrier, so a driver package defines its own
   options without dalgo knowing them: `dalgo2ingitdb.WithRecordFile(…)` returns
   a `ddl.Option` built with `ddl.WithValue(key, value, required)`, and
   dalgo2ingitdb reads the value back with `Options.Value(key)`. Each option
   declares whether it is **required** (a driver that does not support it
   refuses the call with an error matching `dal.ErrNotSupported`) or a **hint**
   (ignored by a driver that does not support it).

3. **Safe drops.** A drop through a path with a `{placeholder}` is a schema
   drop: it removes only the definition, and refuses with
   `ddl.ErrCollectionNotEmpty` while any instance under any parent record holds
   data; no option makes it delete data across parents. A drop through a fully
   concrete path such as `ext/datatug/projects/p1/queries` is an instance
   operation on that one parent's data and never touches the definition.
   `ddl.DeleteRecords()` and `ddl.DeleteNested()`, ordinary required options,
   authorise data loss only for a single instance. `ddl.DeleteDependents()` is
   reserved.

`SchemaModifier` stays minimal: `CreateCollection`, `DropCollection` and
`AlterCollection`, nothing else. The `ddl` helpers validate paths; every other
refusal is the driver's, and a shared conformance suite (`ddl/ddltest`) proves
each in-org driver implements it (REQ:in-org-drivers-comply).

Founder direction for this shape (2026-09-17): "Drop it. We should have minimal
generic interface and if we need to pass additional options we should use
functional args and they should be either ignored or returning not supported
error if not supported."

DALgo is in private beta, so this Feature changes the `SchemaModifier` interface
and `record.EscapeID` rather than working around them (founder direction,
2026-09-17: "Don't worry too much about what already exists, we are still in
private beta stage and can do changes.").

The concrete scope this Feature unblocks is the DataTug project store's
collection schema on the local inGitDB path (see REQ:collection-def-parent for
the list). It does not cover nested database roots or OpenVaultDB-backed stores
(see Not Doing).

DALgo core stays driver-agnostic: it defines only the path grammar, the option
carrier and the refusal rule. Concrete options, such as an inGitDB
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
collections, namespaced under `ext/datatug` as DataTug already keeps them in
Firestore (`ext/datatug/projects/<id>/…`; founder direction, 2026-09-17: the same
layout in git storage, for consistency and so several apps' data can share one
store without conflicts), each with `record_file` `name: '{key}/{key}.<suffix>.json'`,
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
- **Minimal interface.** `SchemaModifier` has three methods and no capability
  methods. Anything extra travels as a functional option.
- **No silent loss of what matters.** A nested path on a flat driver, or a
  required option the driver does not support, is an error matching
  `dal.ErrNotSupported`, never a quiet default. That silent default is the exact
  defect behind dalgo2ingitdb#16. Only options declared as hints may be ignored.
- **Proven by one shared suite.** Driver obligations are verified by the same
  conformance tests in every in-org driver, not advertised by the driver.

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
  characters, `{` `}` `,` `=` `\` become `%7B` `%7D` `%2C` `%3D` `%5C` (`\` so a
  backslash can never be read as a path separator on Windows). The table is exact-case:
  `UnescapeID` accepts upper-case codes only and rejects raw escapable characters. Every escaped id
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
than with its own splitting or escaping rules. `record` exports the primitives (`SplitPath`,
`PathAddressesCollection`, `ValidateCollectionName`, `ClassifyIDSegment`, `ValidPlaceholderName`);
whole-path composition, including placeholder-name uniqueness within one path, is owned by
`dbschema.ParseSchemaPath` / `SchemaPath.Validate`.

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
  (`key.go:54-58`), so logging any key, incomplete or invalid, is safe. An
  incomplete key stringifies the same however it was built: for an id that is
  `nil` (as `NewIncompleteKey` leaves it, which today prints `users/<nil>`) or an
  empty string, `String()` MUST emit an empty id segment (`users/`). That output
  is not a valid path under REQ:path-grammar and MUST NOT be parsed.
- `Key.Validate()` (`key.go:130`) checks **structure and characters, not
  completeness**: it MUST apply the `%` check to non-empty string ids at every
  level, and MUST return `nil` for an otherwise valid incomplete key (`nil` or
  empty id), so keys can be validated before an id is assigned. Completeness is
  checked at **write time**: the write path (driver or id generator) MUST reject
  a key that is still incomplete when the record is persisted.

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
  the subcollection under that **one** parent record. This is how apps
  namespace their data (`ext/datatug/…`, `ext/<other>/…`) without conflicts,
  matching DataTug's Firestore layout.

Rules for drivers that support nesting:

- A driver MUST keep the scope: a definition created at
  `ext/datatug/projects` MUST be found by `SameCollection` at that path, and
  MUST NOT apply to records under `ext/other/projects`. Where and how the driver
  stores the scope is the driver's choice (for `dalgo2ingitdb`,
  ingitdb/dalgo2ingitdb#16, which depends on REQ:ingitdb-format-dependency).
- Scoped definitions under different concrete ids are independent:
  `ext/datatug/projects` and `ext/sneat/projects` MAY both exist, with
  different fields and options.
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
uniqueness within one path. Founder decision, 2026-09-17 ("A"): enforcement is
mandatory, with no optional path:

- Every driver that supports nesting MUST persist the
  placeholder name of every placeholder id position of every definition it
  creates. A driver with no storage for names MUST add it; it MUST NOT skip the
  check.
- On `CreateCollection` it MUST refuse, removing and creating nothing, a
  declaration whose placeholder name for an existing collection definition
  differs from the stored one, with an error satisfying
  `errors.Is(err, dbschema.ErrPlaceholderNameConflict)`. The error message MUST
  name the collection path, the stored name and the declared name.
- Addressing ignores names (`SameCollection`), so Drop and Alter accept any
  placeholder name.

`dbschema` MUST export
`ErrPlaceholderNameConflict = errors.New("dbschema: placeholder name conflicts with the stored name")`.

Rationale: typed-key and code generation tooling maps `{projectID}` to one key
field, so one name per collection keeps generated code stable.

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
one addressed by the normalised `c.SchemaPath()`. A driver that supports
nesting MUST return a non-nil error, and MUST NOT
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

`SchemaModifier` MUST have exactly these three methods: no capability methods
are embedded or required. A driver that supports nesting MUST resolve `path` with
`SameCollection` to the definition that `CreateCollection` created, and MUST
treat a scoped path and a placeholder path as different definitions. Dropping a
collection that has subcollections is governed by REQ:drop-refuses-data-loss-by-default.

#### REQ: drop-refuses-data-loss-by-default

Founder decisions, 2026-09-17: "1-A. We probably should have explicit flags like
delete nested and delete recursive dependents?", and, correcting an earlier
draft that deleted records under every parent record, "Yes. You got it I
think." to the rule below.

A drop (`DropCollection`, through any helper or directly) is one of three kinds,
decided from its `SchemaPath`:

| Kind | Path | Removes | Data flags |
|---|---|---|---|
| **Schema drop** | has at least one `{placeholder}` (e.g. `ext/datatug/projects/{projectID}/queries`) | the definition only | refused: an error satisfying `errors.Is(err, ddl.ErrInvalidOption)` |
| **Single-instance definition drop** | no placeholder, and a definition exists at exactly this path: a root collection (`users`) or a concrete-scoped definition (`ext/datatug/projects`) | the definition, plus its one instance's data if the flags allow | allowed |
| **Instance drop** | no placeholder, no definition at exactly this path, and a placeholder definition covers it (e.g. `ext/datatug/projects/p1/queries` under `ext/datatug/projects/{projectID}/queries`) | data of that one instance only; the definition MUST stay | allowed |

The three kinds cannot be confused: REQ:schema-path-scopes forbids overlapping
definitions, so a fully concrete path matches either a definition at exactly
that path or a covering placeholder definition, never both. A fully concrete
path that matches neither is not found (a no-op under `IfExists()`).

By default a drop MUST delete no data. If the drop would delete anything beyond
what its kind removes without flags, it MUST remove nothing and return
`*ddl.CollectionNotEmptyError`:

```go
var (
    ErrCollectionNotEmpty = errors.New("ddl: collection is not empty")
    ErrInvalidOption      = errors.New("ddl: option is not valid for this call")
)

type CollectionNotEmptyError struct {
    Path           dbschema.SchemaPath   // the path asked to drop
    Records        int                   // records of the addressed collection found
    MoreRecords    bool                  // true if the driver stopped counting early
    NestedRecords  int                   // records of descendant subcollections found
    SubCollections []dbschema.SchemaPath // descendant definitions (definition drops only)
    Referrers      []dbschema.Referrer   // collections whose records reference records to be deleted
}

func (e *CollectionNotEmptyError) Error() string
func (e *CollectionNotEmptyError) Is(target error) bool // true for ErrCollectionNotEmpty
```

Blockers, each reported in its field when present:

1. **records** of the addressed collection: for a schema drop, in any instance
   under any parent record (always blocks); for the other two kinds, in the one
   instance, unless `DeleteRecords()` is given;
2. **nested data** (records of descendant subcollections): for a schema drop,
   under any parent record (always blocks); for the other two kinds, under the
   one instance's records, unless `DeleteNested()` is given;
3. **descendant definitions**, for the two definition-drop kinds (always block:
   drop descendants first, deepest first);
4. **referencing records** in other collections that reference records to be
   deleted, where the driver knows such references (for example
   `dalgo2ingitdb`'s foreign keys, `foreign_keys.go:102`, or an SQL foreign
   key). `DeleteDependents()` is reserved (REQ:drop-flags), so this blocker
   cannot be lifted today.

A driver MAY stop counting records after finding one and set `MoreRecords`.

This supersedes the current root `DropCollection` in `dalgo2ingitdb`, which
removes the whole collection directory, records included (`os.RemoveAll`,
`schema_modifier.go:122`). A root drop becomes a single-instance definition
drop, which deletes records only with `DeleteRecords()` and nested data only
with `DeleteNested()`, and refuses while descendant definitions exist, in the
scope of ingitdb/dalgo2ingitdb#16.

#### REQ: drop-flags

The `ddl` package MUST export the drop flags as ordinary **required** options on
the generic carrier of REQ:option-values:

```go
func DeleteRecords() Option    // delete the single instance's records
func DeleteNested() Option     // delete the single instance's nested subcollection data
func DeleteDependents() Option // reserved: cascade to referencing records

var (
    DeleteRecordsKey    = deleteRecordsKey{}    // keys, so drivers can list them as supported
    DeleteNestedKey     = deleteNestedKey{}
    DeleteDependentsKey = deleteDependentsKey{}
)
```

- `DeleteRecords()` allows deleting the records of the addressed collection in
  its **one** instance: the root collection, the concrete-scoped definition, or
  the concrete instance path. It never deletes records under other parents.
- `DeleteNested()` allows deleting records of descendant subcollections under
  that one instance's records. It never removes a definition. It is
  **independent** of `DeleteRecords()`, so each flag authorises exactly one
  kind of loss and the error names precisely which one is missing.
- Either flag with a path containing a `{placeholder}` MUST be refused with an
  error satisfying `errors.Is(err, ddl.ErrInvalidOption)`, by the drop helpers
  before dispatch and by drivers on direct calls: no option makes a schema drop
  delete data across parents.
- A driver that cannot execute a flag (for example `DeleteNested()` on a driver
  without nesting) does not list its key as supported, so the flag is refused
  with an error matching `dal.ErrNotSupported` under REQ:unsupported-options.
  Being required, a flag passed to `CreateCollection` or an `AlterOp` is refused
  the same way.
- `DeleteDependents()` is **reserved**. DALgo has no driver-agnostic
  foreign-key declaration or cascade model today: `dbschema.ConstraintDef`
  carries only `Name` and `Type` and defers "foreign-key target + cascade
  actions" (`dbschema/constraint.go:3-9`), and `dbschema.Referrer`
  (`dbschema/referrer.go`) is read-side introspection through the optional
  `SchemaReader.ListReferrers`. No driver may list `DeleteDependentsKey` as
  supported, and every drop helper (`ddl.DropCollection` and
  `ddl.DropCollectionAt`) MUST refuse it with
  `*dbschema.NotSupportedError{Reason: "DeleteDependents is reserved"}`.
  A drop that would orphan referencing records refuses with blocker 4.

#### REQ: nesting-refused-by-flat-drivers

A driver that does not support nesting MUST itself refuse, on `CreateCollection`
with a non-empty `Parent` (after `Normalize`) and on `DropCollection` /
`AlterCollection` with a non-empty `path.Parent`, with
`*dbschema.NotSupportedError{Op, Backend, Reason: "subcollections are not supported"}`,
which matches `dal.ErrNotSupported`. It MUST NOT create, drop or alter anything.
There is no capability method; the `ddl` helpers dispatch valid nested paths to
every driver, and AC:flat-in-org-drivers-conformance proves each flat driver
refuses.

### Driver options

#### REQ: option-values

`ddl.Options` MUST gain a generic value carrier with no domain vocabulary. The
existing `IfNotExists` and `IfExists` fields and the `Option func(*Options)`
type stay:

```go
// WithValue returns an Option that stores value under key. key MUST be
// comparable; like a context key, it SHOULD be a value of an unexported type
// of the package that defines the option. required declares what a driver
// that does not support the option does: refuse the call (true) or ignore
// the option (false).
func WithValue(key, value any, required bool) Option

// Value returns the value stored under key by the last WithValue for it.
func (o Options) Value(key any) (value any, ok bool)

// Unsupported returns the keys of required options that are not in
// supported, in the order they were first set.
func (o Options) Unsupported(supported ...any) []any

// RefuseUnsupported returns nil when Unsupported(supported...) is empty, and
// otherwise *dbschema.NotSupportedError{Op: op, Reason: <the key types>}.
func RefuseUnsupported(op string, o Options, supported ...any) error
```

- `WithValue` MUST panic on a non-comparable key, as `context.WithValue` does.
- Setting the same key again replaces its value and its `required` flag; the
  key keeps its first position.
- The carrier is unexported state of `Options`, so existing keyed literals of
  `Options` keep compiling.

A driver package defines an option with a key type only it can construct, for
example:

```go
package dalgo2ingitdb

type recordFileKey struct{}

// WithRecordFile sets the inGitDB record_file of the created collection.
// Required: a driver that does not support it refuses the call.
func WithRecordFile(def ingitdb.RecordFileDef) ddl.Option {
    return ddl.WithValue(recordFileKey{}, def, true)
}
```

and reads it with `opts.Value(recordFileKey{})`.

#### REQ: unsupported-options

Every driver method that receives options (`CreateCollection`, `DropCollection`,
and each `Applier` method for an `AlterOp`) MUST, before changing anything:

- call `ddl.RefuseUnsupported(op, opts, <the keys it supports for this op>...)`,
  or perform the equivalent check, and return its error. A required option it
  does not support, including one from another package, is refused with an
  error matching `dal.ErrNotSupported`;
- ignore a hint (non-required) option it does not support.

Each option's documentation MUST state whether it is required or a hint and why.
A correctness-critical option, such as `dalgo2ingitdb.WithRecordFile`, MUST be
required; an option whose absence changes only performance or presentation MAY
be a hint.

`IfNotExists` and `IfExists` remain hints under their existing mismatched-option
rule (`ddl/options.go:10-16`).

Recommendation, over "a driver refuses options it recognises but cannot apply
and ignores options from other packages": under that rule a driver that has
never heard of `dalgo2ingitdb.WithRecordFile` ignores it, so a SQL or
OpenVaultDB driver would silently drop a record layout, which is the defect this
Feature exists to remove. With the declaration carried by the option, every
driver, including one written before the option existed, reaches the right
decision with one generic check and no knowledge of other packages.

#### REQ: guard-precedence

Every `ddl` helper MUST check in this order and return the first failure:

1. **Path validity.** `CreateCollection`: `c.Normalize()`. String helpers:
   `ParseSchemaPath`. `*At` helpers: `path.Validate()`. Failures are the
   path errors of REQ:path-grammar and REQ:schema-path-type.
2. **Reserved and invalid options.** On drops, `DeleteDependents()`, then
   `DeleteRecords()` or `DeleteNested()` with a path containing a placeholder
   (`ddl.ErrInvalidOption`) (REQ:drop-flags).
3. **Interface.** The DB implements `SchemaModifier`, otherwise the existing
   `*dbschema.NotSupportedError` ("driver does not implement
   ddl.SchemaModifier", `ddl/operations.go:25-31`).

The helpers perform no other checks: they do not inspect options, nesting
support or `AlterOp` contents. Unsupported options, nesting refusals and
data-dependent refusals (`CollectionNotEmptyError`, missing ancestors,
overlaps) come from the driver after dispatch.

### Driver obligations

#### REQ: helpers-are-the-guarantee

The `ddl` helpers are the designed entry point for path handling: they refuse
malformed and record paths, the reserved `DeleteDependents()`, data flags on a
schema drop, and a DB without
`SchemaModifier` before dispatch, so nothing is skipped silently at that level.
Every other refusal (a nested path on a flat driver, an unsupported required
option, data-dependent refusals) is decided by the driver and verified by the
shared conformance suite of REQ:shared-conformance-suite.

#### REQ: shared-conformance-suite

`dalgo` MUST ship a test-only package `ddl/ddltest` with

```go
type Config struct {
    Nesting       bool // the driver supports subcollections
    DeleteRecords bool // the driver supports DeleteRecords()
    DeleteNested  bool // the driver supports DeleteNested(); requires Nesting
}

func RunConformance(t *testing.T, newDB func(t *testing.T) dal.DB, cfg Config)
```

`Config` describes what the test expects of the driver under test; it is not an
API the driver exposes. `RunConformance` MUST cover, by calling the driver's
methods directly: REQ:nesting-refused-by-flat-drivers when `Nesting` is false;
REQ:unsupported-options for an unknown required option (refused) and an unknown
hint (ignored) on `CreateCollection`, `DropCollection` and an `AlterOp`;
REQ:drop-refuses-data-loss-by-default for an empty and a non-empty collection;
each drop flag according to `Config`, including an instance drop that leaves
the definition in place when `Nesting` is true; a data flag on a placeholder
path refused with `ErrInvalidOption`; and a refused `DeleteDependents()`. Every
in-org driver MUST run it in CI.

#### REQ: in-org-drivers-comply

Every driver in the founder's organisations (`dal-go`, `ingitdb`, `openvaultdb`,
`datatug`, `sneat-dev`, `synchestra-io`) that implements `SchemaModifier` MUST
implement the changed interface and enforce, on direct calls, the driver-side
MUSTs of REQ:string-and-structured-forms-round-trip, REQ:drop-refuses-data-loss-by-default, REQ:drop-flags,
REQ:schema-path-scopes, REQ:one-collection-per-create,
REQ:path-addressed-drop-and-alter, REQ:nesting-refused-by-flat-drivers and
REQ:unsupported-options, and run REQ:shared-conformance-suite. There is no
legacy exemption. As of 2026-09-17 these are:

| Module | `SchemaModifier` today (origin/main) | Required change | `ddltest.Config` |
|---|---|---|---|
| `dal-go/dalgo2sqlite` | `schema_modifier.go:16`, `:89`, `:106` | Adopt the `SchemaPath` signatures; refuse `Parent`; refuse unsupported required options; refuse non-empty drops without `DeleteRecords()` | `{DeleteRecords: true}` |
| `dal-go/dalgo2postgres` | `schema_modifier.go:15`, `:62`, `:75` | Same as dalgo2sqlite | `{DeleteRecords: true}` |
| `dal-go/dalgo2mysql` | `schema_modifier.go:27`, `:58`, `:69` | Same as dalgo2sqlite | `{DeleteRecords: true}` |
| `ingitdb/dalgo2ingitdb` | `schema_modifier.go:42`, `:102`, `:137` | Nesting, scopes, placeholder names, `WithRecordFile`, safe drops of all three kinds; its slash parsing is kept but conformed to REQ:path-grammar via `Normalize` (ingitdb/dalgo2ingitdb#16) | `{Nesting: true, DeleteRecords: true, DeleteNested: true}` |
| `dal-go/dalgo` `mocks/mock_ddl` | generated by MockGen | Regenerated for the three-method interface | n/a |

`dal-go/record` is not a driver, but it carries the grammar change
(`EscapeID` change, new `UnescapeID`, placeholder identifier rule) of
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

**Given** a stub driver implementing the three `SchemaModifier` methods that records every call
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
**Then** the two empty-id constructors return keys without panicking; `k.String()` returns `"users/"` without panicking; `k.Validate()` returns `nil`; `record.NewIncompleteKey("users", reflect.String, nil).String()` also returns `"users/"` and its `Validate()` returns `nil`; `NewKeyWithID("users", "a%2Fb")` panics with a message naming `record.ErrInvalidStringID`; `NewKeyWithOptions` returns an error satisfying `errors.Is(err, record.ErrInvalidStringID)`; and `String()` on a key assembled with an invalid id also returns without panicking.

### AC: invalid-path-rejected-before-dispatch (verifies REQ:guard-precedence)

**Given** the recording stub of AC:malformed-and-record-paths-rejected
**When** `ddl.CreateCollection` is called with `Parent: []ParentStep{{"..", ID("x")}}, Name: "queries"`, and `ddl.DropCollectionAt` and `ddl.AlterCollectionAt` are called with `SchemaPath{Parent: []ParentStep{{"projects", PathID{}}}, Name: "queries"}` and with the zero `SchemaPath{}`
**Then** each call returns an error satisfying `errors.Is(err, dbschema.ErrInvalidCollectionPath)` and the stub records no call.

### AC: helpers-dispatch-nested-paths (verifies REQ:guard-precedence, REQ:string-and-structured-forms-round-trip)

**Given** a stub driver implementing only the three `SchemaModifier` methods, which records every call
**When** `ddl.CreateCollection` is called with `CollectionDef{Name: "/ext/datatug/projects/{projectID}/queries"}` and an option `o := ddl.WithValue(k, 1, true)` for a key `k` the stub does not know
**Then** the helper returns the stub's result without any check of its own, and the stub receives a `CollectionDef` with `Name == "queries"` and `Parent == []ParentStep{{"ext", ID("datatug")}, {"projects", Placeholder("projectID")}}`, and options whose `Value(k)` is `1`.

### AC: drop-and-alter-dispatch-paths (verifies REQ:path-addressed-drop-and-alter)

**Given** the recording stub of AC:helpers-dispatch-nested-paths
**When** `ddl.DropCollection(ctx, db, "/ext/datatug/projects/{projectID}/environments/{envID}/servers", ddl.IfExists())`, `ddl.AlterCollection(ctx, db, "ext/datatug/projects/{pid}/queries", op)` and `ddl.DropCollection(ctx, db, "users")` are called
**Then** the stub's `DropCollection` receives `SchemaPath{Parent: [{ext, ID("datatug")}, {projects, Placeholder("projectID")}, {environments, Placeholder("envID")}], Name: "servers"}` with `IfExists` set, then `SchemaPath{Name: "users"}`; and its `AlterCollection` receives a path for which `SameCollection` with `ParseSchemaPath("ext/datatug/projects/{projectID}/queries")` is `true`, with `op`.

### AC: schema-modifier-shape (verifies REQ:path-addressed-drop-and-alter)

**Given** a Go test file declaring `var _ ddl.SchemaModifier = (*stub)(nil)`
**When** `stub` has exactly `CreateCollection`, `DropCollection` and `AlterCollection` in the shape of REQ:path-addressed-drop-and-alter, and, separately, when `stub` declares `DropCollection(ctx, name string, ...)` instead
**Then** the first compiles with no further methods, and the second does not compile.

### AC: no-schema-modifier-is-typed-error (verifies REQ:helpers-are-the-guarantee, REQ:guard-precedence)

**Given** a stub `dal.DB` that does not implement `SchemaModifier` (as for a store whose schema is defined elsewhere)
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/queries"}, ddl.WithValue(k, 1, true))` is called
**Then** it returns `*dbschema.NotSupportedError` with `Reason` `"driver does not implement ddl.SchemaModifier"`.

### AC: option-values-carried (verifies REQ:option-values)

**Given** two key types `k1`, `k2` defined in a test package
**When** `o := ddl.ResolveOptions(ddl.WithValue(k1{}, "a", true), ddl.IfNotExists(), ddl.WithValue(k2{}, 2, false), ddl.WithValue(k1{}, "b", false))` is evaluated, and `ddl.WithValue([]int{1}, 1, true)` is called
**Then** `o.Value(k1{})` is `("b", true)`, `o.Value(k2{})` is `(2, true)`, a third key gives `(nil, false)`, `o.IfNotExists` is `true`, `o.Unsupported()` is empty because `k1` was re-set as a hint, and the non-comparable key panics.

### AC: unsupported-required-options-listed (verifies REQ:option-values, REQ:unsupported-options)

**Given** `o := ddl.ResolveOptions(ddl.WithValue(k1{}, 1, true), ddl.WithValue(k2{}, 2, true), ddl.WithValue(k3{}, 3, false))`
**When** `o.Unsupported(k2{})` and `ddl.RefuseUnsupported("CreateCollection", o, k2{})` and `ddl.RefuseUnsupported("CreateCollection", o, k1{}, k2{})` are called
**Then** the first returns `[]any{k1{}}` (the hint `k3` is not listed); the second returns `*dbschema.NotSupportedError` with `Op == "CreateCollection"`, a `Reason` naming the type of `k1`, and `errors.Is(err, dal.ErrNotSupported)` true; the third returns `nil`.

### AC: guard-order (verifies REQ:guard-precedence, REQ:drop-flags)

**Given** a stub `dal.DB` that does not implement `SchemaModifier`
**When** `ddl.DropCollection` is called with `"ext/datatug"` and `ddl.DeleteDependents()`; then with `"ext/datatug/projects"` and `ddl.DeleteDependents()`; then with `"ext/datatug/projects/{projectID}/queries"` and `ddl.DeleteRecords()`; then with `"ext/datatug/projects/{projectID}/queries"` and `ddl.DeleteNested()`; then with `"ext/datatug/projects/p1/queries"` and `ddl.DeleteRecords()`
**Then** the calls fail, in order, with `dbschema.ErrRecordPathNotCollection`; `*dbschema.NotSupportedError` with `Reason` `"DeleteDependents is reserved"`; an error satisfying `errors.Is(err, ddl.ErrInvalidOption)` twice; and `*dbschema.NotSupportedError` (driver does not implement `ddl.SchemaModifier`), because a data flag on a fully concrete path is valid.

### AC: flat-in-org-drivers-conformance (verifies REQ:in-org-drivers-comply, REQ:nesting-refused-by-flat-drivers, REQ:unsupported-options, REQ:shared-conformance-suite)

Driver conformance statement, verified by `ddltest.RunConformance` with the
`Config` of REQ:in-org-drivers-comply in each SQL driver's suite (`dalgo2sqlite`,
`dalgo2postgres`, `dalgo2mysql`) once it adopts this Feature.

**Given** a database value from the driver, with no collections, a required option `req := ddl.WithValue(unknownKey{}, 1, true)` and a hint `hint := ddl.WithValue(hintKey{}, 1, false)`, both from a package the driver does not know
**When** its methods are called directly (not through `ddl`): `CreateCollection` with `CollectionDef{Name: "ext/datatug/projects"}`; `CreateCollection` with `CollectionDef{Name: "users"}` plus `req`; `CreateCollection` with `CollectionDef{Name: "users"}` plus `hint`; `AlterCollection(ctx, SchemaPath{Name: "users"}, ddl.AddField(f, req))`; `DropCollection(ctx, SchemaPath{Parent: []ParentStep{{"ext", ID("datatug")}}, Name: "projects"})`; and `DropCollection(ctx, SchemaPath{Name: "users"}, ddl.DeleteNested())`
**Then** the nested create, the nested drop, the create with `req`, the alter with `req` and the drop with `DeleteNested()` each return an error matching `dal.ErrNotSupported` and change nothing; the create with `hint` returns `nil` and creates `users`; and no `projects` or `ext` table exists afterwards.

### AC: required-option-refused-hint-ignored (verifies REQ:unsupported-options, REQ:shared-conformance-suite)

Driver conformance statement for every in-org driver, through `ddltest.RunConformance`.

**Given** any in-org driver and an empty store
**When** `DropCollection` is called on an existing empty collection with an unknown required option, and again with an unknown hint
**Then** the first returns an error matching `dal.ErrNotSupported` and the collection still exists; the second returns `nil` and the collection is dropped.

### AC: dalgo2ingitdb-creates-datatug-queries (verifies REQ:collection-def-parent, REQ:one-collection-per-create, REQ:schema-path-scopes, REQ:string-and-structured-forms-round-trip, REQ:unsupported-options, REQ:in-org-drivers-comply)

Consumer conformance statement. The implementation, and the tests that run
this AC and the three below, belong to
[ingitdb/dalgo2ingitdb#16](https://github.com/ingitdb/dalgo2ingitdb/issues/16),
including where the driver stores a scoped definition. This Feature only
guarantees that the API makes them expressible.

**Given** a `dalgo2ingitdb` database on an empty project directory, and the required option `dalgo2ingitdb.WithRecordFile(ingitdb.RecordFileDef)`
**When** the caller runs, in order,
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext", Fields: fx})`,
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects", Fields: f}, dalgo2ingitdb.WithRecordFile(<name '{key}/{key}.datatug-project.json', format json, type map[string]any, records_dir '.'>))`
and
`ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/queries", Fields: f}, dalgo2ingitdb.WithRecordFile(<name '{key}/{key}.query.json', format json, type map[string]any, records_dir '.'>))`
**Then** all three calls return `nil`; no `ext` record with id `datatug` is required or created; the `ext/datatug/projects` and `ext/datatug/projects/{projectID}/queries` definitions are persisted with exactly the `record_file` values of their options (`name`, `format: json`, `type: map[string]any`, `records_dir: '.'`) and with their scope `datatug`; no `{key}.yaml` default record file is written for either; a record written at key `ext/datatug/projects/p1/queries/q1` is stored under the `.query.json` layout; the same result is produced when the third call uses `Name: "queries", Parent: []dbschema.ParentStep{{"ext", dbschema.ID("datatug")}, {"projects", dbschema.Placeholder("projectID")}}` instead; and `db.CreateCollection(ctx, dbschema.CollectionDef{Name: "ext/datatug"})` called directly fails with `errors.Is(err, dbschema.ErrRecordPathNotCollection)`.

### AC: dalgo2ingitdb-scopes-are-independent (verifies REQ:schema-path-scopes)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/sneat/projects", Fields: g})` is called, and then `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/{extID}/projects", Fields: g})`
**Then** the first returns `nil` and does not change the `ext/datatug/projects` definition; the second returns a non-nil error because it overlaps both scoped definitions, and creates nothing.

### AC: dalgo2ingitdb-placeholder-name-conflict-refused (verifies REQ:schema-path-scopes)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, where `ext/datatug/projects/{projectID}/queries` stored the placeholder name `projectID`
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{pid}/entities"})` is called, then `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/entities"})`, and then `ddl.DropCollection(ctx, db, "ext/datatug/projects/{pid}/entities")`
**Then** the first fails with `errors.Is(err, dbschema.ErrPlaceholderNameConflict)`, its message names `ext/datatug/projects`, `projectID` and `pid`, and no `entities` definition is persisted; the second returns `nil` and persists `projectID` for the new definition; and the drop returns `nil`, because addressing ignores placeholder names.

### AC: dalgo2ingitdb-depth-two-ancestor-required (verifies REQ:one-collection-per-create)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with no `ext/datatug/projects/{projectID}/environments` definition
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/environments/{envID}/servers"}, dalgo2ingitdb.WithRecordFile(<'{key}/{key}.server.json', json, map[string]any, '.'>))` is called
**Then** it returns a non-nil error, and no `servers` definition is persisted.

### AC: dalgo2ingitdb-depth-two-created-and-dropped (verifies REQ:one-collection-per-create, REQ:path-addressed-drop-and-alter)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries with `ext/datatug/projects/{projectID}/dbdrivers` also created (record file `'{key}/{key}.dbdriver.json'`), and no `dbservers` records
**When** `ddl.CreateCollection(ctx, db, dbschema.CollectionDef{Name: "ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers"}, dalgo2ingitdb.WithRecordFile(<'{key}/{key}.dbserver.json', json, map[string]any, '.'>))` is called, followed by `ddl.AlterCollection(ctx, db, "ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers", ddl.AddField(fd))` and `ddl.DropCollection(ctx, db, "/ext/datatug/projects/{projectID}/dbdrivers/{dbdriverID}/dbservers")`
**Then** the create returns `nil` and persists the `dbservers` definition with that `record_file`; the alter returns `nil` and adds the field to that same definition; the drop, given no flags, returns `nil` because `dbservers` has no records and no descendants, and removes that definition while the `dbdrivers` definition remains.

### AC: collection-not-empty-error-shape (verifies REQ:drop-refuses-data-loss-by-default)

**Given** `err := &ddl.CollectionNotEmptyError{Path: p, Records: 3, SubCollections: []dbschema.SchemaPath{q}}`
**When** it is checked with `errors.Is(err, ddl.ErrCollectionNotEmpty)` and `err.Error()` is read
**Then** `errors.Is` is `true`, and the message contains `p.String()`, `3` and `q.String()`.

### AC: dalgo2ingitdb-drop-empty-without-flags (verifies REQ:drop-refuses-data-loss-by-default)

Consumer conformance statement, owned by ingitdb/dalgo2ingitdb#16, as are the three ACs below.

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with no records
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/{projectID}/queries")` is called with no flags, and then `ddl.DropCollection(ctx, db, "users")` for an empty root collection `users` created beforehand
**Then** both return `nil` and remove only those definitions.

### AC: dalgo2ingitdb-schema-drop-refused-while-any-instance-has-data (verifies REQ:drop-refuses-data-loss-by-default, REQ:drop-flags)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with records `ext/datatug/projects/p1`, `ext/datatug/projects/p2` and `ext/datatug/projects/p2/queries/q2`
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/{projectID}/queries")` is called with no flags, then with `ddl.DeleteRecords()`, and `ddl.DropCollection(ctx, db, "ext/datatug/projects")` is called with no flags
**Then** the first returns an error satisfying `errors.Is(err, ddl.ErrCollectionNotEmpty)` with `Records >= 1` (or `MoreRecords`), because `p2` still holds a query; the second returns an error satisfying `errors.Is(err, ddl.ErrInvalidOption)`; the third returns `ErrCollectionNotEmpty` with `Records >= 1` (or `MoreRecords`), `NestedRecords >= 1` and `SubCollections` containing `ext/datatug/projects/{projectID}/queries`; and every definition and record is still present.

### AC: dalgo2ingitdb-instance-drop-deletes-one-parents-records (verifies REQ:drop-flags, REQ:drop-refuses-data-loss-by-default)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with records `ext/datatug/projects/p1/queries/q1`, `ext/datatug/projects/p2/queries/q2` and the two project records
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/p1/queries")` is called first with no flags and then with `ddl.DeleteRecords()`
**Then** the first returns `ErrCollectionNotEmpty` with `Records == 1` (or `MoreRecords`) and deletes nothing; the second returns `nil` and deletes `q1` only; `q2`, the `p1` and `p2` project records and the `ext/datatug/projects/{projectID}/queries` definition all remain.

### AC: dalgo2ingitdb-instance-drop-deletes-one-parents-nested-data (verifies REQ:drop-flags)

**Given** the `dalgo2ingitdb` database of AC:dalgo2ingitdb-creates-datatug-queries, with `ext/datatug/projects/{projectID}/environments` and `ext/datatug/projects/{projectID}/environments/{envID}/servers` also created, and records `ext/datatug/projects/p1/environments/e1`, `ext/datatug/projects/p1/environments/e1/servers/s1` and `ext/datatug/projects/p2/environments/e2/servers/s2`
**When** `ddl.DropCollection(ctx, db, "ext/datatug/projects/p1/environments", …)` is called three times: with `ddl.DeleteNested()`, with `ddl.DeleteRecords()`, and with both
**Then** the first returns `ErrCollectionNotEmpty` with `Records == 1` (or `MoreRecords`), because `e1` would be deleted without `DeleteRecords()`, and deletes nothing; the second returns `ErrCollectionNotEmpty` with `NestedRecords == 1` (or `MoreRecords`), because `s1` would be deleted without `DeleteNested()`, and deletes nothing; the third returns `nil` and deletes `e1` and `s1` only; `e2`, `s2` and both the `environments` and `servers` definitions remain.

## Architecture

| File | Change |
|---|---|
| `github.com/dal-go/record` `key.go` | `EscapeID` also escapes `{` `}` `,` `=` `\`; new `UnescapeID`; constructors and `Key.Validate` reject `%` in non-empty ids; `String()` never panics; the path grammar (segment splitting, parity, id-segment classification, placeholder identifier) is defined here once. |
| `access/resource.go` | Capture names use the `record` placeholder identifier rule. |
| `dbschema/schema_path.go` | New: `PathID`, `ID`, `Placeholder`, `ParentStep`, `SchemaPath` with `String`, `Validate`, `SameCollection`, `Overlaps`; `ParseSchemaPath` (built on the `record` grammar); the three path errors. |
| `dbschema/collection_def.go` | Add `Parent []ParentStep`, `SchemaPath()`, `Normalize()`; godoc for nesting, scopes and the two forms. |
| `ddl/modifier.go` | `SchemaModifier` keeps three methods; `DropCollection` and `AlterCollection` take `dbschema.SchemaPath`. |
| `ddl/options.go` | Add the generic value carrier (`WithValue`, `Options.Value`, `Options.Unsupported`, `RefuseUnsupported`) and the drop-flag options and keys; godoc states the required-or-hint rule next to the mismatched-option rule. |
| `ddl/drop.go` | New: `ErrCollectionNotEmpty`, `ErrInvalidOption`, `CollectionNotEmptyError`. |
| `ddl/operations.go` | Helpers apply REQ:guard-precedence and normalise paths; new `DropCollectionAt` and `AlterCollectionAt`; `DropCollection` / `AlterCollection` parse a schema path string. |
| `ddl/ddltest/` | New test-only conformance suite (`Config`, `RunConformance`). |
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
| Placeholder name for an existing collection differs from the one a nesting driver stored | `errors.Is(err, dbschema.ErrPlaceholderNameConflict)`; nothing created |
| Required option the driver does not support, on any operation or `AlterOp`, from any package | error matching `dal.ErrNotSupported` (`*dbschema.NotSupportedError` naming the key type); nothing changed |
| Hint option the driver does not support | ignored |
| DB does not implement `SchemaModifier` (for example a store whose schema is defined elsewhere) | existing `*dbschema.NotSupportedError`; no dispatch |
| Nested path or `Parent`, driver lacks nesting | driver returns `*dbschema.NotSupportedError` (matches `dal.ErrNotSupported`); nothing changed |
| Drop would delete records or nested data without the matching flag, any schema drop while an instance holds data, a definition drop with descendant definitions, or a drop that would orphan referencing records | `*ddl.CollectionNotEmptyError` (`errors.Is(err, ddl.ErrCollectionNotEmpty)`) listing counts and paths; nothing removed |
| `DeleteRecords()` / `DeleteNested()` with a path containing a `{placeholder}` | error satisfying `errors.Is(err, ddl.ErrInvalidOption)`; no dispatch through the helpers |
| `DeleteDependents()` on any drop | `*dbschema.NotSupportedError` (reserved); no dispatch |
| `DeleteRecords()` / `DeleteNested()` on a driver that does not support it, or on a non-drop operation | driver returns an error matching `dal.ErrNotSupported`; nothing changed |
| An ancestor collection definition does not exist (nesting driver) | driver-specific non-nil error; nothing created. A missing scoping parent **record** is not an error. |
| Supported option with an invalid value (e.g. an inGitDB `RecordFileDef` failing `Validate`) | driver-specific error; the surface is supported, the value is not |

## Testing Strategy

In-package Go tests in `record`, `dbschema` and `ddl` with stub `dal.DB` values,
following the existing `ddl/operations_test.go` pattern, plus a compile-shape
test for `SchemaModifier` and a round-trip test over `ParseSchemaPath` /
`String`. `ddl/ddltest.RunConformance` runs in every in-org driver's suite,
and the `dalgo2ingitdb-*` ACs in `ingitdb/dalgo2ingitdb`'s suite,
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
- **Concrete driver options in core.** No `RecordFile`, table options,
  Firestore settings or similar in `ddl` or `dbschema`.
- **Capability interfaces.** No `SupportsSubCollections`, `SupportsExtension`,
  `SupportsDrop` or similar; drivers refuse what they cannot do, and the shared
  conformance suite proves it (founder direction, 2026-09-17).
- **Implementing module changes in this repo.** The work in
  REQ:in-org-drivers-comply is tracked on dal-go/dalgo#163. dalgo2ingitdb's
  `WithRecordFile` option, nesting, depth-N ancestor check, path-based
  Drop/Alter and conforming its slash parsing to the grammar are
  ingitdb/dalgo2ingitdb#16.
- **A foreign-key and cascade model.** A driver-agnostic declaration of
  references with cascade actions, which `DeleteDependents()` needs, is a later
  Feature. Until then the flag is reserved and refused.
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
  SQL drivers refuse nested paths until a separate Feature designs it.
- **Declaring a subcollection tree in one call** (`SubCollections []CollectionDef`),
  rejected in REQ:one-collection-per-create.
- **Read-side round trip.** `dbschema.DescribeCollection` takes a
  `*dal.CollectionRef` (`dbschema/reader_helpers.go:44`) and does not report
  `Parent` or driver options in this Feature.

## Open Questions

None. Resolved on 2026-09-17 by founder decision:

- **Placeholder-name enforcement:** "A". Every nesting driver persists
  placeholder names and refuses a conflicting name (REQ:schema-path-scopes,
  AC:dalgo2ingitdb-placeholder-name-conflict-refused).
- **Nested drop:** "1-A", refined: drops refuse data loss by default; a
  placeholder-path drop is schema-only and never deletes data across parents;
  `DeleteRecords()` and `DeleteNested()` apply to one instance only; a
  reserved `DeleteDependents()` (REQ:drop-refuses-data-loss-by-default,
  REQ:drop-flags).
- **Schema-path ids:** `{placeholder}` names, with concrete ids for scoped
  definitions such as `ext/datatug/…` (REQ:schema-path-scopes).
- **Minimal interface:** no capability interfaces; driver-specific settings are
  functional options, ignored or refused per their declaration
  (REQ:option-values, REQ:unsupported-options).

---
*This document follows the https://specscore.md/feature-specification*
