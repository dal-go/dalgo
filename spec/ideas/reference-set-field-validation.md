---
format: https://specscore.md/idea-specification
status: Draft
---
# Idea: Reference-set field validation

**Status:** Draft
**Date:** 2026-10-03
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** depends_on:framework-enforced-record-invariants

## Problem Statement

How might a field be declared as "its value must be one of the values in this
reference record set", so that DALgo rejects a write with an unknown value, the
same way in every adapter?

## Context

The founder's words, 2026-10-03: "We can also use this mapping ref recordsets as an
additional country field validator performed on dalgo level."

"This mapping" is a value mapping that a DataTug project holds: each of a dataset's
own values for a concept, mapped to the concept's universal codes. The first case is
countries. Chinook stores a country as free text in `Customer.Country`,
`Invoice.BillingCountry` and `Employee.Country` (24 distinct names such as `USA`,
`Brazil`, `United Kingdom`); another database might hold a numeric `CountryId`. The
mapping turns either into ISO 3166 codes from a shared set of universal country
concepts. The mapping is itself a record set: rows of (dataset value → code).

The same record set can answer a second question for free: is a value written to a
country field a known country at all? Today nothing in DALgo can express that.
`dbschema.FieldDef` carries name, type, length, precision, nullability, default and
auto-increment, and no allowed-values rule. A foreign key does not fit either: the
reference set may not live in the same database, or in any database (it can be a
file in a Git repository).

## Recommended Direction

Add an allowed-values rule to field definitions that points at a reference record
set, and enforce it on write through the framework, not per adapter.

- **Declaration.** A field may name a reference set and the column of that set that
  holds the allowed values (for Chinook: the mapping's "dataset value" column; for a
  database that already stores ISO codes: the universal set's code column).
- **Where the set comes from.** Any record set DALgo can read: a collection in the
  same database, a collection in another DALgo source, or a static file. The rule
  names it; the framework resolves and caches it.
- **Enforcement.** On insert and update, as part of the write-time validation that
  [framework-enforced-record-invariants](framework-enforced-record-invariants.md)
  moves into the framework, so a test double cannot disagree with production.
- **Modes.** `reject` (the write fails with an error naming the field and the
  value) and `report` (the write succeeds and the unknown value is reported), so a
  dataset with dirty history can adopt the rule without failing every write.
- **Null.** A null value is governed by the field's nullability, not by the rule.

## Alternatives Considered

- **A foreign key.** Only works when the reference set is a table in the same
  database; the universal country set is not.
- **An enum in the schema.** Copies the values into every schema; they drift from
  the shared set, and a 250-value enum is unreadable.
- **Validation in the application.** What exists today, and what the related idea
  shows goes wrong: rules declared in one place and enforced in another drift.

## MVP Scope

One rule on `FieldDef` naming a reference set and its value column; resolution from
a collection in the same source and from a static file; `reject` and `report`
modes; enforced on insert and update through the framework path; one test with the
Chinook country names against a country mapping.

## Not Doing (and Why)

- **Value transformation on write** (storing the ISO code instead of the name).
  Validation only; mapping values is the reader's job.
- **Remote fetching at write time without a cache.** A write must not depend on a
  network call per row.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A reference set small enough to cache (countries: about 250 rows) covers the first uses | Build the Chinook country mapping and the universal country set; measure rows and load time |
| Must-be-true | The framework write path from framework-enforced-record-invariants lands first or together with this, so the rule is enforced by every adapter | Sequence the two features; refuse to ship this rule on an adapter-by-adapter basis |
| Should-be-true | Exact matching is the right default; normalisation is an explicit option | Run the Chinook names and a second real dataset through both and compare unknown counts |

## SpecScore Integration

Promotes to a feature under `spec/features/dbschema/` (a field rule) and touches
the write-validation feature that
[framework-enforced-record-invariants](framework-enforced-record-invariants.md)
promotes to.

## Open Questions

- Case and whitespace: is `usa` a match for `USA`? Recommendation: exact match by
  default, with an explicit normalisation option on the rule.
- Cache lifetime and refresh for a set read from another source or a file.
- Does `report` mode write its findings somewhere DALgo defines, or only return
  them to the caller?

---
*This document follows the https://specscore.md/idea-specification*
