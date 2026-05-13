# ddl.Applier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public `ddl.Applier` interface plus an `ApplyTo(ctx, applier) error` method on each of the 6 AlterOp concrete types and on the sealed `AlterOp` interface itself, so driver-side schema modifiers can dispatch AlterOps via the visitor pattern.

**Architecture:** Purely additive. New file `ddl/applier.go` declares the `Applier` interface (6 methods, ctx-first). Existing `ddl/alter_op.go` is modified to add `ApplyTo` to the `AlterOp` interface and to add the per-type `ApplyTo` methods on the 6 concrete op structs (`addFieldOp`, `dropFieldOp`, `modifyFieldOp`, `renameFieldOp`, `addIndexOp`, `dropIndexOp`). New test file `ddl/applier_test.go` covers the dispatch contract with a `recordingApplier` stub. No existing tests change.

**Tech Stack:** Go ≥ 1.24, `context`, `errors` (for `errors.Is`), `reflect` (for interface-shape tests), `testing` (stdlib).

---

## Conventions

- **Working directory:** `/Users/alexandertrakhimenok/projects/dal-go/dalgo`. All commands assume that cwd.
- **Test framework:** Go stdlib `testing`. Use `t.Parallel()` on all leaf tests.
- **Commit style:** Conventional commits with the `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` footer.
- **Verification:** After each task, `go test ./ddl/...` must be clean before commit.

## File Structure

| File | Change |
|---|---|
| `ddl/applier.go` | Create. Declares the `Applier` interface (6 methods, ctx-first). |
| `ddl/applier_test.go` | Create. `recordingApplier` test stub + interface-shape test + per-op dispatch tests + error-propagation test. |
| `ddl/alter_op.go` | Modify. Add `ApplyTo(ctx context.Context, a Applier) error` to the `AlterOp` interface (line 22 area); add an `ApplyTo` method on each of the 6 concrete op structs. |

## Audit (already verified)

The 6 concrete op types in `ddl/alter_op.go` and their stored fields:

| Type | Fields | Constructor |
|---|---|---|
| `addFieldOp` | `field dbschema.FieldDef`, `options Options` | `AddField(f, opts...)` |
| `dropFieldOp` | `name dal.FieldName`, `options Options` | `DropField(name, opts...)` |
| `modifyFieldOp` | `name dal.FieldName`, `newDef dbschema.FieldDef`, `options Options` | `ModifyField(name, newDef, opts...)` |
| `renameFieldOp` | `oldName dal.FieldName`, `newName dal.FieldName`, `options Options` | `RenameField(old, new, opts...)` |
| `addIndexOp` | `index dbschema.IndexDef`, `options Options` | `AddIndex(idx, opts...)` |
| `dropIndexOp` | `name string`, `options Options` | `DropIndex(name, opts...)` |

The `AlterOp` interface currently has one unexported marker: `alterOp()`. Adding a public `ApplyTo` method to the interface is safe because the sealed marker prevents external implementations — only the 6 in-package types can satisfy `AlterOp`, and Task 3 adds `ApplyTo` to all of them atomically.

---

## Task 1: `Applier` interface

**Files:**
- Create: `ddl/applier.go`

- [ ] **Step 1: Create the Applier interface**

Create `ddl/applier.go`:

```go
package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// Applier is the visitor interface for AlterOp dispatch. Driver-side
// implementations of [SchemaModifier.AlterCollection] construct an
// Applier (typically a struct holding the in-flight transaction and
// the target table name), then call [AlterOp.ApplyTo] on each op
// in their batch. ApplyTo dispatches to the matching ApplyXxx method
// on the Applier, where the driver translates the operation into
// engine-specific SQL or other side effects.
//
// All six methods take ctx as the first parameter (forwarded by
// AlterOp.ApplyTo) and return error so driver-side failures surface
// to the AlterCollection caller.
//
// The resolved [Options] value (already processed by [ResolveOptions]
// at AlterOp-constructor time) is passed last. Drivers MAY consult
// opts.IfNotExists / opts.IfExists for the field/index-level
// AlterOps where those flags are semantically meaningful (see the
// alter-ops Feature for which combinations matter per op).
//
// Applier is NOT sealed — driver packages MUST implement it. Test
// stubs (e.g. a recording fakeApplier) MAY also implement it. New
// AlterOp variants added in future dalgo releases will extend this
// interface, which is a breaking change for existing implementers
// (compile-time error) — the same property the AlterOp sealed
// interface has on the producer side.
type Applier interface {
	// ApplyAddField is called by addFieldOp.ApplyTo.
	ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts Options) error

	// ApplyDropField is called by dropFieldOp.ApplyTo.
	ApplyDropField(ctx context.Context, name dal.FieldName, opts Options) error

	// ApplyModifyField is called by modifyFieldOp.ApplyTo. name is
	// the current field name; newDef is its replacement (which MAY
	// have a different Name, indicating a rename as part of modify).
	ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts Options) error

	// ApplyRenameField is called by renameFieldOp.ApplyTo.
	ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts Options) error

	// ApplyAddIndex is called by addIndexOp.ApplyTo.
	ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts Options) error

	// ApplyDropIndex is called by dropIndexOp.ApplyTo.
	ApplyDropIndex(ctx context.Context, name string, opts Options) error
}
```

