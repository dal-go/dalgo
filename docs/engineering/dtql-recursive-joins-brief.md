# Engineering Prompt: DTQL JOINs for DALGO Go and DALGO-JS

## Mission
Specify, plan, and implement first-class **JOIN support for DTQL and DALGO** in both **DALGO Go** and **DALGO-JS/TypeScript**. Work autonomously through repository discovery, specification, planning, adversarial review, implementation, tests, documentation, and final verification. Do not stop after producing a spec or plan unless genuinely blocked externally.

First save this brief in an appropriate repository document so its intent survives context loss.

## Working process
1. Inspect all relevant repositories/packages, current DTQL specification/models, DALGO query abstractions, adapters, capabilities, result models, errors, tests, and conventions.
2. Identify the canonical DTQL specification location.
3. Save/update the authoritative specification **before substantial implementation**.
4. Produce an implementation plan covering Go and TypeScript.
5. Critically/adversarially review the specification and plan for ambiguity, unnecessary complexity, compatibility problems, execution correctness, missing cases, and Go/JS divergence.
6. Resolve routine questions autonomously from repository evidence and this design intent.
7. Implement progressively in coherent milestones, with tests alongside implementation.
8. Run relevant tests, linters, formatters, type checks, and builds.
9. Update examples/documentation.
10. Perform a final adversarial review of the actual implementation against the specification and this brief, then fix issues found.
11. Leave repositories clean and provide a concise final report: spec changes, implementation, Go/JS parity, execution strategies, adapters changed, tests/checks run, decisions, deferred work, and known limitations.

Use faster/cheaper suitable models or subagents for routine exploration, mechanical work, test generation, and straightforward reviews where supported. Reserve stronger reasoning for semantics, architecture, planning, difficult bugs, and final review.

## DTQL is structured/YAML-based
DTQL is **not textual SQL**. A real existing query is:

```yaml
from: {schema: main, name: Invoice}
orderBy:
  - field: InvoiceId
    desc: true
limit: 10
```

Do not introduce SQL-like textual JOIN syntax. JOIN must be part of DTQL's structured YAML/JSON query model.

# Core design: JOIN belongs inside FROM
`joins` is a child of `from`, not a separate top-level query property. A `from` specification becomes a **recursive relation specification**:

```text
From
├── relation/source identification
├── alias
└── joins[]
    ├── type
    ├── from: From
    └── on[]
```

Example:

```yaml
from:
  schema: main
  name: Invoice
  as: i
  joins:
    - type: inner
      from: {schema: main, name: Customer, as: c}
      on:
        - left: i.CustomerId
          op: eq
          right: c.CustomerId
orderBy:
  - field: i.InvoiceId
    desc: true
limit: 10
```

This recursive shape is intentional. It enables nested joins, multiple joins at every node, reusable relation trees, future FROM references, future cross-source joins, and clean DALGO execution planning. Do **not** flatten JOINs into a top-level property.

# Nested JOINs are MVP functionality
Nested joins are required now, not deferred:

```yaml
from:
  schema: main
  name: Invoice
  as: i
  joins:
    - type: inner
      from:
        schema: main
        name: Customer
        as: c
        joins:
          - type: left
            from: {schema: main, name: Employee, as: e}
            on:
              - left: c.SupportRepId
                op: eq
                right: e.EmployeeId
      on:
        - left: i.CustomerId
          op: eq
          right: c.CustomerId
```

Semantically:

```text
Invoice i
└── INNER JOIN Customer c
    └── LEFT JOIN Employee e
```

Support arbitrary recursive nesting rather than an artificial depth limit. Detect/reject malformed cycles if in-memory models can contain them. Every `from` node may contain multiple sibling joins. `joins` is an **ordered list**, not a map.

# JOIN types
MVP must support `inner` and `left`. Omitted `type` means `inner`. Design the representation so `right`, `full`, `cross`, etc. can later be added without redesign. Do not silently accept unsupported types. Additional types are optional only if trivial and safe.

