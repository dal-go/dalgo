# dbschema + ddl — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship two new top-level dalgo sub-packages — `dbschema` (schema description vocabulary + introspection capability) and `ddl` (schema modification execution surface) — implementing the 15 Approved Features across `spec/features/dbschema/` and `spec/features/ddl/` as a single integrated change.

**Architecture:** Top-level sub-packages siblings of `dal/` (matches the existing `orm/`, `update/`, `recordset/` convention). `dbschema` owns engine-neutral Tier-1 types + `SchemaReader` capability + the shared `NotSupportedError`. `ddl` imports `dbschema` and owns the write-side surface: `SchemaModifier` capability (3 methods), composable `AlterOp` model (6 constructors), `TransactionalDDL` capability, `Option` functional-options, `PartialSuccessError`. Both packages are purely additive — `dal.DB` is NOT modified.

**Tech Stack:** Go 1.24+, `testing` package, `github.com/stretchr/testify/assert` (matches existing `dal/` test convention; already in go.mod). Internal test package (`package dbschema`, `package ddl`) to match dalgo precedent.

**Spec roots:**
- [`spec/features/dbschema/README.md`](../features/dbschema/README.md)
- [`spec/features/ddl/README.md`](../features/ddl/README.md)

**Source Idea:** [`spec/ideas/dalgo-schema-modification.md`](../ideas/dalgo-schema-modification.md)

---

## File Map

### `dbschema/` (new top-level package)

| File | Contents | Created in |
|---|---|---|
| `dbschema/doc.go` | Package godoc | Task 1 |
| `dbschema/types.go` | `Type` enum + `Precision` | Task 1 |
| `dbschema/types_test.go` | Tests for Type/Precision | Task 1 |
| `dbschema/default_expr.go` | Sealed `DefaultExpr` + `DefaultLiteral` + `DefaultCurrentTimestamp` | Task 2 |
| `dbschema/default_expr_test.go` | Tests for DefaultExpr | Task 2 |
| `dbschema/errors.go` | `NotSupportedError` | Task 3 |
| `dbschema/errors_test.go` | Tests for NotSupportedError | Task 3 |
| `dbschema/field_def.go` | `FieldDef` | Task 4 |
| `dbschema/field_def_test.go` | Tests for FieldDef | Task 4 |
| `dbschema/index_def.go` | `IndexDef` | Task 5 |
| `dbschema/index_def_test.go` | Tests for IndexDef | Task 5 |
| `dbschema/collection_def.go` | `CollectionDef` | Task 6 |
| `dbschema/collection_def_test.go` | Tests for CollectionDef | Task 6 |
| `dbschema/constraint.go` | `ConstraintDef` | Task 7 |
| `dbschema/referrer.go` | `Referrer` | Task 7 |
| `dbschema/reader.go` | `SchemaReader` interface | Task 7 |
| `dbschema/reader_helpers.go` | Top-level helper functions | Task 7 |
| `dbschema/reader_test.go` | Tests for SchemaReader + helpers + supporting types | Task 7 |

### `ddl/` (new top-level package)

| File | Contents | Created in |
|---|---|---|
| `ddl/doc.go` | Package godoc | Task 8 |
| `ddl/options.go` | `Option`, `Options`, `IfNotExists`, `IfExists` | Task 8 |
| `ddl/options_test.go` | Tests for options | Task 8 |
| `ddl/errors.go` | `PartialSuccessError` | Task 9 |
| `ddl/errors_test.go` | Tests for PartialSuccessError | Task 9 |
| `ddl/transactional.go` | `TransactionalDDL` interface + `SupportsTransactionalDDL` helper | Task 10 |
| `ddl/transactional_test.go` | Tests for TransactionalDDL | Task 10 |
| `ddl/alter_op.go` | Sealed `AlterOp` interface + 6 constructors | Task 11 |
| `ddl/alter_op_test.go` | Tests for AlterOp constructors | Task 11 |
| `ddl/modifier.go` | `SchemaModifier` 3-method interface | Task 12 |
| `ddl/modifier_test.go` | Tests for SchemaModifier shape | Task 12 |
| `ddl/operations.go` | 3 top-level helper functions | Task 13 |
| `ddl/operations_test.go` | Tests for helpers (dispatch + not-supported paths) | Task 13 |

### Status transitions (Task 14)

15 Feature READMEs flip `Approved → Implemented`. Top-level `spec/features/README.md` index updates. Plan README index entry flips `Ready → Done`.

**No files outside `dbschema/` and `ddl/` are touched** until Task 14 (spec status flips). The `dal.DB` interface, existing drivers, mocks, and all other DALgo packages remain untouched — this is purely additive.

---

## Task 1: `dbschema/types` — `Type` enum and `Precision` struct

**Spec:** [`spec/features/dbschema/types/README.md`](../features/dbschema/types/README.md)

**Files:**
- Create: `dbschema/doc.go`
- Create: `dbschema/types.go`
- Create: `dbschema/types_test.go`

- [ ] **Step 1: Write the failing test**

Create `dbschema/types_test.go`:

```go
package dbschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestType_ConstantsExist(t *testing.T) {
	// Per REQ:type-enum AC-1 — all eight constants exist and are distinct.
	types := []Type{Null, Bool, Int, Float, String, Bytes, Time, Decimal}
	seen := make(map[Type]bool)
	for _, ty := range types {
		assert.False(t, seen[ty], "duplicate Type constant value: %d", ty)
		seen[ty] = true
	}
	assert.Len(t, seen, 8)
}

func TestType_NullIsZeroValue(t *testing.T) {
	// Per REQ:type-enum AC-2 — Null is the zero value.
	var t0 Type
	assert.Equal(t, Null, t0)
}

func TestType_String(t *testing.T) {
	// Per REQ:type-string AC-1 and AC-2 — non-empty lowercase strings, distinct.
	cases := map[Type]string{
		Null:    "null",
		Bool:    "bool",
		Int:     "int",
		Float:   "float",
		String:  "string",
		Bytes:   "bytes",
		Time:    "time",
		Decimal: "decimal",
	}
	seen := make(map[string]bool)
	for ty, want := range cases {
		got := ty.String()
		assert.Equal(t, want, got, "Type(%d).String()", ty)
		assert.False(t, seen[got], "duplicate string: %q", got)
		seen[got] = true
	}
}

func TestPrecision_Structure(t *testing.T) {
	// Per REQ:precision-struct AC-1.
	p := Precision{Total: 18, Scale: 4}
	assert.Equal(t, 18, p.Total)
	assert.Equal(t, 4, p.Scale)
}
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
go test ./dbschema/ -run "TestType|TestPrecision"
```

Expected: build failure with errors like `dbschema/types_test.go:9:2: package github.com/dal-go/dalgo/dbschema is not in std`.

- [ ] **Step 3: Create `dbschema/doc.go`**

```go
// Package dbschema provides a portable schema-description vocabulary
// and read-side (introspection) capability for DALgo.
//
// The package contains Tier-1 engine-neutral types — FieldDef,
// CollectionDef, IndexDef, Type, Precision, DefaultExpr and concretes —
// designed for three-tier composition: engine-specific extensions in
// each driver repo (Tier 2) embed Tier 1; application-specific
// wrappers in consumer repos (Tier 3) embed Tier 2.
//
// The package also defines the SchemaReader capability interface for
// schema introspection (the read-side mirror of [ddl.SchemaModifier])
// and the shared NotSupportedError typed error used by both the read
// and write sides.
//
// dbschema does NOT contain operations. CREATE / DROP / ALTER live in
// the sibling [ddl] sub-package, which imports dbschema for the types
// it operates on.
package dbschema
```

- [ ] **Step 4: Create `dbschema/types.go`**

```go
package dbschema

// Type is the portable column-type enumeration. Drivers translate
// these to engine-specific column types when emitting DDL.
//
// The zero value is [Null] so an unset FieldDef.Type is meaningful
// rather than silently Bool.
type Type int8

const (
	// Null indicates an unset or null-typed field. Also the zero value of Type.
	Null Type = iota
	// Bool is a boolean column.
	Bool
	// Int is an integer column. Drivers pick an appropriate width
	// (e.g. INT, BIGINT) based on optional Length hints.
	Int
	// Float is a floating-point column.
	Float
	// String is a textual column. Drivers honor optional Length
	// hints (e.g. VARCHAR(N) on PostgreSQL).
	String
	// Bytes is a binary blob column.
	Bytes
	// Time is a date-time column.
	Time
	// Decimal is a fixed-point numeric column. Drivers honor optional
	// Precision hints (e.g. NUMERIC(total, scale) on PostgreSQL).
	Decimal
)

// String returns a non-empty lowercase identifier suitable for
// diagnostic output. The returned strings are pairwise distinct.
func (t Type) String() string {
	switch t {
	case Null:
		return "null"
	case Bool:
		return "bool"
	case Int:
		return "int"
	case Float:
		return "float"
	case String:
		return "string"
	case Bytes:
		return "bytes"
	case Time:
		return "time"
	case Decimal:
		return "decimal"
	default:
		return "unknown"
	}
}

// Precision describes decimal precision. Total is the total number of
// significant digits; Scale is the number of digits to the right of
// the decimal point. Both are non-negative; the package does NOT
// enforce Scale <= Total.
type Precision struct {
	Total int
	Scale int
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./dbschema/ -run "TestType|TestPrecision" -v
```

Expected: `TestType_ConstantsExist`, `TestType_NullIsZeroValue`, `TestType_String`, `TestPrecision_Structure` all PASS.

- [ ] **Step 6: Commit**

```bash
git add dbschema/doc.go dbschema/types.go dbschema/types_test.go
git commit -m "feat(dbschema): add Type enum and Precision struct

Spec: spec/features/dbschema/types/.
Eight Type constants (Null/Bool/Int/Float/String/Bytes/Time/Decimal)
with Null as the zero value. String() method returns lowercase
identifiers for diagnostic output. Precision struct holds Total and
Scale for Decimal columns.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `dbschema/default-expr` — sealed interface + concretes

**Spec:** [`spec/features/dbschema/default-expr/README.md`](../features/dbschema/default-expr/README.md)

**Files:**
- Create: `dbschema/default_expr.go`
- Create: `dbschema/default_expr_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/default_expr_test.go`:

```go
package dbschema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExpr_InterfaceExists(t *testing.T) {
	// Per REQ:default-expr-interface AC-1.
	var d DefaultExpr
	assert.Nil(t, d) // interface zero value is nil
}

func TestDefaultExpr_HasUnexportedMarker(t *testing.T) {
	// Per REQ:default-expr-interface AC-2 — sealed via unexported marker method.
	typ := reflect.TypeOf((*DefaultExpr)(nil)).Elem()
	assert.Equal(t, reflect.Interface, typ.Kind())
	require := typ.NumMethod()
	assert.Equal(t, 1, require, "expected exactly one method on DefaultExpr")
	method := typ.Method(0)
	// Unexported methods are reported by reflect with their package path set.
	assert.NotEmpty(t, method.PkgPath, "method %q should be unexported (have a PkgPath)", method.Name)
}

