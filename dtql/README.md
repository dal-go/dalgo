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

DTQL represents a recursive `From` relation tree over **root** `CollectionRef`
(`Parent() == nil`), `Where`, `GroupBy`, `Having`, `OrderBy`, `Limit`/`Offset`,
and the final `Columns` projection. Expressions include fields, constants,
arrays, parameters, arithmetic, aggregate functions and `COUNT(*)`. Anything
outside this subset — `CollectionGroupRef` / parented `CollectionRef`,
a cursor (`StartFrom`), or an operator outside the in-scope set — is **rejected**
by `Serialize` with a descriptive error rather than silently dropped.

## Node → YAML mapping

| `dal` node | YAML representation |
|---|---|
| `From` over root `CollectionRef` | `from: { schema?: <string>, name: <string>, alias?: <string>, joins?: [<join>, ...] }` |
| `JoinedSource` | `{ type?: inner|left, from: <from>, on: [<qualified equality>, ...], hints?: {algorithms: [<name>, ...]} }`; omitted `type` is `inner` |
| `Column` | a sequence item under `columns:`, an expression plus optional `as: <alias>` |
| wildcard exclusion projection | `{ wildcard: { source?: <alias-or-name>, exclude: [<column>, ...] } }` under `columns:` |
| `Comparison` | `{ op: <operator>, left: <expr>, right: <expr> }` |
| `GroupCondition` (And) | `{ and: [ <condition>, ... ] }` |
| `GroupCondition` (Or) | `{ or: [ <condition>, ... ] }` |
| `IsNullCondition` | `{ isNull: <expression> }` / `{ isNotNull: <expression> }` |
| `OrderExpression` | a sequence item under `orderBy:`, an expression plus optional `desc: true` |
| `GroupBy` | `groupBy: [ <expression>, ... ]` |
| money amount policy | `money: {minorUnitScale: 2, divisionScale: 4, rounding: halfEven}` |
| `Having` | `having: <condition>` |

The optional `money` policy applies to federated streaming aggregates. Amounts are
canonical decimal strings or whole integers; fractional binary floats and amounts
with more digits than `minorUnitScale` are rejected. `SUM` accumulates integer
minor units and returns decimal text. `AVG` and binary per-capita division return
decimal text rounded half-even to `divisionScale`; division by zero errors.
Existing queries keep their numeric behavior. The `strongo/decimal` cents type
cannot serve as this accumulator because its `int64` range excludes large sums
and its current parser passes through binary floating point.
| `FieldRef` | `{ field: <name>, source?: <alias> }` |
| `Constant` | `{ value: <scalar> }` (inline string, bool, int or float) |
| `Array` | `{ values: [ <scalar>, ... ] }` (inline, for `In` membership) |
| `Param` | `{ param: <name> }` — a runtime parameter (`$name` in query text), substituted with a constant or array before execution; names are dotted identifiers such as `currentUser` or `principal.roles` |
| aggregate | `{ aggregate: { function: count|sum|avg|min|max|first|last, distinct?: true, args: [<expr>] } }` |
| arithmetic | `{ binary: { op: +|-|*|/, left: <expr>, right: <expr> } }` |
| `COUNT(*)` argument | `{ star: true }` |
| `Operator` | the `dal.Operator` string itself: `==`, `In`, `>`, `>=`, `<`, `<=` |
| `Limit` / `Offset` | `limit: <int>` / `offset: <int>` (omitted when zero) |

An expression node sets exactly one discriminator. Aggregate arguments are
expressions, so `SUM(quantity * unit_price)` needs no later AST redesign.
Qualified fields keep the source structural, for example
`{field: country, source: c}`; the source is never flattened into a dotted
field name.

## Null tests

`isNull` and `isNotNull` take one expression (usually a field) and are true when
it is, respectively is not, null. They are valid wherever a condition is: in
`where`, in `having`, inside `and` / `or` groups, and in a nested query's
conditions; they are not valid in a join's `on` list, which stays equality only.

```yaml
from: {name: Chat, alias: c, joins: [{type: left, from: {name: Invoice, alias: i}, on: [{left: {field: id, source: c}, op: '==', right: {field: chat, source: i}}]}]}
where:
  and:
    - isNull: {field: Company, source: c}      # no company, or the column is absent
    - isNotNull: {field: chat, source: i}       # ...and the chat has an invoice
columns: [{field: id, source: c}]
```

```yaml
from: {name: sales}
groupBy: [{field: category}]
having:
  isNotNull: {aggregate: {function: max, args: [{field: amount}]}}
columns: [{field: category}]
```

