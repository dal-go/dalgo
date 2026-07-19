---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Hierarchical access and audit policies

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies?op=request-change) |
**Status:** Stable
**Date:** 2026-07-17
**Owner:** alex
**Source Ideas:** access-policies

## Summary

Add portable, adapter-independent access policies to DALgo. A secured `dal.DB` enforces named, hierarchical rules over logical DAL paths for point reads, queries, mutations, and the reserved `TRUNCATE` operation. Database-bound policies and context-bound capabilities compose by intersection. The same hierarchy also supports an independent audit-selection decision so applications can record mutations to sensitive collections while excluding the audit-log collection itself.

The product promise is: **define least privilege once and enforce it across every DALgo adapter.**

## Problem

DALgo currently provides a common API for records, queries, and transactions, but a component receiving a `dal.DB` or transaction receives all authority exposed by that handle. Applications with extensions, plug-ins, tenants, background jobs, analytics users, or support tooling must rely on convention to keep code inside an allowed subtree. A mistake can write a root collection, enumerate sensitive records, or query data outside the current tenant.

Database-native security systems do not solve this portably: Firestore rules, PostgreSQL row-level security, filesystem permissions, and in-memory test databases expose different models. Applications either duplicate authorization per backend or lose the boundary when switching adapters and in tests.

Auditing has a related path-selection problem. Applications need to record selected mutations (for example changes to `users` and `transactions`) without recursively recording writes to the audit-log collection. Reusing one deterministic path and operation matcher avoids a second policy language while keeping access authorization and audit selection independent.

## Design Principles

- **Capability boundary, not validation hook.** Enforcement wraps DAL interfaces before an adapter sees an operation.
- **Least privilege is explicit.** Write never implies read; `Get`, `Exists`, and `Query` are distinct.
- **Hierarchical and order-independent.** A more specific rule overrides an inherited rule within the same policy; declaration order has no effect.
- **Monotonic composition.** Adding another policy can only remove authority.
- **Fail closed.** Missing matches, unknown operations, opaque queries, and ambiguous rules are denied.
- **Explainable.** Named policies and matched rules are available in denial errors and decision explanations.
- **Adapter-independent.** Policies reason over DAL keys and structured queries, not backend-native path strings or SQL text.

## Behavior

### REQ: operation-vocabulary

The access-policy package MUST define separate leaf operations for `Get`, `Exists`, `Query`, `Insert`, `Set`, `Update`, `Delete`, and `Truncate`. It MUST expose convenience groups equivalent to `Read = Get + Exists + Query`, `Write = Insert + Set + Update + Delete + Truncate`, and `ReadWrite = Read + Write`. Granting any leaf or group MUST grant exactly its member operations: in particular, write MUST NOT imply read and query MUST NOT imply get.

`Truncate` MUST be available for policy construction and direct policy evaluation even though `dal.WriteSession` does not yet expose a truncate method. Adding a future DAL truncate capability MUST reuse this operation rather than changing existing policy meaning.

#### AC-1: write-only-insert

**Given** a policy that allows only `Insert` under `/logs/*`
**When** `Insert`, `Get`, `Query`, `Update`, `Delete`, and `Truncate` requests for `/logs/entry-1` are evaluated
**Then** only the `Insert` request is allowed.

#### AC-2: get-without-query

**Given** a policy that allows `Get` but not `Query` under `/secrets/*`
**When** a point-get request and a collection-query request are evaluated for `secrets`
**Then** the point get is allowed and the query is denied.

#### AC-3: truncate-is-reserved

**Given** a policy that explicitly allows `Truncate` on the `stagingEvents` collection
**When** a truncate request for that collection is evaluated directly through the policy API
**Then** the request is allowed without requiring `dal.WriteSession` to expose a truncate method.

### REQ: structural-path-patterns

Policies MUST match structural DAL paths rather than escaped path strings. A path pattern MUST support literal collection names, literal record IDs, an any-ID matcher, record paths, terminal collection paths, and inherited subtree matching. The package MUST provide explicit constructors for collection-group and opaque-query resources; neither resource kind may match an ordinary path rule accidentally.

