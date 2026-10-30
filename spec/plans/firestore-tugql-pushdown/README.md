---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: Firestore Tugql Pushdown

**Status:** Approved
**Source Feature:** firestore-tugql-pushdown
**Date:** 2026-10-30
**Owner:** alex
**Supersedes:** —

## Summary

Implements the Firestore adapter rule in `dal-go/dalgo-http-adapters` (`packages/firestore`): a pure TugQL pushdown planner, a read seam, and a planned executor behind `runFirestoreTugQL`, plus a DataTug app test and the version-pin bump in `datatug/datatug-apps`.

## Approach

All adapter work lands as one PR in dalgo-http-adapters. The pure planner comes first, because the executor and the emulator tests depend on its plan and refusal shape. The DataTug app change follows the merged adapter, because it pins the adapter's merge commit; it joins datatug-apps PR #630 so app changes ship in one batch.

## Tasks

### Task 1: Pure pushdown planner

**Id:** task-1
**Verifies:** firestore-tugql-pushdown#ac:non-primitive-and-empty-values, firestore-tugql-pushdown#ac:unpushable-refused-before-read
**Depends-On:** —
**Status:** planning

Add a pure function from a bound resolved TugQL query and the options to one of: a pushed plan (filters, orders, limit), a key-equality read, an empty result, or a refusal (`in_memory_evaluation` or `query_limit`, with a path and reason). Cover every pushable-shape condition with unit tests.

### Task 2: Read seam and planned executor

**Id:** task-2
**Verifies:** firestore-tugql-pushdown#ac:equality-filter-reads-only-matches, firestore-tugql-pushdown#ac:bound-parameter-and-in-list, firestore-tugql-pushdown#ac:order-limit-offset, firestore-tugql-pushdown#ac:order-by-excludes-missing-field, firestore-tugql-pushdown#ac:opt-in-runs-as-today, firestore-tugql-pushdown#ac:index-error-surfaces, firestore-tugql-pushdown#ac:key-read-kept, firestore-tugql-pushdown#ac:my-data-project-fully-pushed
**Depends-On:** 1
**Status:** planning

Wire the planner into `runFirestoreTugQL`. Add the injectable reader and `allowInMemoryEvaluation`. Answer DALgo-JS's leaf scan with the pushed query's documents. Add emulator tests with a read-counting seam, the opt-in golden file captured at 57188a1, and the My DataTug Data fixture. Document Firestore semantics, the billing of OFFSET rows, and the opt-in in the package README.

### Task 3: DataTug app pin and no-opt-in test

**Id:** task-3
**Verifies:** firestore-tugql-pushdown#ac:datatug-app-passes-no-opt-in
**Depends-On:** 2
**Status:** planning

In datatug-apps PR #630, bump the `@dalgo/firestore` pin to the merged adapter commit, and add a unit test showing `browser-firestore-query.service.ts` calls `runFirestoreTugQL` without `allowInMemoryEvaluation`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
