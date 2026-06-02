# Feature: dbschema FieldDef

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/field-def?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/field-def?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/field-def?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/field-def?op=request-change) |

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the `FieldDef` struct — the portable description of one field (a.k.a. column) in a `CollectionDef`. `FieldDef` carries the field's name, portable type, optional length/precision hints, nullability, optional default expression, and auto-increment marker.

## Behavior

### REQ: field-def-struct

The `dbschema` package MUST export the struct:

```go
type FieldDef struct {
    Name          dal.FieldName       // reuses existing typed identifier
    Type          Type
    Length        *int                // optional, for String / Bytes
    Precision     *Precision          // optional, for Decimal
    Nullable      bool                // default false = NOT NULL
    Default       DefaultExpr         // optional; nil = no default
    AutoIncrement bool                // optional
}
```

`Name` reuses `dal.FieldName` (an existing typed string) for codebase consistency. All fields are exported.

The godoc on `FieldDef` MUST disambiguate from the unrelated runtime types: `dal.Column` (a SELECT-clause expression+alias), `dal.FieldRef` (a field reference in queries), and `dal.FieldVal` (runtime name+value pair).

#### AC-1: field-def-compiles

**Given** a Go program importing `dbschema`
**When** the program declares `f := dbschema.FieldDef{Name: "email", Type: dbschema.String, Length: ptr(255), Nullable: false}`
**Then** the program compiles.

#### AC-2: field-def-zero-value

**Given** a Go program importing `dbschema`
**When** the program declares `var f dbschema.FieldDef`
**Then** `f.Name == ""`, `f.Type == dbschema.Null`, `f.Length == nil`, `f.Precision == nil`, `f.Nullable == false`, `f.Default == nil`, `f.AutoIncrement == false`.

#### AC-3: field-def-decimal-precision

**Given** a Go program importing `dbschema`
**When** the program declares `f := dbschema.FieldDef{Name: "amount", Type: dbschema.Decimal, Precision: &dbschema.Precision{Total: 18, Scale: 4}}`
**Then** the program compiles and `f.Precision.Total == 18 && f.Precision.Scale == 4`.

#### AC-4: field-def-default-literal

**Given** a Go program importing `dbschema`
**When** the program declares `f := dbschema.FieldDef{Name: "status", Type: dbschema.String, Default: dbschema.DefaultLiteral{Value: "active"}}`
**Then** the program compiles and `f.Default != nil`.

### REQ: auto-increment-advisory

The godoc on `FieldDef.AutoIncrement` MUST note that drivers MAY restrict this attribute to integer types in the primary key and return `*dbschema.NotSupportedError` if a caller violates the constraint. `dbschema` itself does NOT enforce the restriction.

#### AC-1: godoc-mentions-restriction

**Given** the `dbschema` package
**When** inspected via `go doc github.com/dal-go/dalgo/dbschema.FieldDef`
**Then** the doc block mentions that `AutoIncrement` is typically restricted to integer primary-key columns and that drivers MAY reject other combinations.

## Architecture

| File | Contents |
|---|---|
| `dbschema/field_def.go` | `FieldDef` struct + godoc. |
| `dbschema/field_def_test.go` | Tests covering the ACs above. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