- [ ] **Step 2: Verify build**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go build ./ddl/...`
Expected: clean exit.

- [ ] **Step 3: Commit**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
git add ddl/applier.go
git commit -m "$(cat <<'EOF'
feat(ddl): add Applier interface (visitor for AlterOp dispatch)

Public interface declaring six ApplyXxx methods, one per AlterOp
variant (AddField, DropField, ModifyField, RenameField, AddIndex,
DropIndex). Each takes ctx as the first parameter and returns error.

Applier is the consumer side of the visitor pattern; AlterOp.ApplyTo
(added in a follow-up commit) is the dispatcher.

Refs: spec/features/ddl/applier REQ:applier-interface

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `recordingApplier` test stub + interface-shape test

**Files:**
- Create: `ddl/applier_test.go`

- [ ] **Step 1: Create the test file**

Create `ddl/applier_test.go`:

```go
package ddl

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// recordingApplier captures each method call for assertion. Fields
// hold the most-recent call's arguments per method; unused fields
// stay zero. The numbered counters let tests verify which method was
// called.
type recordingApplier struct {
	addFieldCalls    int
	dropFieldCalls   int
	modifyFieldCalls int
	renameFieldCalls int
	addIndexCalls    int
	dropIndexCalls   int

	lastCtx context.Context

	addField    dbschema.FieldDef
	dropField   dal.FieldName
	modifyOld   dal.FieldName
	modifyNew   dbschema.FieldDef
	renameOld   dal.FieldName
	renameNew   dal.FieldName
	addIndex    dbschema.IndexDef
	dropIndex   string

	lastOpts Options

	// returnErr is returned from every ApplyXxx call when non-nil.
	// Used for error-propagation tests.
	returnErr error
}

func (r *recordingApplier) ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts Options) error {
	r.addFieldCalls++
	r.lastCtx = ctx
	r.addField = f
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyDropField(ctx context.Context, name dal.FieldName, opts Options) error {
	r.dropFieldCalls++
	r.lastCtx = ctx
	r.dropField = name
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts Options) error {
	r.modifyFieldCalls++
	r.lastCtx = ctx
	r.modifyOld = name
	r.modifyNew = newDef
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts Options) error {
	r.renameFieldCalls++
	r.lastCtx = ctx
	r.renameOld = oldName
	r.renameNew = newName
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts Options) error {
	r.addIndexCalls++
	r.lastCtx = ctx
	r.addIndex = idx
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyDropIndex(ctx context.Context, name string, opts Options) error {
	r.dropIndexCalls++
	r.lastCtx = ctx
	r.dropIndex = name
	r.lastOpts = opts
	return r.returnErr
}

// TestApplier_InterfaceShape verifies that the Applier interface has
// exactly six methods with the expected signatures. Catches accidental
// rename / signature drift at refactor time.
func TestApplier_InterfaceShape(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*Applier)(nil)).Elem()
	if got, want := typ.NumMethod(), 6; got != want {
		t.Errorf("Applier method count = %d, want %d", got, want)
	}
	wantMethods := map[string]bool{
		"ApplyAddField":    false,
		"ApplyDropField":   false,
		"ApplyModifyField": false,
		"ApplyRenameField": false,
		"ApplyAddIndex":    false,
		"ApplyDropIndex":   false,
	}
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if _, known := wantMethods[m.Name]; !known {
			t.Errorf("unexpected method %q on Applier", m.Name)
			continue
		}
		wantMethods[m.Name] = true
		// First param of every method MUST be context.Context.
		if got := m.Type.In(0); got != reflect.TypeOf((*context.Context)(nil)).Elem() {
			t.Errorf("%s first param = %v, want context.Context", m.Name, got)
		}
		// Last return MUST be error.
		if got, want := m.Type.NumOut(), 1; got != want {
			t.Errorf("%s NumOut = %d, want %d", m.Name, got, want)
			continue
		}
		if got := m.Type.Out(0); got != reflect.TypeOf((*error)(nil)).Elem() {
			t.Errorf("%s return = %v, want error", m.Name, got)
		}
	}
	for name, seen := range wantMethods {
		if !seen {
			t.Errorf("Applier missing method %q", name)
		}
	}
}