#### AC-1: typed-id-and-slash-safe

**Given** a policy path containing a literal ID whose value contains `/`
**When** a DAL key with the same unescaped ID is evaluated
**Then** it matches structurally without parsing or splitting the ID value.

#### AC-2: terminal-collection-rule

**Given** a rule attached to the collection path `/spaces/*/ext/trackus/events`
**When** a query of that collection and an insert of a record directly in that collection are evaluated
**Then** both resources match the collection rule and an unrelated sibling collection does not.

### REQ: hierarchical-policy-precedence

Within one policy, decisions MUST inherit into descendant paths. The matching rule with the greatest path depth MUST win; at equal depth a rule with more literal segments MUST outrank one with more wildcard segments; if equally specific rules conflict, deny MUST win. Rule declaration order MUST NOT affect the result. A child allow MAY reopen a subtree denied by its parent within that same policy, and a child deny MAY carve a subtree out of a parent allow.

Policy construction MUST reject malformed path shapes. It MUST reject any remaining ambiguity that cannot be resolved by depth, literal specificity, and deny-on-tie.

#### AC-1: allowed-parent-denied-child

**Given** one policy that allows `ReadWrite` under `/ext/trackus/**` and denies `Delete` under `/ext/trackus/audit/**`
**When** delete requests target `/ext/trackus/items/i1` and `/ext/trackus/audit/a1`
**Then** the items deletion is allowed and the audit deletion is denied.

#### AC-2: denied-parent-reopened-children

**Given** one policy that denies `Read` under `/spaces/*/**` and allows `Read` under `/spaces/*/ext/trackus/**`
**When** reads target a space root, the trackus subtree, and a sibling extension subtree
**Then** only the read in the trackus subtree is allowed.

#### AC-3: order-independent

**Given** two policies containing identical hierarchical rules in opposite declaration orders
**When** the same set of requests is evaluated against each policy
**Then** every request receives the same decision from both policies.

### REQ: database-and-context-composition

A secured database MUST accept zero or more database-bound policies and MUST also read zero or more policies from `context.Context`. Every applicable policy MUST independently allow every resource in a request; a denial from any policy MUST deny the request. A context policy MUST therefore be unable to reopen a resource denied by a database policy. The wrapper MUST offer an option that requires at least one context-bound policy and denies operations when none is present.

The package MUST support binding the policies currently carried by a context to a secured database handle so that later use of a different operation context cannot discard those captured restrictions. Additional policies supplied by a later operation context MUST narrow the captured authority further.

#### AC-1: context-cannot-widen-global

**Given** a database policy denying writes under `/system/**` and a context policy allowing writes under `/system/public/**`
**When** a write to `/system/public/item-1` is attempted through the secured database
**Then** the write is denied before the underlying adapter is called.

#### AC-2: bound-policy-survives-context-replacement

**Given** a secured database bound to an extension policy allowing only `/ext/trackus/**`
**When** code uses that bound handle with `context.Background()` to write `/users/u1`
**Then** the write remains denied.

#### AC-3: required-context-policy

**Given** a secured database configured to require a context policy
**When** a read or transaction is started with a context carrying no access policy
**Then** it fails with an access-denied error before the adapter operation or transaction begins.

### REQ: portable-policy-documents

Access and audit policies MUST have a versioned, storage-neutral document representation. YAML MUST be the canonical human-authored format and JSON MUST encode the same document model without semantic differences. The document MUST carry an API version, policy kind (`AccessPolicy` or `AuditPolicy`), name, default effect, hierarchical scopes, operation groups or leaf operations, and named rules.

The package MUST provide YAML and JSON marshal/unmarshal helpers for byte slices and encode/decode helpers over `io.Writer` and `io.Reader`; these APIs MUST NOT require filesystem paths. Decoding MUST validate the complete document before returning a policy and MUST reject unknown API versions, unknown operations/effects, malformed paths, mixed access/audit effects, and ambiguous rules. Encoding a policy that contains a custom callback or another non-declarative rule MUST return a descriptive not-serializable error.

