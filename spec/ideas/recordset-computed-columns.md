---
format: https://specscore.md/idea-specification
status: Specified
---

# Idea: Computed Columns in recordset (neutral evaluator)

**Status:** Specified
**Date:** 2026-06-02
**Owner:** alex
**Promotes To:** recordset-computed-columns
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we give dalgo a backend-agnostic notion of computed columns — so a recordset row can lazily expose derived values by name — without dragging a scripting engine into the abstraction layer?

## Context

Triggered by a request to move the computed-columns computation engine out of ingitdb-cli and into dalgo. ingitdb already ships an approved computed-columns Feature backed by inline Starlark formulas (parse/validate/compile + read-time evaluation). The proposal: ingitdb keeps FORMULA declarations but delegates the *computation wiring* to dalgo, passing a computed-column definition into the recordset and letting returned rows provide computed values via GetValue(name). Grounding in dalgo: recordset.Row already exposes GetValueByName/GetValueByIndex and recordset.Column[T] is columnar; dal.Record is only a Data()-any gateway; dalgo also already has an Expression AST (q_expression/q_functions); record.DataToMap already yields stored-fields-as-map. Related approved ideas: [[recordops]].

**Verified against the code (the carrier question).** ingitdb's *working* `select` path does NOT flow through `recordset.Row` today: `ExecuteQueryToRecordsReader` returns a `dal.RecordsReader` of `dal.Record`, and computed values are applied **eagerly** by `ApplyFormulasToRead(data, cols, …)` baking results straight into the record's `map[string]any` (`pkg/dalgo2ingitdb/tx_readonly.go:63-64,86-87`). The `recordset.Row` path *exists in the interface but is an unimplemented stub* — `ExecuteQueryToRecordsetReader(…) (dal.RecordsetReader, error)` returns `dal.ErrNotSupported` in all three backends (`dalgo2ingitdb`, `dalgo2fsingitdb`, `dalgo2ghingitdb`). dalgo already declares the target rail: `RecordsetReader.Next() (recordset.Row, recordset.Recordset, error)` (`dal/reader.go:22`). **Decision (owner): commit to the `recordset.Row` migration** — implement the stubbed recordset query path in ingitdb and flip its computed-value model from eager-into-map to lazy-on-access. This grows the scope below.

## Recommended Direction

Add a **backend-neutral computed-column capability** to dalgo's `recordset` package, built on three small pieces and zero scripting dependency.

**1. A neutral `Evaluator` contract — the "precompiled formula".** dalgo defines an interface, not a language:

```go
// in package recordset
type Evaluator interface {
    Eval(stored map[string]any) (value any, err error)
}
```

`stored` is the row's stored (non-computed) fields, which dalgo already produces via `record.DataToMap`. dalgo never parses, validates, or compiles anything — it holds an opaque `Evaluator` and calls it. ingitdb compiles its inline Starlark expression **once** at schema-load time (work it already does) into a struct that implements `Evaluator`; `starlark-go` stays entirely inside ingitdb. The signature passes the full stored-field map rather than a pull-on-demand getter precisely because expression engines like Starlark bind all in-scope variables up front.

**2. A `ComputedColumn` definition registered on the Recordset.** A computed column pairs a name + declared type with an `Evaluator`. The Recordset knows which columns are stored and which are computed; each `Row` carries a pointer to the recordset's computed-column set (your "pointer to precompiled formulas").

**3. Lazy resolution through the existing `Row` interface.** `recordset.Row` already exposes `GetValueByName(name, rs) (any, error)` and `GetValueByIndex`. When the requested column is computed, the Row resolves it lazily: build the stored-field map, call `Evaluator.Eval`, **memoize** the result on the row, and return `(value, err)`. The existing `(any, error)` return already carries fail-loud semantics — no new error channel needed. Coercion to the declared column type can ride on the existing typed-column machinery or be left to the consumer; the MVP returns the evaluator's value and notes coercion as a design detail.

This gives the user exactly the requested `record.GetValue(columnName)` ergonomics and the "record holds a pointer to precompiled formulas" model — while keeping dalgo a thin, dependency-light abstraction. ingitdb's role shrinks to: validate + compile formulas at load time, wrap each as an `Evaluator`, register them as computed columns, and stop owning the read-path evaluation wiring.

