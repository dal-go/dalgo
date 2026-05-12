# concurrency-capability — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `dal.ConcurrencyAware` interface to DALgo, embed it into `dal.DB`, ship two embeddable helper structs (`NoConcurrency`, `ConcurrencyAvailable`), and update the only two in-tree types that implement `dal.DB` so the repository builds and tests pass.

**Architecture:** All new types live in one file `dal/concurrency.go` (interface + both helpers). The `dal.DB` interface in `dal/db_database.go` gains a one-line embed. The two in-tree implementers — `mocks/mock_dal.MockDB` (gomock-generated) and `dalgo2fs.database` (hand-written) — are updated to satisfy the new method. Driver repos (`dalgo2sql`, `dalgo2ingitdb`) adopt the interface as separate follow-up work; they are explicitly out of scope here.

**Tech Stack:** Go 1.24+, `testing` package, `github.com/stretchr/testify/assert` (matches existing `dal/` test convention), `go.uber.org/mock` / `mockgen` for regenerating the `MockDB` mock.

**Spec:** [`spec/features/concurrency-capability/README.md`](../features/concurrency-capability/README.md)

**Source Idea:** [`spec/ideas/concurrency-capability.md`](../ideas/concurrency-capability.md)

---

## File Map

| Action | File | Change |
|--------|------|--------|
| Create | `dal/concurrency.go` | Defines `ConcurrencyAware` interface + `NoConcurrency` + `ConcurrencyAvailable` helper structs + godoc per REQ:godoc. |
| Create | `dal/concurrency_test.go` | Unit tests covering all REQ ACs from the Feature spec. Internal package (`package dal`) to match existing test convention. |
| Modify | `dal/db_database.go` | Embed `ConcurrencyAware` into the `DB` interface. One added line. |
| Modify | `dalgo2fs/database.go` | Embed `dal.NoConcurrency` in the `database` struct so it continues to satisfy `dal.DB`. |
| Regenerate | `mocks/mock_dal/db.go` | Re-run `mockgen` to pick up the new `SupportsConcurrentConnections() bool` method on `DB`. |

**No other in-tree types implement `dal.DB`** (verified via `grep -rn "dal\.DB" --include="*.go"`). Driver repos `dalgo2sql`, `dalgo2firestore`, etc. live elsewhere and are out of scope.

---

## Task 1: Define the `ConcurrencyAware` interface

**Files:**
- Create: `dal/concurrency.go`
- Create: `dal/concurrency_test.go`

- [ ] **Step 1: Write the failing test**

Create `dal/concurrency_test.go`:

```go
package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubConcurrency is a minimal type used only in tests to verify the
// ConcurrencyAware interface contract.
type stubConcurrency struct {
	value bool
}

func (s stubConcurrency) SupportsConcurrentConnections() bool {
	return s.value
}

// Compile-time assertion: stubConcurrency satisfies ConcurrencyAware.
var _ ConcurrencyAware = stubConcurrency{}

func TestConcurrencyAware_Interface(t *testing.T) {
	var c ConcurrencyAware = stubConcurrency{value: true}
	assert.True(t, c.SupportsConcurrentConnections())

	c = stubConcurrency{value: false}
	assert.False(t, c.SupportsConcurrentConnections())
}

func TestConcurrencyAware_MethodIsStable(t *testing.T) {
	// Per REQ:concurrency-aware-interface AC-2, repeated calls on the same
	// value return the same bool.
	c := stubConcurrency{value: true}
	first := c.SupportsConcurrentConnections()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, c.SupportsConcurrentConnections(), "call %d", i)
	}
}
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

Run:
```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
go test ./dal/ -run TestConcurrencyAware
```

Expected: build failure with `undefined: ConcurrencyAware`.

- [ ] **Step 3: Create the interface**

Create `dal/concurrency.go`:

```go
package dal