The codec boundary MUST be extensible so another package can add HCL or a storage-specific encoding without changing policy evaluation. Native HCL parsing and expression evaluation are not required by this Feature.

#### AC-1: yaml-roundtrip

**Given** a hierarchical access policy document in YAML containing parent deny and child allow scopes
**When** it is decoded, evaluated, encoded to YAML, and decoded again
**Then** both decoded policies return identical decisions and explanations for the same request table.

#### AC-2: json-has-identical-semantics

**Given** one versioned policy document represented once as YAML and once as JSON
**When** both representations are decoded
**Then** they produce equivalent policy names, rules, defaults, and decisions.

#### AC-3: storage-neutral-streams

**Given** a policy document stored in an in-memory buffer, object-store response body, database blob, or file
**When** its bytes are exposed through `io.Reader` and decoded, or a policy is encoded through `io.Writer`
**Then** the codec succeeds without inspecting or requiring the underlying storage type or path.

#### AC-4: invalid-or-future-document-rejected

**Given** a document with an unknown API version, operation, effect, or malformed path
**When** it is decoded
**Then** decoding returns a descriptive validation error and no executable policy.

### REQ: complete-session-enforcement

The secured DAL wrapper MUST authorize all current read and write session methods: `Exists`, `Get`, `GetMulti`, both query-executor methods, `Set`, `SetMulti`, `Insert`, `InsertMulti`, `Update`, `UpdateRecord`, `UpdateMulti`, `Delete`, and `DeleteMulti`. `GetMulti` and every multi-mutation MUST preflight every resource and MUST call the adapter zero times when any resource is denied.

The wrapper MUST expose secured read-session, write-session, read-transaction, and read-write-transaction handles without exposing the wrapped raw session through a public accessor.

#### AC-1: every-leaf-method-enforced

**Given** a recording session and a policy that denies all resources
**When** each read-session and write-session leaf method is invoked through its secured wrapper
**Then** every method returns an access-denied error and the recording session observes no delegated operation.

#### AC-2: batch-preflight-is-all-or-nothing

**Given** a three-record batch where the first and third resources are allowed and the second is denied
**When** any secured multi-operation is invoked
**Then** the call is denied and the adapter receives none of the three resources.

### REQ: transaction-capability-capture

`RunReadonlyTransaction` and `RunReadwriteTransaction` MUST capture the effective policies at transaction start, pass a secured transaction to the worker, and replace any transaction stored in the worker context with that secured transaction. Every operation on the transaction MUST enforce the captured policies even when supplied another context. Transaction retry behavior and options MUST continue to be delegated to the adapter.

#### AC-1: readwrite-transaction-is-secured

**Given** a context policy allowing writes only under `/ext/trackus/**`
**When** a read-write transaction worker tries an allowed insert and then a forbidden root write using `context.Background()`
**Then** the worker receives a secured transaction, the allowed insert reaches the adapter, and the root write is denied.

#### AC-2: context-transaction-is-secured

**Given** an adapter that stores its raw transaction in the worker context
**When** the secured transaction worker calls `dal.GetTransaction(workerContext)`
**Then** it receives the secured transaction rather than the adapter's raw transaction.

### REQ: generated-insert-authorization

An insert whose record key has no ID yet MUST be authorized against its parent and target collection before adapter-native or DALgo ID generation. An any-ID collection rule MAY authorize it. A rule that requires a specific record ID MUST NOT authorize an incomplete-key insert. Adapter-internal collision checks MUST NOT require the caller to hold `Exists` or `Get` permission.

#### AC-1: generated-id-under-allowed-collection

**Given** an insert-only policy for `/logs/*` and an incomplete key for the `logs` collection
**When** an insert with an ID generator is performed through a secured write session
**Then** the insert reaches the adapter without read permission and the adapter may assign the ID.

#### AC-2: exact-id-does-not-match-incomplete-key

**Given** a policy allowing insert only at `/jobs/fixed-id`
**When** an incomplete-key insert into `jobs` is attempted
**Then** it is denied before ID generation.

