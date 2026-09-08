---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: Layered ACL MVP across DTQL, DALgo, OpenVaultDB, InGitDB and DataTug

**Status:** Executing
**Source:** none
**Date:** 2026-09-08
**Owner:** astra
**Coordination:** dal-go/dalgo@layered-acl-query
**Supersedes:** —

## Summary

One cross-repository implementation plan for the approved layered ACL design.
DTQL owns the interoperable contracts; DALgo evolves its existing ACL evaluator;
each data owner enforces mandatory policies; DataTug uses the remote APIs and
explains effective access. The initial real read slice is already implemented
locally. Its completed tasks below are reconciled with actual commit/test evidence,
not represented as new work or as the complete MVP.

User approval covers implementation. Latest scope excludes the policy viewer/editor,
prioritizes DTQL over OpenVaultDB to InGitDB and SQLite, uses local dependency
rewrites, and avoids frequent remote pushes/merges. No new sign-off is required to
execute these authorized tasks. Deployment/publication is distinct from implementation.

The approved design is `datatug/dtql@c2d0487` on `layered-acl-design`, including
`design/layered-acl/01-current-state.md` through `19-scoped-mask-stages.md`,
`contracts/acl.schema.json`, acceptance vectors and `reviews/round-3`.
The original reviewed work packages remain design evidence; this plan is the
current implementation progress authority. Existing DALgo plans for access-policies,
principal-bindings and row-level conditions remain reusable completed foundations.

## Approach

Execute dependencies in order. Task completion means code committed and verified
locally against the documented scope; it does not mean pushed, released or deployed.
Only task 24 establishes publication. Update task status through SpecScore CLI,
attach tests/commits as evidence, and never mark an umbrella task complete because
one proof-of-concept path passes. Root coordinates this plan; helpers report evidence
rather than writing the same plan/worktree concurrently.

The shared local workspace is `/tmp/layered-acl-query-local.work` with the four
`layered-acl-query` worktrees for DALgo, dalgo2sql, dalgo2ingitdb and OpenVaultDB.
Add consumer worktrees only when their tasks begin. Scope contracts/serialization
and enforcement precede parallel owner/client integration. Tasks 6, 7 and 12 can be
researched independently; after stable shared interfaces, identity, owner lifecycle
and transport work can run in parallel. Use cheaper models for bounded patches and
checks, with architecture/security review of shared enforcement changes.

MVP: portable files, masks/execution gates, principals, owner persistence, real reads
and UPDATE, structured safe blockers, bounded dry runs and DataTug Explain integration.
Follow-up: full policy administration APIs/UI, executing native SQL/GraphQL/procedures,
additional adapters and elaborate policy authoring. Out of scope: long-running
logical transactions, distributed ACID and an external ACL service. Retain only
bounded compatibility notes and a future provider seam for those topics.

## Tasks

### Task 1: Reviewed architecture and shared contract baseline

**Id:** acl-01
**Depends-On:** —
**Status:** complete
**Implemented-by:** datatug/dtql@c2d0487 (layered-acl-design)
**Note:** Prior work verified locally; not published. Approved A3 design packet and independent round-3 review evidence.
**Evidence:** datatug/dtql@c2d0487

**Repositories:** datatug/dtql

Preserve the approved A3 design packet, dual independent reviews, reconciliation and human approval. This task records existing work; it does not claim later implementation is reviewed.

**Acceptance:** The 19 design documents, contract fixtures and round-3 review evidence remain available at the recorded design commit.

**Validation:** Review packet and decision log; no implementation tests required.

### Task 2: Portable policy loader and safe DALgo query foundation

**Id:** acl-02
**Depends-On:** 1
**Status:** complete
**Implemented-by:** dal-go/dalgo@11cbf50 (layered-acl-query)
**Note:** Prior work verified locally; not published. Full Go suite; access and DTQL codec/security tests.
**Evidence:** dal-go/dalgo@11cbf50

**Repositories:** dal-go/dalgo

Load bounded owner-selected DTQL policy files using existing policies and principal bindings; reject malformed documents and unsupported features; guard caller filter/order/projection fields across nested secure layers.

**Acceptance:** Enabled invalid configuration fails closed; original caller query remains distinct from policy-injected predicates; legacy DALgo behavior is retained.

**Validation:** access and dtql suites, including hidden-field and strict codec regressions.

### Task 3: InGitDB owner enforcement for the first query slice

**Id:** acl-03
**Depends-On:** 2
**Status:** complete
**Implemented-by:** ingitdb/dalgo2ingitdb@7ac93c5 (layered-acl-query)
**Note:** Prior work verified locally; not published. Full Go suite and vet; owner-manifest, direct enforcement and remount tests.
**Evidence:** ingitdb/dalgo2ingitdb@7ac93c5

**Repositories:** ingitdb/dalgo2ingitdb

