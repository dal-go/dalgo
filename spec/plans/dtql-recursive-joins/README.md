---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Recursive DTQL JOINs in Go and TypeScript

**Status:** Draft
**Source Feature:** dtql-recursive-joins
**Date:** 2026-09-21
**Owner:** alex
**Supersedes:** —

## Summary

The recursive JOIN base landed in DALgo Go at `d393914bf9f9fe4a09fbba3188219e24f86ea284` and was released as `v0.84.0`. Tasks 1–9 below preserve the original plan; their `planning` labels describe the plan when authored and are not a new work queue. At this follow-up's start, SQL PR #191 and DALgo-JS PR #3 were open, and SQLite integration was in progress. Task 10 is the new algorithm-hint scope. Land each remaining repository only after checking its current head, CI, and release artifacts; do not infer completion from this snapshot.

## Approach

The user saves a nested Invoice/Customer/Employee DTQL document, validates it, runs it, filters/selects a joined field, and receives ordered invoices with Employee missing where the LEFT side did not match. Each step has an observable result: schema validation accepts the document; Go and JS parsers produce the same tree; both executors produce the same records; SQLite uses native JOIN only for representable trees; unsupported adapters reject clearly; the release publishes tested commits and package artifacts. The implementation follows model and fixture dependencies, then generic execution, clause composition, native translation, adapter guards, and coordinated release. Mixed native/generic subtrees and distributed sources remain explicit future work. Review gates occur after the spec/plan, after model/serialization, after generic execution, after native SQL, and after all code.

For the algorithm-hint addition, the user adds ordered preferences to one JOIN, validates the query, serializes and reopens it, then runs it with the same rows as the unhinted query. A nested JOIN and a sibling JOIN keep their own preferences. An unavailable first preference proceeds to the next applicable strategy or the documented ordinary fallback; an invalid identifier fails at its exact JOIN path before source reads. The observable end-to-end result is unchanged rows and ordering through the existing Chinook fixture journey. The Go generic and JS generic tests exercise this whole journey; native SQLite acceptance proves that ignoring hints still preserves results.

## Repository and fixture contract

The WB task `dtql-recursive-joins` owns separate worktrees from verified `origin/main` bases: `dal-go/dalgo` at `d8751e3`, `dal-go/dalgo-js` at `364eaa3`, `dal-go/dalgo2sql` at `6815fd1`, and `dal-go/dalgo2sqlite` at `69cef3f`. DALgo is the language/schema/fixture authority. Canonical fixtures live in `dalgo/dtql/testdata/joins/` as YAML inputs plus normalized JSON expected rows/errors. Before release, copy exact fixture bytes into `dalgo-js/test/testdata/joins/`, `dalgo2sql/testdata/joins/`, and `dalgo2sqlite/testdata/joins/`; each repo commits the canonical content SHA-256 in a small manifest and CI recomputes it locally without relying on sibling checkouts. After DALgo lands, dependent manifests record its exact canonical source commit next to the digests; the canonical manifest cannot contain its own commit SHA. Verify every vendored copy still matches before dependent releases. Normalization maps Go `nil` and JS `null` to JSON null, omits absent unprojected LEFT fields, normalizes finite numeric values by value, preserves repeated base keys as repeated rows, and compares ordered result arrays and field maps. No JS `undefined` appears in serialized result objects.

`dalgo-js` has an independently owned, clean but unlanded parser commit `806caa2` on `datatug-browser-chat-phase1`. The JOIN owner will cherry-pick that exact commit into the isolated JOIN worktree before TypeScript implementation, preserve its public `parseDTQL` behavior, and send the resulting commit SHA to the browser-chat owner. The JOIN owner owns the integration gate; DALgo-JS release is blocked until that owner confirms the browser-chat branch will not independently land conflicting parser changes. The SQL repositories are separate Go modules; use local WB dependency links during development, then release DALgo first, update `dalgo2sql` and `dalgo2sqlite` module versions in order, and release DALgo-JS after its parser-branch integration. Inspect each repository's CI/tag workflow before choosing versions; record exact tags and artifacts in the landing report rather than guessing them now.

## Tasks

### Task 1: Freeze the DTQL contract and parity fixtures

**Verifies:** dtql-recursive-joins#ac:recursive-round-trip, dtql-recursive-joins#ac:scopes, dtql-recursive-joins#ac:malformed-input
**Status:** planning

Use the canonical Go `dtql` package and this Feature as authority. Add `dtql/testdata/joins/` fixtures for nested Chinook, siblings, defaults, typed keys (`1`, `1.0`, `'1'`, null, invalid objects), ON scoping, malformed references, clause composition, and single-source regression. Record error category/path and normalized expected row JSON with each fixture; pin a fixture digest in DALgo-JS. Resolve independent spec/plan review findings and rerun SpecScore lint before significant code. Record how `as`/`eq` input normalizes to `alias`/`==` output.