# ON predicates
Use structured predicates, never raw SQL:

```yaml
on:
  - left: i.CustomerId
    op: eq
    right: c.CustomerId
```

Multiple predicates are implicitly ANDed. MVP requires equality (`eq`). Design so `ne`, `lt`, `lte`, `gt`, `gte`, etc. can later be added without changing the structure. Inspect existing DTQL filter/expression abstractions and reuse/generalise them where appropriate, but do not force an unsuitable WHERE representation onto JOINs.

# ON scoping semantics
This semantic is approved and must be explicitly specified and tested:

> A JOIN's `on` predicates may reference fields from its own joined `from` subtree and any relation already visible in its parent scope.

JOIN order is therefore semantically significant. A later sibling may reference relations introduced by an earlier sibling when visible in that scope. A JOIN may not reference a relation that has not yet been introduced.

Example:

```yaml
from:
  name: A
  as: a
  joins:
    - from: {name: B, as: b}
      on:
        - {left: a.id, op: eq, right: b.aId}
    - from: {name: C, as: c}
      on:
        - {left: b.id, op: eq, right: c.bId}
```

The second JOIN may use `b`; an earlier JOIN referring to a later alias must fail. Define nested visibility rigorously. Test parent/child references, nested references, earlier siblings, invalid forward references, unknown/duplicate aliases, ambiguous fields, and nested-scope visibility. Never silently guess.

# Aliases and field references
Aliases are supported at every FROM node. JOIN queries need qualified fields such as `i.InvoiceId`, `c.CustomerId`, `e.EmployeeId`. Inspect existing DTQL field-reference representation first. Reuse structured references if canonical; retain dotted strings if canonical. Unqualified fields may work only where existing DTQL permits and resolution is unambiguous. Ambiguity must error.

# Recursive FROM as reusable relation abstraction
Treat recursive FROM as a reusable relation tree, not merely JOIN syntax. A future facility may support:

```yaml
from:
  ref: InvoiceWithCustomer
```

Do not implement FROM references unless an existing mechanism makes it naturally part of this work, but preserve the extension point and document the motivation.

# DTQL versus DALGO responsibilities
**DTQL defines what relation/query is requested.** It owns recursive FROM/JOIN structure, type, ON conditions, aliases, and backend-independent semantics/validation.

**DALGO decides how it is executed.** The same query may be executed natively, translated by an adapter, composed by DALGO from multiple reads, or eventually use mixed native/DALGO execution. Do not leak execution choices into DTQL.

# DALGO execution architecture
Not every adapter supports native JOINs—especially Firestore, document/key-value stores, HTTP/API sources, etc. JOIN support cannot simply be implemented in SQL adapters.

Conceptually:

```text
DTQL relation tree
        ↓
    validation
        ↓
      planner
        ↓
 native subtree and/or generic DALGO join
```

Inspect existing capability mechanisms first. Extend them if suitable; otherwise introduce the minimum coherent abstraction required. Avoid database-specific JOIN conditionals scattered through generic DALGO code.

# Generic DALGO JOIN execution
When an adapter cannot execute a JOIN natively, DALGO should execute supported JOINs itself. Correctness comes first, but avoid obvious N+1 behaviour. For equality joins consider hash joins, an index/map over one side, batched reads, adapter `IN` queries, and streaming where practical. Do not build a sophisticated cost-based optimiser for MVP. Do not design an implementation inherently requiring one backend query per left row when straightforward batching/hash execution is available. Document limitations.

# Native pushdown
When an adapter can execute a complete JOIN subtree natively, allow pushdown. SQL/SQLite adapters are obvious candidates. Inspect existing adapters and implement native translation where reasonably scoped. Do not claim native capability unless implemented and tested. Generic correctness matters more than maximum adapter coverage.

# Mixed execution
Design for relation trees where some subtree can execute natively but another boundary requires DALGO composition. Implement mixed execution now if cleanly feasible; otherwise explicitly defer it while ensuring the architecture does not block it, unsupported plans fail clearly, and no incorrect result is silently returned.

