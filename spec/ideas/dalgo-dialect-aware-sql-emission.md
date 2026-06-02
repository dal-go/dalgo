# Idea: DALgo dialect-aware SQL emission

**Status:** Draft
**Date:** 2026-05-15
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let DALgo's StructuredQuery.String() emit SQL dialect-appropriate output (e.g. ANSI LIMIT N vs T-SQL TOP N, single-quoted vs bracketed identifiers, etc.) so that text-passthrough drivers like dalgo2sql can hand the result verbatim to any supported backend without local rewrites?

## Context

DALgo's `structuredQuery.String()` emits a single hardcoded SQL flavor (T-SQL: `SELECT TOP N ... OFFSET M`). Drivers like `dalgo2sql` that consume `String()` verbatim and forward to engines (`dalgo2sqlite`, hypothetical `dalgo2postgres`) then trip on dialect mismatches:

| Dialect | SELECT LIMIT | Identifier quoting | Boolean literal | NULL test |
|---|---|---|---|---|
| ANSI / SQL-92 | `LIMIT N` (trailing) | `"x"` | `TRUE`/`FALSE` | `IS NULL` |
| SQLite | `LIMIT N` | `"x"` (accepts `[x]`, `` `x` ``) | `1`/`0` (also accepts `TRUE`/`FALSE`) | `IS NULL` |
| PostgreSQL | `LIMIT N` | `"x"` | `TRUE`/`FALSE` | `IS NULL` |
| MySQL | `LIMIT N` | `` `x` `` (accepts `"x"` with ANSI_QUOTES) | `1`/`0` (also accepts `TRUE`/`FALSE`) | `IS NULL` |
| T-SQL (SQL Server) | `SELECT TOP N ...` (leading, before columns) | `[x]` (accepts `"x"` with QUOTED_IDENTIFIER ON) | `1`/`0` | `IS NULL` |

The triggering case: datatug-cli's Task 7 (db-copy filtering) needed `LIMIT N` for SQLite and got T-SQL `TOP N`. The current stopgap is a `dalgo2sql/sqlite_emit.go` shim that takes dalgo's `String()` output and rewrites `SELECT TOP N` into trailing `LIMIT N`. That works for one dialect via string rewriting, but it's fragile and won't scale to identifier quoting / boolean literals / structurally-different dialects.

`dalgo2ingitdb` sidesteps `String()` entirely — it consumes `StructuredQuery` fields directly. That escape hatch is good for non-SQL backends but doesn't help SQL drivers that need a text query.

## Recommended Direction

Introduce a `Dialect` interface in `dal-go/dalgo/dal/dialect/` that exposes the small set of *parameterized* SQL-formatting decisions, and refactor `structuredQuery.String()` to take an optional `Dialect` parameter. The interface is composable (drivers implement only what they need) and the default behavior is ANSI/SQL-92 emit.

```go
// dal/dialect/dialect.go
package dialect

type Dialect interface {
    // QuoteIdentifier wraps a table/column name per dialect rules.
    // ANSI: `"x"`; T-SQL: `[x]`; MySQL: `` `x` ``.
    QuoteIdentifier(name string) string

    // LimitOffsetClause returns the clause text appended after ORDER BY
    // for ANSI dialects (`"\nLIMIT N\nOFFSET M"`), or a structural override
    // returned from EmitTopClause for dialects like T-SQL.
    LimitOffsetClause(limit, offset int) string

    // EmitTopClause returns text emitted between `SELECT` and the column
    // list — empty string for ANSI dialects; `"TOP N "` for T-SQL.
    // Only one of EmitTopClause/LimitOffsetClause may be non-empty for a
    // given (limit, offset) tuple.
    EmitTopClause(limit int) string

    // BooleanLiteral renders true/false per dialect rules
    // (`TRUE`/`FALSE` ANSI; `1`/`0` SQLite/MySQL/T-SQL).
    BooleanLiteral(b bool) string

    // EscapeStringLiteral wraps a string value in dialect-appropriate quotes.
    EscapeStringLiteral(s string) string
}
```

The core `StructuredQuery` gains a `StringForDialect(d Dialect) string` method (or the existing `String() string` is reinterpreted as `StringForDialect(dialect.ANSI)`). `dal-go/dalgo/dal/dialect/` ships four pre-built implementations: `ANSI`, `SQLite`, `Postgres`, `TSQL`, plus `MySQL`. Each implementation is small (~20 LoC) and focused. Drivers like `dalgo2sql` receive a `Dialect` via their `NewDatabase(...)` constructor (or auto-detect from the SQL driver name — e.g. `database/sql.OpenDB("sqlite3", ...)` implies `dialect.SQLite`). The existing `String()` continues to work — deprecated for one release cycle, then removed.

