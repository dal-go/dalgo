# Agent rules for dal-go/dalgo

## Public interface changes require an explicit human justification

DALgo is a dependency of many drivers and applications. Any change to an exported
interface, method signature, type, function, or to the documented behaviour of an
exported API is a breaking change for someone.

- Do NOT change, remove, rename or extend an exported interface or its behaviour
  unless a human has explicitly stated **why it is OK to change it**.
- Record that reason, quoted verbatim with the person and date, in the pull
  request description, and in the spec if the change is specified there.
- An agent's own reasoning, a reviewer's suggestion, a spec author's design, or
  "the change is breaking anyway" is NOT a justification.
- If no such reason exists, prefer an additive change (new optional function or
  functional option) — and still ask before adding exported API.
- A spec that implies an interface change is not approval of the change; ask.

## Storage-format concerns belong to the storage module, not DALgo

DALgo carries only what is generic across drivers. Storage-specific concepts — for
example the inGitDB schema (collection definition files, `record_file`, subcollection
definitions) — belong to that storage's own modules (`ingitdb-go`, `dalgo2ingitdb`),
never to DALgo's interfaces, types or options.

## DALgo core is engine-free: engine code lives in dalgo2<engine> adapters

This repository holds DALgo's interfaces, its generic logic, and exactly two adapters:
`adapters/dalgo2memory` (the in-memory implementation) and `adapters/dalgo2fs` (the OS file
system, kept here as a named exception: it needs no driver and DataTug imports it; founder,
2026-11-04: "Keep as exception"). Everything specific to one database engine lives in that
engine's own repository, `dal-go/dalgo2<engine>` (for example `dal-go/dalgo2postgres`), never
here:

- no database drivers or engine clients in `go.mod` (pgx, lib/pq, go-sql-driver/mysql, sqlite,
  mssql, mongo, Firestore, BigQuery, …) and no `database/sql`;
- no engine-specific SQL or catalog queries (`pg_catalog`, `pg_roles`, `information_schema`,
  `sqlite_master`, `sys.objects`, …) in code; a doc comment may name them as examples;
- no new adapter under `adapters/`.

When a capability needs engine knowledge (schema, roles and privileges, statistics), DALgo gets
an optional, engine-neutral interface (subject to the exported-API rule above) and each engine's
adapter implements it.

Founder, 2026-11-04: "if we need to read Postgres schema, that should be done in Dalgo adapter,
not custom something."

Enforced by `internal/architecture/engine_free_test.go`. Do not weaken the test to get a change
in; put the code in an adapter.