### Task 2: Extend the Go relation model and validator additively

**Verifies:** dtql-recursive-joins#ac:recursive-round-trip, dtql-recursive-joins#ac:scopes, dtql-recursive-joins#ac:malformed-input
**Status:** planning

Freeze the additive public AST before executor work: preserve `NewJoinedSource(RecordsetSource, JoinType, ...Condition)` and `FromSource` signatures; add a new constructor/accessor for a `JoinedSource` whose right operand is `FromSource`. The old constructor maps to a leaf right relation. Make `Joins()`/`NewQuery()` copies safe for nested trees without losing ON order or sharing mutable slices. Test from an external package. Structural validation owns shape, alias order, cycles, and error path/category. Schema-aware validation owns known missing fields and declared key types; execution owns actual key values (`join_key_type`).

### Task 3: Extend Go DTQL serialization and generated schema

**Verifies:** dtql-recursive-joins#ac:recursive-round-trip, dtql-recursive-joins#ac:malformed-input
**Status:** planning

Encode/decode recursive `from.joins` and structured ON through the model validator. Regenerate `schema.json` and `schema.yaml`, update examples and README, and replace the old join-rejection test with valid/invalid round trips. Keep old canonical output byte-stable for documents without JOINs.

### Task 4: Execute recursive joins through DALgo's generic path

**Verifies:** dtql-recursive-joins#ac:inner-left-recursion, dtql-recursive-joins#ac:composition, dtql-recursive-joins#ac:native-and-unsupported
**Status:** planning

Add an optional JOIN capability to `dal.QueryCapabilities` and an inspectable join plan (`native`, `generic`, `unsupported`) while preserving exported adapter method signatures. Route every caller-facing read entrypoint through one planning helper: `validatedDB`, validated read/write transactions, their `Select` methods, and recordset readers. JOIN relation construction precedes generic aggregation, which precedes result ordering/pagination/projection; a transaction whose backend cannot provide bounded relation scans rejects before output. The capability matrix: full native only with an adapter claim, generic for bounded relation scans, unsupported for unscannable sources, unrepresentable mixed plans, or exhausted bounds. Generic execution materializes each relation at most once per query and uses an equality index when ON has a cross-side key; other valid equality predicates use bounded candidate evaluation. Default limits are explicit constants for total fetched rows and retained bytes, with deterministic `join_plan` diagnostics before result emission. Test parent-correlated nested ON in generic Go, uncorrelated nested and sibling trees, null/typed keys, read counts, and the unsupported matrix across DB/transaction/Select/recordset entrypoints.

### Task 5: Compose all existing query clauses over joined rows

**Verifies:** dtql-recursive-joins#ac:composition, dtql-recursive-joins#ac:chinook-journey
**Status:** planning

Apply WHERE, grouping/aggregates, HAVING, ordering, pagination, and final projection at the specified stages. Reuse Go's aggregation pipeline where possible; make qualified field lookup and wildcard exclusions join-aware. Acceptance tests assert projection remains after ORDER/LIMIT, duplicate explicit output keys fail, source-qualified exclusion ignores absent fields but fails without schema metadata, `COUNT(*)` differs from `COUNT(right.field)` for LEFT rows, COUNT DISTINCT de-duplicates, and tie ordering is stable. Keep current single-source suites green.

### Task 6: Add native SQL/SQLite translation and capability checks

**Verifies:** dtql-recursive-joins#ac:native-and-unsupported, dtql-recursive-joins#ac:chinook-journey
**Status:** planning

In the separate `dalgo2sql` worktree, compile representable relation trees with parentheses preserving nested LEFT/INNER semantics, typed ON equality, and quoted identifiers. Advertise capability only after SQLite integration tests prove `1 == 1.0`, `1 != '1'`, null behavior, invalid-key preflight, nested parity, and two ordered siblings where the later ON references the earlier alias. Reject parent-correlated or cross-source shapes before output unless an equivalent SQL translation is verified. In the `dalgo2sqlite` worktree, use a checked-in Chinook-style SQLite fixture with `Invoice`, `Customer`, `Employee`, its vendored query file, and a documented one-command acceptance test; CI requires no sibling checkout.

### Task 7: Implement matching DALgo-JS DTQL model and validation

**Verifies:** dtql-recursive-joins#ac:recursive-round-trip, dtql-recursive-joins#ac:scopes, dtql-recursive-joins#ac:malformed-input
**Status:** planning

