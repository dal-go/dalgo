# Plan: First-class INNER/LEFT joins in dal's query model

**Status:** Implementing
**Source Feature:** query-joins
**Date:** 2026-06-05
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `query-joins` Feature into six linear, bottom-up tasks: first the `dal` AST changes (the breaking `NewFieldRef(src, name)` migration, the join-type enum, the public `NewJoinedSource` constructor), then the `dalgo2memory` executor (INNER + qualified resolution, then LEFT, then the error paths). All nine acceptance criteria are covered by a task; none are deferred.

## Approach

Tasks follow the build-dependency chain: the AST must express a qualified, typed join before the in-memory adapter can execute one. Task 1 lands the breaking `NewFieldRef(src, name)` signature and migrates its two constructor surfaces (`dal` wrappers + `dtql/deserialize.go`), establishing that empty `src` preserves single-source behaviour. Task 2 adds the join-type enum and Task 3 the public constructor that makes a typed, ON-populated join buildable — and readable by value — from outside `dal`. Tasks 4–6 build the executor on that AST: Task 4 introduces the nested-loop INNER join and the qualified-field resolver (base vs. joined by `Alias()`/`Name()`, empty `src` → base) that Tasks 5 and 6 reuse; Task 5 adds LEFT null-filling; Task 6 adds the descriptive errors for unsupported join shapes and unresolvable source qualifiers. ACs are grouped by unit of implementation work, never split into AC-wrapper tasks.

## Tasks

### Task 1: Source-qualified `NewFieldRef(src, name)` and call-site migration

**Verifies:** query-joins#ac:qualified-field-carries-source, query-joins#ac:existing-single-source-unaffected
**Status:** done

Add a source qualifier to `dal.FieldRef` and change the constructor to `NewFieldRef(src, name string)` with a `Source()` accessor; an empty `src` denotes the `From` base. Migrate the in-module constructor call sites — the `Field`/`AscendingField`/`DescendingField` wrappers in `dal` and `dtql/deserialize.go` — and update `dal` tests, leaving the full `dal` + `dtql` + `dalgo2memory` suites green so existing single-source queries are unaffected.

### Task 2: Join-type enum on `JoinedSource`

**Verifies:** query-joins#ac:join-type-readable
**Status:** done

Introduce an exported join-type value (e.g. `dal.JoinType` with `JoinInner` and `JoinLeft`, leaving room for `RIGHT`/`FULL`/`CROSS`) carried on each `JoinedSource` and readable through its public API, so an INNER join is distinguishable from a LEFT join in the AST.

### Task 3: Public `NewJoinedSource` constructor with value-readable `On()`

**Verifies:** query-joins#ac:build-join-from-outside-dal
**Status:** done

Add an exported `NewJoinedSource(src RecordsetSource, joinType JoinType, on ...Condition)` constructor, and make a join readable by value (a value receiver on `On()` or have `Joins()` return addressable joins) so a caller outside `dal` can build a typed equi-join with a populated `ON` clause and read back its type and conditions without touching unexported fields.

### Task 4: `dalgo2memory` INNER nested-loop join with qualified resolution

**Verifies:** query-joins#ac:inner-join-matches-only, query-joins#ac:qualified-resolution-in-where
**Status:** done

Extend `dalgo2memory` to walk `From().Joins()` and execute an INNER equi-join over the two in-memory collections via nested loop, returning only matched pairs. Add the qualified-field resolver — empty `src` → `From` base, non-empty `src` → the joined source whose `Alias()`/`Name()` matches — and apply it when evaluating `ON`, `Where`, `Columns`, and `OrderBy`.

### Task 5: `dalgo2memory` LEFT join with null-filled right side

**Verifies:** query-joins#ac:left-join-keeps-unmatched-left
**Status:** done

Add LEFT semantics on top of Task 4's resolver and loop: emit every `From`-base row, with the joined source's fields present where the `ON` equality holds and absent/`nil` where no right row matches.

### Task 6: `dalgo2memory` error paths for unsupported and unresolvable shapes

**Verifies:** query-joins#ac:unsupported-join-errors, query-joins#ac:unresolvable-source-errors
**Status:** pending

Return descriptive, row-suppressing errors when `dalgo2memory` meets a join type it does not support (anything other than INNER/LEFT, including a chained second join) or a non-empty field `src` that matches no recordset in the query, rather than silently producing wrong or empty results.

## Open Questions

- Exact exported names for the join-type constants and the `FieldRef` source accessor — finalized during implementation, tracked from the Feature's Open Questions.
- Whether `On()` becomes a value receiver or `Joins()` returns pointers to make a join readable by value (Task 3 picks one).

---
*This document follows the https://specscore.md/plan-specification*