Three structural patterns the interface handles:
1. **Trailing-clause dialects** (ANSI, SQLite, Postgres, MySQL): `LimitOffsetClause` returns text appended after ORDER BY; `EmitTopClause` returns `""`.
2. **Leading-clause dialects** (T-SQL): `EmitTopClause` returns `"TOP N "` inserted between `SELECT` and the column list; `LimitOffsetClause` returns `""`. (T-SQL's OFFSET/FETCH-NEXT-only form for OFFSET without LIMIT is the one edge case where this gets weird — punt to "explicit ORDER BY required" with a clear error.)
3. **Identifier/literal escaping**: every emit-site that writes a name or a literal calls through the Dialect interface instead of inlining quotes.

This lets the SQL skeleton stay shared across dialects (one `structuredQuery.StringForDialect()` implementation, not four parallel emitters), with per-dialect rules pluggable through a tiny interface. New dialects (Oracle, DB2, etc.) add one ~30-line file in `dal/dialect/` without touching the core emit code.

## Alternatives Considered

- **Big enum + giant switch (`StringForDialect(d Dialect) string` where `Dialect` is an `int8`).** Simplest possible: every per-dialect difference is a `switch d {}` block inside one monolithic emit function. Lost because: (a) adding a new dialect requires editing `structuredQuery.StringForDialect()` directly — touches the highest-traffic core file every time; (b) dialect-specific quirks accumulate in one place rather than co-located with the dialect they belong to; (c) testing requires golden-file fixtures across every dialect × every clause combination. The interface approach trades a small amount of indirection for separability and per-dialect testability.

- **Driver-supplied full Emitter interface** (drivers implement `EmitSelect(q) string`, `EmitInsert(q) string`, etc. — no shared skeleton). Maximum extensibility — a driver can emit any SQL shape it wants. Lost because: (a) every driver re-implements the same SELECT/FROM/WHERE skeleton with subtle drift; (b) consistency across drivers becomes a code-review concern, not an enforced contract; (c) onboarding a new dialect requires reading the existing implementations to understand the conventions, rather than implementing a tiny interface against a documented spec. Reserve this as the escape hatch — drivers can still override the StringForDialect path entirely (e.g. by intercepting before it's called) when a dialect needs structurally different SQL.

- **Two-stage: emit ANSI from core; drivers post-process via `Postprocess(text string) string` hook.** Formalizes the current `dalgo2sql/sqlite_emit.go` shim. Lost because: (a) string post-processing on generated SQL is fragile — the regex/match patterns depend on emit details that the core could change at any time without notifying post-processors; (b) it can't handle structurally-different dialects (T-SQL's `TOP N` must be inserted between `SELECT` and the column list — post-processing the ANSI form would need a brittle "find SELECT then ORDER BY then move LIMIT to between them" rewrite); (c) doesn't help with identifier quoting (every quoted identifier in the SQL would need to be detected and re-quoted). Worth keeping as an emergency override for unusual cases but unsuitable as the primary mechanism.

- **Context-based dialect propagation** (Dialect carried on `context.Context` rather than passed as a method parameter). Avoids method-signature changes. Lost because: (a) ambient context for SQL emission is bad practice — emit is a pure function of (query, dialect); context entanglement makes testing painful; (b) drivers would have to remember to set it, leading to silent ANSI fallback on missed sites; (c) goroutine/request boundaries make context less reliable than an explicit parameter.

## MVP Scope

A two-week spike landing the `dal/dialect/` package with:
1. The `Dialect` interface (five methods listed in Recommended Direction).
2. Four concrete implementations: `dialect.ANSI`, `dialect.SQLite`, `dialect.Postgres`, `dialect.TSQL`. (`dialect.MySQL` deferred unless a real consumer surfaces.)
3. `structuredQuery.StringForDialect(d Dialect) string` method that walks the existing fields and calls through the Dialect interface at the four parameterized points (TOP/LIMIT, identifier quoting in `FROM`, boolean literals in WHERE comparisons, string-literal escaping in WHERE comparisons).
4. Existing `String() string` becomes a one-line wrapper: `func (q structuredQuery) String() string { return q.StringForDialect(dialect.ANSI) }`. The current T-SQL-flavored emit is retired (an explicit dialect choice that was never advertised as such).
5. `dalgo2sql.NewDatabase(...)` accepts an optional `Dialect` parameter (defaults to `dialect.ANSI` for backward compatibility, with `dalgo2sqlite` and a future `dalgo2postgres` setting `dialect.SQLite` / `dialect.Postgres` respectively).
6. Golden-file table-driven tests for each dialect × each clause: `dal/dialect/testdata/<dialect>_select_*.golden`. The matrix: 4 dialects × 5 query shapes (no-LIMIT, LIMIT-only, OFFSET-only, LIMIT+OFFSET, WHERE-with-boolean) = 20 golden files. Diffable, reviewable, regression-safe.

datatug-cli's `dalgo2sql/sqlite_emit.go` shim is removed; datatug-cli's `replace` directive for `dalgo2sql` is dropped; the Task 7 LIMIT test runs against tagged versions of both `dalgo` and `dalgo2sql` with the new emission pipeline.

## Not Doing (and Why)

- Per-call dialect override beyond the driver default — drivers know their own flavor, callers shouldn't have to
- Full SQL dialect normalization across every conceivable difference — only the cases that bite real consumers (LIMIT/OFFSET, identifier quoting, boolean literals) in MVP
- Breaking String() in the same release — keep it as a deprecated ANSI alias for at least one cycle to avoid breaking downstream callers
- Dialect-aware Condition.String() — leaf condition formatting is mostly ANSI-compatible; revisit if a real divergence surfaces

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The four target SQL dialects (ANSI, SQLite, Postgres, T-SQL) share enough structural overlap — same overall `SELECT ... FROM ... WHERE ... GROUP BY ... ORDER BY ...` skeleton — that a single emit walker driven by a small Dialect interface can express all four without one needing a structurally different skeleton. | Build the prototype emitter for ANSI; layer SQLite (only `LIMIT` semantics differ); layer T-SQL (`TOP N` position differs but is the only structural divergence — handled via `EmitTopClause` vs `LimitOffsetClause` mutual-exclusion). If a fifth dialect (Oracle? DB2?) needs a fundamentally different skeleton, escape hatch via Alternative #2 (driver-supplied full Emitter) and document the boundary. |
| Must-be-true | The five Dialect interface methods (`QuoteIdentifier`, `LimitOffsetClause`, `EmitTopClause`, `BooleanLiteral`, `EscapeStringLiteral`) cover ≥95% of real-world dialect divergences for the SELECT path. | Walk every per-dialect difference cataloged in the Context table; verify each can be expressed as one of the five hooks. If a sixth method becomes necessary mid-MVP, add it and update the Idea. |
| Must-be-true | `dalgo2sql`'s `Database` constructor can accept an optional Dialect without breaking existing callers. | Add `Dialect` as a functional-option parameter (`dalgo2sql.NewDatabase(driverName, dsn, dalgo2sql.WithDialect(dialect.SQLite))`); default to `dialect.ANSI` when omitted. Existing `dalgo2sql.NewDatabase(driverName, dsn)` callers see no behavior change *as long as* the new ANSI default emits SQL the existing engines (SQLite, MySQL, Postgres) all accept — which it does for the no-LIMIT case (most existing usage) and for the LIMIT case (all three accept trailing `LIMIT N`). T-SQL callers must opt in explicitly. |
| Should-be-true | INSERT, UPDATE, DELETE emit paths also need dialect awareness but the dialect surface for those is smaller (identifier quoting + literal escaping, no LIMIT/TOP analog). | Audit `dal/q_*.go` for emit code; confirm INSERT/UPDATE/DELETE only call through the two literal/identifier hooks. Extend the prototype to cover them in the same spike. |
| Should-be-true | Condition.String() (the WHERE-clause fragment formatter) is mostly ANSI-compatible and doesn't need a separate dialect-aware variant for MVP. | Read `q_conditions.go`, `q_field_ref.go`, `q_group_condition.go`. Verify the only emit-time divergences are literal/identifier formatting — which the Dialect interface handles via `BooleanLiteral` and `EscapeStringLiteral`. If anything else surfaces (e.g. dialect-specific `IS NULL` form), extend the interface. |
| Should-be-true | Golden-file tests with 20 fixtures are tractable to maintain and easy to update as the emit format evolves. | Standard Go testing pattern; precedent in the dalgo codebase if any similar tests exist. Set up `testdata/` directory and `go test -update` flag for regenerating goldens. |
| Might-be-true | A future Oracle or DB2 dialect will need additional Dialect methods. | Defer; revisit when a real Oracle/DB2 driver lands. Likely candidates: `EmitFromDual()` for Oracle's `SELECT 1 FROM DUAL`, `EmitOffsetFetchNext()` for T-SQL OFFSET-without-LIMIT cases. |
| Might-be-true | Some dialects require parameterized query placeholders (`?` vs `$1` vs `:name`) and the Dialect should also format those. | Defer; current emit hardcodes `?`-style placeholders via `database/sql` driver translation. Revisit if any driver needs different placeholder rendering at emit time. |
| Might-be-true | The legacy `String() string` method's T-SQL-flavored output is relied upon by an existing consumer who would break under the ANSI-default switch. | Audit grep across known dalgo consumers (`dalgo2sql`, `dalgo2sqlite`, `dalgo2ingitdb`, datatug-cli) for `.String()` calls on structured queries; document which dialect each expects today. Likely outcome: nobody actually depends on T-SQL-specific behavior — the historical default was an accident, not a contract. |


## SpecScore Integration

- **New Features this would create:**
  - `dal-go/dalgo/spec/features/dal/dialect/` — the `Dialect` interface + five built-in implementations (`ANSI`, `SQLite`, `Postgres`, `TSQL`, `MySQL`).
  - `dal-go/dalgo/spec/features/dal/query-emit/` — the `StructuredQuery.StringForDialect(d Dialect) string` method and its golden-file test matrix.
- **Existing Features affected:**
  - `dal-go/dalgo/dal/q_*.go` files (query_struct.go, q_field_ref.go, q_group_condition.go, q_conditions.go, q_builder.go) — emit code refactored to call through Dialect at five hook points.
  - `dal-go/dalgo2sql` — gains a `WithDialect(...)` functional option; existing constructor stays compatible.
  - `ingitdb/ingitdb-cli/pkg/dalgo2ingitdb` — unaffected (consumes StructuredQuery fields directly, not `String()`).
  - `datatug/datatug-cli` — drops the `dalgo2sql/sqlite_emit.go` shim and the worktree `replace` directive; updates `engine_rows.go` to pass `dialect.SQLite` via `dalgo2sql.WithDialect` when opening a `sqlite://` source.
- **Dependencies:**
  - None upstream. This is the leaf Idea — sibling work cascades downstream from it.
  - **Downstream consumers waiting on this:** `dal-go/dalgo2sql`'s `emitSQL` shim removal; `datatug/datatug-cli`'s LIMIT-N test infrastructure consolidation; future PostgreSQL DALgo driver.

## Open Questions

- **Dialect propagation mechanism: constructor option vs auto-detect from SQL driver name vs both.** The plan picks "constructor option, defaults to ANSI". An alternative is to auto-detect from the `database/sql` driver name passed to `dalgo2sql.NewDatabase` (e.g. `"sqlite3"` → `dialect.SQLite`, `"postgres"` → `dialect.Postgres`, `"sqlserver"` → `dialect.TSQL`). Auto-detect is friendlier but introduces a string-keyed map at the dalgo2sql boundary that could go stale. Plan-time decision: probably "explicit option, but provide `dalgo2sql.AutoDialect(driverName)` as a convenience that returns the right Dialect for known names".
- **Where does `Dialect` live?** Option A: `dal-go/dalgo/dal/dialect/` (in core, ships with every dalgo install — recommended). Option B: `dal-go/dalgo-dialects` (separate module, drivers depend on the dialects they need). Plan picks A for ergonomic reasons (one import, no version skew across modules); B is the answer if dalgo core wants to stay zero-dependency.
- **Identifier quoting policy: always quote, or only when needed?** ANSI-quoting every identifier (`"users"`, `"created_at"`) is safe but ugly when readable SQL is needed for logging. T-SQL `[users]` reads better. SQLite/Postgres accept unquoted lowercase identifiers but break on reserved-word names. Plan: always quote — readability is sacrificed for safety. (Logging-friendly emit is a separate concern; debug output can pass through `dialect.UnquotedANSI` if anyone wants it.)
- **Migration cadence for the legacy `String()` method.** Plan picks "one release cycle of deprecation, then remove". Open question: should removal happen at all, or should `String()` stay as a permanent alias for `StringForDialect(dialect.ANSI)`? The arguments cut both ways — keeping it forever is friendlier to downstream consumers; removing it eliminates the "what does `String()` actually emit?" ambiguity that caused this Idea in the first place.
- **Operator-token escape-hatch for dialect-specific operators.** The `dal.Operator` constants today are typed `Operator string` with values like `"=="` for `Equal`. SQLite/MySQL/Postgres all accept `=` (single equals). If the operator constants' string values are dialect-specific (T-SQL also accepts `=`, but the existing constant is `"=="`), should the Dialect have an `OperatorText(op Operator) string` hook too? Plan-time decision.
- **Test-fixture management.** Golden files are easy to update with `go test -update`, but reviewing 20+ files of SQL text in a PR is tedious. Plan: organize fixtures by query-shape × dialect; provide a `dialect/testdata/README.md` that explains the naming convention and the `-update` workflow.

---
*This document follows the https://specscore.md/idea-specification*