In the separate JOIN worktree, cherry-pick parser commit `806caa2` before further TypeScript edits and report the resulting SHA to its owner. Keep existing flat `StructuredQuery` calls valid, add recursive relation/ON types and safe parsing, and consume vendored fixtures with a pinned digest. Inventory each in-use JS adapter (`dalgo2firestore-js`, `dalgo2firebase-rtdb-js`, `dalgo2indexeddb-js`, SQL-like adapters) and assign native/generic/reject behavior with a negative query test; do not claim an adapter safe from structural typing alone. The JOIN owner obtains a parser-branch landing-order receipt before DALgo-JS release.

### Task 8: Implement TypeScript generic execution and parity

**Verifies:** dtql-recursive-joins#ac:inner-left-recursion, dtql-recursive-joins#ac:composition, dtql-recursive-joins#ac:chinook-journey
**Status:** planning

Build a generic relation executor over `QueryExecutor` scans with per-relation reads, the same row/byte bounds and key normalization as Go, equality indexing, matched/null-extended rows, clause composition, and explicit unsupported-plan errors. Test parent-correlated nested ON, missing/undefined/null keys, one-to-many duplicate base keys, depth-first collision precedence, stable ordering, aggregates, and projection. Compare exact normalized JSON rows and error category/path from the pinned fixtures. Run TypeScript lint, tests, typecheck, and build.

### Task 9: Adversarial review, documentation, and release

**Verifies:** dtql-recursive-joins#ac:recursive-round-trip, dtql-recursive-joins#ac:scopes, dtql-recursive-joins#ac:inner-left-recursion, dtql-recursive-joins#ac:composition, dtql-recursive-joins#ac:native-and-unsupported, dtql-recursive-joins#ac:malformed-input, dtql-recursive-joins#ac:chinook-journey
**Status:** planning

Have an independent reviewer try to break the code and SQL/null parity; fix or record each finding. Run focused checks and required CI, update public DTQL examples and changelogs, then use WB to land `dalgo`, `dalgo2sql`, `dalgo2sqlite`, and `dalgo-js` in dependency order, coordinating the parser branch. Inspect each module's own CI version/tag policy and npm provenance workflow, publish only by those workflows, and record exact version/tag, remote target SHA, required CI result, published Go module or npm artifact, canonical synchronization, and WB cleanup receipt per repository. A local green test or unlanded commit is not release evidence.

### Task 10: Add ordered physical JOIN hints and release the follow-up

**Verifies:** dtql-recursive-joins#ac:algorithm-hints, dtql-recursive-joins#ac:inner-left-recursion, dtql-recursive-joins#ac:chinook-journey
**Status:** planning

Update the canonical Feature and document the five case-sensitive identifiers, validation path, ordered fallback, and O(N × M) nested-loop cost. Add an optional, defensively copied hint list to each Go and JS JOIN model node without changing existing constructors or unhinted encoding. Parse and emit `hints.algorithms` in Go DTQL and DALgo-JS, regenerate Go JSON/YAML schemas, and add a public example. Validate direct-model and document queries at each recursive JOIN path before any provider read; reject malformed, duplicate, empty, and unknown preferences as `join_algorithm`. Add one canonical fixture with outer, nested, and sibling hints, then pin its digest in dependent fixture manifests.

In Go and JS generic executors, select per JOIN from the ordered list: `hash` uses the existing direct-key index, `nestedLoop` bypasses it and uses the existing candidate cap, and currently unavailable `merge`, `lookup`, and `batchedLookup` are skipped. If no hint applies, use the unhinted strategy. Keep native SQL/SQLite eligible and allow it to ignore hints. Implement a deterministic, directly testable per-JOIN selection function so tests prove preference order (`nestedLoop, hash` versus `hash, nestedLoop`) and unavailable-first fallback rather than inferring strategy from identical output rows. Test independent recursive/sibling lists, direct-model validation, and equivalent ordered results under all executable preferences. Test that a selected nested loop exceeding the candidate cap returns `join_plan` before rows without retrying hash; this resource rejection may differ from a successful unhinted execution while successful result semantics remain equal. Add an end-to-end Chinook journey with hints and compare it with the original fixture. Run Go full tests and coverage, TypeScript lint/typecheck/build/tests, SpecScore lint, and SQL/SQLite dependency checks. Obtain independent adversarial review of spec, plan, code, and documentation; fix findings. Land and publish through WB and repository release workflows, verifying exact remote heads, CI, tags, Go proxy/npm artifacts, canonical sync, and worktree/branch cleanup.

## Open Questions

- The active `datatug-browser-chat-phase1` DALgo-JS parser branch remains unlanded while its user tests it. Its exact commit is integrated first; the JOIN owner records the cherry-pick SHA and branch-order reply before release.
- The native SQL compiler may reject correlated nested ON unless a correct lateral form is proven; generic execution remains the correctness path.

---
*This document follows the https://specscore.md/plan-specification*