### REQ: structured-query-enforcement

`Get`, `Exists`, and `Query` MUST remain independently authorizable. For a `dal.StructuredQuery`, the secured query executor MUST authorize the base source and every joined source before execution. Parent-scoped collections MUST be represented by their full logical collection path. A collection-group query MUST require an explicit matching collection-group rule. A non-structured or otherwise opaque query MUST require an explicit opaque-query rule and MUST NOT be authorized by inspecting query text. A query filter alone MUST NOT make an otherwise forbidden source authorized.

#### AC-1: query-only-does-not-grant-get

**Given** a policy that allows only `Query` on `/events`
**When** the caller queries `events` and separately gets `/events/e1`
**Then** the query is allowed and the point get is denied.

#### AC-2: every-join-source-authorized

**Given** a policy allowing query of `users` but not `transactions`
**When** a structured query joins `users` to `transactions`
**Then** the query is denied before the adapter executes it.

#### AC-3: collection-group-is-explicit

**Given** a path rule allowing queries under one tenant subtree but no collection-group rule
**When** a collection-group query targets a collection with the same name
**Then** the query is denied.

#### AC-4: opaque-query-is-explicit

**Given** a policy containing only structural path rules
**When** a `dal.TextQuery` or another non-structured query is executed
**Then** it is denied without attempting to infer resources from its text.

### REQ: denial-errors-and-explanations

Denied operations MUST return an error matching an exported sentinel through `errors.Is`. The concrete error MUST expose the operation, safe resource description, denying policy name, and an explanation of the winning or missing rule. Policies MUST support side-effect-free direct evaluation suitable for unit tests and tooling.

Unknown or invalid operation values MUST be denied. A future DALgo release MUST NOT silently grant a newly introduced operation through an existing compiled policy group.

#### AC-1: denial-is-inspectable

**Given** a named policy denying update of `/users/u1`
**When** that update is attempted
**Then** the returned error matches `ErrAccessDenied` and exposes `Update`, `/users/u1`, the policy name, and the matched deny rule.

#### AC-2: unknown-operation-denied

**Given** an operation value unknown to the policy implementation
**When** it is evaluated against a policy allowing `ReadWrite`
**Then** it is denied rather than inheriting a broad permission group.

### REQ: hierarchical-audit-selection

The package MUST provide an audit-selection policy using the same operations, resources, wildcard matching, hierarchy, specificity, and order-independence as access authorization. Its effects MUST be independent `Audit` and `IgnoreAudit` decisions, with default `IgnoreAudit`. Access `Allow`/`Deny` rules MUST NOT change audit selection, and audit rules MUST NOT grant or deny access.

The audit selector MUST be usable without a database and MUST return a structured explanation. Persisting, transporting, redacting, or retrying audit events is outside this Feature.

#### AC-1: audit-selected-mutations

**Given** an audit policy that audits `Write` under `/users/**` and `/transactions/**`
**When** get, insert, update, delete, and truncate requests are classified for those resources
**Then** the mutation requests are selected for audit and the get request is ignored.

#### AC-2: audit-log-recursion-carveout

**Given** an audit policy that audits all mutations and more specifically ignores mutations under `/auditLog/**`
**When** writes to `/users/u1` and `/auditLog/a1` are classified
**Then** the users write is selected and the audit-log write is ignored.

#### AC-3: audit-does-not-authorize

**Given** an access policy denying writes and an audit policy selecting the same writes
**When** a write is authorized and separately classified for audit
**Then** authorization remains denied while audit classification reports that the denied attempt is selected.

### REQ: compatibility-and-threat-boundary

Existing unwrapped `dal.DB` and session implementations MUST retain their current behavior. The access package MUST introduce no adapter dependency and MUST work with the built-in memory adapter. Documentation MUST state that code holding the raw adapter, a native database client, or another unrestricted handle can bypass a DALgo wrapper; the capability boundary covers operations routed through secured DALgo handles and is not an in-process sandbox.

#### AC-1: unwrapped-suite-remains-green

