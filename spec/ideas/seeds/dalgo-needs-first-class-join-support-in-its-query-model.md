---
type: sidekick-seed
slug: dalgo-needs-first-class-join-support-in-its-query-model
captured_at: 2026-06-05T17:24:56Z
captured_by: user
captured_during: spec/features/dtql
trigger: explicit
status: queued
synchestra_task: null
---
# dalgo needs first-class join support in its query model

DTQL deferred joins because `dal.StructuredQuery`'s join model cannot yet
express a real join losslessly. Before DTQL (or any consumer) can serialize
joins without losing semantics, dal's query AST needs first-class join support.

Three concrete gaps in `dal` today:

1. **No join type.** `dal.JoinedSource` is just `RecordsetSource` + `on []Condition`.
   There is no `INNER` / `LEFT` / `RIGHT` / `FULL` / `CROSS` distinction anywhere —
   you cannot tell a LEFT join from an INNER join in the AST. Needs a join-type enum.
2. **ON conditions can't be set outside `dal`.** `JoinedSource.on` is unexported with
   no public constructor (same gap as `GroupCondition`, addressed there by
   `NewGroupCondition`). `FromSource.Join` exists but external callers can't populate
   the ON clause. Needs `NewJoinedSource(src, type, on...)`.
3. **`FieldRef` has no source qualifier.** It is `{name, isID}` — no way to express
   `u.id` vs `o.userId`. With two tables, every field in ON / WHERE / columns / ORDER BY
   is ambiguous. Needs source/alias qualification (e.g. `Field("u", "id")`), which
   ripples into every adapter that renders `FieldRef`.

Scope to decide later: which join types (inner+left likely the 90%), equality-only vs
richer ON conditions, and multiple/chained joins. Once dal can express joins, the DTQL
side is small: add `from.joins[]` with `type`, the joined source, and qualified field
refs in ON, then drop the subset-gate rejection.