A field missing from a document counts as null, like an explicit `null` or a
nil pointer. A null test is never unknown, so `or` over it behaves as two-valued
logic. The operand is a field, a literal, arithmetic over those, or a scalar
subquery; an aggregate is allowed only in `having`, and a `values` list, `star`
and `param` are rejected at parse (`query_shape at where.isNull: ...`). A
document that sets two condition forms (`isNull` with `isNotNull`, `and`, `op`,
...) is rejected as mixing forms.

**What `op: ==` with `{value: null}` means, and why it is not enough.** It does
not mean one thing; it depends on where the condition is evaluated:

| where the condition runs | `x == null` | `x < 1`, `In`, ... against null |
|---|---|---|
| `where` (and join `on`) of a query the generic executor runs: any join, nested query, or federated aggregation | UNKNOWN: matches no row | UNKNOWN |
| `having` (local aggregation) | true when `x` is null, so `having: g == null` returns the null group | `<` `<=` `>` `>=` false; `In`/`NotIn` unsupported |
| `where` of a single-source query handed whole to an adapter | the adapter's meaning (`IS NULL` in the SQLite emitter; the in-memory adapter also matches an absent field) | the adapter's meaning |

So `where: Company == null` returns the null rows from one table and none once
the query is joined, while `having` treats null as a value. That `where` versus
`having` inconsistency predates null tests and this change leaves it alone;
`isNull` / `isNotNull` are the one spelling whose meaning is the same in every
row of that table. The serializer never rewrites an existing `== null`
comparison, and `!=` does not exist in DTQL, so `isNotNull` is the only way to
ask for non-null values. Rendered as SQL a null test is `x IS NULL` /
`x IS NOT NULL`.

**Which engines evaluate it.** The generic executor (joins, nested queries,
aggregation, HAVING) evaluates null tests whatever adapter supplies the rows. An
adapter that runs a query itself, a single-source query or one it claims
natively (a native join or aggregation), must evaluate or translate them: the
in-memory adapter (`dalgo2memory`) does, and every other adapter currently
rejects the query with an "unsupported condition" error until it implements
them (an adapter must fail rather than ignore a condition it cannot translate). The TypeScript
engine parses a bare single-source query with a null test into its relation
model so the same document runs there; see dalgo-js's README.

**A misspelt field is not an error in Go unless the executor knows the
schema.** A record that lacks a field and a field that does not exist look the
same, so `isNull: {field: compnay}` matches every row, and `isNotNull` none.
When the backend reports its fields (`dal.JoinFieldsProvider`) the executor
rejects an unknown field before reading a row (`join_field`), the way it does
for a comparison; the TypeScript parser always has a schema and rejects it at
parse. Without field metadata, check field names before running a null test.

**A `null` list element is dropped, not rejected.** `and: [{isNull: ...}, null]`
reads as `and: [{isNull: ...}]`, because the YAML decoder skips a null element of
a list of mappings (this predates null tests and applies to every condition).
The TypeScript parser rejects it.

**Not in policies.** Access policy predicates (`access.DocumentCondition`,
`condeval`) keep the comparison and group subset and reject a null test: there a
missing field never matches (layered-ACL acceptance V02), and `isNull` would
make it match. The query engine reads a missing field as null, as it already
does for every other operator.

## Recursive joins

Every relation node may contain ordered `joins`. A join's `from` is another
relation node, `on` contains one or more `==` predicates, and every ON operand
is a qualified field reference. `inner` is the default; `left` is the other
supported type. The serializer emits `alias` and `==`; deserialization also
accepts `as` and `eq` and normalizes them on output.

```yaml
from:
  name: Invoice
  alias: i
  joins:
    - from:
        name: Customer
        alias: c
        joins:
          - type: left
            from: {name: Employee, alias: e}
            on:
              - left: {field: SupportRepId, source: c}
                op: '=='
                right: {field: EmployeeId, source: e}
            hints:
              algorithms: [nestedLoop, hash]
      on:
        - left: {field: CustomerId, source: i}
          op: '=='
          right: {field: CustomerId, source: c}
      hints:
        algorithms: [merge, hash]
```

Each JOIN can independently request an ordered list of `hash`, `merge`,
`lookup`, `batchedLookup`, and `nestedLoop`. Lists must be nonempty and contain
distinct, case-sensitive names. The generic executor currently selects `hash`
for a direct cross-side key and `nestedLoop` for bounded candidate evaluation.
It skips unavailable algorithms and uses its ordinary strategy when no hint
applies. A preferred `nestedLoop` bypasses an available hash index and can hit
the candidate bound after up to O(N × M) comparisons for N left and M right
rows. Hints never change successful query results or their order.

Validation reports stable JOIN category and zero-based path pairs, such as
`join_scope at from.joins[1].on[0]`. It checks relation shape, `inner`/`left`,
qualified equality predicates, duplicate aliases, forward references, lexical
scope, and cyclic in-memory relation trees. Field existence and key runtime
types require schema or execution data and are validated by later layers.