**Given** the existing DALgo test suite
**When** the access-policy package is added without wrapping existing database constructors
**Then** existing tests and unwrapped database behavior remain unchanged.

#### AC-2: memory-adapter-roundtrip

**Given** the built-in memory adapter wrapped with an extension policy
**When** allowed and forbidden reads, writes, batches, and transactions are exercised
**Then** allowed operations preserve the adapter's results and forbidden operations never mutate or disclose the forbidden resources.

## Acceptance Criteria

### AC: write-only-insert (verifies REQ:operation-vocabulary)

**Given** a policy that allows only `Insert` under `/logs/*`
**When** insert, get, query, update, delete, and truncate requests for `/logs/entry-1` are evaluated
**Then** only the insert request is allowed.

### AC: get-without-query (verifies REQ:operation-vocabulary)

**Given** a policy that allows `Get` but not `Query` under `/secrets/*`
**When** a point-get request and a collection-query request are evaluated for `secrets`
**Then** the point get is allowed and the query is denied.

### AC: truncate-is-reserved (verifies REQ:operation-vocabulary)

**Given** a policy that explicitly allows `Truncate` on the `stagingEvents` collection
**When** a truncate request for that collection is evaluated directly through the policy API
**Then** the request is allowed without requiring `dal.WriteSession` to expose a truncate method.

### AC: typed-id-and-slash-safe (verifies REQ:structural-path-patterns)

**Given** a policy path containing a literal ID whose value contains `/`
**When** a DAL key with the same unescaped ID is evaluated
**Then** it matches structurally without parsing or splitting the ID value.

### AC: terminal-collection-rule (verifies REQ:structural-path-patterns)

**Given** a rule attached to the collection path `/spaces/*/ext/trackus/events`
**When** a query of that collection and an insert of a record directly in that collection are evaluated
**Then** both resources match the collection rule and an unrelated sibling collection does not.

### AC: allowed-parent-denied-child (verifies REQ:hierarchical-policy-precedence)

**Given** one policy that allows `ReadWrite` under `/ext/trackus/**` and denies `Delete` under `/ext/trackus/audit/**`
**When** delete requests target `/ext/trackus/items/i1` and `/ext/trackus/audit/a1`
**Then** the items deletion is allowed and the audit deletion is denied.

### AC: denied-parent-reopened-children (verifies REQ:hierarchical-policy-precedence)

**Given** one policy that denies `Read` under `/spaces/*/**` and allows `Read` under `/spaces/*/ext/trackus/**`
**When** reads target a space root, the trackus subtree, and a sibling extension subtree
**Then** only the read in the trackus subtree is allowed.

### AC: order-independent (verifies REQ:hierarchical-policy-precedence)

**Given** two policies containing identical hierarchical rules in opposite declaration orders
**When** the same set of requests is evaluated against each policy
**Then** every request receives the same decision from both policies.

### AC: context-cannot-widen-global (verifies REQ:database-and-context-composition)

**Given** a database policy denying writes under `/system/**` and a context policy allowing writes under `/system/public/**`
**When** a write to `/system/public/item-1` is attempted through the secured database
**Then** the write is denied before the underlying adapter is called.

### AC: bound-policy-survives-context-replacement (verifies REQ:database-and-context-composition)

**Given** a secured database bound to an extension policy allowing only `/ext/trackus/**`
**When** code uses that bound handle with `context.Background()` to write `/users/u1`
**Then** the write remains denied.

### AC: required-context-policy (verifies REQ:database-and-context-composition)

**Given** a secured database configured to require a context policy
**When** a read or transaction is started with a context carrying no access policy
**Then** it fails with an access-denied error before the adapter operation or transaction begins.

### AC: yaml-roundtrip (verifies REQ:portable-policy-documents)

**Given** a hierarchical access policy document in YAML containing parent deny and child allow scopes
**When** it is decoded, evaluated, encoded to YAML, and decoded again
**Then** both decoded policies return identical decisions and explanations for the same request table.

### AC: json-has-identical-semantics (verifies REQ:portable-policy-documents)

