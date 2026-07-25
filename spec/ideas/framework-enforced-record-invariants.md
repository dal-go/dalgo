---
format: https://specscore.md/idea-specification
status: Draft
---
# Idea: Framework-enforced record invariants

**Status:** Draft
**Date:** 2026-07-25
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we make it structurally impossible for a DALgo adapter to skip an
invariant the framework declares, so that a test double can never quietly
disagree with production about whether a write is legal?

## Context

DALgo declares an invariant — a record whose data implements
`dal.ValidatableRecord` is validated before it is written — and then leaves every
adapter to decide whether to enforce it. Four facts, all verified in the source
on 2026-07-25:

1. **`dal.BeforeSave` already implements the rule** (`dal/hooks.go`): it calls
   `Validate()` on any record data implementing `ValidatableRecord`, then runs
   registered hooks.
2. **Nothing calls it.** A search across the whole `dal-go` organisation finds no
   caller in any adapter. It is dead code. The internal helper is even spelled
   `beforeSafe`, and the hook slice `beforeSafeHooks` — a typo that survived
   precisely because nothing ever exercised the path.
3. **`dal.InsertRecordWithDataAndID`** — the framework's own insert helper, and
   the one downstream frameworks actually call — does not validate either. It
   builds a record and delegates straight to `s.Insert`.
4. So adapters improvised. **`dalgo2firestore` validates inline** in its own
   `inserter.go`; **`dalgo2memory` does not validate at all**.

The consequence is not theoretical. Because `dalgo2memory` is what tests wire and
`dalgo2firestore` is what production runs, validation code in
`bots-go-framework` was **dead in test and alive only in production**. Five bugs
accumulated there undetected — an inverted comparison that rejected every valid
id, two required fields nothing ever populated, and two missing write-time
stamps — which between them meant no brand-new platform user's first record could
be written at all. Two of the five even had tests; one asserted the bug.

This is the general failure: **an invariant declared by the framework but
enforced by the implementations is not an invariant.** It is a convention, and
conventions drift silently across twelve adapters.

Scale, so the solution is judged against reality: the write surface is eight
primitives (`Inserter`, `MultiInserter`, `Setter`, `MultiSetter`, `Updater`,
`MultiUpdater`, `Deleter`, `MultiDeleter`) composed into `WriteSession`. There
are twelve adapters — `dalgo2memory` and `dalgo2fs` in-repo, plus `datastore`,
`files`, `firestore`, `git`, `mysql`, `namecheap`, `openvaultdb`, `postgres`,
`sql`, `sqlite`.

## Recommended Direction

**Move enforcement from the implementations into the framework, so an adapter
never gets the chance to skip it.** Three layers, in decreasing order of how much
work they do:

### Layer 1 — the framework owns the write path (does the real work)

Rename today's `dal.DB` to **`dal.Backend`**: the interface adapters implement,
unchanged in shape. Make **`dal.DB` a framework-owned type** that wraps a
`Backend` and owns the write pipeline:

```
DB.Insert(ctx, record, opts…)
  → BeforeSave(ctx, db, record)      // validate + registered hooks — one implementation
  → backend.Insert(ctx, record, opts…)
```

An adapter cannot skip validation because **validation runs before the adapter's
code is entered.** The rule stops being something adapters are trusted to honour
and becomes something they are structurally incapable of avoiding.

The critical feasibility point: **this does not rewrite the twelve adapters.**
They keep their existing implementations verbatim. Only each adapter's public
constructor changes to return the wrapped value — `return dal.New(db)` instead of
`return db`. One line per adapter.

Then **delete `dalgo2firestore`'s inline validation.** Two implementations of one
rule is how they drifted in the first place; the point of this work is that there
is exactly one.

### Layer 2 — sealing, so bypass is a compile error rather than a silent risk

Layer 1's weak point is an adapter that exposes its raw `Backend` to callers, who
then write through it directly. Close that by giving the caller-facing `dal.DB` an
**unexported marker method**, so only the framework can produce a value satisfying
it. An adapter returning its raw backend then fails to compile as a `dal.DB`.

This does not enforce *behaviour* — it enforces *provenance*. That is exactly the
right division of labour: layer 1 guarantees the behaviour, layer 2 guarantees
you went through layer 1.

### Layer 3 — a conformance suite, as the backstop and the future-proofing

Ship **`dalgotest.RunConformance(t, factory)`**: a shared, behavioural suite each
adapter runs from its own tests, asserting the declared invariants against a live
instance — an invalid record is rejected on `Insert`, `Set`, `Update` and each
multi-variant; a valid one is accepted; the error is the validation error, not a
storage error.

This is the mechanism that answers *"so we don't have to trust adapters"* for
invariants layers 1 and 2 cannot express, and for adapters that legitimately
bypass the wrapper. It converts divergence from a silent difference into a
**failing build**, and it is the standard Go answer to this problem —
`fstest.TestFS`, the `database/sql` driver suites.