Load the local owner manifest before returning a secured database handle. Preserve mandatory policies under direct access, transactions and new contexts; retain safe schema introspection.

**Acceptance:** Existing ACL directory with absent or invalid manifest fails opening; remount preserves restrictions; no raw backend escapes.

**Validation:** Owner-manifest and direct table/row/column enforcement tests.

### Task 4: Safe SQLite query compilation and record identity

**Id:** acl-04
**Depends-On:** 2
**Status:** complete
**Implemented-by:** dal-go/dalgo2sql@c42e751 (layered-acl-query)
**Note:** Prior work verified locally; not published. Full Go suite and vet; safe SQLite compiler and projected-key regressions.
**Evidence:** dal-go/dalgo2sql@c42e751

**Repositories:** dal-go/dalgo2sql

Provide the opt-in SQLite compiler with quoted identifiers, bound values, correct pagination and private primary-key transport for projected record reads.

**Acceptance:** No SQL display-text execution in the opted-in path; unknown fields fail; explicit name-only projection retains key; other dialects keep existing behavior.

**Validation:** SQL compiler/reader tests and real SQLite injection, missing-column and pagination regressions.

### Task 5: OpenVaultDB real HTTP query integration

**Id:** acl-05
**Depends-On:** 3, 4
**Status:** complete
**Implemented-by:** openvaultdb/openvaultdb-go@5e4bc80 (layered-acl-query)
**Note:** Prior work verified locally; not published. Full Go suite; TestLayeredACL_DTQL; standalone HTTP smoke before and after remount.
**Evidence:** openvaultdb/openvaultdb-go@5e4bc80

**Repositories:** openvaultdb/openvaultdb-go

Load upper-owner policies, authenticate and resolve principals, scope token checks to the parsed collection, and execute DTQL through all mandatory layers. Provide a runnable local demonstration.

**Acceptance:** Real reads against InGitDB and SQLite enforce rows before pagination and column restrictions; owner token cannot bypass data ACL; policy viewer/editor absent.

**Validation:** TestLayeredACL_DTQL, full server suite, standalone HTTP smoke before and after remount.

### Task 6: Query-profile and enforcement boundary completion

**Id:** acl-06
**Depends-On:** 5
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; dal-go/dalgo2sql

Finish the supported-profile matrix, resource limits, cancellation behavior and alternate-reader/transaction bypass checks. Audit custom SQL function, computed-field and hidden-predicate behavior; reject unsupported cases.

**Acceptance:** A tested capability matrix describes exactly which query shapes are supported per adapter; no silent weakening or hard-resource-limit claims unsupported by adapters.

**Validation:** Cross-engine query fixtures, malformed inputs, cancellation, hidden-filter/order and alternative entry-point tests.

### Task 7: Canonical scoped masks and portable round-trip model

**Id:** acl-07
**Depends-On:** 2
**Status:** in_progress
**Note:** Implementing approved canonical scoped mask model and round-trip contracts.

**Repositories:** dal-go/dalgo; datatug/dtql

Implement the A3 ordered include/exclude stage AST and arbitrary-position segment globs. Merge adjacent same-action runs before evaluation; normalize stars and canonical pattern order; expose parse/validate/serialize through one portable model.

**Acceptance:** Approved scoped-mask vectors and canonical YAML/JSON fixtures round-trip with identical meaning; comments/format retention remains future; no DALgo-only language.

**Validation:** A3 vectors: include a*/b*, exclude *c*, include **d/a*f; invalid shapes; depth/pattern limits; Unicode/case and normalization.

### Task 8: Masks in existing field and collection enforcement

**Id:** acl-08
**Depends-On:** 7
**Status:** queued

**Repositories:** dal-go/dalgo

Integrate collectionMask and fieldMask into existing policy decisions, field-set intersections, redaction and affected-path checks. Preserve restored leaf projection without granting whole-parent access.

**Acceptance:** Every mandatory policy intersects; leaf restoration cannot bypass another owner; parent writes include all affected descendants; legacy Fields matcher unchanged.

**Validation:** Layered query/read/update tests, hidden-field probes, restored leaves, denied sibling/parent replacement, opaque arrays fail closed.

### Task 9: Execution-class gates and stored-procedure masks

**Id:** acl-09
**Depends-On:** 7, 8
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go

Classify DTQL, caller-native SQL/GraphQL and stored-procedure requests at trusted boundaries; apply execution gates and namespaced callable masks. Internally compiled SQL remains DTQL. Native execution remains unsupported until its safe adapter path exists.

**Acceptance:** An administrator can prohibit native queries or all procedures with scoped exceptions represented consistently; caller labels cannot conceal native/procedure operations.

**Validation:** Execution class/mask fixtures, User_* and sys_* examples, misclassification and unsupported-native tests.