# Cross-database / cross-source JOINs
Cross-source joins are an intentional future use case, especially for DataTug. Inspect how DTQL/DALGO identifies databases, connections, schemas, tables/collections, and sources. Avoid assumptions that permanently require every node to belong to one SQL database. Full distributed cross-source execution is not required unless current abstractions make it straightforward.

# INNER and LEFT semantics
INNER JOIN: only combinations satisfying all ON predicates survive; multiple matches produce multiple result combinations; unmatched left rows are removed.

LEFT JOIN: every qualifying left-side row remains when the right side has no match. Joined-side values must use existing DALGO null/missing semantics consistently. Inspect distinctions among null, absent/missing, Go zero value, and JS `undefined`; ensure Go and JS are semantically equivalent.

Explicitly test nested combinations such as:

```text
A
└── LEFT B
    └── LEFT C
```

and:

```text
A
└── LEFT B
    └── INNER C
```

Make semantics deliberate, not accidental.

# Result shape
Inspect current DALGO row/result representation before changing it. Specify how joined fields are exposed: flattened vs nested, alias participation, duplicate names, SELECT addressing, unmatched LEFT values, and schema metadata. Prefer compatibility. If insufficient, make the smallest coherent extension and implement parity in Go and JS.

# Interaction with existing DTQL features
JOINs must compose with existing features where implemented: WHERE, SELECT, negative/excluded SELECT fields, ORDER BY, LIMIT, OFFSET, GROUP BY, HAVING, COUNT, COUNT DISTINCT, SUM, AVG, MIN, MAX, FIRST, LAST, and other query operations. Do not regress non-JOIN queries.

Explicitly define logical semantics/order. A conceptual model may be:

```text
FROM + JOIN relation construction
            ↓
          WHERE
            ↓
     GROUP / aggregate
            ↓
          HAVING
            ↓
          SELECT
            ↓
 ORDER / LIMIT / OFFSET
```

But reconcile this with the actual DTQL specification rather than blindly imposing it. Pay particular attention to one-to-many joins followed by aggregates, LEFT JOIN + COUNT, and COUNT DISTINCT.

# Validation and diagnostics
Provide useful errors for at least: missing JOIN `from`; missing `on` where required; unsupported join type/operator; malformed ON; unknown/duplicate alias; ambiguous field; invalid forward reference; field unavailable in scope; malformed recursive FROM; cycles where applicable; unsupported execution plan; adapter capability mismatch; incompatible types when safely detectable.

Include useful context such as alias, relation, field, JOIN index/path, and operator. Avoid generic `invalid join` errors when precise diagnostics are possible.

# Go and JS parity
The DTQL contract is language-independent. Do not independently invent subtly different semantics. Prefer shared YAML/JSON fixtures where practical; otherwise equivalent fixtures with identical expectations. Verify parity for serialization, defaults, type names, nested FROM, ON, scoping, validation, null/missing LEFT behaviour, field resolution, result shape, ordering, and aggregate interactions.

# Compatibility
Existing DTQL must continue working unchanged:

```yaml
from: {schema: main, name: Invoice}
orderBy:
  - field: InvoiceId
    desc: true
limit: 10
```

Avoid unnecessary public API breakage. If unavoidable, document migration. Adapters without native JOIN support must not pretend they support it.

# Required tests
Cover at minimum:

- INNER matches and unmatched rows.
- LEFT matches and preservation of unmatched rows.
- omitted type defaults to INNER.
- one-to-many and duplicate matches.
- two/three sibling joins.
- later sibling referencing earlier visible alias.
- invalid forward reference.
- one nested join and several nesting levels.
- nested INNER, nested LEFT, INNER→LEFT, LEFT→INNER.
- multiple joins at multiple levels.
- one and multiple equality ON predicates.
- unknown/duplicate aliases, ambiguous references, invalid/unsupported operators.
- JOIN + WHERE, SELECT, exclusions, ORDER BY, LIMIT/OFFSET, GROUP BY, aggregates, HAVING.
- null/missing join keys and unmatched joined values.
- YAML/JSON/model serialization round trips.
- existing non-JOIN test suites unchanged.
- equivalent Go/JS behaviour.