// Compile-time assertion: *recordingApplier satisfies Applier.
var _ Applier = (*recordingApplier)(nil)
```

- [ ] **Step 2: Run test**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go test ./ddl/ -run TestApplier_InterfaceShape -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
git add ddl/applier_test.go
git commit -m "$(cat <<'EOF'
test(ddl): add recordingApplier + Applier interface-shape test

Stub Applier that records each method call for assertion in
follow-up tests. Interface-shape test verifies six methods with
context.Context as first param and error as return — catches
accidental drift at refactor time.

Refs: spec/features/ddl/applier REQ:applier-interface AC-2

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `ApplyTo` to `AlterOp` interface + all six concrete types

**Files:**
- Modify: `ddl/alter_op.go`

This task combines the interface extension with the six per-type implementations in one commit because adding `ApplyTo` to the interface without simultaneously implementing it on all six types would break package compilation. TDD discipline is preserved by writing the dispatch tests (Task 4) AFTER this change lands and runs them green — the impl-first-then-test ordering is appropriate here given the atomic-or-broken nature of the interface change.

- [ ] **Step 1: Modify the AlterOp interface declaration**

Edit `ddl/alter_op.go`. The current interface declaration at lines 22-24:

```go
type AlterOp interface {
	alterOp() // sealed marker
}
```

Replace with:

```go
type AlterOp interface {
	alterOp() // sealed marker
	// ApplyTo dispatches this AlterOp to the matching ApplyXxx method
	// on the given Applier, passing the op's stored fields and resolved
	// Options. Returns the Applier's error verbatim. Used by driver-side
	// SchemaModifier.AlterCollection implementations to handle a slice
	// of AlterOp values without type-switching on unexported concrete
	// types.
	ApplyTo(ctx context.Context, a Applier) error
}
```

- [ ] **Step 2: Add the `context` import**

The current `ddl/alter_op.go` imports `dal` and `dbschema`. Add `context`. The import block becomes:

```go
import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)
```

- [ ] **Step 3: Add `ApplyTo` to each of the six concrete op types**

Edit `ddl/alter_op.go`. For each concrete type, ADD an `ApplyTo` method immediately AFTER the type's existing `alterOp()` marker method. The six additions (each is a single-line dispatch):

```go
// After `func (addFieldOp) alterOp() {}`:
func (o addFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyAddField(ctx, o.field, o.options)
}

// After `func (dropFieldOp) alterOp() {}`:
func (o dropFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyDropField(ctx, o.name, o.options)
}

// After `func (modifyFieldOp) alterOp() {}`:
func (o modifyFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyModifyField(ctx, o.name, o.newDef, o.options)
}

// After `func (renameFieldOp) alterOp() {}`:
func (o renameFieldOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyRenameField(ctx, o.oldName, o.newName, o.options)
}

// After `func (addIndexOp) alterOp() {}`:
func (o addIndexOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyAddIndex(ctx, o.index, o.options)
}

// After `func (dropIndexOp) alterOp() {}`:
func (o dropIndexOp) ApplyTo(ctx context.Context, a Applier) error {
	return a.ApplyDropIndex(ctx, o.name, o.options)
}
```

(The receivers gain a name `o` because the body references the stored fields. The existing `alterOp()` methods use unnamed receivers and stay unchanged.)

- [ ] **Step 4: Verify build**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go build ./ddl/...`
Expected: clean exit.