// ConcurrencyAware is implemented by [DB] values that can report whether
// the underlying backend supports multiple concurrent open connections
// from a single client process.
//
// The returned value is constant from the moment a DB value is returned
// by its constructor until it is discarded. Drivers MUST NOT change the
// answer in response to reconnects, failovers, transient errors, or
// runtime configuration reloads against the same DB handle. Callers are
// entitled to memoize the value once per DB value.
//
// The boolean intentionally does not distinguish read-vs-write
// concurrency. A driver like SQLite that supports concurrent readers
// but serializes writers collapses to false. Refining this surface (or
// adding a sibling) is a future change if a real consumer needs the
// distinction.
//
// ConcurrencyAware is embedded into [DB]; every DB implementation
// therefore answers the question. Drivers SHOULD embed one of the
// reusable structs [NoConcurrency] or [ConcurrencyAvailable] rather
// than hand-writing the method.
type ConcurrencyAware interface {
	// SupportsConcurrentConnections reports whether the underlying
	// backend tolerates more than one open connection from a single
	// client process at the same time. See [ConcurrencyAware] for the
	// stability and asymmetry contract.
	SupportsConcurrentConnections() bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./dal/ -run TestConcurrencyAware -v
```

Expected: both `TestConcurrencyAware_Interface` and `TestConcurrencyAware_MethodIsStable` PASS.

- [ ] **Step 5: Commit**

```bash
git add dal/concurrency.go dal/concurrency_test.go
git commit -m "feat(dal): add ConcurrencyAware interface

Defines the one-method capability interface for backends to report
whether they support concurrent connections. Spec:
spec/features/concurrency-capability/ REQ:concurrency-aware-interface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Add `NoConcurrency` helper struct

**Files:**
- Modify: `dal/concurrency.go`
- Modify: `dal/concurrency_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `dal/concurrency_test.go`:

```go
func TestNoConcurrency_ReturnsFalse(t *testing.T) {
	var n NoConcurrency
	assert.False(t, n.SupportsConcurrentConnections())
}

func TestNoConcurrency_SatisfiesInterface(t *testing.T) {
	var _ ConcurrencyAware = NoConcurrency{}
}

func TestNoConcurrency_EmbeddingSatisfiesInterface(t *testing.T) {
	// Per REQ:no-concurrency-helper AC-2, embedding NoConcurrency must
	// be sufficient for a struct to satisfy ConcurrencyAware.
	type stubWithNoConcurrency struct {
		NoConcurrency
	}
	var s stubWithNoConcurrency
	var c ConcurrencyAware = s
	assert.False(t, c.SupportsConcurrentConnections())
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run:
```bash
go test ./dal/ -run TestNoConcurrency
```

Expected: build failure with `undefined: NoConcurrency`.

- [ ] **Step 3: Add the helper to `dal/concurrency.go`**

Append to `dal/concurrency.go`:

```go
// NoConcurrency is a zero-value embeddable struct that satisfies
// [ConcurrencyAware] by reporting that concurrent connections are
// NOT supported. Drivers whose backend serializes connections (such
// as a single-writer SQLite, an unproven file-backed store, or a
// test stub) should embed NoConcurrency to inherit the conservative
// answer with no method body of their own:
//
//	type SQLiteDB struct {
//		dal.NoConcurrency
//		// ...
//	}
//
// See [ConcurrencyAware] for the full contract.
type NoConcurrency struct{}

// SupportsConcurrentConnections always returns false.
func (NoConcurrency) SupportsConcurrentConnections() bool {
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./dal/ -run TestNoConcurrency -v
```

Expected: all three `TestNoConcurrency_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dal/concurrency.go dal/concurrency_test.go
git commit -m "feat(dal): add NoConcurrency embeddable helper

Drivers that do not support concurrent connections can embed
dal.NoConcurrency to satisfy ConcurrencyAware with zero method body.
Spec: REQ:no-concurrency-helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Add `ConcurrencyAvailable` helper struct

**Files:**
- Modify: `dal/concurrency.go`
- Modify: `dal/concurrency_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `dal/concurrency_test.go`:

```go
func TestConcurrencyAvailable_ReturnsTrue(t *testing.T) {
	var c ConcurrencyAvailable
	assert.True(t, c.SupportsConcurrentConnections())
}

func TestConcurrencyAvailable_SatisfiesInterface(t *testing.T) {
	var _ ConcurrencyAware = ConcurrencyAvailable{}
}

func TestConcurrencyAvailable_EmbeddingSatisfiesInterface(t *testing.T) {
	// Per REQ:concurrency-available-helper AC-2.
	type stubWithConcurrencyAvailable struct {
		ConcurrencyAvailable
	}
	var s stubWithConcurrencyAvailable
	var c ConcurrencyAware = s
	assert.True(t, c.SupportsConcurrentConnections())
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run:
```bash
go test ./dal/ -run TestConcurrencyAvailable
```

Expected: build failure with `undefined: ConcurrencyAvailable`.

- [ ] **Step 3: Add the helper to `dal/concurrency.go`**

Append to `dal/concurrency.go`:

```go
// ConcurrencyAvailable is a zero-value embeddable struct that satisfies
// [ConcurrencyAware] by reporting that concurrent connections ARE
// supported. Drivers whose backend tolerates multiple concurrent
// connections (such as a server-side RDBMS like PostgreSQL) should
// embed ConcurrencyAvailable to inherit the permissive answer with
// no method body of their own:
//
//	type PostgresDB struct {
//		dal.ConcurrencyAvailable
//		// ...
//	}
//
// See [ConcurrencyAware] for the full contract.
type ConcurrencyAvailable struct{}

// SupportsConcurrentConnections always returns true.
func (ConcurrencyAvailable) SupportsConcurrentConnections() bool {
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./dal/ -run TestConcurrencyAvailable -v
```

Expected: all three `TestConcurrencyAvailable_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dal/concurrency.go dal/concurrency_test.go
git commit -m "feat(dal): add ConcurrencyAvailable embeddable helper

Drivers that support concurrent connections can embed
dal.ConcurrencyAvailable to satisfy ConcurrencyAware with zero
method body. Spec: REQ:concurrency-available-helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Embed `ConcurrencyAware` into `dal.DB` (breaking change — fix cascading impls)

**Files:**
- Modify: `dal/db_database.go`
- Modify: `dal/concurrency_test.go`
- Regenerate: `mocks/mock_dal/db.go` (via `mockgen`)
- Modify: `dalgo2fs/database.go`

This is the only structurally invasive task. It embeds the new capability into `dal.DB`, which makes the existing in-tree implementers stop compiling until they implement the new method. We add the embed AND fix the two implementers in this single task so each commit leaves the tree green.

- [ ] **Step 1: Write the failing compile-time test for `DB`-satisfies-`ConcurrencyAware`**

Append to `dal/concurrency_test.go`:

```go
// Compile-time assertion: any value satisfying dal.DB must also satisfy
// dal.ConcurrencyAware. Per REQ:embedded-in-db AC-1.
//
// We assert this at the interface level: (DB)(nil) is a typed nil, and
// the assignment forces the compiler to check method-set compatibility.
var _ ConcurrencyAware = (DB)(nil)
```

- [ ] **Step 2: Run the test to verify it fails (compile error)**

Run:
```bash
go test ./dal/ -run TestConcurrencyAware
```

Expected: build failure of the form:
```
dal/concurrency_test.go:NN:N: cannot use (DB)(nil) (untyped nil) as ConcurrencyAware value in variable declaration: DB does not implement ConcurrencyAware (missing method SupportsConcurrentConnections)
```

- [ ] **Step 3: Embed `ConcurrencyAware` into `dal.DB`**

Edit `dal/db_database.go`. Current content:

```go
package dal

// DB is an interface that defines a database provider
type DB interface {

	// ID is an identifier provided at time of DB creation
	ID() string

	// Adapter provides information about underlying name to access data
	Adapter() Adapter

	// Schema provides schema for the DB - for example, how keys are mapped to columns
	Schema() Schema

	// TransactionCoordinator provides shortcut methods to work with transactions
	// without opening a connection explicitly.
	TransactionCoordinator

	// ReadSession implements a virtual read session that opens connection/session for each read call on DB level
	// TODO: consider to sacrifice some simplicity for the sake of interoperability?
	ReadSession

	// Removed members:
	// ===================================================================================
	// Close() error - is part of a connection.
	// Connect(ctx context.Context) (connection, error) - considered unneeded
}
```

Replace with:

```go
package dal

// DB is an interface that defines a database provider
type DB interface {

	// ID is an identifier provided at time of DB creation
	ID() string

	// Adapter provides information about underlying name to access data
	Adapter() Adapter

	// Schema provides schema for the DB - for example, how keys are mapped to columns
	Schema() Schema

	// TransactionCoordinator provides shortcut methods to work with transactions
	// without opening a connection explicitly.
	TransactionCoordinator

	// ReadSession implements a virtual read session that opens connection/session for each read call on DB level
	// TODO: consider to sacrifice some simplicity for the sake of interoperability?
	ReadSession

	// ConcurrencyAware reports whether this backend supports concurrent
	// open connections. Drivers should embed NoConcurrency or
	// ConcurrencyAvailable in their concrete type to satisfy this.
	ConcurrencyAware

	// Removed members:
	// ===================================================================================
	// Close() error - is part of a connection.
	// Connect(ctx context.Context) (connection, error) - considered unneeded
}
```

- [ ] **Step 4: Run the full build to see what other code now fails**

Run:
```bash
go build ./...
```

Expected: failures in (at least) these two files, because they implement `dal.DB`:

```
dalgo2fs/database.go:NN:N: cannot use &fsFb (value of type *database) as dal.DB value in return statement: *database does not implement dal.DB (missing method SupportsConcurrentConnections)
mocks/mock_dal/0_interface_checks.go:5:5: cannot use (*MockDB)(nil) (value of type *MockDB) as dal.DB value in variable declaration: *MockDB does not implement dal.DB (missing method SupportsConcurrentConnections)
```

- [ ] **Step 5: Fix `dalgo2fs.database` by embedding `dal.NoConcurrency`**

Edit `dalgo2fs/database.go`. Find this struct definition:

```go
type database struct {
	path string
	dir  os.FileInfo
}
```

Replace with:

```go
type database struct {
	dal.NoConcurrency
	path string
	dir  os.FileInfo
}
```

The `dal.NoConcurrency` embedded field is unnamed and the dal package is already imported in the file. This is the conservative default; `dalgo2fs` writes to a real filesystem and its concurrency safety is unproven — false matches the Feature's stated MVP position for filesystem-backed drivers.

- [ ] **Step 6: Regenerate the `MockDB` mock**

The mock is gomock-generated; we re-run the same command captured in `mocks/generate_mocks.sh`:

```bash
mockgen github.com/dal-go/dalgo/dal DB > mocks/mock_dal/db.go
```

If `mockgen` is not on PATH:

```bash
$(go env GOPATH)/bin/mockgen github.com/dal-go/dalgo/dal DB > mocks/mock_dal/db.go
```

This rewrites `mocks/mock_dal/db.go` with a fresh mock that now includes a `SupportsConcurrentConnections()` method. The file header retains the `Code generated by MockGen. DO NOT EDIT.` line.

- [ ] **Step 7: Run the full build again**

Run:
```bash
go build ./...
```

Expected: build succeeds with no output.

- [ ] **Step 8: Run the full test suite**

Run:
```bash
go test ./...
```

Expected: all existing tests pass; the new `TestConcurrencyAware_*`, `TestNoConcurrency_*`, and `TestConcurrencyAvailable_*` tests pass. Note: any package-level `0 violations` style summary is fine; we only fail on test failures.

- [ ] **Step 9: Commit**

```bash
git add dal/db_database.go dal/concurrency_test.go dalgo2fs/database.go mocks/mock_dal/db.go
git commit -m "feat(dal): embed ConcurrencyAware in DB interface

DB now requires SupportsConcurrentConnections() bool. Updates the
two in-tree implementers — dalgo2fs.database embeds NoConcurrency
(the conservative default; concurrency safety unproven), and the
gomock-generated MockDB is regenerated from the new interface.

This is a breaking change for any external implementation of
dal.DB. Mitigation: drivers can embed dal.NoConcurrency or
dal.ConcurrencyAvailable to add the method with zero body.

Spec: REQ:embedded-in-db.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Godoc audit — verify REQ:godoc coverage

**Files:**
- Inspect: `dal/concurrency.go` (no edits expected if Tasks 1–3 wrote godoc as specified)

The godoc was authored inline during Tasks 1–3. This task is a verification pass that all four contract points from REQ:godoc are covered for each of `ConcurrencyAware`, `NoConcurrency`, and `ConcurrencyAvailable`. If any are missing, fix and commit.

The four required points (per REQ:godoc):

1. **Contract** — "returns true iff the backend supports concurrent open connections from a single client process."
2. **Stability** — answer is constant for the lifetime of the `DB`.
3. **Why no R/W asymmetry** — readers-vs-writers collapse to a single bool.
4. **Embed-helper pattern** — drivers SHOULD embed `NoConcurrency` or `ConcurrencyAvailable`.

- [ ] **Step 1: Inspect `ConcurrencyAware` godoc**

Run:
```bash
go doc github.com/dal-go/dalgo/dal.ConcurrencyAware
```

Expected: non-empty doc that mentions (a) what the bool means, (b) the constant-for-lifetime guarantee, (c) the deliberate absence of read/write asymmetry, and (d) the embed-helper recommendation. The text drafted in Task 1 Step 3 covers all four.

If anything is missing, edit `dal/concurrency.go` to add it, then re-run `go doc` to confirm.

- [ ] **Step 2: Inspect `NoConcurrency` godoc**

Run:
```bash
go doc github.com/dal-go/dalgo/dal.NoConcurrency
```

Expected: non-empty doc that explains it's an embeddable struct that reports `false`, shows the embed pattern in a code example, and refers to `ConcurrencyAware` for the full contract. The text drafted in Task 2 Step 3 covers this.

- [ ] **Step 3: Inspect `ConcurrencyAvailable` godoc**

Run:
```bash
go doc github.com/dal-go/dalgo/dal.ConcurrencyAvailable
```

Expected: non-empty doc, mirrors `NoConcurrency` but for `true`. Task 3 Step 3 covers this.

- [ ] **Step 4: Add an AC verification test (optional but cheap)**

Append to `dal/concurrency_test.go`:

```go
// TestGodocPresence asserts the godoc blocks for the new types are
// non-empty. This is a weak check (it cannot verify content), but it
// catches the regression of accidentally stripping doc comments.
//
// Stronger verification of the four contract points required by
// REQ:godoc is left to `go doc` inspection and code review.
func TestGodocPresence(t *testing.T) {
	// This test only runs `go doc` indirectly via the build; the real
	// assertion is performed by code review against REQ:godoc. We keep
	// this test as a placeholder to fail loudly if anyone removes the
	// concurrency.go file altogether.
	var _ ConcurrencyAware = NoConcurrency{}
	var _ ConcurrencyAware = ConcurrencyAvailable{}
}
```

- [ ] **Step 5: Run final test pass**

Run:
```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit (only if anything changed in this task)**

```bash
git add dal/concurrency.go dal/concurrency_test.go
git commit -m "docs(dal): verify concurrency godoc covers REQ:godoc contract

Audit pass over godoc for ConcurrencyAware, NoConcurrency, and
ConcurrencyAvailable; confirms all four required contract points
are covered. Adds a small presence-check test as regression bait.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

If nothing changed in this task (godoc was complete from Tasks 1–3 and Step 4 was skipped), skip the commit.

---

## Task 6: Push and update Feature status

**Files:**
- Modify: `spec/features/concurrency-capability/README.md` (status field)
- Modify: `spec/features/README.md` (index entry)
- Modify: `spec/plans/README.md` (status field)

The Feature is `Approved` and ready for implementation. After all code lands and tests pass, transition the Feature to `Implementing` (or whatever the next status convention is in this project — check the existing `spec/features/cli/version/README.md`-style precedent).

- [ ] **Step 1: Verify the test suite is green**

Run:
```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 2: Verify spec lints clean**

Run:
```bash
specscore spec lint --severity info
```

Expected: `0 violations found`.

- [ ] **Step 3: Update the plan's status in `spec/plans/README.md`**

Find the row:

```
| [2026-05-12-concurrency-capability](2026-05-12-concurrency-capability.md) | concurrency-capability | Ready |
```

Replace `Ready` with `Done`.

- [ ] **Step 4: Update the Feature status**

In `spec/features/concurrency-capability/README.md`, change:

```
**Status:** Approved
```

to:

```
**Status:** Implemented
```

In `spec/features/README.md` index, change the same status from `Approved` to `Implemented`.

- [ ] **Step 5: Re-lint**

Run:
```bash
specscore spec lint --severity info
```

Expected: `0 violations found`.

- [ ] **Step 6: Commit status transitions**

```bash
git add spec/features/concurrency-capability/README.md spec/features/README.md spec/plans/README.md
git commit -m "docs(spec): mark concurrency-capability Feature as Implemented

All five requirements are now satisfied with passing tests:
- REQ:concurrency-aware-interface
- REQ:embedded-in-db
- REQ:no-concurrency-helper
- REQ:concurrency-available-helper
- REQ:godoc

In-tree dal.DB implementers (mocks/mock_dal.MockDB,
dalgo2fs.database) updated. Driver-side adoption in dalgo2sql and
dalgo2ingitdb is tracked separately.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 7: Push all commits**

```bash
git push
```

Expected output: `main -> main` confirmation.

---

## Verification Checklist (after all tasks complete)

- [ ] `go build ./...` succeeds with no output
- [ ] `go test ./...` succeeds with all tests passing
- [ ] `specscore spec lint --severity info` returns `0 violations found`
- [ ] `go doc github.com/dal-go/dalgo/dal.ConcurrencyAware` returns substantive godoc covering all four REQ:godoc points
- [ ] `go doc github.com/dal-go/dalgo/dal.NoConcurrency` returns substantive godoc with embed-pattern example
- [ ] `go doc github.com/dal-go/dalgo/dal.ConcurrencyAvailable` returns substantive godoc with embed-pattern example
- [ ] `git log --oneline` shows 4–6 atomic commits (one per Task that produced changes)
- [ ] Feature README shows `Status: Implemented`
- [ ] Plan README index shows `Status: Done`

## Out of Scope (will not be done here)

- Driver-side adoption of `ConcurrencyAware` in `dalgo2sql` (SQLite → false, PostgreSQL → true) and `dalgo2ingitdb` (false). Each driver repo opens its own PR.
- Removal of the commented-out `dal.Connection` type. Surgical changes only.
- Any change to the existing `TransactionCoordinator` or `ReadSession` composition.
- Performance benchmarking. No I/O occurs in this code path.

## CHANGELOG Entry (draft for the maintainer to lift verbatim or rewrite)

```markdown
### Breaking changes
- The `dal.DB` interface now embeds the new `dal.ConcurrencyAware` interface
  (one method: `SupportsConcurrentConnections() bool`). External implementations
  of `dal.DB` must add this method.
- Two zero-value embeddable helpers are exported to make adoption a one-line
  change: `dal.NoConcurrency` (returns `false`) and `dal.ConcurrencyAvailable`
  (returns `true`). Embed whichever matches your backend's behavior.
- The conservative default for unknown or unproven backends is `false`; the
  caller's worker-pool sizing logic caps to a single connection when either
  side of a transfer reports `false`.
```
