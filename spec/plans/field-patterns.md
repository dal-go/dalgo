---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Implement field patterns

**Status:** Implemented
**Source Feature:** access-policies/field-patterns
**Date:** 2026-09-03
**Owner:** alex
**Supersedes:** —

## Summary

Implement `access-policies/field-patterns`: `fields` allow-lists on allow
rules, validated at construction; column projection (or reader redaction) on
queries; record redaction on point reads; touched-field checks on writes;
intersection of the lists every applicable policy imposes; portable documents
carrying `fields`.

## Approach

A rule's list compiles to a matcher (`fieldSet`); the decision surfaces it on
the deciding alternative or terminal of each policy's `WriteResidual`, which
the read and write paths already receive. Reads take the intersection of the
lists of the alternatives that decide the loaded record (per policy); queries
take the intersection of every list a row could come through, projecting the
query's columns when the lists enumerate top-level names and wrapping the
reader in a redactor otherwise. Writes refuse before delegation when any
touched field falls outside the intersection. No adapter changes.

## Tasks

### Task 1: Pattern vocabulary and rule builder

**Id:** task-1
**Verifies:** access-policies/field-patterns#ac:pattern-forms
**Depends-On:** —
**Status:** complete

`parseFieldPatterns` (literal, `*` segment, `prefix*`/`*suffix`, trailing
`.*` subtree), `Rule.Fields(...)` on allow rules only, document codec `fields`,
rule names suffixed with the list in explanations.

### Task 2: Read projection and redaction

**Id:** task-2
**Verifies:** access-policies/field-patterns#ac:query-projected, access-policies/field-patterns#ac:get-redacted, access-policies/field-patterns#ac:condition-may-read-hidden-field
**Depends-On:** 1
**Status:** complete

`projectQuery` narrows `Columns` through `dal.WithColumns`; `redactingReader`
prunes rows an adapter returns unprojected; `enforceRead` evaluates row
conditions on the full record first, then redacts to the deciding
alternative's list; a recordset query that cannot be projected is denied.

### Task 3: Write field checks

**Id:** task-3
**Verifies:** access-policies/field-patterns#ac:update-outside-fields
**Depends-On:** 1
**Status:** complete

Insert and Set data paths, Update paths (`FieldName`/`FieldPath`) checked
against the deciding alternative's or terminal's list; a violation denies the
whole batch naming the field.

### Task 4: Intersection across policies and joins

**Id:** task-4
**Verifies:** access-policies/field-patterns#ac:intersection-across-policies
**Depends-On:** 2, 3
**Status:** complete

`fieldSets` intersects the lists of every applicable policy for reads, queries
and writes; field rules on a joined source are refused in this version.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
