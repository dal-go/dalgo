---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Time precision contract (microseconds); dalgo2memory enforces it

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/time-precision?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/time-precision?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/time-precision?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/time-precision?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

No DALgo adapter keeps a `time.Time` more precisely than one microsecond, and some keep less. The `dal` package states this ceiling as `dal.TimePrecision` and gives consumers `dal.NormalizeTime` to normalise times without naming an engine. `dalgo2memory` truncates every time it stores to that precision, so unit tests see the precision loss production storage causes.

## Problem

datatug/backend#150 failed its real-storage journey test: business code compared a time kept in a JSON payload (nanoseconds) with the same moment stored as a record timestamp, which production storage keeps in microseconds. Unit tests ran on `dalgo2memory`, which kept nanoseconds, so they never saw the mismatch. The first fix put microsecond truncation into backend business code, leaking storage knowledge into a DALgo consumer.

Founder, 2026-11-04: "We are supposed to use dalgo and backend should know nothing about Firestore". Founder, 2026-11-04, approving this change: "Yes, fix in DALgo first".

## Behavior

### REQ: precision-contract

The `dal` package MUST export `TimePrecision = time.Microsecond` and document it as a ceiling, not a guarantee: no adapter keeps a `time.Time` more precisely than this, and some keep less (MongoDB's BSON datetime keeps milliseconds). Callers MUST NOT rely on sub-microsecond precision surviving a write and a read, and MUST normalise a time they also keep elsewhere (a hash, a payload, a cache key) to at least this precision.

### REQ: normalize-helper

The `dal` package MUST export `NormalizeTime(t time.Time) time.Time`, returning `t.Truncate(TimePrecision)`: the location is kept and any monotonic clock reading is stripped.

### REQ: memory-truncates-on-write

`dalgo2memory` MUST truncate every `time.Time` it persists to `TimePrecision`, on every write path (insert, set, update, and the optimistic transaction buffer) and for every storage engine, wherever encoding/json persists the time: exported struct fields, nested structs, exported fields promoted from embedded structs (exported or unexported, by value or by pointer), pointers, slices, arrays, map values and interface values (`map[string]any`, `[]any`). The caller's value MUST NOT be mutated. A self-referencing value MUST still fail the write with the encoder's cycle error rather than hang.

### REQ: memory-truncates-query-bounds

`dalgo2memory` MUST truncate every `time.Time` in a query's WHERE constants to `TimePrecision` once, before planning, so every engine (including the columnar engine's equality lookup) and the per-row match compare a bound at storage precision, and filtering by the nanosecond original matches the stored value.

## Acceptance Criteria

### AC: struct-round-trip

**Requirements:** time-precision#req:memory-truncates-on-write

**Given** a struct record whose times (direct, pointer, nested, slice, array, map, `any`) carry nanoseconds
**When** it is written to `dalgo2memory` with any storage engine and read back, by `Get` or by a query
**Then** every time equals `dal.NormalizeTime` of the original, and exact comparison with the un-normalised original fails.

### AC: map-round-trip

**Requirements:** time-precision#req:memory-truncates-on-write

**Given** a `map[string]any` record with nested nanosecond times
**When** it is written and read back
**Then** every time comes back at microsecond precision and the caller's map is unchanged.

### AC: update-truncates

**Requirements:** time-precision#req:memory-truncates-on-write

**Given** an update that sets a nanosecond time by field name or field path
**When** it is applied, in a default or an optimistic transaction
**Then** reads inside and after the transaction return the truncated time.

### AC: query-bound

**Requirements:** time-precision#req:memory-truncates-query-bounds

**Given** a record stored with a nanosecond time
**When** it is queried with `==`, `>=` or `<=` against that same nanosecond time, on the serialized or the columnar engine
**Then** the record matches.

### AC: helper

**Requirements:** time-precision#req:precision-contract, time-precision#req:normalize-helper

**Given** a nanosecond time in a fixed zone
**When** `dal.NormalizeTime` is applied
**Then** the result is truncated to the microsecond, keeps the zone, has no monotonic reading, and is idempotent.

## Out of Scope

- Changing any production adapter: each `dalgo2<engine>` adapter already stores at microsecond or coarser precision.
- Unexported struct fields that are not embedded structs, which encoding/json does not persist.

## Assumption Carryover

- **Assumed:** production adapters compare a query bound at their storage precision, so a nanosecond bound matches the microsecond value it stored. pgx's binary encoding and the Firestore client truncate. Postgres's text input rounds sub-microsecond digits instead of truncating, so a bound sent as text can differ by one microsecond. This is not verified against each engine in this Feature.
- **Assumed:** a coarser adapter (milliseconds) needs its callers to normalise more coarsely than `dal.NormalizeTime`; `dalgo2memory` models the microsecond ceiling only.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
