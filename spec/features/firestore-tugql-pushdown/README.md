---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Firestore adapter: TugQL runs as Firestore queries; in-memory evaluation only by opt-in

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/firestore-tugql-pushdown?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/firestore-tugql-pushdown?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/firestore-tugql-pushdown?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/firestore-tugql-pushdown?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

The read-only TugQL entry point of the TypeScript Firestore adapter, `runFirestoreTugQL` in `@dalgo/firestore/tugql` (package `packages/firestore` in `dal-go/dalgo-http-adapters`), MUST:
- run a single-collection query as one Firestore query carrying its WHERE, ORDER BY and LIMIT, so Firestore reads only matching documents through its indexes;
- refuse, before any read, a query it cannot express that way, unless the caller passes `allowInMemoryEvaluation: true`.

This is a decision of the Firestore adapter only. The DALgo core executors, their specs and every other adapter are unchanged. Each other adapter may adopt the same rule later in its own spec.

## Problem

`runFirestoreTugQL` hands the query to DALgo's generic recursive executor (`executeRecursiveTugQLQuery` in DALgo-JS). That executor reads each collection with no filters, no ordering and a limit of 10,001 documents, then applies WHERE, ORDER BY and LIMIT in the browser. [tugql-subqueries](../tugql-subqueries/README.md) permits these "bounded relation scans". The only exception today is a query whose sole condition is `__key__ = <value>`, which reads one document.

Firestore bills every document read, so this is costly and slow:
- `from items where status = 'active'` over 5,000 documents pays 5,000 reads to return 3 rows;
- `from spaces/{SneatUserPersonalSpaceID}/ext/datatug/projects order by title asc` (a My DataTug Data query) reads every project and sorts it in the browser;
- a collection over 10,000 documents fails with `query_limit`, even when the filter matches one document.

Firestore can answer such queries itself from its automatic single-field indexes, or from a composite index whose creation link it reports.

## Behavior

### Scope

#### REQ: scope-typescript-firestore-tugql

This feature covers `runFirestoreTugQL` and the planner it uses. It does not change:
- DALgo-JS `executeRecursiveTugQLQuery`, `join.ts` or `aggregation.ts`;
- Go `dal` executors or the Go `dalgo2firestore` adapter;
- `compileFirestoreQuery` for ordinary (non-TugQL) DALgo queries;
- any other adapter.

Applying the same rule to the Go Firestore adapter is a separate future feature.

### Pushed queries

#### REQ: pushable-shape