### Task 10: Typed internal principals and trusted propagation

**Id:** acl-10
**Depends-On:** 2, 5
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; datatug/datatug-cli

Implement realm/kind/id principal references, stable internal user IDs with provider bindings, current roles/groups, and service identities. Reuse existing auth; separate actor capability from subject authorization.

**Acceptance:** Services never accidentally match human user bindings; clients cannot inject roles or trusted policy parameters; identity and memberships propagate consistently.

**Validation:** Human/service collision, realm mismatch, membership revocation, multiple provider binding and token-capability intersection tests.

### Task 11: Owner policy generations, activation and reload

**Id:** acl-11
**Depends-On:** 3, 5, 7, 10
**Status:** queued

**Repositories:** ingitdb/dalgo2ingitdb; openvaultdb/openvaultdb-go

Evolve the initial file snapshots into the approved owner generation/revision contract, immutable snapshots and atomic activation/reload. Separate policy ETags from internal generation revisions; retain crash/recovery ordering.

**Acceptance:** Invalid activation retains a valid old snapshot or fails closed; missing enabled generation never becomes legacy mode; reconnect observes persisted owner state.

**Validation:** CAS conflicts, invalid generation, remount, interrupted activation and disabled/enabled transition tests.

### Task 12: DTQL structured decision and blocker types

**Id:** acl-12
**Depends-On:** 1, 2
**Status:** queued

**Repositories:** datatug/dtql; dal-go/dalgo

Implement the reviewed C2 wire contract and equivalent reusable internal decisions: operation/resources, participating sources, policy references, blockers, restrictions and completeness. Collect safe independent blockers instead of only first failure.

**Acceptance:** Types and encoded responses match frozen schema fixtures; provenance survives composition; no presentation-specific English text is required.

**Validation:** Schema fixtures, deterministic composition, multiple blockers, evaluation failure and incomplete decision serialization tests.

### Task 13: Diagnostic disclosure and private-policy protection

**Id:** acl-13
**Depends-On:** 10, 12
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; ingitdb/dalgo2ingitdb

Enforce public-default/private-admin-only policy visibility, safe ordinary diagnostics, Explain privileges and enhanced internal evidence grants. Never leak hidden row values or predicate literals through errors.

**Acceptance:** Disclosure is separately authorized at every owner; bounded/opaque restrictions replace oversized or private predicates; nonexistent and protected targets do not become existence oracles.

**Validation:** Caller/admin/explain/internal disclosure matrix; private predicate, revision, timing and row-existence probes.

### Task 14: Policy discovery and read-only custom provider metadata

**Id:** acl-14
**Depends-On:** 10, 11, 12, 13
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; ingitdb/dalgo2ingitdb

Expose privileged provider/layer discovery and metadata needed by Explain Access; retain identity, ownership, visibility and non-editability for custom policies. Administration remains owner-controlled files for this MVP.

**Acceptance:** All visible mandatory owners and read-only custom policies remain represented; private metadata requires owner authorization; discovery never grants data access.

**Validation:** Multi-owner discovery, inaccessible owner, private policy and custom read-only provider tests. No viewer/editor UI.

### Task 15: Shared plan and explicit-key dry-run machinery

**Id:** acl-15
**Depends-On:** 9, 10, 12, 13
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; ingitdb/dalgo2ingitdb

Implement metadata-only plan and bounded explicit-key inspection with the same evaluators as execution. Separate inspection capability/session from executable mutation capability; return safe conditions without rows when requested.

**Acceptance:** Dry run cannot execute or cause side effects; per-key results distinguish denial, conditional, unavailable and incomplete evidence; hidden evidence requires separate authority.

**Validation:** No-write spies plus real storage snapshots; plan without row reads; missing/hidden key and unavailable source cases.

### Task 16: Bounded top-N dry run and conditional results

**Id:** acl-16
**Depends-On:** 15
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go

Select bounded samples from the permitted candidate intersection with explicit budgets. A sample never proves access to all rows; disclose safe rejection conditions or opaque references when no specific rows are requested.

**Acceptance:** Top-N results identify sampling scope and completeness; protected rows are not enumerated; partial evaluation never appears as unconditional overall allow.

**Validation:** Explicit IDs vs sample; zero records; budget exhaustion; source unavailable; too-large/private conditions.

### Task 17: Safe write assessment and execution coordination

**Id:** acl-17
**Depends-On:** 8, 9, 10, 12, 15
**Status:** queued

**Repositories:** dal-go/dalgo

Complete the reusable write path for insert/update/delete, private pre-images, final post-image and affected-column checks, preserving existing transaction abstractions. Reject unsupported dynamic callbacks before invoking them.

**Acceptance:** Inspection cannot be upgraded into execution; all participating owners assess the actual write inside the protected storage boundary; later execution reauthorizes current state.

