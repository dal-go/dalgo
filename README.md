# 🔌 DALgo

https://dalgo.io/

**DALgo** is a database abstraction layer for Go applications. It gives your
business code one small, consistent API for records, queries, transactions,
hooks, and schema-aware key mapping while letting the storage backend remain an
implementation choice.

```bash
go get github.com/dal-go/dalgo
```

[![Build, Test, Vet, Lint](https://github.com/dal-go/dalgo/actions/workflows/ci.yml/badge.svg)](https://github.com/dal-go/dalgo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dal-go/dalgo)](https://goreportcard.com/report/github.com/dal-go/dalgo)
[![Coverage Status](https://coveralls.io/repos/github/dal-go/dalgo/badge.svg?branch=main&kill-cache=3)](https://coveralls.io/github/dal-go/dalgo?branch=main)
[![Version](https://img.shields.io/github/v/tag/dal-go/dalgo?filter=v*.*.*&logo=Go)](https://github.com/dal-go/dalgo/tags)
[![GoDoc](https://godoc.org/github.com/dal-go/dalgo?status.svg)](https://godoc.org/github.com/dal-go/dalgo)
[![Sourcegraph](https://sourcegraph.com/github.com/dal-go/dalgo/-/badge.svg)](https://sourcegraph.com/github.com/dal-go/dalgo?badge)

<!-- dev-approach:v1 -->
## Our approach to development

We build with our own tooling:

- **[SpecScore](https://specscore.md)** — specify requirements as `SpecScore.md` artifacts
- **[SpecStudio](https://specscore.studio)** — author & manage specs across their lifecycle
- **[inGitDB](https://ingitdb.com)** — store structured data in Git where applicable
- **[DALgo](https://dalgo.io)** — data access layer for Go
- **[cover100.dev](https://cover100.dev)** — drive toward 100% test coverage
- **[DataTug](https://datatug.io)** — query & explore data
<!-- /dev-approach -->

> **Specification scope:** The [`spec/`](./spec) tree specifies DALgo code and
> runtime behavior only. The separate [dalgo.io](https://dalgo.io/) website,
> its content, visual design, and marketing pages are intentionally outside the
> SpecScore scope of this repository.

## 🎯 Why Use DALgo

DALgo is useful when an application needs stable data-access code without
coupling the domain layer to Firestore, SQL, or a test-only database.

- Keep application logic independent from a concrete database client.
- Use the same record, query, and transaction shape across supported adapters.
- Test business logic with the built-in in-memory adapter.
- Put extensions, tenants, jobs, and support tools behind portable,
  adapter-independent [access policies](./docs/access-policies.md).
- Add logging, validation, metrics, and other behavior through hooks.
- Model both document/key-value stores and relational tables through one key and
  schema abstraction.

DALgo does not try to hide every database difference. Adapters can return
`dal.ErrNotSupported` for capabilities their backend cannot provide. This keeps
the core API honest while still giving applications a shared path for the common
operations.

### Exact decimal arithmetic for federated aggregates

Federated queries can opt into exact decimal text arithmetic with the existing
`MoneyConfig` option. In this mode, fractional values must arrive as decimal
strings; fractional or unsafe binary floating-point values, exponent notation,
malformed values,
and values exceeding 38 significant digits are rejected. Source fields may
contain at most `MinorUnitScale` fractional digits. Safe whole-number Go
floating-point inputs remain accepted. Addition, subtraction,
multiplication, and `SUM` preserve finite decimal precision; division and
`AVG` round half-even to `DivisionScale`. Exact decimal ordering applies to
numeric-looking values in money comparisons, while other text values remain
lexical. The default query path is unchanged.

Money mode supports aggregate queries over one source, and flat hash-join
federated aggregates. For example, a line total can be aggregated without
converting either operand to `float64`:

```go
options := dal.FederatedQueryOptions{Money: &dal.MoneyConfig{
	MinorUnitScale: 2,
	DivisionScale: 4,
	Rounding: "halfEven",
}}
reader, err := dal.ExecuteFederatedQueryWithOptions(ctx, query, resolve, options)
```

## 🛡️ Access Policies: Least Privilege at the DAL Boundary

DALgo can wrap any adapter with a capability boundary that is enforced before
the adapter sees a read, query, batch, mutation, or transaction operation.
Policies distinguish `Get`, `Exists`, `Query`, `Insert`, `Set`, `Update`,
`Delete`, and the reserved `Truncate` operation. Write access never silently
grants read access.

```go
extension := access.MustPolicy("extension-trackus",
	access.Root(access.Deny(access.ReadWrite, "outside-extension")),
	access.Scope("ext", "trackus",
		access.Allow(access.ReadWrite, "own-global-data")),
	access.Scope("spaces", access.AnyID,
		access.Scope("ext", "trackus",
			access.Allow(access.ReadWrite, "own-space-data"))),
)

db := access.MustSecureDB(rawDB, access.RequireContextPolicy())
ctx := access.WithPolicy(context.Background(), extension)
```

Rules are hierarchical: a more-specific child may carve a denial out of an
allowed parent or reopen a subtree denied by its parent. Separate database and
context policies intersect, so adding a policy can only narrow authority.
Every denied call returns an inspectable `access.DeniedError` with the
operation, resource, policy name/source, matched rule ID, and explanation.

Policies can be authored as versioned YAML, represented equivalently as JSON,
and loaded from any `io.Reader`—files are only one possible storage choice.
The same hierarchy also supports independent audit selection without granting
data access. See the [Access Policies guide](./docs/access-policies.md) for the
complete model, loading APIs, error handling, and security boundary.

## ⚡ Quick Example

This example uses `dalgo2memory`, the built-in in-memory adapter. The same
application code can be written against `dal.DB` and supplied with another
adapter in production.

```go
package main

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
)

type User struct {
	Name  string
	Email string
}

func main() {
	ctx := context.Background()
	db := dalgo2memory.NewDB()

	key := dal.NewKeyWithID("users", "u1")
	if err := db.Set(ctx, dal.NewRecordWithData(key, &User{
		Name:  "Ada Lovelace",
		Email: "ada@example.com",
	})); err != nil {
		panic(err)
	}

	var user User
	record := dal.NewRecordWithData(key, &user)
	if err := db.Get(ctx, record); err != nil {
		panic(err)
	}
	if record.Exists() {
		fmt.Println(user.Email)
	}
}
```

## 🪄 Typed Collections (Simplified API)

For everyday point CRUD you usually do not need to build keys, wrap records, and
type-assert data by hand. DALgo provides a generic, session-less
`dal.Collection[K, T]` handle (id type `K`, record type `T`) that returns typed
values directly. It is additive over the core API, uses no reflection of its
own, and works with every adapter.

```go
type User struct {
	Name  string
	Email string
}

// CollectionName (value receiver) names the collection.
func (User) CollectionName() string { return "users" }

// A Collection[K, T] holds no session, so declare it once and reuse it
// (e.g. as a package-level var). Here ids are strings (K = string).
var Users = dal.CollectionOf[string, User]()

func demo(ctx context.Context, db dal.DB) error {
	// Writes go through a read-write transaction. Because dal.DB is not a
	// WriteSession, calling a write terminal with a plain db is a compile error.
	if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return Users.SetByID(ctx, tx, "u1", User{Name: "Ada Lovelace", Email: "ada@example.com"})
	}); err != nil {
		return err
	}

	// Reads take a dal.ReadSession (a plain dal.DB satisfies it) and return T.
	user, err := Users.GetData(ctx, db, "u1")
	if err != nil {
		return err // not-found is reported via dal.IsNotFound(err)
	}
	fmt.Println(user.Email)
	return nil
}
```

The handle exposes the common operations as typed terminals:

- **Reads:** `GetData` (one record → `T`), `GetRecord` (→ `dal.Record`),
  `GetRecordWithID` (→ `dal.RecordWithID[K]`), `GetRecordWithDataAndID`
  (→ `dal.RecordWithDataAndID[K, *T]`), `All` (whole collection → `[]T`),
  `First`, `Count`, `Exists`. For interface-typed model data created by a
  factory, use the free function `dal.GetRecordWithIDIntoData(ctx, s, key, id,
  data)`, which decodes into the value you pass.
- **Writes:** `Insert` (generated id → `*dal.Key`), `InsertWithID` (known id),
  `InsertRecord`, `SetByID` (upsert), `SetRecord`, `UpdateByID`, `UpdateByKey`,
  `DeleteByID`, `DeleteByKey`, and batch `InsertMany` via the opt-in
  `dal.ManyInserter[K, T]` interface. For interface-typed model data, insert via
  the free function `dal.InsertRecordWithDataAndID(ctx, s, key, id, data)` (the
  write twin of `GetRecordWithIDIntoData`).
- **Composite / multi-field keys:** pass `dal.WithKeyOptions(...)` to the
  constructor, or build a `*dal.Key` with `dal.NewKeyWithFields` and use the
  `*ByKey` terminals.
- **Deprecated aliases:** `Get`/`Set`/`Update`/`Delete` remain as thin
  delegators to `GetData`/`SetByID`/`UpdateByID`/`DeleteByID`.
- **Nesting:** `In(parentKey)` scopes the handle to a subcollection such as
  `users/u1/contacts`.
- **Compile-time safety:** read terminals take `dal.ReadSession` and write
  terminals take `dal.WriteSession`, so writes are only reachable inside
  `RunReadwriteTransaction`.

### Standard `database/sql` vs DALgo

The same "read one user by id" written against the standard library and against
a DALgo typed collection. The DALgo version is backend-agnostic: the identical
code runs on Firestore, SQL, the filesystem, or the in-memory adapter.

<table>
<tr><th>Standard <code>database/sql</code></th><th>DALgo typed collection</th></tr>
<tr>
<td>

```go
type User struct {
	ID, Name, Email string
}

func GetUser(ctx context.Context, db *sql.DB, id string) (*User, error) {
	row := db.QueryRowContext(ctx,
		"SELECT id, name, email FROM users WHERE id = ?", id)

	u := &User{}
	err := row.Scan(&u.ID, &u.Name, &u.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
```

</td>
<td>

```go
type User struct {
	Name, Email string
}

func (User) CollectionName() string { return "users" }

var Users = dal.CollectionOf[string, User]()

func GetUser(ctx context.Context, db dal.DB, id string) (User, error) {
	return Users.GetData(ctx, db, id)
}
```

</td>
</tr>
</table>

## 🔌 Core API

The main package is [`dal`](./dal). It defines the interfaces application code
usually depends on:

- `dal.DB` for database-level reads, transactions, schema metadata, and adapter
  identity. It is produced by `dal.NewDB` from an adapter's `dal.Backend` and
  owns the write pipeline, so record validation and before-save hooks run before
  any adapter is entered.
- `dal.Backend` for what a storage adapter implements — the same shape as
  `dal.DB`, without the seal.
- `dal.ReadSession` and `dal.ReadwriteSession` for read and write operations.
- `dal.Record` and `dal.Key` for database records and hierarchical keys.
- `dal.Query` for structured and text queries.
- `dal.Schema` for mapping DALgo keys to relational columns.

Most applications should accept `dal.DB`, `dal.ReadSession`, or
`dal.ReadwriteSession` in their own services instead of accepting a concrete
adapter type.

```go
func LoadUser(ctx context.Context, db dal.ReadSession, id string) (*User, error) {
	user := new(User)
	record := dal.NewRecordWithData(dal.NewKeyWithID("users", id), user)

	if err := db.Get(ctx, record); err != nil {
		return nil, err
	}
	if !record.Exists() {
		return nil, dal.ErrRecordNotFound
	}
	return user, nil
}
```

## 🌳 Hierarchical Collections

DALgo keys can represent nested document paths, which maps naturally to
Firestore-style collections such as `countries/ireland/cities/dublin`.

```go
countryKey := dal.NewKeyWithID("countries", "ireland")
cityKey := dal.NewKeyWithParentAndID(countryKey, "cities", "dublin")

err := db.Set(ctx, dal.NewRecordWithData(cityKey, &City{
	Name:       "Dublin",
	Population: 592713,
}))
```

The same parent key can scope a query to a nested collection. For Firestore this
is the shape of a query under `countries/ireland/cities`.

```go
ireland := dal.NewKeyWithID("countries", "ireland")
cities := dal.NewCollectionRef("cities", "", ireland)

q := dal.From(cities).NewQuery().
	WhereField("Population", dal.GreaterThen, 100000).
	OrderBy(dal.DescendingField("Population")).
	SelectColumns(
		dal.Column{Expression: dal.Field("Name")},
		dal.Column{Expression: dal.Field("Population")},
	)
```

## 🔎 Queries

DALgo includes a structured query builder for common database-style reads:
filters, ordering, joins, column projection, and aggregation. Aggregation uses
granular provider capabilities: full native execution where possible, ordered
streaming when a provider can sort raw rows by group keys, and a typed hash
fallback otherwise.

```go
q := dal.From(dal.NewRootCollectionRef("cities", "")).NewQuery().
	WhereField("Country", dal.Equal, "IE").
	OrderBy(dal.DescendingField("Population")).
	Limit(10).
	SelectColumns(
		dal.Column{Expression: dal.Field("Name")},
		dal.Column{Expression: dal.Field("Population")},
	)

records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, db)
```

Recent query capabilities include:

- Column projection through `SelectColumns`.
- `GROUP BY`, alias-aware `HAVING`, ungrouped aggregates, DISTINCT
  `COUNT`/`SUM`/`AVG`, `MIN`/`MAX`, and deterministic `FIRST`/`LAST`.
- Inner and left equi-joins in the structured query model.
- Source-qualified field references for joins and `ORDER BY`.
- Recordset readers with typed columns where the adapter supports columnar
  output.

Structured queries reach SQL engines through `dalgo2sql`. Its SQLite compiler
(`StructuredQueryDialect: "sqlite"`) and its PostgreSQL compiler
(`StructuredQueryDialect: "postgres"`) bind query values; with an empty dialect
every `database/sql` engine, SQLite included, uses the legacy text emitter,
which writes query values into the SQL text. Do not pass untrusted values
through it.

## 🔁 Transactions

Transactions use callback-style workers. This keeps transaction lifetime scoped
and lets adapters implement retries or backend-specific transaction behavior.

```go
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
	key := dal.NewKeyWithID("users", "u1")
	return tx.Set(ctx, dal.NewRecordWithData(key, &User{Name: "Ada"}))
}, dal.TxWithMessage("create user u1"))
```

Transaction options can carry isolation-level requests and a human-readable
message. Some adapters can surface the message in logs or backend history.

## 🧰 Built-In Adapters

This repository includes:

- [`dalgo2memory`](adapters/dalgo2memory) - in-memory DALgo database for tests,
  examples, local development, and query behavior verification. It supports
  schema registration, typed records, serialized storage, columnar storage, and
  mixed-mode `map[string]any` columnar storage.
- [`dalgo2fs`](adapters/dalgo2fs) - filesystem-backed adapter useful for simple local
  persistence and examples.

`dalgo2memory` is intentionally useful beyond trivial tests. It can run many
structured query features end to end, which makes it a practical default for
unit tests around application data access.

## 🌐 Supported External Adapters

DALgo supports production use through separate adapter modules:

- [`dalgo2firestore`](https://github.com/dal-go/dalgo2firestore) for Google
  Cloud Firestore.
- [`dalgo2sql`](https://github.com/dal-go/dalgo2sql) for SQL databases through
  Go SQL drivers. Two engines have a structured-query compiler that binds query
  values as parameters: SQLite (`DbOptions.StructuredQueryDialect: "sqlite"`)
  and PostgreSQL (`DbOptions.StructuredQueryDialect: "postgres"`, available
  from dalgo2sql v0.26.0). With an empty dialect every `database/sql` engine,
  SQLite and PostgreSQL included, uses the legacy text emitter, which is not
  dialect-aware and writes query values into the SQL text instead of binding
  them; do not pass untrusted values through it. MySQL, Microsoft SQL Server and
  Oracle have no compiler yet and use the legacy text emitter.
- [`dalgo2sqlite`](https://github.com/dal-go/dalgo2sqlite) for SQLite-specific
  schema, DDL, and concurrency-aware behavior on top of SQL support.

Deprecated BuntDB and BadgerDB adapters are not listed as supported production
targets.

## 📦 Packages

- [`dal`](./dal) - core database abstraction, keys, records, sessions,
  transactions, queries, hooks, and schema mapping.
- [`dalgo2memory`](adapters/dalgo2memory) - built-in in-memory adapter.
- [`dalgo2fs`](adapters/dalgo2fs) - filesystem adapter.
- [`orm`](./orm) - object and collection mapping helpers.
- [`record`](./record) - helpers for strongly typed record handling.
- [`recordset`](./recordset) - row and column-oriented recordset structures.
- [`recordops`](./recordops) - compare, diff, and render helpers for records.
- [`dbschema`](./dbschema) - schema definitions for collections, fields,
  indexes, constraints, and defaults.
- [`ddl`](./ddl) - schema modification operations and applier interfaces.
- [`dtql`](./dtql) - serialized query format and schema for DALgo queries.
- [`update`](./update) - field update helpers.
- [`mocks`](./mocks) - generated mocks for tests.

## ✅ Quality And Compatibility

The project is maintained with automated checks and adapter-oriented test
coverage:

- CI runs build, tests, `go vet`, and lint checks.
- Core packages target full unit-test coverage.
- Shared end-to-end tests in [`end2end`](./end2end) exercise adapter behavior
  against the same scenarios.
- Feature specifications in [`spec/features`](./spec/features) document
  behavior that has been designed, implemented, and verified.

## 📚 Documentation

Start with these topic pages when you need more than the README:

- [Interfaces](./docs/interfaces.md)
- [Records](./docs/records.md)
- [Queries](./docs/queries.md)
- [Transactions](./docs/transactions.md)
- [Hooks](./docs/hooks.md)
- [Schema](./docs/schema.md)
- [Adapters](./docs/adapters.md)
- [Examples](./docs/examples.md)

## 🚀 Projects Using DALgo

- [`ingitdb`](https://github.com/ingitdb) - Git-native database tooling.
- [`inmemdb`](https://github.com/inmemdb) - in-memory database tooling.
- [`strongo/bots-framework`](https://github.com/strongo/bots-framework) -
  framework for building chatbots.
- [`DataTug`](https://github.com/datatug) - context-aware data
  viewer and collaborative query manager.

## 🤝 Contributing

Contributions are welcome, especially adapter improvements, end-to-end coverage,
and documentation that makes backend capabilities clearer. See
[`CONTRIBUTING.md`](./CONTRIBUTING.md) for project conventions.

## Optional source data rights

`dbschema.CollectionDef.SourceRights` describes optional effective source data
terms for tables/views. Providers can omit this metadata. The standalone
`datarights` package carries source identity, declaration (name/SPDX/URL/text),
true declaration scope, evidence pins and notices. It evaluates no licence and
grants no access; provider code owns inheritance, authorization and validation.

Query readers may implement the separate optional `dal.QueryMetadataProvider`
capability. Existing `Reader`, `RecordsReader`, `RecordsetReader` and database
interfaces do not grow. `dal.ReadQueryMetadata` obtains a detached snapshot before
reading rows; `WithRecordsQueryMetadata` and `WithRecordsetQueryMetadata` attach
a fixed snapshot while forwarding the original reader's methods.

`datarights.QueryMetadata.SourceRights` is the complete authorized planned input
inventory captured before output, not a licence assigned to the derived result.
`UsedSourceIDs` identifies sources actually read or considered, including empty
or projected-away inputs, independently of output rows. Nil means omitted;
non-nil empty inventories serialize as empty arrays. IDs, scopes, evidence
origins and pin roles remain provider-defined, with no OVDB requirement. Providers
own deterministic ordering, evidence budgets and snapshot consistency across
pages. Missing declarations mean unknown, not permission.

Generic joins/recursive execution and local aggregation currently cannot preflight
source rights. They refuse annotated inputs with `dal.ErrNotSupported` and a
`source_rights` diagnostic instead of losing metadata. Native provider execution
can expose metadata through either reader kind. A rights-aware provider must
supply a complete immutable inventory at reader creation; later introduction of
rights violates that contract. The wrappers snapshot supplied metadata and do
not inspect row contents or certify the supplied terms.