Prefer an existing realistic fixture such as **Chinook** if already available.

# End-to-end acceptance example
Ensure something equivalent to this works end-to-end:

```yaml
from:
  schema: main
  name: Invoice
  as: i
  joins:
    - from:
        schema: main
        name: Customer
        as: c
        joins:
          - type: left
            from: {schema: main, name: Employee, as: e}
            on:
              - left: c.SupportRepId
                op: eq
                right: e.EmployeeId
      on:
        - left: i.CustomerId
          op: eq
          right: c.CustomerId
orderBy:
  - field: i.InvoiceId
    desc: true
limit: 10
```

Also test filtering and SELECT against joined fields.

# Specification requirements
Before substantial implementation, make the canonical DTQL specification authoritative about:

1. recursive FROM;
2. location/order of `joins`;
3. JOIN structure and supported types;
4. default INNER type;
5. ON representation and multiple predicates;
6. ON scoping;
7. alias visibility and sibling ordering;
8. nested semantics;
9. INNER and LEFT semantics;
10. field qualification/resolution and ambiguity;
11. validation/errors;
12. result semantics;
13. WHERE/SELECT/ORDER/LIMIT/OFFSET interaction;
14. GROUP BY/HAVING/aggregate interaction;
15. serialization;
16. backend-independent nature of DTQL;
17. future reusable FROM references;
18. future cross-source joins.

Use examples liberally. Go and TypeScript must implement this specification rather than becoming competing de facto specifications.

# Suggested milestones and success criteria

## 1. Discovery + specification
Inspect repos, save this brief, update canonical DTQL spec, review it adversarially.

**Success:** semantics are unambiguous enough for independent Go and JS implementations to produce equivalent results.

## 2. Shared model + validation
Implement recursive FROM/JOIN model, serialization, aliases, ON predicates, scoping, validation, and equivalent fixtures.

**Success:** both languages parse/serialize/validate the same representative nested queries and reject the same invalid structures.

## 3. Generic execution
Implement correct INNER and LEFT execution, siblings and nesting, avoiding obvious N+1 behaviour.

**Success:** semantic tests pass in both implementations including nested LEFT/INNER combinations.

## 4. DTQL composition
Integrate JOIN results with existing WHERE/SELECT/ORDER/LIMIT/OFFSET/GROUP/HAVING/aggregates.

**Success:** combined feature tests pass without regressions to non-JOIN queries.

## 5. Native adapter execution
Add native pushdown to suitable adapters where reasonably scoped and capability reporting/selection as needed.

**Success:** native adapters execute supported JOIN trees correctly and unsupported adapters fall back or fail explicitly according to the plan.

## 6. Parity + final review
Run complete checks, compare Go/JS semantics, adversarially review actual code/spec/tests, fix findings, update docs.

**Success:** clean builds/tests, documented limitations, no known semantic divergence, and the end-to-end Chinook-style example works.

# Engineering principles
- Prefer a small coherent abstraction over adapter-specific hacks.
- Preserve existing APIs and semantics where possible.
- Do not make SQL the semantic definition of DTQL.
- Keep DTQL declarative and backend-independent.
- Make recursive FROM a first-class relation model.
- Make nested joins genuinely work in MVP.
- Fail explicitly rather than silently producing partial/incorrect results.
- Keep Go and JS semantics aligned.
- Optimise obvious pathological execution, but avoid premature optimiser complexity.
- Write the specification and tests so future RIGHT/FULL/CROSS joins, richer predicates, reusable FROM references, and cross-source execution can be added without redesigning the core model.

Proceed autonomously from discovery to a fully implemented, tested, documented result.