A query is pushable when all of the following hold:
1. it reads one collection (a `table` relation with no JOIN, derived source, subquery, GROUP BY, HAVING or aggregate);
2. no relation scan limit is set;
3. its WHERE, if any, is an AND of conjuncts, each comparing one field or `__key__` with a literal or a bound parameter, using `==`, `!=`, `<`, `<=`, `>`, `>=`, `in` or `not-in`;
4. no conjunct compares with NULL, whether a NULL literal or a parameter bound to null, because TugQL treats `= NULL` as unknown while Firestore matches stored nulls;
5. every compared value, after parameters are bound, is a string, a finite number or a boolean, or an `in`/`not-in` list of these. DALgo-JS re-checks rows with JavaScript `===` (see [REQ: one-firestore-query](#req-one-firestore-query)), so a Timestamp, Bytes, GeoPoint, reference, array, map or NaN value would match in Firestore and then be dropped. Such values are refused;
6. the conjuncts are within Firestore's query limits, checked after parameters are bound:
   - at most 10 values in a `not-in` list;
   - the product of all `in` list sizes is at most 30, Firestore's disjunction limit;
   - at most one `!=` or `not-in`;
   - no `not-in` together with `in` or `!=`;
7. LIMIT plus OFFSET, when LIMIT is set, is at most the executor's fetched-row budget (`maxFetchedRows`, 10,000 by default). Otherwise the query is refused before any read with `query_limit` at `limit`, rather than failing after the read. This applies whether or not `allowInMemoryEvaluation` is set;
8. ORDER BY, if any, lists fields (or `__key__`), each ascending or descending.

An empty `in` list matches nothing: the query returns zero rows without reading. An empty `not-in` list adds no constraint.

`__key__` conjuncts compile to Firestore's document-id filter. Their values pass through the adapter's existing document-id validation.

#### REQ: one-firestore-query

The adapter MUST run a pushable query as exactly one Firestore query:
- every WHERE conjunct as a `where` constraint;
- every ORDER BY entry as an `orderBy` constraint;
- a `limit` of LIMIT plus OFFSET when LIMIT is set, or `maxFetchedRows` plus one (from the execution options; 10,000 by default) when it is not.

The adapter answers DALgo-JS's single leaf scan with the documents this query returns, the way the existing key-equality read does. DALgo-JS then re-applies WHERE, ORDER BY, OFFSET, LIMIT and projection; the adapter itself never skips rows. Firestore's Web SDK has no OFFSET, so the first OFFSET documents are read, billed and then skipped by DALgo-JS; the package README MUST say so.

Under [REQ: pushable-shape](#req-pushable-shape) the re-check keeps every row Firestore returned except those whose compared field is null or missing, which TugQL treats as unknown. It reads nothing more. More matches than the budget without a LIMIT still fail with `query_limit`, as today.

#### REQ: firestore-semantics

A pushed query has Firestore's semantics. The package README MUST document these differences from in-memory TugQL:
- ORDER BY on a field excludes documents that lack that field;
- range and `!=` filters match only values of the same type as the operand;
- `!=` and `not-in` exclude documents that lack the field;
- the rows are those Firestore selected, but DALgo-JS re-sorts them with JavaScript comparison (UTF-16 code units, no cross-type order). Their order can therefore differ from Firestore's for non-ASCII strings or mixed-type fields.

The adapter MUST NOT try to emulate in-memory TugQL here.

#### REQ: key-equality-read-kept

A query whose only condition is `__key__ = <value>` keeps the existing single-document read (`getDoc`).

### Refusal and opt-in

#### REQ: refuse-unless-allowed

`FirestoreTugQLOptions` gains `allowInMemoryEvaluation?: boolean`, false when omitted. A query that is neither pushable nor a key-equality read MUST, without the opt-in, fail before the first Firestore read with:
- an error whose `code` is `in_memory_evaluation`;
- a zero-based path of the first offending node (for example `where`, `where.and[1]`, `orderBy[0]`, `from.joins[0]`);
- a reason naming the condition from REQ:pushable-shape that failed (for example "OR", "field compared with field", "comparison with NULL", "`in` disjunctions exceed 30", "JOIN").

With the opt-in, the adapter runs such a query exactly as today: bounded scans evaluated by DALgo-JS, within its existing budgets.

#### REQ: no-silent-fallback

An error from a pushed Firestore query, such as the missing-composite-index error with its index-creation link, MUST reach the caller unchanged. The adapter MUST NOT retry with in-memory evaluation, even when the opt-in is set.

#### REQ: browser-host-does-not-opt-in

The DataTug browser app MUST NOT set `allowInMemoryEvaluation` when it runs queries against a Firestore connection.

The browser path applies no DALgo masks or row filters; Firestore security rules on the signed-in session decide what can be read. Pushing filters to Firestore therefore cannot widen access.

#### REQ: read-seam

The adapter MUST let tests inject the functions it reads with (`getDocs` and `getDoc`, or one reader object), so a test can count reads, inspect constraints and return fakes.

The planner that decides between a pushed query, a key-equality read and a refusal MUST be a pure function of the bound query and the options, so it can be unit-tested without Firestore.

## Dependencies

- tugql-subqueries
- tugql-recursive-joins

[tugql-recursive-joins](../tugql-recursive-joins/README.md) requires DALgo to execute JOINs over bounded scans when a provider lacks native JOINs. That requirement still holds for DALgo. This adapter, by its own choice, refuses such queries with `in_memory_evaluation` unless the caller opts in. With the opt-in, the DALgo behavior is unchanged.

## Acceptance Criteria

Planner ACs run as unit tests of the pure planner. Read-count ACs run against the Firestore emulator through the read seam of [REQ: read-seam](#req-read-seam), which counts reads and records constraints. The emulator fixture lives in the package tests, for example `test/fixtures/items.ts`.

### AC: equality-filter-reads-only-matches

**Requirements:** firestore-tugql-pushdown#req:pushable-shape, firestore-tugql-pushdown#req:one-firestore-query

Given a collection of 50 documents, 3 of which have `status == 'active'`
When `from items where status = 'active'` runs without the opt-in
Then one Firestore query is made with `where('status', '==', 'active')`, 3 documents are read and 3 rows are returned.

### AC: bound-parameter-and-in-list

**Requirements:** firestore-tugql-pushdown#req:pushable-shape, firestore-tugql-pushdown#req:one-firestore-query

Given the same collection
When `from items where status in @states` runs with `states` bound to `['active', 'paused']`
Then the query carries `where('status', 'in', ['active', 'paused'])` and reads only matching documents.
And when `states` is bound to 31 values, it fails with `in_memory_evaluation` at `where` before any read.

### AC: order-limit-offset

**Requirements:** firestore-tugql-pushdown#req:one-firestore-query

Given 50 documents with distinct `title` values
When `from items order by title asc limit 5 offset 10` runs
Then the query carries `orderBy('title', 'asc')` and `limit(15)`, 15 documents are read, and the rows equal titles 11–15 in ascending order.
And `limit 9995 offset 10` is refused with `query_limit` at `limit` before any read.

### AC: non-primitive-and-empty-values

**Requirements:** firestore-tugql-pushdown#req:pushable-shape

Given the planner
When a conjunct compares with a parameter bound to a Firestore Timestamp, a map or NaN
Then the query is refused with `in_memory_evaluation` at `where`.
And `where status in @states` with `states` bound to `[]` returns zero rows with no read.
And two `in` lists of 6 values each (36 disjunctions) are refused.

### AC: order-by-excludes-missing-field

**Requirements:** firestore-tugql-pushdown#req:firestore-semantics

Given 5 documents, 1 of which has no `title`
When `from items order by title asc` runs
Then 4 rows are returned, matching Firestore's own result.

### AC: unpushable-refused-before-read

**Requirements:** firestore-tugql-pushdown#req:refuse-unless-allowed

Given no opt-in
When each of these runs:
- `from items where a = 1 or b = 2` (path `where`);
- `from items where a = b` (path `where`);
- `from items where a = null` (path `where`);
- a query with a JOIN (path `from.joins[0]`);
- a GROUP BY query (path `groupBy`);
- `select (select count(*) from other) as n from items` (path `columns[0]`);
Then each fails with `in_memory_evaluation` at the path shown, and the read counter is zero.

### AC: opt-in-runs-as-today

**Requirements:** firestore-tugql-pushdown#req:refuse-unless-allowed

Given `allowInMemoryEvaluation: true`
When the OR query above runs over 50 documents
Then it returns the rows recorded in a committed golden file, which was captured from the adapter at 57188a1, and a collection over 10,000 documents still fails with `query_limit`.

### AC: index-error-surfaces

**Requirements:** firestore-tugql-pushdown#req:no-silent-fallback, firestore-tugql-pushdown#req:read-seam

Given a read seam whose `getDocs` throws Firestore's `failed-precondition` error for a missing composite index (the emulator never enforces indexes)
When it runs, with or without the opt-in
Then the caller receives that error unchanged, and no second query is made.

### AC: key-read-kept

**Requirements:** firestore-tugql-pushdown#req:key-equality-read-kept

Given `from users where __key__ = @SneatUserID`
When it runs without the opt-in
Then exactly one `getDoc` is made and no `getDocs`.

### AC: my-data-project-fully-pushed

**Requirements:** firestore-tugql-pushdown#req:browser-host-does-not-opt-in, firestore-tugql-pushdown#req:pushable-shape

Given the five queries of `datatug/my-data` (my-user-record, my-datatug-settings, my-personal-space, my-projects, project-queries), resolved with sample context variables against an emulator fixture of one user, one personal space and two projects, with no opt-in
When each runs
Then each succeeds:
- my-user-record, my-datatug-settings and my-personal-space each make one `getDoc`;
- my-projects makes one query with `orderBy('title', 'asc')`;
- project-queries makes one unfiltered query of its collection.

### AC: datatug-app-passes-no-opt-in

**Requirements:** firestore-tugql-pushdown#req:browser-host-does-not-opt-in

Given the DataTug browser app's Firestore query service (`browser-firestore-query.service.ts` in `datatug/datatug-apps`)
When it runs a query against a Firestore connection
Then a unit test in that repository shows `runFirestoreTugQL` is called without `allowInMemoryEvaluation`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