**Validation:** Write-only actor, parent/leaf changes, transformation/post-image, callback non-invocation and no partial mutation tests.

### Task 18: InGitDB and SQLite atomic protected UPDATE

**Id:** acl-18
**Depends-On:** 11, 17
**Status:** queued

**Repositories:** ingitdb/dalgo2ingitdb; dal-go/dalgo2sql; openvaultdb/openvaultdb-go

Integrate owner write assessment with adapter transactions and per-row revisions. Prove UPDATE on each real backend while keeping generic insert/delete support compatible.

**Acceptance:** Denied or stale writes leave stored data unchanged; successful writes persist; policy/data changes invalidate stale dry-run expectations.

**Validation:** Real transactional update, conflict, policy change, rollback and reconnect tests for both engines.

### Task 19: OpenVaultDB write API and lower-layer composition

**Id:** acl-19
**Depends-On:** 18
**Status:** queued

**Repositories:** openvaultdb/openvaultdb-go

Propagate principal, operation and revision preconditions into protected writes; preserve lower denials and structured results. Use the approved one-row UPDATE contract as the first real write.

**Acceptance:** Upper policies cannot bypass lower owner denial; response matches actual committed result; actor capabilities remain mandatory.

**Validation:** Authenticated HTTP UPDATE success/denial/conflict, bypassing DataTug, changed columns and post-image restrictions.

### Task 20: Complete safe layered blocker collection

**Id:** acl-20
**Depends-On:** 13, 15, 16, 19
**Status:** queued

**Repositories:** dal-go/dalgo; openvaultdb/openvaultdb-go; ingitdb/dalgo2ingitdb

After an upper-layer denial collect safely evaluable lower blockers through inspection only. Preserve per-owner completeness and unavailable-source facts; no speculative write or rollback-as-dry-run.

**Acceptance:** One response reports independently blocking row and columns from multiple owners when safely available, and explicitly reports incomplete diagnostics otherwise.

**Validation:** Required multi-blocker UPDATE scenario; upper denial plus lower hidden row; permission-limited inspector and unavailable owner cases.

### Task 21: DataTug query/write and Explain Access integration

**Id:** acl-21
**Depends-On:** 14, 16, 19, 20
**Status:** queued

**Repositories:** datatug/datatug-cli; datatug/datatug-apps

Integrate authenticated OpenVaultDB query/write transport and render structured errors and Explain results for principal/operation/paths. Show participating layers and effective access without a policy viewer/editor.

**Acceptance:** DataTug -> OpenVaultDB -> InGitDB and SQLite prove read/write/Explain agreement; custom/read-only policies remain identifiable where authorized.

**Validation:** Client API tests and browser/daemon E2E; private diagnostic redaction; selected IDs/top-N forms; no policy CRUD/viewer/editor UI.

### Task 22: Full acceptance, security and operational documentation

**Id:** acl-22
**Depends-On:** 6, 9, 11, 20, 21
**Status:** queued

**Repositories:** all participating repos

Run the reviewed full acceptance matrix with masks, principal kinds, independent owner enforcement, persistence, real query and UPDATE, dry-run consistency and complete/incomplete blockers. Document bootstrap, operation limits and unsupported features.

**Acceptance:** Every MVP acceptance scenario has evidence; no long-running transaction protocol or external ACL service is implemented.

**Validation:** Integration/security fixtures, direct lower access, restart/activation failures, SQL injection, replay and resource-budget tests.

### Task 23: Independent implementation security review and resolution

**Id:** acl-23
**Depends-On:** 22
**Status:** queued

**Repositories:** all participating repos

Review the final implementation against the approved packet using independent reviewers where available, reconcile actionable findings and record fixes. Use cheaper models for bounded checks; frontier review for enforcement contracts.

**Acceptance:** Critical/high correctness or security findings are resolved with tests; unsupported reviewer/model availability is disclosed rather than fabricated.

**Validation:** Review reports tied to exact local commits and post-fix regression evidence.

### Task 24: Coordinated publication and dependency integration

**Id:** acl-24
**Depends-On:** 23
**Status:** queued

**Repositories:** all participating repos

Keep local Go workspace/import links during development. After acceptance, perform one coordinated provider-first publication/version update and consumer verification wave, minimizing pushes/merges.

**Acceptance:** Normal unlinked builds/CI consume the intended versions and pass; local rewrites are removed only after equivalent published dependencies exist.

**Validation:** Provider release receipts, consumer CI, unlinked build and final E2E. Local implementation completion does not claim publication.

## Open Questions

No blocking questions for the authorized implementation. Resolve routine details
against the approved contracts; record any genuine contract conflict before dependent
work proceeds. Publication timing and any expansion into native execution or policy
administration UI remain outside this implementation authorization's immediate scope.

---
*This document follows the https://specscore.md/plan-specification*