## Aggregation

DTQL keeps projection at the end of the YAML pipeline:

```yaml
from:
  name: orders
where:
  op: ==
  left: {field: status}
  right: {value: paid}
groupBy:
  - field: country
having:
  op: '>'
  left: {field: revenue}
  right: {value: 10000}
orderBy:
  - field: revenue
    desc: true
columns:
  - field: country
  - aggregate:
      function: count
      args: [{star: true}]
    as: orders
  - aggregate:
      function: count
      distinct: true
      args: [{field: customer_id}]
    as: customers
  - aggregate:
      function: sum
      args: [{field: total}]
    as: revenue
```

With `groupBy` and no `columns`, grouping expressions are selected implicitly.
Without `groupBy`, aggregate columns operate on one implicit group. `WHERE`
filters rows; `HAVING` filters groups; requested ordering, offset and limit
apply to aggregate rows.

`COUNT(*)` counts rows. Other aggregates ignore nulls except `FIRST`/`LAST`,
where null is a legitimate first/last value. Empty ungrouped input returns one
row (`COUNT(*) = 0`, other aggregates null); empty explicit grouping returns no
rows. `SUM`/`AVG` use finite `float64` accumulation and `COUNT` returns `int64`.
Arithmetic normalizes numeric operands to `float64`; non-numeric operands and
division by zero evaluate to null.
`FIRST`/`LAST` require a provider-declared stable input order; aggregate-local
ordering is a future extension.

## Negative projection

A wildcard column item may exclude one or more unqualified column names. The
unqualified form selects every available column except the requested names:

```yaml
from:
  name: customers
columns:
  - wildcard:
      exclude:
        - email
        - password_hash
```

When the source has an alias, `source` scopes the wildcard while the names in
`exclude` remain unqualified:

```yaml
from:
  name: customers
  alias: c
columns:
  - wildcard:
      source: c
      exclude:
        - email
```

An excluded name that is absent from the result schema is ignored. Duplicate
names are harmless, and removing columns preserves the relative order of every
remaining column. This makes defensive requests such as excluding `password`,
`password_hash`, `secret`, and `api_key` useful without prior schema discovery.
It is a projection convenience, not a security boundary; access control and
sensitive-data policy remain separate concerns. The AST retains the requested
exclusion list so future response metadata can distinguish matched and
unmatched exclusions without changing query semantics.

## Document shape

A DTQL-YAML document has this canonical key order (top-level keys are omitted
when empty/zero, except `from` which is required):

```yaml
from:
  name: users
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
columns:
  - field: name
  - field: age
    as: years
```

For a schema-qualified source, `schema` is a separate optional identifier:

```yaml
from:
  schema: main
  name: Customer
limit: 50
```

The same shape represents SQL Server `dbo.Customer` by setting `schema: dbo`.
Schema values are not special-cased. SQL adapters must quote the schema and
relation as separate identifier segments, such as `"main"."Customer"` for
SQLite or `[dbo].[Customer]` for SQL Server. Non-SQL adapters that cannot
represent a schema-qualified source must reject it explicitly; they must not
silently flatten the two segments into a dotted collection name.

DALgo access policies likewise do not collapse a qualified source onto the
ordinary root collection path. Secured sessions classify it as an opaque query,
so execution requires an explicit `OpaqueQueryScope` rule until schema-aware
path resources are defined.

## Round-trip guarantees

- **Structural** — `Deserialize(Serialize(q))` reconstructs a `StructuredQuery`
  structurally equal to `q` across `From`, `Where`, `GroupBy`, `Having`,
  `OrderBy`, `Limit`/`Offset`, and `Columns` (see `dtql.Equal`).
- **Canonical** — `Serialize` emits a canonical document (stable key order and
  2-space indentation). `Serialize(Deserialize(d))` normalizes accepted input
  aliases (`as` to `alias`, `eq` to `==`) and omitted INNER types, then remains
  byte-identical on later round trips.

## Errors

`Deserialize` returns a descriptive error and **no** partially-populated query on
malformed or schema-invalid input: unknown keys, wrong value types, a missing
required `from.name`, an unknown operator, a comparison missing `left`/`right`,
an expression that does not select exactly one expression form, or a
condition that mixes the comparison and group forms.

## Published artifacts

The package also publishes DTQL as a tool-validatable artifact: a JSON Schema
(draft 2020-12) generated from the Go types — `schema/schema.json` (canonical
`$id` `https://dal-go.github.io/dtql/schema.json`) and `schema/schema.yaml` — a
set of example documents under `examples/`, and a styled index page, served at
**https://dal-go.github.io/dtql/**.
