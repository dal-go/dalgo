# Idea: Migrate dal.RecordsReader to iter.Seq

**Status:** Draft
**Date:** 2026-05-15
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we align dalgo's RecordsReader idiom with Go 1.23+ iter.Seq so consumers get pull-based iteration, error propagation, and range-over-func ergonomics across the entire dalgo surface?

## Context

Triggered by the recordops Diff spec, which adopted iter.Seq2[record.WithID[K], error] as its streaming input contract. recordops ships a one-off bridge from dal.RecordsReader → iter.Seq2 so it can interoperate today, but the bridge is a workaround — every consumer of streaming dalgo queries would benefit from iter.Seq at the source. Go 1.24 (dalgo's current floor) supports iter.Seq natively.

## Recommended Direction

Add iter.Seq2-returning sibling methods to existing reader interfaces — preserving backwards compatibility — and migrate in-tree consumers. The exact shape (replace vs. add-alongside, generic vs. interface-method) needs scoping. The recordops bridge becomes obsolete if/when this lands.

## Alternatives Considered

- **Wholesale replacement: redesign `dal.RecordsReader` as `iter.Seq2`-shaped, deprecate the existing method-based interface.** Lost on cost: every dalgo driver and every consumer would have to migrate in lockstep. The dalgo ecosystem can't absorb a fleet-wide breaking change for what is primarily an ergonomics gain.
- **Wrapper helpers in each consumer.** Each consumer (recordops being the first) writes its own bridge. Lost on duplication and on the lack of a coherent vision: the same conversion gets reinvented in every package, with subtly different error-propagation conventions.
- **No change; keep `dal.RecordsReader` as the only idiom.** Lost because `iter.Seq2` is the language-blessed answer to "pull-based iteration with errors" in modern Go, and dalgo consumers will increasingly reach for it. Resisting alignment indefinitely creates a long-tail mismatch.

## MVP Scope

Scope: deciding the migration shape (sibling method vs. wholesale replacement vs. generic helper). MVP is a proposal doc + a working PoC that wraps one existing dal reader with iter.Seq2 and demonstrates a consumer using range-over-func. No driver migration yet.

## Not Doing (and Why)

- Migrating every dalgo driver in MVP — that's a fleet-wide change requiring driver-by-driver work
- Removing or deprecating dal.RecordsReader — backwards compatibility is a hard requirement
- Rewriting recordops to require this — recordops ships its own bridge so it works today regardless of when this idea lands

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | `iter.Seq2[T, error]` is the right shape for a streaming reader with error propagation in modern Go (not custom interfaces, not channels). | Cross-reference with Go 1.23+ stdlib and emerging community packages (`hash/maphash`, `database/sql` migration discussions); confirm there is no competing standard. |
| Must-be-true | A non-trivial number of dalgo consumers actually want streaming iteration today — not just recordops. | Survey the dalgo ecosystem: count consumers that hit `dal.RecordsReader.Next()` in a loop. If the answer is "just one or two," this idea is premature. |
| Should-be-true | The sibling-method approach (add `Iter() iter.Seq2[...]` alongside the existing `Next()` interface methods) keeps backwards compatibility without splitting drivers into "old" and "new" worlds. | PoC against one existing driver; confirm the sibling method can wrap the existing implementation without duplicating logic. |
| Might-be-true | Once added, the iter.Seq2 method becomes the preferred idiom and `Next()` falls into "compatibility only" mode within 2–3 releases. | Watch consumer migration rate after the sibling method ships. If `Next()` usage stays equal, the idea didn't deliver. |


## SpecScore Integration

- **New Features this would create:** TBD at design time
- **Existing Features affected:** none
- **Dependencies:** none

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/idea-specification*