- [ ] **Step 5: Run the existing ddl tests for no regressions**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go test ./ddl/`
Expected: every pre-existing test PASSes. The interface-shape test from Task 2 also still passes.

- [ ] **Step 6: Commit**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
git add ddl/alter_op.go
git commit -m "$(cat <<'EOF'
feat(ddl): add ApplyTo to AlterOp interface + 6 concrete types

AlterOp interface gains a new public method:
  ApplyTo(ctx context.Context, a Applier) error

Each of the six in-package concrete op types (addFieldOp, dropFieldOp,
modifyFieldOp, renameFieldOp, addIndexOp, dropIndexOp) gains an
ApplyTo implementation that dispatches to the matching ApplyXxx
method on the Applier, forwarding the op's stored fields and resolved
Options.

Safe to add a public method to the sealed AlterOp interface: the
unexported alterOp() marker prevents external implementations, so
only the six in-package types ever satisfied AlterOp — all six
gain ApplyTo atomically in this commit.

Refs: spec/features/ddl/applier REQ:apply-to-method,
REQ:alter-op-interface-extended

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Per-AlterOp dispatch + error-propagation tests

**Files:**
- Modify: `ddl/applier_test.go`

- [ ] **Step 1: Append dispatch tests**

Append to `ddl/applier_test.go`:

```go
func TestApplyTo_AddField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	f := dbschema.FieldDef{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: false}
	op := AddField(f, IfNotExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.addFieldCalls != 1 {
		t.Errorf("addFieldCalls = %d, want 1", rec.addFieldCalls)
	}
	if rec.addField.Name != f.Name || rec.addField.Type != f.Type {
		t.Errorf("addField = %+v, want %+v", rec.addField, f)
	}
	if !rec.lastOpts.IfNotExists {
		t.Error("IfNotExists option not forwarded")
	}
}

func TestApplyTo_DropField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := DropField(dal.FieldName("email"), IfExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.dropFieldCalls != 1 {
		t.Errorf("dropFieldCalls = %d, want 1", rec.dropFieldCalls)
	}
	if string(rec.dropField) != "email" {
		t.Errorf("dropField = %q, want email", rec.dropField)
	}
	if !rec.lastOpts.IfExists {
		t.Error("IfExists option not forwarded")
	}
}

func TestApplyTo_ModifyField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	newDef := dbschema.FieldDef{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: false}
	op := ModifyField(dal.FieldName("email"), newDef)

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.modifyFieldCalls != 1 {
		t.Errorf("modifyFieldCalls = %d, want 1", rec.modifyFieldCalls)
	}
	if string(rec.modifyOld) != "email" {
		t.Errorf("modifyOld = %q, want email", rec.modifyOld)
	}
	if rec.modifyNew.Type != dbschema.String {
		t.Errorf("modifyNew.Type = %v, want String", rec.modifyNew.Type)
	}
}

func TestApplyTo_RenameField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := RenameField(dal.FieldName("email"), dal.FieldName("email_address"))

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.renameFieldCalls != 1 {
		t.Errorf("renameFieldCalls = %d, want 1", rec.renameFieldCalls)
	}
	if string(rec.renameOld) != "email" || string(rec.renameNew) != "email_address" {
		t.Errorf("rename = (%q, %q), want (email, email_address)", rec.renameOld, rec.renameNew)
	}
}

func TestApplyTo_AddIndex(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	idx := dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}}
	op := AddIndex(idx)

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.addIndexCalls != 1 {
		t.Errorf("addIndexCalls = %d, want 1", rec.addIndexCalls)
	}
	if rec.addIndex.Name != idx.Name {
		t.Errorf("addIndex.Name = %q, want %q", rec.addIndex.Name, idx.Name)
	}
}

func TestApplyTo_DropIndex(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := DropIndex("ix_users_email", IfExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.dropIndexCalls != 1 {
		t.Errorf("dropIndexCalls = %d, want 1", rec.dropIndexCalls)
	}
	if rec.dropIndex != "ix_users_email" {
		t.Errorf("dropIndex = %q, want ix_users_email", rec.dropIndex)
	}
	if !rec.lastOpts.IfExists {
		t.Error("IfExists option not forwarded")
	}
}

// TestApplyTo_PropagatesError verifies that ApplyTo returns the
// Applier's error verbatim (errors.Is identity).
func TestApplyTo_PropagatesError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("driver-side failure")
	rec := &recordingApplier{returnErr: sentinel}
	ctx := context.Background()

	// One representative op suffices; all six dispatch via the same
	// pattern (return a.ApplyXxx(...)).
	op := AddField(dbschema.FieldDef{Name: dal.FieldName("x"), Type: dbschema.Int})
	got := op.ApplyTo(ctx, rec)
	if !errors.Is(got, sentinel) {
		t.Errorf("ApplyTo error = %v, want %v (via errors.Is)", got, sentinel)
	}
}