**Given** one versioned policy document represented once as YAML and once as JSON
**When** both representations are decoded
**Then** they produce equivalent policy names, rules, defaults, and decisions.

### AC: storage-neutral-streams (verifies REQ:portable-policy-documents)

**Given** a policy document exposed through an `io.Reader` rather than a filesystem path
**When** it is decoded or a policy is encoded through an `io.Writer`
**Then** the codec succeeds without inspecting or requiring the underlying storage type.

### AC: invalid-or-future-document-rejected (verifies REQ:portable-policy-documents)

**Given** a document with an unknown API version, operation, effect, or malformed path
**When** it is decoded
**Then** decoding returns a descriptive validation error and no executable policy.

### AC: every-leaf-method-enforced (verifies REQ:complete-session-enforcement)

**Given** a recording session and a policy that denies all resources
**When** each read-session and write-session leaf method is invoked through its secured wrapper
**Then** every method returns an access-denied error and the recording session observes no delegated operation.

### AC: batch-preflight-is-all-or-nothing (verifies REQ:complete-session-enforcement)

**Given** a three-record batch where the first and third resources are allowed and the second is denied
**When** any secured multi-operation is invoked
**Then** the call is denied and the adapter receives none of the three resources.

### AC: readwrite-transaction-is-secured (verifies REQ:transaction-capability-capture)

**Given** a context policy allowing writes only under `/ext/trackus/**`
**When** a read-write transaction worker tries an allowed insert and then a forbidden root write using `context.Background()`
**Then** the worker receives a secured transaction, the allowed insert reaches the adapter, and the root write is denied.

### AC: context-transaction-is-secured (verifies REQ:transaction-capability-capture)

**Given** an adapter that stores its raw transaction in the worker context
**When** the secured transaction worker calls `dal.GetTransaction(workerContext)`
**Then** it receives the secured transaction rather than the adapter's raw transaction.

### AC: generated-id-under-allowed-collection (verifies REQ:generated-insert-authorization)

**Given** an insert-only policy for `/logs/*` and an incomplete key for the `logs` collection
**When** an insert with an ID generator is performed through a secured write session
**Then** the insert reaches the adapter without read permission and the adapter may assign the ID.

### AC: exact-id-does-not-match-incomplete-key (verifies REQ:generated-insert-authorization)

**Given** a policy allowing insert only at `/jobs/fixed-id`
**When** an incomplete-key insert into `jobs` is attempted
**Then** it is denied before ID generation.

### AC: query-only-does-not-grant-get (verifies REQ:structured-query-enforcement)

**Given** a policy that allows only `Query` on `/events`
**When** the caller queries `events` and separately gets `/events/e1`
**Then** the query is allowed and the point get is denied.

### AC: every-join-source-authorized (verifies REQ:structured-query-enforcement)

**Given** a policy allowing query of `users` but not `transactions`
**When** a structured query joins `users` to `transactions`
**Then** the query is denied before the adapter executes it.

### AC: collection-group-is-explicit (verifies REQ:structured-query-enforcement)

**Given** a path rule allowing queries under one tenant subtree but no collection-group rule
**When** a collection-group query targets a collection with the same name
**Then** the query is denied.

### AC: opaque-query-is-explicit (verifies REQ:structured-query-enforcement)

**Given** a policy containing only structural path rules
**When** a `dal.TextQuery` or another non-structured query is executed
**Then** it is denied without attempting to infer resources from its text.

### AC: denial-is-inspectable (verifies REQ:denial-errors-and-explanations)

**Given** a named policy denying update of `/users/u1`
**When** that update is attempted
**Then** the returned error matches `ErrAccessDenied` and exposes `Update`, `/users/u1`, the policy name, and the matched deny rule.

### AC: unknown-operation-denied (verifies REQ:denial-errors-and-explanations)

**Given** an operation value unknown to the policy implementation
**When** it is evaluated against a policy allowing `ReadWrite`
**Then** it is denied rather than inheriting a broad permission group.

### AC: audit-selected-mutations (verifies REQ:hierarchical-audit-selection)

