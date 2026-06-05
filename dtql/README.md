# DTQL — YAML serialization of `dal.StructuredQuery`

DTQL is a 1:1, lossless, human-readable **YAML serialization** of dalgo's
`dal.StructuredQuery` for the core relational read-only subset. It is plain YAML
over the existing `dal` query model — there is no bespoke grammar and no
hand-written parser; the standard `gopkg.in/yaml.v3` library produces and
consumes it.

```go
data, err := dtql.Serialize(q)   // dal.StructuredQuery -> DTQL-YAML
q2, err := dtql.Deserialize(data) // DTQL-YAML -> dal.StructuredQuery
dtql.Equal(q, q2)                 // structural equality over the covered surface
```

This package imports `dal` but adds **no YAML dependency to `dal`**.

## Covered subset

DTQL represents a single `From` (with no `Joins()`) over a **root** `CollectionRef`
(`Parent() == nil`), the selected `Columns`, the `Where` condition (`Comparison`
and And/Or `GroupCondition` trees), `OrderBy`, and `Limit`/`Offset`. The
in-scope expression nodes are field references, constants and constant arrays;
literal values are carried **inline**. Anything outside this subset — joins,
`CollectionGroupRef` / parented `CollectionRef`, `GroupBy`, functions/aggregates,
a cursor (`StartFrom`), or an operator outside the in-scope set — is **rejected**
by `Serialize` with a descriptive error rather than silently dropped.

## Node → YAML mapping

| `dal` node | YAML representation |
|---|---|
| `From` over root `CollectionRef` | `from: { name: <string>, alias?: <string> }` |
| `Column` | a sequence item under `columns:`, an expression plus optional `as: <alias>` |
| `Comparison` | `{ op: <operator>, left: <expr>, right: <expr> }` |
| `GroupCondition` (And) | `{ and: [ <condition>, ... ] }` |
| `GroupCondition` (Or) | `{ or: [ <condition>, ... ] }` |
| `OrderExpression` | a sequence item under `orderBy:`, an expression plus optional `desc: true` |
| `FieldRef` | `{ field: <name> }` |
| `Constant` | `{ value: <scalar> }` (inline string, bool, int or float) |
| `Array` | `{ values: [ <scalar>, ... ] }` (inline, for `In` membership) |
| `Operator` | the `dal.Operator` string itself: `==`, `In`, `>`, `>=`, `<`, `<=` |
| `Limit` / `Offset` | `limit: <int>` / `offset: <int>` (omitted when zero) |

An expression node sets **exactly one** of `field`, `value` or `values`, which
discriminates a `FieldRef`, a `Constant` or an `Array`.

## Document shape

A DTQL-YAML document has this canonical key order (top-level keys are omitted
when empty/zero, except `from` which is required):

```yaml
from:
  name: users
columns:
  - field: name
  - field: age
    as: years
where:
  and:
    - op: '>='
      left:
        field: age
      right:
        value: 18
    - or:
        - op: In
          left:
            field: status
          right:
            values:
              - active
              - pending
        - op: ==
          left:
            field: country
          right:
            value: US
orderBy:
  - field: name
  - field: age
    desc: true
limit: 10
offset: 20
```

## Round-trip guarantees

- **Structural** — `Deserialize(Serialize(q))` reconstructs a `StructuredQuery`
  structurally equal to `q` across `From`, `Columns`, `Where` (including inline
  `Constant`/`Array` values), `OrderBy`, `Limit` and `Offset` (see `dtql.Equal`).
- **Canonical** — `Serialize` emits a canonical document (stable key order and
  2-space indentation), so `Serialize(Deserialize(d))` is byte-identical to a
  valid in-scope document `d` and saved queries diff cleanly across edits.

## Errors

`Deserialize` returns a descriptive error and **no** partially-populated query on
malformed or schema-invalid input: unknown keys, wrong value types, a missing
required `from.name`, an unknown operator, a comparison missing `left`/`right`,
an expression that is not exactly one of `field`/`value`/`values`, or a
condition that mixes the comparison and group forms.

## Published artifacts

The package also publishes DTQL as a tool-validatable artifact: a JSON Schema
(draft 2020-12) generated from the Go types — `schema/schema.json` (canonical
`$id` `https://dal-go.github.io/dtql/schema.json`) and `schema/schema.yaml` — a
set of example documents under `examples/`, and a styled index page, served at
**https://dal-go.github.io/dtql/**.