// TestAlterOp_InterfaceHasApplyTo verifies the AlterOp interface
// itself now declares ApplyTo (catches accidental removal).
func TestAlterOp_InterfaceHasApplyTo(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*AlterOp)(nil)).Elem()
	var found bool
	for i := 0; i < typ.NumMethod(); i++ {
		if typ.Method(i).Name == "ApplyTo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AlterOp interface missing ApplyTo method")
	}
}
```

Add `"errors"` to the imports at the top of `ddl/applier_test.go`.

- [ ] **Step 2: Run tests**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go test ./ddl/ -run "TestApplyTo|TestAlterOp_Interface" -v`
Expected: all 8 tests PASS.

- [ ] **Step 3: Run the full ddl package — no regressions**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go test ./ddl/`
Expected: every test PASSes.

- [ ] **Step 4: Run go vet for sanity**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && go vet ./ddl/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
git add ddl/applier_test.go
git commit -m "$(cat <<'EOF'
test(ddl): dispatch + error-propagation tests for ApplyTo

Six per-AlterOp dispatch tests (AddField, DropField, ModifyField,
RenameField, AddIndex, DropIndex) verify that each constructor's
ApplyTo invokes the matching ApplyXxx method on the Applier with
the right arguments and forwards the resolved Options.

One error-propagation test verifies ApplyTo returns the Applier's
error verbatim (errors.Is identity).

One reflection test verifies the AlterOp interface declares ApplyTo.

Refs: spec/features/ddl/applier REQ:apply-to-method AC-1/AC-2/AC-3,
REQ:alter-op-interface-extended AC-1/AC-2

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Flip Feature Status to Implemented + push

**Files:**
- Modify: `spec/features/ddl/applier/README.md`
- Modify: `spec/features/ddl/README.md` (the Children table's Status implicit through prose — no edit needed unless the spec convention enforces explicit per-child status, which this repo's doesn't)

- [ ] **Step 1: Flip Status**

In `spec/features/ddl/applier/README.md`, replace `**Status:** Approved` with `**Status:** Implemented`.

- [ ] **Step 2: Re-lint**

Run: `cd /Users/alexandertrakhimenok/projects/dal-go/dalgo && specscore spec lint --severity error 2>&1 | tail -5`
Expected: `0 violations found`.

- [ ] **Step 3: Commit + push**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
git add spec/features/ddl/applier/README.md
git commit -m "$(cat <<'EOF'
docs(spec): mark ddl.Applier Implemented

All four REQs (applier-interface, apply-to-method, alter-op-interface-
extended, applier-callable-from-AlterCollection) are now shipped in
the ddl package. Unblocks AlterCollection dispatch in dalgo2sqlite
(Tasks 20-21 of its plan) and the upcoming dalgo2ingitdb coverage
Feature.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push
```

---

## Verification Checklist

After all 5 tasks land:

- [ ] `ddl/applier.go` exists; `ddl.Applier` interface with 6 methods declared
- [ ] `ddl/alter_op.go` modified: `AlterOp` interface declares `ApplyTo`; each of the 6 concrete types has an `ApplyTo` implementation
- [ ] `ddl/applier_test.go` exists; 8 tests pass (interface shape, 6 dispatch, 1 error-propagation, 1 AlterOp-has-ApplyTo)
- [ ] `go test ./ddl/` clean across all tests (new + pre-existing)
- [ ] `go vet ./ddl/...` clean
- [ ] `specscore spec lint --severity error` returns `0 violations found`
- [ ] Feature `Status: Implemented` in `spec/features/ddl/applier/README.md`
- [ ] All 5 task commits pushed to `origin/main`

## Out of Scope / Plan-Time Deferrals

- **Documenting which existing AlterOp Options each `ApplyXxx` method semantically cares about.** The alter-ops Feature already documents per-op Option semantics (e.g. `IfNotExists` on `AddField` is meaningful; `IfExists` on `AddField` is ignored). Driver-side decisions about what to forward / ignore are out of scope here.
- **Performance benchmarks.** ApplyTo is a single virtual-method call; no need to benchmark.
- **A default no-op Applier implementation.** Drivers implement Applier directly; if a no-op is ever needed for testing, it's a 30-line addition in the consumer's test package.
- **Updating the dalgo2sqlite plan's `sqliteAlterApplier` sketch** to match the ctx-in-signature contract. That's plan-time work in the dalgo2sqlite repo when Tasks 20-21 execute.

---

*This document follows the plan structure recommended by `superpowers:writing-plans`.*