func TestDefaultLiteral_Satisfies(t *testing.T) {
	// Per REQ:default-literal AC-1.
	var d DefaultExpr = DefaultLiteral{Value: 0}
	_ = d
}

func TestDefaultLiteral_ValueAccessible(t *testing.T) {
	// Per REQ:default-literal AC-2.
	d := DefaultLiteral{Value: "guest"}
	v, ok := d.Value.(string)
	assert.True(t, ok)
	assert.Equal(t, "guest", v)
}

func TestDefaultCurrentTimestamp_Satisfies(t *testing.T) {
	// Per REQ:default-current-timestamp AC-1.
	var d DefaultExpr = DefaultCurrentTimestamp{}
	_ = d
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run TestDefault
```

Expected: build failure with `undefined: DefaultExpr`, `undefined: DefaultLiteral`, `undefined: DefaultCurrentTimestamp`.

- [ ] **Step 3: Create `dbschema/default_expr.go`**

```go
package dbschema

// DefaultExpr is the sealed interface for column default value
// expressions. The interface is sealed via an unexported marker method
// (defaultExpr) so the set of valid DefaultExpr cases is closed at the
// package boundary — drivers know which cases exist and translate
// accordingly. New cases require adding to this package.
//
// MVP concretes:
//   - [DefaultLiteral] carries a plain Go value
//   - [DefaultCurrentTimestamp] signals "use the engine's current
//     timestamp default at insertion time"
type DefaultExpr interface {
	defaultExpr() // sealed marker
}

// DefaultLiteral is a column default that uses a literal Go value.
// Drivers MUST handle at minimum: int, int64, float64, string, bool,
// []byte, and nil. Drivers MAY return a driver-specific error if the
// underlying Go type cannot be serialized to the target engine.
type DefaultLiteral struct {
	Value any
}

func (DefaultLiteral) defaultExpr() {}

// DefaultCurrentTimestamp signals "use the engine's current-timestamp
// default at insertion time." Drivers translate to engine-specific
// SQL (CURRENT_TIMESTAMP on SQLite, now() or CURRENT_TIMESTAMP on
// PostgreSQL) or the equivalent.
//
// Cross-engine precision: SQLite returns timestamps at second
// resolution by default; PostgreSQL at microsecond. The Feature does
// not pin precision — drivers translate as their engine allows.
type DefaultCurrentTimestamp struct{}

func (DefaultCurrentTimestamp) defaultExpr() {}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./dbschema/ -run TestDefault -v
```

Expected: all five `TestDefault*` and `TestDefaultLiteral*` and `TestDefaultCurrentTimestamp*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dbschema/default_expr.go dbschema/default_expr_test.go
git commit -m "feat(dbschema): add sealed DefaultExpr interface + concretes

Spec: spec/features/dbschema/default-expr/.
Sealed DefaultExpr interface (unexported marker method) with two MVP
concretes: DefaultLiteral{Value any} for plain literal defaults and
DefaultCurrentTimestamp{} for the cross-engine current-timestamp
default.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `dbschema/errors` — `NotSupportedError` typed wrapper

**Spec:** [`spec/features/dbschema/errors/README.md`](../features/dbschema/errors/README.md)

**Files:**
- Create: `dbschema/errors.go`
- Create: `dbschema/errors_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/errors_test.go`:

```go
package dbschema

import (
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestNotSupportedError_Fields(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-1.
	e := NotSupportedError{Op: "CreateCollection", Backend: "dalgo2sql/sqlite", Reason: "read-only"}
	assert.Equal(t, "CreateCollection", e.Op)
	assert.Equal(t, "dalgo2sql/sqlite", e.Backend)
	assert.Equal(t, "read-only", e.Reason)
}

func TestNotSupportedError_ErrorString(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-2.
	e := &NotSupportedError{Op: "CreateCollection", Backend: "dalgo2sql/sqlite", Reason: "read-only mode"}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "CreateCollection"), "missing Op in: %q", s)
	assert.True(t, strings.Contains(s, "dalgo2sql/sqlite"), "missing Backend in: %q", s)
	assert.True(t, strings.Contains(s, "read-only mode"), "missing Reason in: %q", s)
}

func TestNotSupportedError_ErrorStringWithEmptyFields(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-3 — non-empty, no panic, no verbatim empties.
	e := &NotSupportedError{Op: "DescribeCollection"}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "DescribeCollection"), "missing Op in: %q", s)
}

func TestNotSupportedError_ErrorsIs(t *testing.T) {
	// Per REQ:unwrap-to-sentinel AC-1.
	err := &NotSupportedError{Op: "CreateCollection"}
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
}

func TestNotSupportedError_ErrorsAs(t *testing.T) {
	// Per REQ:unwrap-to-sentinel AC-2.
	var err error = &NotSupportedError{Op: "DropIndex", Backend: "x"}
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "DropIndex", ue.Op)
	assert.Equal(t, "x", ue.Backend)
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run TestNotSupportedError
```

Expected: build failure with `undefined: NotSupportedError`.

- [ ] **Step 3: Create `dbschema/errors.go`**

```go
package dbschema

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// NotSupportedError is the typed error returned by dbschema and ddl
// helper functions when a driver does not support a given operation
// (either because the driver does not implement the relevant
// capability interface at all, or because the specific operation is
// unsupported).
//
// NotSupportedError.Unwrap returns the existing [dal.ErrNotSupported]
// sentinel so callers can do a coarse errors.Is(err, dal.ErrNotSupported)
// check or extract detail via errors.As(err, &ue) with ue of type
// *dbschema.NotSupportedError.
//
// The same error type is used by both the read side (this package's
// helpers) and the write side ([ddl] package helpers) so consumers
// have a single typed error to handle across the whole DDL surface.
type NotSupportedError struct {
	// Op names the operation that was not supported (e.g.
	// "CreateCollection", "DescribeCollection", "DropIndex").
	Op string
	// Backend optionally identifies the driver (e.g.
	// "dalgo2sql/sqlite"). When the helper function constructs the
	// error after a failed type assertion, it sets Backend from
	// db.Adapter().Name() if Adapter() returns a non-nil dal.Adapter;
	// otherwise Backend is left empty.
	Backend string
	// Reason is an optional human-readable explanation.
	Reason string
}

// Error returns a readable single-line message.
func (e *NotSupportedError) Error() string {
	parts := []string{"dbschema: operation not supported"}
	if e.Op != "" {
		parts = append(parts, fmt.Sprintf("op=%s", e.Op))
	}
	if e.Backend != "" {
		parts = append(parts, fmt.Sprintf("backend=%s", e.Backend))
	}
	if e.Reason != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", e.Reason))
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "; " + p
	}
	return out
}

// Unwrap returns the dal.ErrNotSupported sentinel so callers can use
// errors.Is(err, dal.ErrNotSupported) for a coarse check.
func (e *NotSupportedError) Unwrap() error {
	return dal.ErrNotSupported
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./dbschema/ -run TestNotSupportedError -v
```

Expected: all five `TestNotSupportedError_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dbschema/errors.go dbschema/errors_test.go
git commit -m "feat(dbschema): add NotSupportedError typed wrapper

Spec: spec/features/dbschema/errors/.
Typed error wrapping the existing dal.ErrNotSupported sentinel.
Used by BOTH the read side (dbschema.SchemaReader and helpers) AND
the write side (ddl.SchemaModifier and helpers). errors.Is works for
coarse checks; errors.As extracts Op/Backend/Reason detail.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: `dbschema/field-def` — `FieldDef` struct

**Spec:** [`spec/features/dbschema/field-def/README.md`](../features/dbschema/field-def/README.md)

**Files:**
- Create: `dbschema/field_def.go`
- Create: `dbschema/field_def_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/field_def_test.go`:

```go
package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func intPtr(i int) *int { return &i }

func TestFieldDef_Compiles(t *testing.T) {
	// Per REQ:field-def-struct AC-1.
	f := FieldDef{
		Name:     dal.FieldName("email"),
		Type:     String,
		Length:   intPtr(255),
		Nullable: false,
	}
	assert.Equal(t, dal.FieldName("email"), f.Name)
	assert.Equal(t, String, f.Type)
	assert.Equal(t, 255, *f.Length)
	assert.False(t, f.Nullable)
}

func TestFieldDef_ZeroValue(t *testing.T) {
	// Per REQ:field-def-struct AC-2.
	var f FieldDef
	assert.Equal(t, dal.FieldName(""), f.Name)
	assert.Equal(t, Null, f.Type)
	assert.Nil(t, f.Length)
	assert.Nil(t, f.Precision)
	assert.False(t, f.Nullable)
	assert.Nil(t, f.Default)
	assert.False(t, f.AutoIncrement)
}

func TestFieldDef_DecimalPrecision(t *testing.T) {
	// Per REQ:field-def-struct AC-3.
	f := FieldDef{
		Name:      "amount",
		Type:      Decimal,
		Precision: &Precision{Total: 18, Scale: 4},
	}
	assert.Equal(t, 18, f.Precision.Total)
	assert.Equal(t, 4, f.Precision.Scale)
}

func TestFieldDef_DefaultLiteral(t *testing.T) {
	// Per REQ:field-def-struct AC-4.
	f := FieldDef{
		Name:    "status",
		Type:    String,
		Default: DefaultLiteral{Value: "active"},
	}
	assert.NotNil(t, f.Default)
	lit, ok := f.Default.(DefaultLiteral)
	assert.True(t, ok)
	assert.Equal(t, "active", lit.Value)
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run TestFieldDef
```

Expected: build failure with `undefined: FieldDef`.

- [ ] **Step 3: Create `dbschema/field_def.go`**

```go
package dbschema

import "github.com/dal-go/dalgo/dal"

// FieldDef is the portable description of one field (a.k.a. column)
// of a [CollectionDef].
//
// FieldDef is the schema-DEFINITION shape. It is distinct from the
// unrelated runtime types in the dal package: [dal.Column] (a
// SELECT-clause expression+alias), [dal.FieldRef] (a query field
// reference), [dal.FieldVal] (a runtime name+value pair). Those exist
// for query and runtime concerns; FieldDef exists to describe a
// column's structure in a portable way.
//
// AutoIncrement is advisory: drivers MAY restrict it to integer types
// in the primary key and return *NotSupportedError if a caller passes
// AutoIncrement on a non-integer field or a field not in the primary
// key. dbschema itself does NOT enforce the restriction.
type FieldDef struct {
	// Name is the field's identifier.
	Name dal.FieldName
	// Type is the portable column type.
	Type Type
	// Length is an optional length hint for String / Bytes types.
	// nil means "driver default."
	Length *int
	// Precision is an optional precision hint for Decimal types.
	// nil means "driver default."
	Precision *Precision
	// Nullable is true if the field permits NULL values. Default
	// (zero value) is false = NOT NULL.
	Nullable bool
	// Default is an optional default expression. nil means "no
	// default."
	Default DefaultExpr
	// AutoIncrement is true if the field should auto-generate values.
	// Typically restricted by drivers to integer primary-key fields.
	AutoIncrement bool
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./dbschema/ -run TestFieldDef -v
```

Expected: all four `TestFieldDef_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dbschema/field_def.go dbschema/field_def_test.go
git commit -m "feat(dbschema): add FieldDef struct

Spec: spec/features/dbschema/field-def/.
Portable field/column definition: Name (dal.FieldName), Type,
optional Length, optional Precision, Nullable (default false),
optional Default (DefaultExpr), AutoIncrement (advisory).
Godoc cross-references the unrelated dal.Column/FieldRef/FieldVal
runtime types.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `dbschema/index-def` — `IndexDef` struct

**Spec:** [`spec/features/dbschema/index-def/README.md`](../features/dbschema/index-def/README.md)

**Files:**
- Create: `dbschema/index_def.go`
- Create: `dbschema/index_def_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/index_def_test.go`:

```go
package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestIndexDef_Compiles(t *testing.T) {
	// Per REQ:index-def-struct AC-1.
	idx := IndexDef{
		Name:       "ix_users_email",
		Collection: "users",
		Fields:     []dal.FieldName{"email"},
		Unique:     true,
	}
	assert.Equal(t, "ix_users_email", idx.Name)
	assert.Equal(t, "users", idx.Collection)
	assert.Len(t, idx.Fields, 1)
	assert.True(t, idx.Unique)
}

func TestIndexDef_CompositeIndex(t *testing.T) {
	// Per REQ:index-def-struct AC-2.
	idx := IndexDef{
		Name:       "ix_orders_status_created",
		Collection: "orders",
		Fields:     []dal.FieldName{"status", "created_at"},
		Unique:     false,
	}
	assert.Len(t, idx.Fields, 2)
	assert.Equal(t, dal.FieldName("status"), idx.Fields[0])
	assert.Equal(t, dal.FieldName("created_at"), idx.Fields[1])
}

func TestIndexDef_ZeroValue(t *testing.T) {
	// Per REQ:index-def-struct AC-3.
	var idx IndexDef
	assert.Empty(t, idx.Name)
	assert.Empty(t, idx.Collection)
	assert.Nil(t, idx.Fields)
	assert.False(t, idx.Unique)
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run TestIndexDef
```

Expected: build failure with `undefined: IndexDef`.

- [ ] **Step 3: Create `dbschema/index_def.go`**

```go
package dbschema

import "github.com/dal-go/dalgo/dal"

// IndexDef is the portable description of one index on a
// [CollectionDef]. The Fields slice is ordered — order matters for
// composite indexes (field ordinality affects which queries the
// index can serve).
//
// Collection is the simple name of the collection the index belongs
// to. Richer collection addressing (catalog/schema/parent-key) lives
// in [dal.CollectionRef] and is the argument type passed to reader
// and writer methods; this stored value is a plain name. Tier-2
// engine extensions MAY add a richer reference field if needed.
type IndexDef struct {
	// Name is the index name.
	Name string
	// Collection is the simple name of the collection this index belongs to.
	Collection string
	// Fields is the ordered list of fields the index covers.
	Fields []dal.FieldName
	// Unique is true if the index enforces uniqueness.
	Unique bool
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./dbschema/ -run TestIndexDef -v
```

Expected: all three `TestIndexDef_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dbschema/index_def.go dbschema/index_def_test.go
git commit -m "feat(dbschema): add IndexDef struct

Spec: spec/features/dbschema/index-def/.
Portable index definition: Name, Collection (plain name), Fields
(ordered slice of dal.FieldName for composite-index support), Unique.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: `dbschema/collection-def` — `CollectionDef` struct

**Spec:** [`spec/features/dbschema/collection-def/README.md`](../features/dbschema/collection-def/README.md)

**Files:**
- Create: `dbschema/collection_def.go`
- Create: `dbschema/collection_def_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/collection_def_test.go`:

```go
package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestCollectionDef_Compiles(t *testing.T) {
	// Per REQ:collection-def-struct AC-1.
	c := CollectionDef{
		Name: "users",
		Fields: []FieldDef{
			{Name: "id", Type: Int},
			{Name: "email", Type: String},
		},
		PrimaryKey: []dal.FieldName{"id"},
	}
	assert.Equal(t, "users", c.Name)
	assert.Len(t, c.Fields, 2)
	assert.Len(t, c.PrimaryKey, 1)
}

func TestCollectionDef_CompositePK(t *testing.T) {
	// Per REQ:collection-def-struct AC-2.
	c := CollectionDef{
		Name:       "tenant_users",
		Fields:     []FieldDef{{Name: "tenant_id", Type: Int}, {Name: "user_id", Type: Int}},
		PrimaryKey: []dal.FieldName{"tenant_id", "user_id"},
	}
	assert.Len(t, c.PrimaryKey, 2)
	assert.Equal(t, dal.FieldName("tenant_id"), c.PrimaryKey[0])
	assert.Equal(t, dal.FieldName("user_id"), c.PrimaryKey[1])
}

func TestCollectionDef_WithIndexes(t *testing.T) {
	// Per REQ:collection-def-struct AC-3.
	c := CollectionDef{
		Name:   "users",
		Fields: []FieldDef{{Name: "id", Type: Int}, {Name: "email", Type: String}},
		Indexes: []IndexDef{
			{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true},
		},
	}
	assert.Len(t, c.Fields, 2)
	assert.Len(t, c.Indexes, 1)
}

func TestCollectionDef_ZeroValue(t *testing.T) {
	// Per REQ:collection-def-struct AC-4.
	var c CollectionDef
	assert.Empty(t, c.Name)
	assert.Nil(t, c.Fields)
	assert.Nil(t, c.PrimaryKey)
	assert.Nil(t, c.Indexes)
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run TestCollectionDef
```

Expected: build failure with `undefined: CollectionDef`.

- [ ] **Step 3: Create `dbschema/collection_def.go`**

```go
package dbschema

import "github.com/dal-go/dalgo/dal"

// CollectionDef is the portable description of one collection (a.k.a.
// table) — its name, ordered fields, primary key, and inline
// declared secondary indexes.
//
// PrimaryKey is a slice of dal.FieldName. A single-field PK is a
// one-element slice; a composite PK has multiple entries; an empty
// slice means "no primary key declared" and driver-specific behavior
// applies (SQLite may auto-assign ROWID, PostgreSQL may reject, etc.).
//
// Indexes declared inline with CollectionDef are created together
// with the collection in a CreateCollection call. Indexes added or
// removed AFTER the collection exists are passed as ddl.AddIndex /
// ddl.DropIndex AlterOps to ddl.AlterCollection.
//
// The package does NOT validate that PrimaryKey or Indexes reference
// fields actually present in Fields. That's a driver concern —
// drivers MUST return a driver-specific error (or *NotSupportedError
// if the operation itself isn't supported) when validating against
// the engine.
type CollectionDef struct {
	// Name is the collection / table name.
	Name string
	// Fields lists the fields (columns) in declared order.
	Fields []FieldDef
	// PrimaryKey lists the names of fields composing the primary key.
	// Empty = no PK declared (driver-specific behavior applies).
	PrimaryKey []dal.FieldName
	// Indexes lists the secondary indexes declared inline with this
	// collection definition.
	Indexes []IndexDef
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./dbschema/ -run TestCollectionDef -v
```

Expected: all four `TestCollectionDef_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add dbschema/collection_def.go dbschema/collection_def_test.go
git commit -m "feat(dbschema): add CollectionDef struct

Spec: spec/features/dbschema/collection-def/.
Portable collection (table) definition: Name, Fields (ordered slice
of FieldDef), PrimaryKey (slice of dal.FieldName for composite-PK
support), Indexes (inline IndexDef slice).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: `dbschema/schema-reader` — interface + helpers + ConstraintDef + Referrer

**Spec:** [`spec/features/dbschema/schema-reader/README.md`](../features/dbschema/schema-reader/README.md)

**Files:**
- Create: `dbschema/constraint.go`
- Create: `dbschema/referrer.go`
- Create: `dbschema/reader.go`
- Create: `dbschema/reader_helpers.go`
- Create: `dbschema/reader_test.go`

- [ ] **Step 1: Write the failing tests**

Create `dbschema/reader_test.go`:

```go
package dbschema

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

// stubAdapter implements dal.Adapter with a controllable name.
type stubAdapter struct{ name string }

func (s stubAdapter) Name() string { return s.name }

// stubDB is a minimal dal.DB stub. It optionally implements SchemaReader
// when readerEnabled is true. Most methods panic; tests use only the
// ones they need.
type stubDB struct {
	dal.NoConcurrency
	adapter dal.Adapter
	reader  *recordingReader
}

func (s *stubDB) ID() string { return "stub" }
func (s *stubDB) Adapter() dal.Adapter {
	return s.adapter
}
func (s *stubDB) Schema() dal.Schema { return nil }
func (s *stubDB) RunReadonlyTransaction(_ context.Context, _ dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *stubDB) RunReadwriteTransaction(_ context.Context, _ dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *stubDB) Get(_ context.Context, _ dal.Record) error { return errors.New("not used") }
func (s *stubDB) Exists(_ context.Context, _ *dal.Key) (bool, error) {
	return false, errors.New("not used")
}
func (s *stubDB) GetMulti(_ context.Context, _ []dal.Record) error { return errors.New("not used") }
func (s *stubDB) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, errors.New("not used")
}
func (s *stubDB) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("not used")
}

// recordingReader implements SchemaReader and captures the last call.
type recordingReader struct {
	lastOp string
	lastArg any
}

func (r *recordingReader) ListCollections(_ context.Context, parent *dal.Key) ([]dal.CollectionRef, error) {
	r.lastOp = "ListCollections"
	r.lastArg = parent
	return nil, nil
}
func (r *recordingReader) DescribeCollection(_ context.Context, ref *dal.CollectionRef) (*CollectionDef, error) {
	r.lastOp = "DescribeCollection"
	r.lastArg = ref
	return &CollectionDef{Name: "users"}, nil
}
func (r *recordingReader) ListIndexes(_ context.Context, ref *dal.CollectionRef) ([]IndexDef, error) {
	r.lastOp = "ListIndexes"
	r.lastArg = ref
	return nil, nil
}
func (r *recordingReader) ListConstraints(_ context.Context, ref *dal.CollectionRef) ([]ConstraintDef, error) {
	r.lastOp = "ListConstraints"
	r.lastArg = ref
	return nil, nil
}
func (r *recordingReader) ListReferrers(_ context.Context, ref *dal.CollectionRef) ([]Referrer, error) {
	r.lastOp = "ListReferrers"
	r.lastArg = ref
	return nil, nil
}

// readerStubDB embeds stubDB AND a recordingReader, so it satisfies
// both dal.DB and dbschema.SchemaReader.
type readerStubDB struct {
	*stubDB
	*recordingReader
}

func newReaderStubDB(name string) *readerStubDB {
	r := &recordingReader{}
	return &readerStubDB{
		stubDB:          &stubDB{adapter: stubAdapter{name: name}},
		recordingReader: r,
	}
}

func TestSchemaReader_InterfaceExists(t *testing.T) {
	// Per REQ:schema-reader-interface AC-1 + AC-2.
	var _ SchemaReader = (*recordingReader)(nil)
}

func TestConstraintDef_Compiles(t *testing.T) {
	// Per REQ:supporting-types AC-1.
	var c ConstraintDef = ConstraintDef{Name: "uq_email", Type: "unique"}
	_ = c
}

func TestReferrer_Compiles(t *testing.T) {
	// Per REQ:supporting-types AC-1.
	var r Referrer = Referrer{
		Collection: dal.CollectionRef{}, // zero-value CollectionRef
		Fields:     []dal.FieldName{"user_id"},
	}
	_ = r
}

func TestHelpers_Compile(t *testing.T) {
	// Per REQ:helper-functions AC-1.
	_ = ListCollections
	_ = DescribeCollection
	_ = ListIndexes
	_ = ListConstraints
	_ = ListReferrers
}

func TestDescribeCollection_Dispatches(t *testing.T) {
	// Per REQ:helper-functions AC-2.
	db := newReaderStubDB("stub-driver")
	ref := &dal.CollectionRef{}
	result, err := DescribeCollection(context.Background(), db, ref)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "DescribeCollection", db.recordingReader.lastOp)
	assert.Equal(t, ref, db.recordingReader.lastArg)
}

func TestDescribeCollection_NotImplementer(t *testing.T) {
	// Per REQ:helper-functions AC-3.
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := DescribeCollection(context.Background(), db, &dal.CollectionRef{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "DescribeCollection", ue.Op)
	assert.Equal(t, "no-reader", ue.Backend)
}
```

The test file also needs an import of `github.com/dal-go/dalgo/recordset` (for the `recordset.Option` referenced in the stubDB's `ExecuteQueryToRecordsetReader` method). Add to the imports block at the top of the file:

```go
import (
	// ...existing imports...
	"github.com/dal-go/dalgo/recordset"
)
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./dbschema/ -run "TestSchemaReader|TestConstraintDef|TestReferrer|TestHelpers|TestDescribeCollection"
```

Expected: build failure with `undefined: SchemaReader`, `undefined: ConstraintDef`, `undefined: Referrer`, `undefined: ListCollections`, etc.

- [ ] **Step 3: Create `dbschema/constraint.go`**

```go
package dbschema

// ConstraintDef is the portable description of one constraint on a
// collection. Tier-1 keeps this minimal — Name and Type only. The
// richer shape required for specific constraint kinds (check
// expression, unique field list, foreign-key target + cascade
// actions, etc.) is intentionally deferred. Engine-specific reader
// extensions (Tier 2) MAY define richer constraint types in their
// own packages without waiting for Tier 1 to grow.
type ConstraintDef struct {
	// Name is the constraint name.
	Name string
	// Type is the engine-neutral kind:
	// "check", "unique", "primary-key", "foreign-key".
	Type string
}
```

- [ ] **Step 4: Create `dbschema/referrer.go`**

```go
package dbschema

import "github.com/dal-go/dalgo/dal"

// Referrer describes one collection that references the queried
// collection via a foreign key.
type Referrer struct {
	// Collection identifies the referencing collection.
	Collection dal.CollectionRef
	// Fields lists the fields in Collection that reference back to
	// the queried collection.
	Fields []dal.FieldName
}
```

- [ ] **Step 5: Create `dbschema/reader.go`**

```go
package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// SchemaReader is the capability interface for schema introspection.
// Drivers that support introspection (SQL backends via
// information_schema / SQLite pragmas, Firestore via the admin API,
// etc.) opt in by implementing SchemaReader on their dal.DB value or
// a related type reachable via type assertion. Drivers that don't
// implement SchemaReader simply don't satisfy it; the top-level
// helper functions return *NotSupportedError on the failed
// assertion.
//
// ListCollections, DescribeCollection, and ListIndexes are REQUIRED
// for drivers that implement SchemaReader at all. ListConstraints and
// ListReferrers are OPTIONAL: drivers whose backend lacks the
// concept (e.g. Firestore has no SQL-style constraints) MUST return
// *NotSupportedError from those methods. The interface satisfaction
// is structural — all five methods must be present for a driver to
// satisfy SchemaReader; runtime behavior decides which methods
// actually do work.
type SchemaReader interface {
	// ListCollections returns the collections (tables) accessible to
	// db. The optional parent key narrows scope when the backend
	// supports hierarchical addressing (e.g. SQL catalog/schema).
	// Pass nil for "everything visible."
	ListCollections(ctx context.Context, parent *dal.Key) ([]dal.CollectionRef, error)

	// DescribeCollection returns the structural definition of one
	// collection, including its fields, primary key, and inline
	// indexes.
	DescribeCollection(ctx context.Context, ref *dal.CollectionRef) (*CollectionDef, error)

	// ListIndexes returns the indexes on a collection. The returned
	// slice MAY include indexes already reported inline via
	// DescribeCollection's Indexes field.
	ListIndexes(ctx context.Context, ref *dal.CollectionRef) ([]IndexDef, error)

	// ListConstraints is OPTIONAL. Drivers that do not support
	// constraint introspection MUST return *NotSupportedError.
	ListConstraints(ctx context.Context, ref *dal.CollectionRef) ([]ConstraintDef, error)

	// ListReferrers is OPTIONAL. Drivers MAY return
	// *NotSupportedError. Returns the collections that reference ref
	// via foreign keys.
	ListReferrers(ctx context.Context, ref *dal.CollectionRef) ([]Referrer, error)
}
```

- [ ] **Step 6: Create `dbschema/reader_helpers.go`**

```go
package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// backendName extracts the driver name from db.Adapter() if non-nil,
// otherwise returns the empty string.
func backendName(db dal.DB) string {
	if db == nil {
		return ""
	}
	a := db.Adapter()
	if a == nil {
		return ""
	}
	return a.Name()
}

// notSupportedReader returns a *NotSupportedError for the given op
// when db does not implement SchemaReader.
func notSupportedReader(op string, db dal.DB) error {
	return &NotSupportedError{
		Op:      op,
		Backend: backendName(db),
		Reason:  "driver does not implement dbschema.SchemaReader",
	}
}

// ListCollections type-asserts db to SchemaReader and delegates;
// returns *NotSupportedError if the assertion fails.
func ListCollections(ctx context.Context, db dal.DB, parent *dal.Key) ([]dal.CollectionRef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListCollections", db)
	}
	return r.ListCollections(ctx, parent)
}

// DescribeCollection type-asserts db to SchemaReader and delegates.
func DescribeCollection(ctx context.Context, db dal.DB, ref *dal.CollectionRef) (*CollectionDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("DescribeCollection", db)
	}
	return r.DescribeCollection(ctx, ref)
}

// ListIndexes type-asserts db to SchemaReader and delegates.
func ListIndexes(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]IndexDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListIndexes", db)
	}
	return r.ListIndexes(ctx, ref)
}

// ListConstraints type-asserts db to SchemaReader and delegates.
func ListConstraints(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]ConstraintDef, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListConstraints", db)
	}
	return r.ListConstraints(ctx, ref)
}

// ListReferrers type-asserts db to SchemaReader and delegates.
func ListReferrers(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]Referrer, error) {
	r, ok := db.(SchemaReader)
	if !ok {
		return nil, notSupportedReader("ListReferrers", db)
	}
	return r.ListReferrers(ctx, ref)
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./dbschema/ -v
```

Expected: all tests across all six dbschema test files PASS.

- [ ] **Step 8: Commit**

```bash
git add dbschema/constraint.go dbschema/referrer.go dbschema/reader.go dbschema/reader_helpers.go dbschema/reader_test.go
git commit -m "feat(dbschema): add SchemaReader capability + helpers + supporting types

Spec: spec/features/dbschema/schema-reader/.
Five-method SchemaReader interface (ListCollections,
DescribeCollection, ListIndexes, ListConstraints, ListReferrers) +
five top-level helper functions that type-assert and dispatch (or
return *NotSupportedError). Supporting types ConstraintDef and
Referrer are Tier-1 minimal; richer shapes are Tier-2 extensions.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: `ddl/options` — `Option` + `IfNotExists` + `IfExists`

**Spec:** [`spec/features/ddl/options/README.md`](../features/ddl/options/README.md)

**Files:**
- Create: `ddl/doc.go`
- Create: `ddl/options.go`
- Create: `ddl/options_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ddl/options_test.go`:

```go
package ddl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOption_TypesCompile(t *testing.T) {
	// Per REQ:option-type AC-1.
	var fn Option
	var o Options
	_ = fn
	_ = o
}

func TestIfNotExists_SetsFlag(t *testing.T) {
	// Per REQ:option-constructors AC-1.
	var o Options
	IfNotExists()(&o)
	assert.True(t, o.IfNotExists)
	assert.False(t, o.IfExists)
}

func TestIfExists_SetsFlag(t *testing.T) {
	// Per REQ:option-constructors AC-2.
	var o Options
	IfExists()(&o)
	assert.True(t, o.IfExists)
	assert.False(t, o.IfNotExists)
}

func TestOptions_Independent(t *testing.T) {
	// Per REQ:option-constructors AC-3.
	var o Options
	IfNotExists()(&o)
	IfExists()(&o)
	assert.True(t, o.IfNotExists)
	assert.True(t, o.IfExists)
}

func TestResolveOptions_Helper(t *testing.T) {
	// Helper used by driver implementations and the alter_op
	// constructors to resolve a slice of Option into an Options struct.
	got := ResolveOptions(IfNotExists(), IfExists())
	assert.True(t, got.IfNotExists)
	assert.True(t, got.IfExists)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./ddl/ -run "TestOption|TestIfNotExists|TestIfExists|TestOptions|TestResolveOptions"
```

Expected: build failure (package doesn't exist yet) — `package github.com/dal-go/dalgo/ddl is not in std`.

- [ ] **Step 3: Create `ddl/doc.go`**

```go
// Package ddl provides the schema-modification execution surface for
// DALgo. It defines the SchemaModifier capability interface, the
// composable AlterOp model for collection alterations, the
// TransactionalDDL capability for atomicity advertisement, and
// top-level helper functions that wrap a type assertion on dal.DB.
//
// ddl imports [github.com/dal-go/dalgo/dbschema] for the structural
// types it operates on (CollectionDef, FieldDef, IndexDef, etc.) AND
// for the shared NotSupportedError typed error. Drivers that
// implement DDL satisfy ddl.SchemaModifier; drivers that don't
// cause helper functions to return *dbschema.NotSupportedError.
package ddl
```

- [ ] **Step 4: Create `ddl/options.go`**

```go
package ddl

// Options is the resolved set of functional options for the
// collection-level and AlterOp-level operations.
//
// IfNotExists makes Create / Add operations idempotent — the target
// existing already is a no-op rather than an error. IfExists makes
// Drop operations idempotent — the target being absent is a no-op.
//
// Drivers MUST silently ignore semantically-mismatched options:
//   - IfNotExists on Drop operations
//   - IfExists on Create / Add operations
//   - Any option on ModifyField or RenameField
//
// The meaning is unambiguous (there is nothing to do with a
// mismatched hint), so a real error there would be a footgun.
type Options struct {
	IfNotExists bool
	IfExists    bool
}

// Option is the functional-option type accepted by CreateCollection,
// DropCollection, and all six AlterOp constructors via variadic
// opts ...Option. AlterCollection itself does NOT accept Option
// values directly — each AlterOp carries its own resolved Options
// set by the caller through the constructor.
type Option func(*Options)

// IfNotExists makes a Create / Add operation idempotent: the target
// already existing is a no-op rather than an error. Meaningless and
// silently ignored on Drop operations.
func IfNotExists() Option {
	return func(o *Options) { o.IfNotExists = true }
}

// IfExists makes a Drop operation idempotent: the target being
// absent is a no-op rather than an error. Meaningless and silently
// ignored on Create / Add operations.
func IfExists() Option {
	return func(o *Options) { o.IfExists = true }
}

// ResolveOptions applies opts to a zero-value Options and returns
// the result. Drivers and AlterOp constructors use this to fold a
// variadic options slice into a struct.
func ResolveOptions(opts ...Option) Options {
	var o Options
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./ddl/ -run "TestOption|TestIfNotExists|TestIfExists|TestOptions|TestResolveOptions" -v
```

Expected: all five tests PASS.

- [ ] **Step 6: Commit**

```bash
git add ddl/doc.go ddl/options.go ddl/options_test.go
git commit -m "feat(ddl): add Option/Options + IfNotExists/IfExists + ResolveOptions

Spec: spec/features/ddl/options/.
Functional-options pattern shared across CreateCollection,
DropCollection, and all six AlterOp constructors. Drivers silently
ignore mismatched options.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: `ddl/errors` — `PartialSuccessError`

**Spec:** [`spec/features/ddl/errors/README.md`](../features/ddl/errors/README.md)

**Files:**
- Create: `ddl/errors.go`
- Create: `ddl/errors_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ddl/errors_test.go`:

```go
package ddl

import (
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestPartialSuccessError_Fields(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-1.
	e := &PartialSuccessError{
		Op:           "AlterCollection",
		Collection:   "users",
		Backend:      "dalgo2sql/sqlite",
		Applied:      []AlterOp{nil, nil}, // sentinel placeholders; real values come from Task 11
		FirstFailed:  nil,
		NotAttempted: []AlterOp{nil},
		Cause:        errors.New("inner failure"),
	}
	assert.Equal(t, "AlterCollection", e.Op)
	assert.Equal(t, "users", e.Collection)
	assert.Equal(t, "dalgo2sql/sqlite", e.Backend)
	assert.Len(t, e.Applied, 2)
	assert.Len(t, e.NotAttempted, 1)
	assert.NotNil(t, e.Cause)
}

func TestPartialSuccessError_ErrorString(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-2.
	e := &PartialSuccessError{
		Op:           "AlterCollection",
		Collection:   "users",
		Backend:      "dalgo2sql/sqlite",
		Applied:      []AlterOp{nil, nil},
		FirstFailed:  nil,
		NotAttempted: []AlterOp{nil},
		Cause:        errors.New("inner failure"),
	}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "AlterCollection"))
	assert.True(t, strings.Contains(s, "users"))
}

func TestPartialSuccessError_Unwrap(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-3.
	cause := errors.New("inner")
	e := &PartialSuccessError{Cause: cause}
	assert.Same(t, cause, errors.Unwrap(e))
}

func TestPartialSuccessError_ErrorsIs_ViaCause(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-4.
	cause := dal.ErrNotSupported
	e := &PartialSuccessError{Cause: cause}
	assert.True(t, errors.Is(e, dal.ErrNotSupported))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./ddl/ -run TestPartialSuccessError
```

Expected: build failure with `undefined: PartialSuccessError`, `undefined: AlterOp`. The AlterOp references will be resolved in Task 11; for now the test uses `nil` placeholders cast to `[]AlterOp`. **Important:** until Task 11 lands, this test will not compile because `AlterOp` doesn't exist. To make this task self-contained, we add a stub interface type `AlterOp` here and replace it with the real one in Task 11.

To make this task land cleanly: at Step 3, also add a placeholder type:

- [ ] **Step 3: Create `ddl/errors.go` (with a temporary `AlterOp` placeholder)**

```go
package ddl

import (
	"fmt"
	"strings"

	// dal import only needed for godoc reference; the actual import
	// is added when AlterOp type is fleshed out in Task 11.
)

// AlterOp is the sealed interface for collection-altering operations.
// Defined here as a placeholder; Task 11 replaces this with the full
// definition (sealed marker method + six constructors).
//
// NOTE TO IMPLEMENTER: this placeholder will be replaced in Task 11
// by the real declaration in alter_op.go. The placeholder is
// intentionally empty (no methods) so PartialSuccessError can
// reference it now; the seal is added in Task 11.
type AlterOp = interface{}

// PartialSuccessError is the typed error returned by AlterCollection
// (and any future batched DDL call) when a non-transactional driver
// succeeds at some sub-operations and fails at others.
//
// Distinct from *dbschema.NotSupportedError: NotSupportedError means
// "the driver can't do this at all"; PartialSuccessError means "the
// driver started doing this, then failed partway."
//
// Transactional drivers — those for which
// TransactionalDDL.SupportsTransactionalDDL returns true — MUST NOT
// produce *PartialSuccessError. Their failure mode is a regular
// error (rollback already performed; nothing was applied).
//
// Unwrap returns Cause so errors.Is(err, dal.ErrNotSupported)
// propagates transitively if the underlying failure was a
// not-supported case.
type PartialSuccessError struct {
	// Op names the batched operation (currently always "AlterCollection").
	Op string
	// Collection is the target collection name.
	Collection string
	// Backend optionally identifies the driver.
	Backend string
	// Applied lists the ops that completed successfully, in original
	// order.
	Applied []AlterOp
	// FirstFailed is the op that failed.
	FirstFailed AlterOp
	// NotAttempted lists the ops that came after FirstFailed and were
	// not tried. May be empty if the driver attempted every op
	// regardless of earlier failures.
	NotAttempted []AlterOp
	// Cause is the underlying error from the failed op (driver-specific).
	Cause error
}

// Error returns a readable single-line summary.
func (e *PartialSuccessError) Error() string {
	var b strings.Builder
	b.WriteString("ddl: partial success: ")
	if e.Op != "" {
		fmt.Fprintf(&b, "op=%s ", e.Op)
	}
	if e.Collection != "" {
		fmt.Fprintf(&b, "collection=%s ", e.Collection)
	}
	if e.Backend != "" {
		fmt.Fprintf(&b, "backend=%s ", e.Backend)
	}
	fmt.Fprintf(&b, "applied=%d failed=1 not_attempted=%d", len(e.Applied), len(e.NotAttempted))
	if e.Cause != nil {
		fmt.Fprintf(&b, "; cause: %v", e.Cause)
	}
	return b.String()
}

// Unwrap returns Cause so errors.Is and errors.As propagate
// through the underlying driver-specific failure.
func (e *PartialSuccessError) Unwrap() error {
	return e.Cause
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./ddl/ -run TestPartialSuccessError -v
```

Expected: all four `TestPartialSuccessError_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add ddl/errors.go ddl/errors_test.go
git commit -m "feat(ddl): add PartialSuccessError typed error

Spec: spec/features/ddl/errors/.
Typed error for non-transactional drivers that partway-fail a batch.
Carries Applied/FirstFailed/NotAttempted/Cause. Unwrap chains to
Cause so errors.Is(err, dal.ErrNotSupported) propagates if the
underlying failure was a not-supported case.

Includes a temporary 'AlterOp = interface{}' type alias to be
replaced by the real sealed interface in Task 11.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: `ddl/transactional-ddl` — `TransactionalDDL` capability

**Spec:** [`spec/features/ddl/transactional-ddl/README.md`](../features/ddl/transactional-ddl/README.md)

**Files:**
- Create: `ddl/transactional.go`
- Create: `ddl/transactional_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ddl/transactional_test.go`:

```go
package ddl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// transactionalStub implements TransactionalDDL with a configurable answer.
type transactionalStub struct {
	*minStubDB
	supports bool
}

func (s *transactionalStub) SupportsTransactionalDDL() bool {
	return s.supports
}

func TestTransactionalDDL_InterfaceExists(t *testing.T) {
	// Per REQ:transactional-ddl-interface AC-1.
	var _ TransactionalDDL = (*transactionalStub)(nil)
}

func TestTransactionalDDL_StableAnswer(t *testing.T) {
	// Per REQ:transactional-ddl-interface AC-3.
	s := &transactionalStub{supports: true}
	first := s.SupportsTransactionalDDL()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, s.SupportsTransactionalDDL(), "call %d", i)
	}
}

func TestSupportsTransactionalDDL_TrueOnImplementer(t *testing.T) {
	// Per REQ:helper-function AC-1.
	s := &transactionalStub{minStubDB: newMinStubDB("x"), supports: true}
	assert.True(t, SupportsTransactionalDDL(s))
}

func TestSupportsTransactionalDDL_FalseOnNonImplementer(t *testing.T) {
	// Per REQ:helper-function AC-2.
	s := newMinStubDB("x")
	assert.False(t, SupportsTransactionalDDL(s))
}
```

- [ ] **Step 2: Write a shared minimal stub `dal.DB` for ddl tests**

The tests above and tests in later tasks (Task 13 operations) need a minimal `dal.DB` stub. Add it now to keep test files DRY.

Create `ddl/test_helpers_test.go`:

```go
package ddl

import (
	"context"
	"errors"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

// minStubAdapter implements dal.Adapter with a controllable name.
type minStubAdapter struct{ name string }

func (s minStubAdapter) Name() string { return s.name }

// minStubDB is a minimal dal.DB stub used by tests in this package.
// All methods except Adapter() and ID() panic or return errors —
// tests that need more functionality wrap or embed this.
type minStubDB struct {
	dal.NoConcurrency
	adapter dal.Adapter
}

func newMinStubDB(name string) *minStubDB {
	return &minStubDB{adapter: minStubAdapter{name: name}}
}

// newMinStubDBNilAdapter creates a stub whose Adapter() returns nil.
func newMinStubDBNilAdapter() *minStubDB {
	return &minStubDB{adapter: nil}
}

func (s *minStubDB) ID() string             { return "stub" }
func (s *minStubDB) Adapter() dal.Adapter   { return s.adapter }
func (s *minStubDB) Schema() dal.Schema     { return nil }
func (s *minStubDB) RunReadonlyTransaction(_ context.Context, _ dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *minStubDB) RunReadwriteTransaction(_ context.Context, _ dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *minStubDB) Get(_ context.Context, _ dal.Record) error { return errors.New("not used") }
func (s *minStubDB) Exists(_ context.Context, _ *dal.Key) (bool, error) {
	return false, errors.New("not used")
}
func (s *minStubDB) GetMulti(_ context.Context, _ []dal.Record) error { return errors.New("not used") }
func (s *minStubDB) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, errors.New("not used")
}
func (s *minStubDB) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("not used")
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./ddl/ -run "TestTransactionalDDL|TestSupportsTransactionalDDL"
```

Expected: build failure with `undefined: TransactionalDDL`, `undefined: SupportsTransactionalDDL`.

- [ ] **Step 4: Create `ddl/transactional.go`**

```go
package ddl

import "github.com/dal-go/dalgo/dal"

// TransactionalDDL is the optional capability interface drivers
// implement to advertise that they guarantee all-or-nothing
// atomicity for DDL calls that perform multiple sub-operations
// (notably AlterCollection with multiple AlterOps, or
// CreateCollection with inline indexes on some engines).
//
// The pattern mirrors [dal.ConcurrencyAware]: a one-method optional
// interface that consumers type-assert against. Drivers that don't
// implement TransactionalDDL are treated as non-transactional.
//
// When SupportsTransactionalDDL returns true: if any sub-operation
// fails, the driver MUST roll back all previously-applied
// sub-operations in the same call. The whole call returns a non-nil
// error; the DB is left in its pre-call state.
//
// When SupportsTransactionalDDL returns false (or the interface is
// not implemented): the driver MAY apply some sub-operations and
// fail on others. Callers receive a *PartialSuccessError listing
// applied / failed / not-attempted ops.
//
// The return value is constant from the moment a DB value is
// returned by its constructor until it is discarded; the same
// stability contract as dal.ConcurrencyAware.
type TransactionalDDL interface {
	SupportsTransactionalDDL() bool
}

// SupportsTransactionalDDL is the convenience helper that
// encapsulates the type assertion and the convention that "doesn't
// implement = treat as non-transactional." Consumers SHOULD use this
// rather than performing the assertion themselves.
func SupportsTransactionalDDL(db dal.DB) bool {
	a, ok := db.(TransactionalDDL)
	if !ok {
		return false
	}
	return a.SupportsTransactionalDDL()
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./ddl/ -run "TestTransactionalDDL|TestSupportsTransactionalDDL" -v
```

Expected: all four tests PASS.

- [ ] **Step 6: Commit**

```bash
git add ddl/transactional.go ddl/transactional_test.go ddl/test_helpers_test.go
git commit -m "feat(ddl): add TransactionalDDL capability + helper

Spec: spec/features/ddl/transactional-ddl/.
Single-method optional capability interface (mirrors
ConcurrencyAware). Drivers that guarantee all-or-nothing atomicity
for batched DDL ops return true. SupportsTransactionalDDL(db) helper
encapsulates the type assertion with 'not implemented = false'
convention. Also adds a shared minimal dal.DB stub
(ddl/test_helpers_test.go) used by tests in this package.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: `ddl/alter-ops` — sealed `AlterOp` + 6 constructors

**Spec:** [`spec/features/ddl/alter-ops/README.md`](../features/ddl/alter-ops/README.md)

**Files:**
- Modify: `ddl/errors.go` (remove the temporary `AlterOp = interface{}` alias placed in Task 9)
- Create: `ddl/alter_op.go`
- Create: `ddl/alter_op_test.go`

- [ ] **Step 1: Remove the placeholder `AlterOp` alias from `ddl/errors.go`**

Open `ddl/errors.go` and delete these lines (added in Task 9 as a temporary placeholder):

```go
// AlterOp is the sealed interface for collection-altering operations.
// Defined here as a placeholder; Task 11 replaces this with the full
// definition (sealed marker method + six constructors).
//
// NOTE TO IMPLEMENTER: this placeholder will be replaced in Task 11
// by the real declaration in alter_op.go. The placeholder is
// intentionally empty (no methods) so PartialSuccessError can
// reference it now; the seal is added in Task 11.
type AlterOp = interface{}
```

Verify the build still works (it will fail until alter_op.go provides the real type — that's fine for the moment).

- [ ] **Step 2: Write the failing tests**

Create `ddl/alter_op_test.go`:

```go
package ddl

import (
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

func TestAlterOp_InterfaceExists(t *testing.T) {
	// Per REQ:alter-op-interface AC-1.
	var op AlterOp
	assert.Nil(t, op)
}

func TestAlterOp_HasUnexportedMarker(t *testing.T) {
	// Per REQ:alter-op-interface AC-2.
	typ := reflect.TypeOf((*AlterOp)(nil)).Elem()
	assert.Equal(t, reflect.Interface, typ.Kind())
	assert.Equal(t, 1, typ.NumMethod(), "expected exactly one method on AlterOp")
	method := typ.Method(0)
	assert.NotEmpty(t, method.PkgPath, "method %q should be unexported", method.Name)
}

func TestAddField_Constructs(t *testing.T) {
	// Per REQ:add-field-constructor AC-1.
	var op AlterOp = AddField(dbschema.FieldDef{Name: "email", Type: dbschema.String})
	assert.NotNil(t, op)
}

func TestAddField_PreservesField(t *testing.T) {
	// Per REQ:add-field-constructor AC-2.
	f := dbschema.FieldDef{Name: "email", Type: dbschema.String}
	op := AddField(f, IfNotExists())
	// Concrete type accessor: addFieldOp.
	concrete, ok := op.(addFieldOp)
	assert.True(t, ok)
	assert.Equal(t, f, concrete.field)
	assert.True(t, concrete.options.IfNotExists)
}

func TestDropField_Constructs(t *testing.T) {
	// Per REQ:drop-field-constructor AC-1.
	var op AlterOp = DropField("legacy_user_code", IfExists())
	assert.NotNil(t, op)
	concrete, ok := op.(dropFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("legacy_user_code"), concrete.name)
	assert.True(t, concrete.options.IfExists)
}

func TestModifyField_Constructs(t *testing.T) {
	// Per REQ:modify-field-constructor AC-1.
	var op AlterOp = ModifyField("created_at", dbschema.FieldDef{Name: "created_at", Type: dbschema.Time, Nullable: false})
	assert.NotNil(t, op)
	concrete, ok := op.(modifyFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("created_at"), concrete.name)
	assert.Equal(t, dbschema.Time, concrete.newDef.Type)
}

func TestRenameField_Constructs(t *testing.T) {
	// Per REQ:rename-field-constructor AC-1.
	var op AlterOp = RenameField("user_name", "username")
	assert.NotNil(t, op)
	concrete, ok := op.(renameFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("user_name"), concrete.oldName)
	assert.Equal(t, dal.FieldName("username"), concrete.newName)
}

func TestAddIndex_Constructs(t *testing.T) {
	// Per REQ:add-index-constructor AC-1.
	idx := dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true}
	var op AlterOp = AddIndex(idx, IfNotExists())
	assert.NotNil(t, op)
	concrete, ok := op.(addIndexOp)
	assert.True(t, ok)
	assert.Equal(t, idx, concrete.index)
	assert.True(t, concrete.options.IfNotExists)
}

func TestDropIndex_Constructs(t *testing.T) {
	// Per REQ:drop-index-constructor AC-1.
	var op AlterOp = DropIndex("ix_users_legacy", IfExists())
	assert.NotNil(t, op)
	concrete, ok := op.(dropIndexOp)
	assert.True(t, ok)
	assert.Equal(t, "ix_users_legacy", concrete.name)
	assert.True(t, concrete.options.IfExists)
}
```

- [ ] **Step 3: Run tests to verify they fail (compile error)**

```bash
go test ./ddl/ -run TestAlterOp -run "TestAddField|TestDropField|TestModifyField|TestRenameField|TestAddIndex|TestDropIndex"
```

Expected: build failure with `undefined: AlterOp`, `undefined: AddField`, etc.

- [ ] **Step 4: Create `ddl/alter_op.go`**

```go
package ddl

import (
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// AlterOp is the sealed interface for collection-altering operations
// passed to AlterCollection. Sealed via an unexported marker method
// (alterOp) so the set of valid alterations is closed at the package
// boundary — drivers know which cases exist and translate
// accordingly. New alteration kinds require adding to this package.
//
// MVP constructors:
//
//   - Field-level: [AddField], [DropField], [ModifyField], [RenameField]
//   - Index-level: [AddIndex], [DropIndex]
//
// All six constructors accept opts ...Option for opt-in idempotency
// (reusing the same Option type as CreateCollection / DropCollection).
// Drivers MUST silently ignore semantically-mismatched options.
type AlterOp interface {
	alterOp() // sealed marker
}

// ---- Field-level AlterOps ----

type addFieldOp struct {
	field   dbschema.FieldDef
	options Options
}

func (addFieldOp) alterOp() {}

// AddField returns an AlterOp that adds a field to the collection.
// IfNotExists makes it idempotent (existing field of same name = no-op).
// IfExists is meaningless and silently ignored.
func AddField(f dbschema.FieldDef, opts ...Option) AlterOp {
	return addFieldOp{field: f, options: ResolveOptions(opts...)}
}

type dropFieldOp struct {
	name    dal.FieldName
	options Options
}

func (dropFieldOp) alterOp() {}

// DropField returns an AlterOp that drops a field by name.
// IfExists makes it idempotent (missing field = no-op). IfNotExists
// is meaningless and silently ignored.
func DropField(name dal.FieldName, opts ...Option) AlterOp {
	return dropFieldOp{name: name, options: ResolveOptions(opts...)}
}

type modifyFieldOp struct {
	name    dal.FieldName
	newDef  dbschema.FieldDef
	options Options
}

func (modifyFieldOp) alterOp() {}

// ModifyField returns an AlterOp that replaces an existing field's
// definition with newDef. The driver diffs old vs new and emits the
// minimal engine-specific change. When name != newDef.Name, the
// operation also renames the field.
//
// opts is accepted for surface symmetry but both IfNotExists and
// IfExists are semantically meaningless on ModifyField. Drivers MUST
// silently ignore them.
func ModifyField(name dal.FieldName, newDef dbschema.FieldDef, opts ...Option) AlterOp {
	return modifyFieldOp{name: name, newDef: newDef, options: ResolveOptions(opts...)}
}

type renameFieldOp struct {
	oldName dal.FieldName
	newName dal.FieldName
	options Options
}

func (renameFieldOp) alterOp() {}

// RenameField returns an AlterOp that renames a field from oldName
// to newName.
//
// opts is accepted for surface symmetry but both IfNotExists and
// IfExists are semantically meaningless on RenameField. Drivers MUST
// silently ignore them.
func RenameField(oldName, newName dal.FieldName, opts ...Option) AlterOp {
	return renameFieldOp{oldName: oldName, newName: newName, options: ResolveOptions(opts...)}
}

// ---- Index-level AlterOps ----

type addIndexOp struct {
	index   dbschema.IndexDef
	options Options
}

func (addIndexOp) alterOp() {}

// AddIndex returns an AlterOp that creates an index on the
// collection. On engines that support combined ALTER TABLE ... ADD
// INDEX syntax (MySQL), the driver MAY fold this into a single
// statement alongside other AlterOps in the same batch.
//
// IfNotExists makes it idempotent (existing index of same name =
// no-op). IfExists is meaningless and silently ignored.
func AddIndex(idx dbschema.IndexDef, opts ...Option) AlterOp {
	return addIndexOp{index: idx, options: ResolveOptions(opts...)}
}

type dropIndexOp struct {
	name    string
	options Options
}

func (dropIndexOp) alterOp() {}

// DropIndex returns an AlterOp that drops an index by name.
// IfExists makes it idempotent (missing index = no-op). IfNotExists
// is meaningless and silently ignored.
func DropIndex(name string, opts ...Option) AlterOp {
	return dropIndexOp{name: name, options: ResolveOptions(opts...)}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./ddl/ -v
```

Expected: all `TestAlterOp_*` and all six constructor tests PASS. Also the existing `TestPartialSuccessError_*` and `TestTransactionalDDL_*` tests continue to pass (the previous `AlterOp = interface{}` placeholder is now replaced by the real interface; `nil` AlterOp values in PartialSuccessError tests still typecheck).

- [ ] **Step 6: Commit**

```bash
git add ddl/alter_op.go ddl/alter_op_test.go ddl/errors.go
git commit -m "feat(ddl): add AlterOp sealed interface + six constructors

Spec: spec/features/ddl/alter-ops/.
Sealed AlterOp interface with unexported marker. Six constructors,
all accepting opts ...Option for opt-in idempotency:
- Field-level: AddField, DropField, ModifyField, RenameField
- Index-level: AddIndex, DropIndex
Drivers silently ignore semantically-mismatched options.

Replaces the temporary 'AlterOp = interface{}' alias added to
errors.go in Task 9.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: `ddl/schema-modifier` — 3-method interface

**Spec:** [`spec/features/ddl/schema-modifier/README.md`](../features/ddl/schema-modifier/README.md)

**Files:**
- Create: `ddl/modifier.go`
- Create: `ddl/modifier_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ddl/modifier_test.go`:

```go
package ddl

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

// schemaModifierStub satisfies SchemaModifier; used by tests in this
// task and later in operations_test.go.
type schemaModifierStub struct {
	*minStubDB
	createCollectionCalls []recordedCall
	dropCollectionCalls   []recordedCall
	alterCollectionCalls  []recordedAlter
}

type recordedCall struct {
	ctx  context.Context
	name string
	cdef *dbschema.CollectionDef
	opts []Option
}

type recordedAlter struct {
	ctx  context.Context
	name string
	ops  []AlterOp
}

func (s *schemaModifierStub) CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error {
	s.createCollectionCalls = append(s.createCollectionCalls, recordedCall{ctx: ctx, name: c.Name, cdef: &c, opts: opts})
	return nil
}

func (s *schemaModifierStub) DropCollection(ctx context.Context, name string, opts ...Option) error {
	s.dropCollectionCalls = append(s.dropCollectionCalls, recordedCall{ctx: ctx, name: name, opts: opts})
	return nil
}

func (s *schemaModifierStub) AlterCollection(ctx context.Context, name string, ops ...AlterOp) error {
	s.alterCollectionCalls = append(s.alterCollectionCalls, recordedAlter{ctx: ctx, name: name, ops: ops})
	return nil
}

func newSchemaModifierStub(name string) *schemaModifierStub {
	return &schemaModifierStub{minStubDB: newMinStubDB(name)}
}

func TestSchemaModifier_InterfaceExists(t *testing.T) {
	// Per REQ:schema-modifier-interface AC-1 + AC-2.
	var _ SchemaModifier = (*schemaModifierStub)(nil)
}

func TestSchemaModifier_NotEmbeddedInDB(t *testing.T) {
	// Per REQ:opt-in-not-embedded AC-1 + AC-2.
	var db dal.DB = newMinStubDB("x")
	_, ok := db.(SchemaModifier)
	assert.False(t, ok)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./ddl/ -run TestSchemaModifier
```

Expected: build failure with `undefined: SchemaModifier`.

- [ ] **Step 3: Create `ddl/modifier.go`**

```go
package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dbschema"
)

// SchemaModifier is the capability interface drivers implement to
// support DDL operations on dalgo collections. The interface is
// three methods:
//
//   - CreateCollection creates a collection (table) along with any
//     inline indexes declared in CollectionDef.Indexes.
//   - DropCollection drops a collection along with its indexes.
//   - AlterCollection applies a batch of AlterOp values
//     (field-level and index-level alterations) to an existing
//     collection. The driver decides how to apply the batch
//     atomically; consumers check [TransactionalDDL] to know
//     whether to expect rollback.
//
// SchemaModifier is NOT embedded into [dal.DB]. DDL is genuinely
// optional for some backends — read-only wrappers, analytics
// drivers, mocks. Drivers opt in by implementing SchemaModifier on
// their dal.DB value (or a related type reachable via type
// assertion). The top-level helper functions [CreateCollection],
// [DropCollection], and [AlterCollection] type-assert against
// SchemaModifier and return *dbschema.NotSupportedError on the
// failed assertion.
//
// Index-level operations after initial collection creation are NOT
// separate methods — they are AlterOp values passed to
// AlterCollection. See [AddIndex] and [DropIndex].
type SchemaModifier interface {
	// CreateCollection creates the collection (table) and any inline
	// indexes declared on c.Indexes. Caller passes opts for opt-in
	// idempotency.
	CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error

	// DropCollection drops the collection and its indexes. Caller
	// passes opts for opt-in idempotency.
	DropCollection(ctx context.Context, name string, opts ...Option) error

	// AlterCollection applies ops to the existing collection. The
	// driver decides how to apply the batch (one combined ALTER
	// statement on PostgreSQL; a sequence on SQLite; etc.).
	// Transactional drivers (advertised via TransactionalDDL) MUST
	// roll back on partial failure; non-transactional drivers MAY
	// return *PartialSuccessError listing applied/failed/not-attempted ops.
	AlterCollection(ctx context.Context, name string, ops ...AlterOp) error
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./ddl/ -v
```

Expected: all tests pass, including the new `TestSchemaModifier_*` tests.

- [ ] **Step 5: Commit**

```bash
git add ddl/modifier.go ddl/modifier_test.go
git commit -m "feat(ddl): add SchemaModifier 3-method capability interface

Spec: spec/features/ddl/schema-modifier/.
Three methods: CreateCollection, DropCollection, AlterCollection.
Index-level operations are AlterOps, not separate methods.
SchemaModifier is opt-in via type assertion, NOT embedded in dal.DB
(DDL is genuinely optional for read-only / analytics drivers).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: `ddl/operations` — 3 top-level helper functions

**Spec:** [`spec/features/ddl/operations/README.md`](../features/ddl/operations/README.md)

**Files:**
- Create: `ddl/operations.go`
- Create: `ddl/operations_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ddl/operations_test.go`:

```go
package ddl

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

func TestHelpers_Compile(t *testing.T) {
	// Per REQ:helper-signatures AC-1.
	_ = CreateCollection
	_ = DropCollection
	_ = AlterCollection
}

func TestCreateCollection_Dispatches(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-1.
	stub := newSchemaModifierStub("stub-driver")
	ctx := context.Background()
	c := dbschema.CollectionDef{Name: "users"}
	err := CreateCollection(ctx, stub, c, IfNotExists())
	assert.NoError(t, err)
	assert.Len(t, stub.createCollectionCalls, 1)
	call := stub.createCollectionCalls[0]
	assert.Equal(t, "users", call.name)
	assert.Len(t, call.opts, 1)
}

func TestDropCollection_Dispatches(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-2.
	stub := newSchemaModifierStub("stub-driver")
	err := DropCollection(context.Background(), stub, "users", IfExists())
	assert.NoError(t, err)
	assert.Len(t, stub.dropCollectionCalls, 1)
	assert.Equal(t, "users", stub.dropCollectionCalls[0].name)
}

func TestAlterCollection_DispatchesMixedOps(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-3.
	stub := newSchemaModifierStub("stub-driver")
	f := dbschema.FieldDef{Name: "email", Type: dbschema.String}
	idx := dbschema.IndexDef{Name: "ix", Collection: "users", Fields: []dal.FieldName{"email"}}
	err := AlterCollection(context.Background(), stub, "users",
		AddField(f),
		AddIndex(idx),
		DropField("legacy"),
	)
	assert.NoError(t, err)
	assert.Len(t, stub.alterCollectionCalls, 1)
	call := stub.alterCollectionCalls[0]
	assert.Equal(t, "users", call.name)
	assert.Len(t, call.ops, 3)
	// Verify order preserved
	_, isAdd := call.ops[0].(addFieldOp)
	_, isAddIdx := call.ops[1].(addIndexOp)
	_, isDrop := call.ops[2].(dropFieldOp)
	assert.True(t, isAdd, "ops[0] should be addFieldOp")
	assert.True(t, isAddIdx, "ops[1] should be addIndexOp")
	assert.True(t, isDrop, "ops[2] should be dropFieldOp")
}

func TestCreateCollection_NotImplementer(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-1.
	db := newMinStubDB("stub-driver")
	err := CreateCollection(context.Background(), db, dbschema.CollectionDef{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "CreateCollection", ue.Op)
}

func TestAlterCollection_BackendFromAdapter(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-2.
	db := newMinStubDB("stub-driver")
	err := AlterCollection(context.Background(), db, "users", AddField(dbschema.FieldDef{Name: "x", Type: dbschema.Int}))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "stub-driver", ue.Backend)
	assert.Equal(t, "AlterCollection", ue.Op)
}

func TestDropCollection_BackendEmptyWhenAdapterNil(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-3.
	db := newMinStubDBNilAdapter()
	err := DropCollection(context.Background(), db, "x")
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "", ue.Backend)
	assert.NotEmpty(t, ue.Error())
}

func TestAlterCollection_NotImplementer(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-4.
	db := newMinStubDB("stub-driver")
	err := AlterCollection(context.Background(), db, "users",
		AddField(dbschema.FieldDef{Name: "x", Type: dbschema.Int}),
	)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "AlterCollection", ue.Op)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./ddl/ -run "TestHelpers_Compile|TestCreateCollection|TestDropCollection|TestAlterCollection"
```

Expected: build failure with `undefined: CreateCollection`, `undefined: DropCollection`, `undefined: AlterCollection`.

- [ ] **Step 3: Create `ddl/operations.go`**

```go
package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// backendName returns db.Adapter().Name() if Adapter() returns a
// non-nil dal.Adapter, otherwise the empty string.
func backendName(db dal.DB) string {
	if db == nil {
		return ""
	}
	a := db.Adapter()
	if a == nil {
		return ""
	}
	return a.Name()
}

// notSupportedModifier builds the *dbschema.NotSupportedError returned
// by the helpers when db does not implement SchemaModifier.
func notSupportedModifier(op string, db dal.DB) error {
	return &dbschema.NotSupportedError{
		Op:      op,
		Backend: backendName(db),
		Reason:  "driver does not implement ddl.SchemaModifier",
	}
}

// CreateCollection creates a new collection (and any inline indexes
// declared on c.Indexes) via the driver's SchemaModifier. Returns
// *dbschema.NotSupportedError if db does not implement
// SchemaModifier.
func CreateCollection(ctx context.Context, db dal.DB, c dbschema.CollectionDef, opts ...Option) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("CreateCollection", db)
	}
	return m.CreateCollection(ctx, c, opts...)
}

// DropCollection drops a collection and its indexes via the driver's
// SchemaModifier. Returns *dbschema.NotSupportedError if db does not
// implement SchemaModifier.
func DropCollection(ctx context.Context, db dal.DB, name string, opts ...Option) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("DropCollection", db)
	}
	return m.DropCollection(ctx, name, opts...)
}

// AlterCollection applies a batch of AlterOp values to an existing
// collection via the driver's SchemaModifier. Returns
// *dbschema.NotSupportedError if db does not implement
// SchemaModifier.
//
// Transactional drivers (advertised via TransactionalDDL) roll back
// on partial failure. Non-transactional drivers MAY return
// *PartialSuccessError. Consumers wanting strict atomicity should
// check SupportsTransactionalDDL(db) before calling.
func AlterCollection(ctx context.Context, db dal.DB, name string, ops ...AlterOp) error {
	m, ok := db.(SchemaModifier)
	if !ok {
		return notSupportedModifier("AlterCollection", db)
	}
	return m.AlterCollection(ctx, name, ops...)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./ddl/ -v
```

Expected: all tests pass — three new dispatch tests, three new not-implementer tests, plus all prior tests in the ddl package.

- [ ] **Step 5: Run the full repo build + test to verify nothing else broke**

```bash
go build ./...
go test ./...
```

Expected: build succeeds; all packages pass tests (including pre-existing `dal/`, `dalgo2fs/`, `mocks/mock_dal/`, etc.).

- [ ] **Step 6: Commit**

```bash
git add ddl/operations.go ddl/operations_test.go
git commit -m "feat(ddl): add CreateCollection / DropCollection / AlterCollection helpers

Spec: spec/features/ddl/operations/.
Three top-level helper functions that type-assert against
SchemaModifier and dispatch. Non-implementers cause helpers to
return *dbschema.NotSupportedError with Op set to the operation name
and Backend set to db.Adapter().Name() (or empty when Adapter is nil).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: Spec status transitions + push

**Files:**
- Modify: all 15 Feature `README.md` files under `spec/features/dbschema/` and `spec/features/ddl/` — flip `Status: Approved` → `Status: Implemented`.
- Modify: `spec/features/README.md` — flip top-level index entries for dbschema and ddl from `Approved` → `Implemented`.
- Modify: `spec/plans/README.md` — flip this plan's status from `Ready` → `Done`.

- [ ] **Step 1: Verify all tests are green**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
go test ./...
```

Expected: every package passes.

- [ ] **Step 2: Verify spec lints clean**

```bash
specscore spec lint --severity error
```

Expected: `0 violations found`.

- [ ] **Step 3: Flip Feature statuses Approved → Implemented**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
find spec/features/dbschema spec/features/ddl -name "README.md" -exec sed -i '' 's|^\*\*Status:\*\* Approved$|**Status:** Implemented|' {} \;
grep -rn "^\*\*Status:\*\* " spec/features/dbschema spec/features/ddl | head -20
```

Expected: all 15 Feature READMEs show `**Status:** Implemented`.

- [ ] **Step 4: Update top-level features index**

Edit `spec/features/README.md` and change `Approved` to `Implemented` in BOTH the `dbschema` row AND the `ddl` row of the Index table.

- [ ] **Step 5: Update plan index**

Edit `spec/plans/README.md` — change this plan's row from `Ready` to `Done`:

```
| [2026-05-13-dbschema-ddl](2026-05-13-dbschema-ddl.md) | dbschema + ddl | Done |
```

(The plan index entry is added in the same commit; if the row doesn't exist yet, add it.)

- [ ] **Step 6: Re-lint**

```bash
specscore spec lint --severity error
```

Expected: `0 violations found`.

- [ ] **Step 7: Commit status transitions**

```bash
git add spec/features/dbschema spec/features/ddl spec/features/README.md spec/plans/README.md
git commit -m "docs(spec): mark dbschema + ddl Features as Implemented

All 15 requirements across both Feature trees are now satisfied with
passing tests:
- dbschema: types, default-expr, errors, field-def, index-def,
  collection-def, schema-reader
- ddl: schema-modifier, alter-ops, transactional-ddl, options,
  operations, errors

In-tree dal.DB implementers were NOT modified (this change is purely
additive; SchemaModifier and SchemaReader are opt-in via type
assertion). Driver-side adoption in dalgo2sql, dalgo2firestore, and
dalgo2ingitdb is tracked separately per the source Idea's migration
scope.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 8: Push all commits**

```bash
git push
```

Expected output: `main -> main` confirmation showing the range from before this plan's first commit to the status-flip commit (something like `4320c44..<latest>`).

---

## Verification Checklist (after all tasks complete)

- [ ] `go build ./...` succeeds with no output
- [ ] `go test ./...` succeeds; all packages pass (including pre-existing ones)
- [ ] `specscore spec lint --severity error` returns `0 violations found`
- [ ] `go doc github.com/dal-go/dalgo/dbschema` returns substantive doc covering the package purpose
- [ ] `go doc github.com/dal-go/dalgo/ddl` returns substantive doc covering the package purpose
- [ ] `git log --oneline` shows 14 atomic commits (one per task)
- [ ] All 15 Feature READMEs show `Status: Implemented`
- [ ] Top-level features/README.md index shows `Implemented` for dbschema and ddl
- [ ] Plan index shows `Done` for `2026-05-13-dbschema-ddl`

## Out of Scope (will not be done here)

- **`dalgo2sql` Tier-2 SQL extensions** (DbType, CharMaxLength, CharacterSet, IsClustered, IsColumnStore, etc.) — separate Feature in the dalgo2sql repo.
- **`dalgo2sql` driver-side `SchemaReader` + `SchemaModifier` implementations** (SQLite, PostgreSQL, info_schema readers moved from datatug-cli/pkg/schemers/) — separate Feature in dalgo2sql.
- **`dalgo2firestore` driver-side `SchemaReader` implementation** (Firestore reader moved from datatug-cli/pkg/schemers/firestoreschema/) — separate Feature in dalgo2firestore.
- **`dalgo2ingitdb` driver-side `SchemaModifier` implementation** — separate Feature in ingitdb-cli.
- **datatug-cli migration**: retire `pkg/datatug-core/schemer/` and `pkg/schemers/*`; convert `pkg/datatug-core/datatug/db_objects.go` types into Tier-3 wrappers that compose Tier 2 — separate Feature in datatug-cli.
- **Driver migration of in-tree mocks** (`mocks/mock_dal/` regeneration after `dal.DB` changes) — not applicable here; this Feature does NOT modify `dal.DB`. The existing mock continues to compile and pass tests without changes.

## CHANGELOG Entry (draft for the maintainer)

```markdown
### Added
- New top-level `dbschema` sub-package providing the portable schema-
  description vocabulary and read-side (introspection) capability:
  - Tier-1 types: `Type` enum (Null/Bool/Int/Float/String/Bytes/Time/
    Decimal), `Precision`, sealed `DefaultExpr` interface with
    `DefaultLiteral` and `DefaultCurrentTimestamp` concretes,
    `FieldDef`, `IndexDef`, `CollectionDef`, `ConstraintDef`,
    `Referrer`.
  - `SchemaReader` capability interface (ListCollections,
    DescribeCollection, ListIndexes, ListConstraints, ListReferrers)
    + five top-level helper functions that type-assert against
    SchemaReader.
  - Shared `NotSupportedError` typed error wrapping `dal.ErrNotSupported`
    — used by both the read side (this package) and the write side
    (the new `ddl` package).
- New top-level `ddl` sub-package providing the schema-modification
  execution surface:
  - Three-method `SchemaModifier` capability interface:
    CreateCollection, DropCollection, AlterCollection.
  - Sealed `AlterOp` type with six composable constructors —
    AddField, DropField, ModifyField, RenameField, AddIndex,
    DropIndex — all accepting `opts ...Option` for opt-in
    idempotency.
  - `TransactionalDDL` capability interface for atomicity
    advertisement + `SupportsTransactionalDDL(db)` helper.
  - `PartialSuccessError` typed error for non-transactional drivers
    that partway-fail a batch. Unwrap chains to the underlying cause.
  - Three top-level helper functions (`CreateCollection`,
    `DropCollection`, `AlterCollection`) that type-assert against
    `SchemaModifier` and produce consistent error envelopes on
    failure.
  - Functional options `IfNotExists()` and `IfExists()` apply to both
    the collection-level operations AND the six AlterOp constructors.

This is purely additive — no existing interfaces (including `dal.DB`),
types, or behaviors change. Drivers opt in to DDL or schema
introspection by implementing the relevant capability interfaces.
External `dal.DB` implementations do NOT need to update.
```