### The escape hatch, designed rather than discovered

Some writes legitimately must skip validation: bulk import, repair migrations,
writing records that are *known* invalid in order to fix them later. If no
sanctioned bypass exists, people will find an unsanctioned one — reaching for the
raw backend and reintroducing exactly this problem.

So provide an explicit, greppable **`dal.WithoutValidation()`** insert/set option.
The point is not to permit skipping; it is to make skipping **visible at the call
site and searchable across the fleet**, instead of invisible inside an adapter.

## Alternatives Considered

- **Fix `dalgo2memory` only.** Lost because it restores parity today and leaves
  the structure that produced the divergence untouched: the eleven other adapters
  remain unverified, and adapter number thirteen is free to skip it again. It
  treats the instance, not the shape.
- **Documentation — "adapters must validate".** Lost because it is what already
  failed, implicitly. A rule with no mechanism is a wish.
- **A validating decorator callers opt into (`dal.WithValidation(db)`).** Lost
  because opt-in is precisely the property that failed. It relocates the trust
  problem to every call site instead of removing it.
- **Conformance suite alone (layer 3 without layer 1).** Lost as the primary
  mechanism: it detects divergence rather than preventing it, and only for
  adapters that choose to run it. Retained as the backstop, where it is genuinely
  the right tool.
- **Full `database/sql`-style inversion** — adapters implement a minimal storage
  primitive and the framework owns everything. Lost as disproportionate: DALgo
  adapters differ enormously in capability (queries, joins, transactions, field
  paths, columnar), so a minimal common primitive would either cripple the
  capable adapters or become as wide as today's interface. Layer 1 takes the same
  principle and applies it only to the write pipeline, where the invariants live.

## MVP Scope

One job: `dalgo2memory` and `dalgo2firestore` both reject an invalid record on
`Insert` **through the same framework code path**, with Firestore's inline
validation deleted, and a conformance suite that fails if either stops doing so.

## Not Doing (and Why)

- Rewriting adapter internals — layer 1 needs only their constructors
- Hoisting reads, queries or transactions into the framework — the invariants at
  issue are write-time; widening scope would stall the fix
- Inventing new invariants in this work — the goal is enforcing the ones already
  declared; new ones become cheap afterwards, which is the point
- Silent bypass — if validation must be skipped it is `dal.WithoutValidation()`,
  visible and greppable

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Wrapping `DB` needs only a constructor change per adapter, not internal edits | Convert `dalgo2memory` and `dalgo2firestore` first and count the real diff |
| Must-be-true | The unexported marker method actually prevents an adapter returning a raw backend as `dal.DB` | Attempt it in a test package and confirm it fails to compile |
| Must-be-true | Enforcing validation surfaces real fleet breakage rather than being a no-op | Run the fleet's tests against the change; every failure is a finding — an actual bug or a fixture that was never valid |
| Should-be-true | One conformance suite can express the invariants for adapters as different as Firestore, SQL and git | Write it against three deliberately dissimilar adapters before declaring it shared |
| Should-be-true | The added interface hop is not a meaningful write-path cost | Benchmark `dalgo2memory` insert before and after |
| Might-be-true | Twelve adapter maintainers will adopt the constructor change without a long tail | Convert the two in-repo adapters, then measure how long the first external one takes |

## SpecScore Integration

- **New Features this would create:** the framework write pipeline (`dal.DB` over
  `dal.Backend`); the `dalgotest` conformance suite; the `WithoutValidation`
  option
- **Existing Features affected:** every adapter's public constructor signature;
  `dalgo2firestore` loses its inline validation; `dal.BeforeSave` moves from dead
  code to the single enforcement point
- **Dependencies:** none blocking. Downstream fleets (notably
  `bots-go-framework`, which this was found through) should bump only after the
  conformance suite is green, since enforcement will surface latent invalid data

## Open Questions

- Does `dal.DB` become a concrete struct or stay an interface with an unexported
  method? The struct is simpler to reason about and harder to fake; the sealed
  interface preserves the ability to decorate. This decides how layer 2 is built.
- Should `Delete` participate in the pipeline at all? It has no record data to
  validate, but hooks may still want it — and an inconsistent pipeline is its own
  source of surprise.
- Do the `Multi*` variants validate all records before writing any (atomic in
  spirit, and what a caller probably expects), or per-record as they go? The
  answer changes what a partial failure means.
- Does the conformance suite live in `dalgo` (one import for adapters, but a test
  dependency in the core module) or a separate `dalgotest` module (cleaner
  dependency graph, one more repo to release)?
- Is `WithoutValidation` allowed in production code, or only behind a build tag
  or an explicit DB-construction flag? Greppable is good; impossible-in-prod may
  be better.
- Fix the `beforeSafe` / `beforeSafeHooks` spelling as part of this? It is a
  trivial rename, and it is also the clearest possible evidence that the path was
  never executed — worth keeping in the commit history that explains why.

---
*This document follows the https://specscore.md/idea-specification*
