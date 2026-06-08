---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: Concurrency Capability

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/concurrency-capability?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/concurrency-capability?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/concurrency-capability?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/concurrency-capability?op=request-change) |

**Status:** Implemented
**Source Ideas:** —
**Source Idea:** [`concurrency-capability`](../../ideas/concurrency-capability.md)

## Summary

Add a small, mandatory capability to the `dal.DB` interface that lets callers know whether a backend supports concurrent connections. The capability is expressed as a one-method interface `dal.ConcurrencyAware` embedded into `dal.DB` (matching the existing pattern for `TransactionCoordinator` and `ReadSession`). Two reusable embeddable structs — `dal.NoConcurrency` and `dal.ConcurrencyAvailable` — let drivers opt into the conservative or permissive answer with zero method-body code.

The driving consumer is the cross-engine database copy command in `datatug-cli`, whose `--parallel-streams` default is gated on this signal. Without this capability, every consumer that wants to size a worker pool must maintain its own per-engine concurrency table — exactly the situation DALgo exists to abstract away.

## Problem

Consumers of DALgo today must hard-code per-engine knowledge to decide whether to run with concurrent connections:

- SQLite serializes writers → concurrent connections add cost without benefit.
- PostgreSQL is happy with many → concurrent connections are a win.
- `dalgo2ingitdb` is unproven under concurrent writes → conservative cap to 1.

The information lives in every consumer instead of in the backend that actually knows. This Feature moves it where it belongs.

## Non-Goals

Carried from the source Idea and reinforced by Feature-level analysis:

- **No general `Capabilities()` map** — over-engineered for one capability. If a second appears, add a second interface; capability interfaces compose without a registry.
- **No max-concurrent-connections advertisement.** A survey of real-world drivers (libpq, pgx, MySQL Go driver, MongoDB Go driver, Cloud Spanner, Firestore SDK) confirms none of them advertise a recommended ceiling at the SDK layer. The "right" number depends on workload and server config — both of which the driver does not know. The single boolean is sufficient.
- **No read-vs-write asymmetry in MVP.** Drivers like SQLite that support concurrent readers but serialize writers collapse to "false" for now. Refining the interface (or adding a sibling) is a future change if a real consumer needs the distinction.
- **No `context.Context` parameter, no `error` return.** The answer is constant for the lifetime of a `DB` value and side-effect-free.
- **No auto-tuning of stream counts inside DALgo.** Reporting capability is DALgo's job; sizing the worker pool is the consumer's.
- **No removal or formal deprecation of the commented-out `dal.Connection` type.** Out of scope; surgical changes only.

## Behavior

### REQ: concurrency-aware-interface

The `dal` package MUST define an exported interface:

```go
type ConcurrencyAware interface {
    SupportsConcurrentConnections() bool
}
```

The method MUST return a value that is **constant from the moment a `DB` value is returned by its constructor until it is discarded.** Drivers MUST NOT change the answer in response to reconnects, failovers, transient errors, or runtime configuration reloads against the same `DB` handle. A caller is entitled to memoize the value once per `DB` value.

#### AC-1: interface-exists

**Given** a Go program that imports `github.com/dal-go/dalgo/dal`
**When** the program references `dal.ConcurrencyAware`
**Then** the program compiles and `dal.ConcurrencyAware` is an interface with exactly one method `SupportsConcurrentConnections() bool`.

#### AC-2: method-is-stable

**Given** any concrete `dal.DB` implementation `db`
**When** `db.SupportsConcurrentConnections()` is called multiple times within a single test
**Then** all calls return the same `bool` value.

### REQ: embedded-in-db

The `dal.DB` interface MUST embed `ConcurrencyAware` so every `DB` implementation answers the question. The embedding MUST sit alongside the existing `TransactionCoordinator` and `ReadSession` compositions.

#### AC-1: db-satisfies-concurrency-aware

**Given** any concrete `dal.DB` value `db`
**When** a type assertion `_, ok := db.(dal.ConcurrencyAware)` is performed
**Then** `ok` is `true`.

#### AC-2: db-interface-shape

