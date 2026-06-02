# Feature: dbschema DefaultExpr (Sealed Interface)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/default-expr?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/default-expr?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/default-expr?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/default-expr?op=request-change) |

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines a sealed `DefaultExpr` interface for column default values, with two concrete implementations in MVP: `DefaultLiteral{Value any}` for plain literal defaults and `DefaultCurrentTimestamp{}` for the cross-engine "current timestamp" default. The interface is sealed (unexported marker method) so the set of valid default-expression cases is closed at the package boundary — drivers know which cases exist and translate accordingly. New cases require adding to this package, not arbitrary external types.

## Behavior

### REQ: default-expr-interface

The `dbschema` package MUST export `type DefaultExpr interface { … }` with an UNEXPORTED marker method that prevents external packages from satisfying the interface.

#### AC-1: interface-exists

**Given** a Go program importing `dbschema`
**When** the program references `dbschema.DefaultExpr`
**Then** the program compiles.

#### AC-2: sealed-marker-present

**Given** the `dbschema.DefaultExpr` interface declaration
**When** inspected via Go reflection in a test (`reflect.TypeOf((*dbschema.DefaultExpr)(nil)).Elem()`) and iterated over its methods
**Then** the method set includes exactly one method whose name begins with a lowercase letter (the unexported marker), proving the interface cannot be satisfied from outside the `dbschema` package.

#### AC-3: sealed-external-rejection

**Given** a `testdata/external_sealing/` sub-directory under `dbschema/` containing a separate Go package that imports `dbschema` and attempts `var _ dbschema.DefaultExpr = externalStub{}` where `externalStub` has no `defaultExpr()` method
**When** an in-package test runs `go build ./testdata/external_sealing/...` as a subprocess (or uses `go/packages` to load and type-check the package)
**Then** the build/type-check reports a compile error containing the substring `defaultExpr` or "missing method"

Implementation note: AC-3 verifies the sealing behavior at compile time, but the verification runs from within a normal `go test` via a subprocess `go build` invocation against the `testdata/` package. AC-2 provides a faster reflection-based smoke check that the unexported marker exists at all.

### REQ: default-literal

The package MUST export `type DefaultLiteral struct { Value any }` that satisfies `DefaultExpr`. `DefaultLiteral` carries a Go value to be used as a column default. The driver is responsible for translating the value to engine-specific SQL or storage syntax. Drivers MUST handle at minimum: `int`, `int64`, `float64`, `string`, `bool`, `[]byte`, and `nil`. Drivers MAY return a driver-specific error if the underlying Go type is unrepresentable in the target engine.

#### AC-1: literal-satisfies

**Given** a Go program importing `dbschema`
**When** the program declares `var d dbschema.DefaultExpr = dbschema.DefaultLiteral{Value: 0}`
**Then** the program compiles.

#### AC-2: literal-value-accessible

**Given** `d := dbschema.DefaultLiteral{Value: "guest"}`
**When** the program reads `d.Value`
**Then** the result is `"guest"` and has type `any` (assertable to `string`).

### REQ: default-current-timestamp

The package MUST export `type DefaultCurrentTimestamp struct{}` that satisfies `DefaultExpr`. The empty struct signals "use the engine's current-timestamp default at insertion time." Drivers translate to engine-specific SQL: `CURRENT_TIMESTAMP` (SQLite), `now()` or `CURRENT_TIMESTAMP` (PostgreSQL), or the equivalent for inGitDB.

#### AC-1: current-timestamp-satisfies

**Given** a Go program importing `dbschema`
**When** the program declares `var d dbschema.DefaultExpr = dbschema.DefaultCurrentTimestamp{}`
**Then** the program compiles.

## Architecture

| File | Contents |
|---|---|
| `dbschema/default_expr.go` | `DefaultExpr` sealed interface, `DefaultLiteral`, `DefaultCurrentTimestamp`. |
| `dbschema/default_expr_test.go` | Tests covering the ACs above. |

## Open Questions

- **Cross-engine "current timestamp" precision.** SQLite returns timestamps at second resolution by default; PostgreSQL at microsecond; some inGitDB layouts may store ISO-8601 strings. The Feature does not pin precision — drivers translate as their engine allows. Worth a godoc paragraph.

---
*This document follows the https://specscore.md/feature-specification*
