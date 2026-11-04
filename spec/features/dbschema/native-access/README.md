---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: dbschema NativeAccessReader (who can reach a collection natively)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/native-access?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/native-access?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/native-access?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/native-access?op=request-change) |

**Status:** Implementing
**Date:** 2026-11-04
**Owner:** alex
**Source Ideas:** —
**Parent Feature:** [`dbschema`](../README.md)

## Summary

An optional, additive capability interface, `NativeAccessReader`, that answers "which database principals can reach this
collection natively, with which privileges, and why": access through the engine's own accounts and roles, outside any DALgo
access policy. The answer labels each privilege with its source and its confidence, and states what the engine's catalog cannot
show. It never answers "nobody" when it cannot know.

First consumer: DataTug Perimeter's native-holder rows (REQ:native-access-discovery in `datatug/backstage`), which today
cannot be built without engine-specific catalog SQL in DataTug's core. First implementation: `dalgo2postgres`.

## Approval of the exported API

DALgo's AGENTS.md requires a human's explicit reason for new exported API. Alexander Trakhimenok (founder), 2026-11-04:

- Asked: "May DALgo get a new optional, additive interface for reading database roles and privileges?" Answer: "Yes, approve it"
- Reason: "if we need to read Postgres schema, that should be done in Dalgo adapter, not custom something."

Scope as approved: one new optional capability interface plus its value types in `dbschema`. No existing exported interface,
type or behaviour changes.

## Behavior

### REQ: optional-capability

`dbschema` MUST export `NativeAccessReader` with one method,
`ReadNativeAccess(ctx context.Context, ref *dal.CollectionRef) (*NativeAccess, error)`. It is separate from `SchemaReader`, so
no existing driver changes; callers resolve it with `dal.As[NativeAccessReader](db)`. A driver whose engine has no principals
(SQLite, for example; file-system access is outside this model) or that cannot read its authorization catalog does not
implement it. The types carry no serialization tags: a consumer maps an answer to its own wire format.

#### AC: absent-is-unknown

**Given** a driver that does not implement `NativeAccessReader`, or one whose `ReadNativeAccess` returns `*NotSupportedError`
**When** a caller resolves it with `dal.As` and asks
**Then** the assertion fails or the call errors, and the documented reading is "native access unknown", never "nobody".

### REQ: engine-neutral-answer

The answer types MUST name no engine. A principal has a name, a kind (`login`, `group`, `everyone`; only `login` can open a
session), and superuser and owner flags. Each privilege names an operation (`read`, `insert`, `update`, `delete`, `truncate`), optional fields
for field-level privileges, and every grant behind it: a source (`direct`, `membership`, `everyone`, `ownership`, `superuser`,
`builtin-role`), a route (`self`, `inherited`, `switch-role`, `self-grant`), the membership chain, and a confidence (`verified`
from an explicit catalog entry, `inferred` by the adapter, `unknown` where input was missing). The answer also carries the
collection's row-security state, each principal's row-security effect (`off`, `applies`, `bypassed`) and namespace access
(`granted`, `missing`, `unknown`, or `not-applicable` for an engine with no separate namespace gate), and the catalog time
`AsOf`. Confidence is ordered from strongest to weakest: `verified`, `inferred`, `unknown`. When the engine records an owner of
the collection, the owner is always listed.

### REQ: never-nobody

An implementation MUST return an error, or an answer whose coverage is `partial` (catalog read; blind spots named) or `unknown`
(some input could not be read or resolved, listed as gaps). A collection that does not exist MUST be an error. An answer with no
principal MUST have coverage `unknown`. Blind spots shared by engines are named by constants: connection rules, external-identity
(cloud IAM) logins, replicas and backups, shared credentials, definer-rights code and row-security predicates.

#### AC: unresolved-input-is-unknown

**Given** a catalog in which a grant names a principal the adapter cannot resolve
**When** native access is read
**Then** the coverage is `unknown` with a gap naming the input, never a shorter list read as complete.

### REQ: inventory-not-authorization

The answer MUST NOT grant, widen or narrow DALgo access, and MUST NOT be used to authorize an operation.

## Non-Goals

- Mapping principals to people. That is the consumer's, from an owner-declared mapping.
- Engine-specific detail in the types. Each adapter documents its own catalog reads and blind spots.
- A top-level helper function. `SourceViewReader` has none either; `dal.As` is enough.

## Open Questions

- Whether a second engine (MySQL, which needs `SELECT` on `mysql.*` to see other accounts' grants) needs a field the PostgreSQL
  answer does not, for example host-pattern accounts. Decide when that adapter is specified.
- Grant scope. MySQL (`GRANT SELECT ON *.*`, `ON db.*`) and SQL Server (database- and schema-level grants) give access to a
  table from a wider grant, which today can only be reported as `direct`, losing why. An optional scope on a grant (object,
  namespace, database, server) is additive and can be added with the first such adapter.
- Denials. SQL Server `DENY` has no place in the model: its adapter resolves denials itself and reports only the privileges that
  remain, naming any it cannot resolve as gaps.

---
*This document follows the https://specscore.md/feature-specification*