**Given** an audit policy that audits `Write` under `/users/**` and `/transactions/**`
**When** get, insert, update, delete, and truncate requests are classified for those resources
**Then** the mutation requests are selected for audit and the get request is ignored.

### AC: audit-log-recursion-carveout (verifies REQ:hierarchical-audit-selection)

**Given** an audit policy that audits all mutations and more specifically ignores mutations under `/auditLog/**`
**When** writes to `/users/u1` and `/auditLog/a1` are classified
**Then** the users write is selected and the audit-log write is ignored.

### AC: audit-does-not-authorize (verifies REQ:hierarchical-audit-selection)

**Given** an access policy denying writes and an audit policy selecting the same writes
**When** a write is authorized and separately classified for audit
**Then** authorization remains denied while audit classification reports that the denied attempt is selected.

### AC: unwrapped-suite-remains-green (verifies REQ:compatibility-and-threat-boundary)

**Given** the existing DALgo test suite
**When** the access-policy package is added without wrapping existing database constructors
**Then** existing tests and unwrapped database behavior remain unchanged.

### AC: memory-adapter-roundtrip (verifies REQ:compatibility-and-threat-boundary)

**Given** the built-in memory adapter wrapped with an extension policy
**When** allowed and forbidden reads, writes, batches, and transactions are exercised
**Then** allowed operations preserve the adapter's results and forbidden operations never mutate or disclose forbidden resources.

## Architecture

The implementation adds a top-level `access` package that imports `dal` and implements secured wrappers around DAL interfaces. Keeping it outside `dal` avoids import cycles and keeps the core interfaces unchanged. The package contains:

- Operation and operation-group definitions.
- Structural `Resource` and hierarchical path-pattern types.
- Declarative access and audit policies compiled from shared scope rules.
- Context policy attachment and binding.
- Secured DB/session/transaction/query wrappers.
- Typed denial and explanation values.

No adapter is modified. Adapter conformance follows from wrappers delegating only after policy evaluation.

## Error Handling and Failure Modes

- No matching access rule: deny with `ErrAccessDenied`.
- Explicit winning deny: deny with named policy/rule explanation.
- Missing required context capability: deny before transaction or operation delegation.
- One denied batch resource: deny the entire batch before delegation.
- Unsupported/opaque query without an explicit special-resource rule: deny.
- Invalid path or conflicting unresolvable rule construction: constructor error; `Must...` convenience may panic.
- Audit selection with no match: ignore.
- Audit selection errors never change access authorization because the decisions are evaluated independently.

## Testing Strategy

- Pure table tests for operations, structural paths, specificity, wildcard ties, access effects, and audit effects.
- Recording fake sessions to prove every DAL method is preflighted and batches are all-or-nothing.
- Fake transaction coordinators to prove policy capture, retry delegation, options, and context transaction replacement.
- Built-in `dalgo2memory` integration tests for allowed/denied round trips and generated inserts.
- Structured-query tests covering parent collections, joins, collection groups, and opaque queries.
- `errors.Is` and explanation-field tests.
- Existing full suite for compatibility.
- Static-site link/content checks plus responsive browser verification for DALgo.io.

## Out of Scope

- Adding `Truncate` to `dal.WriteSession` or implementing truncate in adapters.
- Field-level response redaction or record-content predicates.
- Query constraints over `WHERE`, projection, ordering, grouping, aggregates, indexes, limits, or cost. The separate `Query` operation and structured request preserve room for these follow-ups.
- Automatically injecting tenant predicates into a query.
- Persisting audit records, choosing an audit schema, delivery guarantees, redaction, retention, or sink-failure semantics.
- A built-in filesystem repository or database table for policy documents; codecs operate on bytes and streams supplied by the caller.
- Native HCL support, variables, functions, interpolation, or expression evaluation. HCL can be implemented later through the codec boundary if a concrete consumer needs it.
- Protecting access that bypasses DALgo through a raw adapter or native client.
- Treating policies as a substitute for database IAM, network controls, encryption, or hostile plug-in sandboxing.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
