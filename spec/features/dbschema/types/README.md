# Feature: dbschema Types (`Type`, `Precision`)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fdbschema%2Ftypes) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the portable column-type enumeration `Type` and the `Precision` struct used by `FieldDef` for decimal precision. The `Type` constants are an int8-backed enum with a `String()` method for diagnostic output.

## Behavior

### REQ: type-enum

The `dbschema` package MUST export `type Type int8` and the constants `Null, Bool, Int, Float, String, Bytes, Time, Decimal` of that type. `Null` MUST be the zero value (assigned first via `iota`) so that the zero value of `Type` is meaningful (representing an unset / null-typed field) rather than silently `Bool`.

#### AC-1: constants-exist

**Given** a Go program that imports `github.com/dal-go/dalgo/dbschema`
**When** the program references `dbschema.Null`, `dbschema.Bool`, `dbschema.Int`, `dbschema.Float`, `dbschema.String`, `dbschema.Bytes`, `dbschema.Time`, `dbschema.Decimal`
**Then** the program compiles and each constant evaluates to a distinct `dbschema.Type` value.

#### AC-2: null-is-zero-value

**Given** a Go program that imports `dbschema`
**When** the program declares `var t dbschema.Type`
**Then** `t == dbschema.Null` is `true`.

### REQ: type-string

`Type` MUST implement a `String() string` method that returns a non-empty lowercase identifier for diagnostic output.

#### AC-1: string-non-empty

**Given** any `dbschema.Type` constant `t`
**When** `t.String()` is called
**Then** the result is a non-empty lowercase string (e.g. `"null"`, `"int"`, `"decimal"`).

#### AC-2: string-distinct

**Given** the eight `Type` constants
**When** `String()` is called on each
**Then** all eight returned strings are pairwise distinct.

### REQ: precision-struct

The package MUST export `type Precision struct { Total, Scale int }`. `Total` is the total number of significant digits; `Scale` is the number of digits to the right of the decimal point. Both are non-negative integers. The package does NOT enforce `Scale <= Total` — consumers (drivers) MAY enforce or surface engine-specific errors.

#### AC-1: precision-structure

**Given** a Go program that imports `dbschema`
**When** the program declares `p := dbschema.Precision{Total: 18, Scale: 4}`
**Then** the program compiles and `p.Total == 18 && p.Scale == 4`.

## Architecture

| File | Contents |
|---|---|
| `dbschema/type.go` | `Type` enum constants, `String()` method, `Precision` struct. |
| `dbschema/type_test.go` | Tests covering the ACs above. |

## Outstanding Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