**4. ingitdb adopts the `recordset.Row` query path.** Because ingitdb's live `select` is `dal.Record`-based with eager-into-map computation (and the `recordset` path is an unimplemented stub), realizing the lazy model requires ingitdb to: (a) implement `ExecuteQueryToRecordsetReader` so `select` materializes a `recordset.Recordset`/`Row`; (b) migrate the CLI `select` consumer and renderers off `dal.RecordsReader` onto the recordset reader (or a bridge); and (c) replace the eager `ApplyFormulasToRead`-into-map step with computed-column registration so values resolve lazily through `GetValueByName`. This is the committed direction, not a tweak — sequence it so the dalgo contract lands and is proven first, then ingitdb's path is migrated.

## Alternatives Considered

- **Embed Starlark (`starlark-go`) directly in dalgo.** Lost decisively: dalgo is a multi-backend abstraction (Firestore, memory, fs, SQL). Baking a scripting runtime into it forces that dependency — and its security/sandbox surface — onto every dalgo consumer, including ones that never want formulas. The neutral `Evaluator` interface gets the same capability with none of the coupling.
- **Reuse dalgo's existing `Expression` AST (`q_expression`/`q_functions`).** Tempting because dalgo already models "FieldRef, Constant, or a formula." Lost because the AST is far less expressive than Starlark (string methods, `min`/`max`/`round`/`floor`/`ceil`, etc.), and ingitdb has already invested in a Starlark engine with a curated deterministic helper set; re-authoring every formula against a weaker AST is a regression with no payoff for the MVP. The `Evaluator` interface does not preclude an AST-backed implementation later.
- **Eager evaluation in a read stage (ingitdb's current model).** Lost on the chosen axis: lazy-on-access avoids computing columns a `select` projection never returns, and matches the "row holds precompiled formulas" mental model. (Trade-off captured as an assumption: fail-loud now fires at access time, not unconditionally at read.)
- **Carry `GetValue(name)` on the `dal.Record` gateway.** Lost because `dal.Record` is a `Data() any` gateway, not a columnar accessor; `recordset.Row` already has `GetValueByName`/`GetValueByIndex` and is the natural columnar home.
- **Leave computation entirely in ingitdb (status quo).** Lost because no other dalgo backend can reuse it, and the explicit goal is to relocate the computation wiring into dalgo so the capability is shared.

## MVP Scope

Two sequenced phases. **Phase A** (dalgo, ~1 week) lands and proves the contract; **Phase B** (ingitdb) migrates the real `select` path onto it. Phase A must be green before Phase B starts.

**Phase A — dalgo contract (the reusable core):**

1. **`recordset.Evaluator` interface** — `Eval(stored map[string]any) (any, error)`.
2. **`ComputedColumn` definition** — name + declared type + `Evaluator`, registerable on a Recordset alongside stored columns.
3. **Lazy resolution in `Row`** — `GetValueByName`/`GetValueByIndex` resolve computed columns by calling the `Evaluator` with the row's stored fields, memoize per row, and propagate errors through the existing `(any, error)` return (fail-loud preserved at access).
4. **Tests in dalgo with a fake `Evaluator`** — no Starlark in dalgo's test suite: success, evaluator error, memoization (evaluator called once), stored-vs-computed name resolution, and value-by-index parity.

**Phase B — ingitdb adoption (the committed migration):**

5. **Implement `ExecuteQueryToRecordsetReader`** so `select` materializes a `recordset.Recordset`/`Row`, registering computed columns with ingitdb's Starlark-backed `Evaluator` instead of calling `ApplyFormulasToRead` into the data map.
6. **Migrate the CLI `select` consumer + renderers** off `dal.RecordsReader` onto `dal.RecordsetReader` (or a bridge), reading computed values via `GetValueByName`.
7. **Parity tests** — the existing computed-columns ACs (read output for string/int/math/string-helper formulas) pass through the new lazy path with byte-identical results to the current eager path.

Out of scope even with the migration committed: filter/sort and foreign keys remain on ingitdb's existing eager path for now (the recordset rows still expose computed values, but moving query-time use onto recordset is a later step); formula parsing/validation stays in ingitdb; type-coercion polish beyond returning the evaluator's value; and migrating the `dalgo2fsingitdb`/`dalgo2ghingitdb` backends if Phase B targets only the primary backend first (see Open Questions).

## Not Doing (and Why)

- Embedding Starlark or any formula language in dalgo — dalgo defines only a neutral Evaluator interface; the language stays in the consumer (ingitdb)
- Filtering/sorting on computed columns inside dalgo — stays in ingitdb for this MVP (scope: record value access only)
- Foreign-key / referential-integrity on computed values — stays in ingitdb; arguably ingitdb-specific
- Eager whole-recordset precomputation — evaluation is lazy, on GetValue access
- Formula parsing/validation in dalgo — dalgo receives already-compiled evaluators; ingitdb keeps schema-load-time validation
- Chained computed columns (a formula referencing another computed column) — ingitdb already defers this; dalgo evaluates from stored fields only
- Adding GetValue(name) to the dal.Record gateway — the carrier is recordset.Row, which is already columnar

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true (VALIDATED) | ingitdb's `select` results actually flow through `recordset.Row`. | **Resolved: NO, not today.** Live path is `ExecuteQueryToRecordsReader` → `dal.RecordsReader`/`dal.Record` with eager `ApplyFormulasToRead`-into-map; the `recordset.Row` path is a stub returning `dal.ErrNotSupported` in all three backends. The rail exists (`dal.RecordsetReader.Next() (recordset.Row, recordset.Recordset, error)`). Owner committed to implementing the stub and migrating (Phase B). Risk now lives in Phase B effort, not in feasibility. |
| Must-be-true | ingitdb's precompiled Starlark program can be wrapped to implement `Eval(map[string]any) (any, error)`, including binding all stored fields as Starlark globals from the passed map. | Adapt one existing ingitdb compiled formula to the interface and evaluate it against a sample stored-field map; confirm output matches the current read-path result. |
| Must-be-true | Passing the full stored-field map (vs a pull-on-demand getter) is the right contract for the evaluator. | Confirm against the Starlark binding model (Starlark binds all globals up front, so a single-field getter is insufficient); confirm `record.DataToMap` yields the same field set ingitdb binds today. |
| Should-be-true | Lazy + memoized evaluation preserves ingitdb's observable behavior for output, filtering, and sorting, all of which access the column. | Compare outputs of a computed column read via the new lazy path against ingitdb's current eager read for the same records. |
| Should-be-true | The fail-loud timing shift is acceptable: a runtime error now surfaces when the column is accessed rather than unconditionally at read, so a `select` that never projects an erroring computed column will not abort. | Decide with the ingitdb owner whether AC `runtime-error-fails-read` semantics must hold for unprojected columns; if so, ingitdb forces access to all computed columns, or the evaluation stays eager on its side. |
| Might-be-true | Other dalgo backends (memory, fs, SQL) will want computed columns through the same `Evaluator` contract. | Sketch one non-ingitdb use (e.g. a derived column over the in-memory backend) and confirm the contract fits without change. |


## SpecScore Integration

- **New Features this would create:** (1) a dalgo `recordset` computed-columns Feature (Evaluator interface + ComputedColumn definition + lazy resolution in Row); (2) an ingitdb-cli Feature implementing the stubbed `ExecuteQueryToRecordsetReader` recordset query path and migrating `select` onto it (Phase B).
- **Existing Features affected:** ingitdb-cli `computed-columns` — its read-path evaluation flips from eager `ApplyFormulasToRead`-into-map to lazy `Evaluator` resolution via the dalgo contract; its read ACs must re-pass through the new path.
- **Dependencies:** Phase B depends on Phase A (dalgo contract) being shipped and green first; consumer-side dependency on ingitdb-cli `computed-columns`; sibling/informational only: [[recordops]].

## Open Questions

- Does type coercion (evaluator result → declared column type) live in dalgo (riding the typed `Column[T]` machinery) or stay in the consumer? MVP returns the raw evaluator value; the boundary needs a decision before the Feature is specified.
- Should the `Evaluator` receive the declared target type as a hint (to coerce at eval time), or remain type-agnostic and leave coercion downstream?
- Memoization lifetime and thread-safety: is per-row, single-threaded read sufficient, or must computed values survive across re-reads / concurrent access?
- Phase B backend scope: does the migration target only `dalgo2ingitdb` first, or must `dalgo2fsingitdb` and `dalgo2ghingitdb` (which share the same stub) move together to avoid divergent select behavior across backends?
- Does the CLI `select` consumer migrate fully to `dal.RecordsetReader`, or do both reader paths coexist behind a bridge during the transition?

---
*This document follows the https://specscore.md/idea-specification*