**Given** the source declaration of the `dal.DB` interface in `dal/db_database.go`
**When** inspected (by reading the source, by AST tooling, or via `go doc`'s rendering of embedded interfaces)
**Then** `ConcurrencyAware` is listed as an embedded interface inside `DB`, alongside `TransactionCoordinator` and `ReadSession`.

### REQ: no-concurrency-helper

The `dal` package MUST export a zero-value embeddable struct:

```go
type NoConcurrency struct{}
func (NoConcurrency) SupportsConcurrentConnections() bool { return false }
```

This struct lets drivers that do not support concurrent connections satisfy the capability by embedding `dal.NoConcurrency` — no method body required at the driver site.

#### AC-1: no-concurrency-returns-false

**Given** a zero-value `dal.NoConcurrency` value `n`
**When** `n.SupportsConcurrentConnections()` is called
**Then** the return value is `false`.

#### AC-2: embedding-satisfies-interface

**Given** a struct `type Stub struct { dal.NoConcurrency }`
**When** the program performs a type assertion `var _ dal.ConcurrencyAware = Stub{}`
**Then** the program compiles and the assertion holds.

### REQ: concurrency-available-helper

The `dal` package MUST export a sibling zero-value embeddable struct:

```go
type ConcurrencyAvailable struct{}
func (ConcurrencyAvailable) SupportsConcurrentConnections() bool { return true }
```

This struct lets drivers that do support concurrent connections satisfy the capability with zero method body.

#### AC-1: concurrency-available-returns-true

**Given** a zero-value `dal.ConcurrencyAvailable` value `c`
**When** `c.SupportsConcurrentConnections()` is called
**Then** the return value is `true`.

#### AC-2: embedding-satisfies-interface

**Given** a struct `type Stub struct { dal.ConcurrencyAvailable }`
**When** the program performs a type assertion `var _ dal.ConcurrencyAware = Stub{}`
**Then** the program compiles and the assertion holds.

### REQ: godoc

The interface and both helper structs MUST carry godoc comments that explain:

1. The contract — "returns true iff the backend supports concurrent open connections from a single client process."
2. The stability guarantee — answer is constant for the lifetime of the `DB`.
3. Why the boolean intentionally does not distinguish read-vs-write concurrency (link to or paraphrase the Idea's Not-Doing entry).
4. The embed-helper pattern — drivers SHOULD embed `dal.NoConcurrency` or `dal.ConcurrencyAvailable` rather than hand-writing the method.

#### AC-1: godoc-present

**Given** the `dal` package
**When** inspected via `go doc github.com/dal-go/dalgo/dal.ConcurrencyAware`, `go doc github.com/dal-go/dalgo/dal.NoConcurrency`, and `go doc github.com/dal-go/dalgo/dal.ConcurrencyAvailable`
**Then** each command returns a non-empty doc block that covers the four points above.

## Architecture

This Feature adds:

| File | Change |
|---|---|
| `dal/concurrency.go` (new) | Defines `ConcurrencyAware` interface, `NoConcurrency` struct, `ConcurrencyAvailable` struct, godoc. |
| `dal/db_database.go` | Embed `ConcurrencyAware` into the `DB` interface. One-line change. |
| `dal/concurrency_test.go` (new) | Tests covering the ACs above with mock `DB` implementations. |
| `dalgo2fs/database.go` | Cascading update: the in-tree `database` struct embeds `dal.NoConcurrency` so it continues to satisfy `dal.DB` after the interface gains the new method. |
| `mocks/mock_dal/db.go` | Cascading update: regenerated via `mockgen` so `MockDB` exposes `SupportsConcurrentConnections()`. The two mock-package test files (`mocks/mock_dal/db_test.go`, `mocks/mock_dal/mocks_test.go`) get matching recorder-method renames. |

No other DALgo packages are touched.

### Component diagram

```
                   ┌─────────────────────────────────┐
                   │           dal.DB                │
                   │  (existing interface)           │
                   │                                 │
                   │  + TransactionCoordinator       │
                   │  + ReadSession                  │
                   │  + ConcurrencyAware  ◄── NEW    │
                   └─────────────────────────────────┘
                                  ▲
                                  │ embedded
                                  │
                   ┌──────────────┴──────────────────┐
                   │      dal.ConcurrencyAware       │
                   │   SupportsConcurrentConnections │
                   │              () bool            │
                   └─────────────────────────────────┘
                          ▲                      ▲
                          │ embedded             │ embedded
                          │                      │
            ┌─────────────┴─────────┐   ┌────────┴──────────────────┐
            │   dal.NoConcurrency   │   │  dal.ConcurrencyAvailable │
            │  (returns false)      │   │  (returns true)           │
            └───────────────────────┘   └───────────────────────────┘
```

## Out of Scope (this Feature)

- **Driver-side implementations** in `dalgo2sql` (SQLite, PostgreSQL) and `dalgo2ingitdb`. Each driver repo adopts the new requirement as a follow-up. Driver adoption blocks the downstream consumer (datatug-cli `db copy`), not this Feature.
- **Mock and test-helper drivers** outside the `dal` package. The `mocks/` directory at the repo root may need a follow-up to embed `dal.NoConcurrency`; that's tracked separately.

## Breaking Change Notice

Embedding a new method into the `dal.DB` interface is **technically a breaking change** for any external implementation of `dal.DB`. Mitigation:

- The two helper structs (`NoConcurrency`, `ConcurrencyAvailable`) make adoption a one-line embedding.
- Internal `mocks/` and any in-tree consumers are updated in the same change.
- External implementations get a CHANGELOG entry pointing at the helper structs.

Acceptable cost given (a) `dal.DB` is a small, intentional interface, and (b) every implementation HAS a concurrency answer — there is no "doesn't apply" case.

## Testing Strategy

In-tree Go tests in `dal/concurrency_test.go`:

- Construct two mock `DB` types — one embedding `NoConcurrency`, one embedding `ConcurrencyAvailable`.
- Assert each satisfies `dal.ConcurrencyAware` at compile time (`var _ dal.ConcurrencyAware = mockNo{}`).
- Assert each satisfies `dal.DB` at compile time (using existing mock stubs from `mocks/` or a minimal in-test stub).
- Call `SupportsConcurrentConnections()` and verify return values.

No external integration tests required at this layer — driver behavior is verified in each driver's repo as part of adoption.

## Rehearse Integration

All ACs are testable via Go's built-in test runner. No external test scaffolding needed; AC verification lands in `dal/concurrency_test.go` as part of the implementation. Rehearse stub files are intentionally skipped — the entire Feature is verifiable in `go test ./dal/...`.

## Assumption Carryover

From the source Idea:

| Idea assumption | Status in Feature |
|---|---|
| Must-be-true: single boolean is sufficient | Carried; reinforced by survey of real-world drivers in Non-Goals. |
| Must-be-true: caller can reach `DB` at the decision point | Carried; embedding into `DB` makes this trivially true. |
| Should-be-true: "not implemented = unknown" is safe | **Invalidated by embedding decision.** Every `DB` now MUST implement; "unknown" no longer exists. |
| Should-be-true: answer is stable for the lifetime of a `DB` | Promoted to a MUST in REQ `concurrency-aware-interface`. |
| Might-be-true: second capability arrives soon | Deferred; no shared registry built ahead of need. |
| Might-be-true: read/write asymmetry becomes a real ask | Deferred; documented in godoc per REQ `godoc`. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
