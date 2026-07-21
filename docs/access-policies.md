# Access Policies

DALgo access policies turn a `dal.DB` or session into a capability: code can
perform only the operations explicitly allowed for the logical DAL paths it
touches. Enforcement happens in the DAL wrapper before an adapter is called,
so the same boundary works with Firestore, SQL, files, Git, and
`dalgo2memory` tests.

This is especially useful for extension systems, tenant-scoped services,
background jobs, ingestion endpoints, analytics, and technical-support tools.

## The model at a glance

- Every access policy is default-deny.
- `Get`, `Exists`, and `Query` are separate read capabilities.
- `Insert`, `Set`, `Update`, `Delete`, and reserved `Truncate` are separate
  write capabilities. Write does not imply read.
- A path rule applies to its descendants. Within one policy, the most-specific
  rule wins; deny wins an equally specific tie.
- Multiple policies intersect. Every applicable database, bound-context, and
  operation-context policy must allow every target resource.
- Batches and joined queries are preflighted in full before execution.
- Collection-group and opaque queries require explicit rules.

These rules support both useful hierarchical shapes:

```text
allow /spaces/*/**          deny /spaces/*/**
  deny /private/**            allow /ext/trackus/**
```

A child can narrow or reopen its parent inside the same policy. A separate
policy cannot reopen another policy's denial.

## An extension capability

The following policy lets the `trackus` extension use only its own global and
per-space subtrees. It cannot write root collections or another extension's
data.

```go
extension := access.MustPolicy("extension-trackus",
	access.Root(access.Deny(access.ReadWrite, "outside-extension")),
	access.Scope("ext", "trackus",
		access.Allow(access.ReadWrite, "own-global-data")),
	access.Scope("spaces", access.AnyID,
		access.Scope("ext", "trackus",
			access.Allow(access.ReadWrite, "own-space-data"))),
)

secured := access.MustSecureDB(rawDB, access.RequireContextPolicy())
ctx := access.WithPolicy(context.Background(), extension)

err := secured.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
	key := record.NewKeyWithID("users", "u1") // outside the capability
	return tx.Set(ctx, record.NewRecordWithData(key, user))
})
// err matches access.ErrAccessDenied; the adapter did not receive Set.
```

`access.BindDB(secured, ctx)` captures the current context policies in a new DB
handle. Later replacing the operation context with `context.Background()`
cannot discard those restrictions. Additional context policies still narrow
the bound handle.

Use `access.WithDatabasePolicies(...)` for application-wide boundaries. A
database policy is also default-deny, so it should positively allow the
application's intended surface and carve out protected subtrees. Context
policies then reduce that surface for a tenant, extension, or request.

## Purpose-specific operations

An append-only producer needs no read capability:

```go
logWriter := access.MustPolicy("delivery-log-writer",
	access.Collection("deliveryLogs",
		access.Allow(access.Insert, "append-delivery-log")),
)
```

A support tool can inspect a known secret without enumerating the collection:

```go
knownSecretReader := access.MustPolicy("known-secret-reader",
	access.Collection("secrets",
		access.Allow(access.Get, "get-known-secret"),
		access.Deny(access.Query, "no-secret-enumeration")),
)
```

`Truncate` is already part of the policy vocabulary even though DALgo has no
truncate session method yet. Existing policies therefore have an explicit,
fail-closed meaning if that operation is added later.

## Portable YAML and JSON

YAML is the canonical human-authored form. JSON has the same document model
and decisions. Nested scopes append structural path fragments; `*` matches a
record ID and `/**` documents inherited subtree intent.

```yaml
apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: extension-trackus
default: deny
scopes:
  - path: /
    rules:
      - id: outside-extension
        effect: deny
        operations: [readwrite]
    scopes:
      - path: /ext/trackus/**
        rules:
          - id: own-global-data
            effect: allow
            operations: [readwrite]
      - path: /spaces/*/ext/trackus/**
        rules:
          - id: own-space-data
            effect: allow
            operations: [readwrite]
```

Load from any storage by supplying an `io.Reader`:

```go
policy, err := access.DecodeAccessPolicy(
	objectBody,
	access.YAMLCodec{},
	access.WithSource("gs://app-config/access/extension-trackus.yaml"),
)
```

Byte helpers include `UnmarshalAccessPolicyYAML`,
`UnmarshalAccessPolicyJSON`, `MarshalAccessPolicyYAML`, and
`MarshalAccessPolicyJSON`; audit policies have matching helpers. Stream APIs
are `DecodeAccessPolicy`, `DecodeAuditPolicy`, `EncodeAccessPolicy`, and
`EncodeAuditPolicy`.

Decoders reject unknown fields, versions, operations, effects, malformed path
shapes, multiple documents, and access/audit effect mixing. The `access.Codec`
interface allows another package to supply HCL or a storage-specific syntax
without changing evaluation. HCL is not built in because its expression and
evaluation semantics would make a deliberately declarative policy harder to
read and audit.

## Denials that explain themselves

All policy denials match `access.ErrAccessDenied`. Use `errors.As` when logs,
tests, developer tools, or an administrative UI need the decision trace:

```go
if err != nil {
	var denied *access.DeniedError
	if errors.As(err, &denied) {
		d := denied.Decision
		logger.Warn("DAL operation denied",
			"operation", d.Operation,
			"resource", d.Resource.String(),
			"policy", d.Policy,
			"policy_source", d.PolicySource,
			"rule", d.Rule,
			"explanation", d.Explanation,
		)
	}
}
```

A loaded policy can produce an error such as:

```text
dalgo access denied: policy="extension-trackus"
source="gs://app-config/access/extension-trackus.yaml"
rule="outside-extension" operation=set resource=/users/u1:
matched rule "outside-extension" (deny)
```

Rule IDs are required to be unique within a policy. Policy and rule IDs should
be stable operational identifiers. The optional
source is storage-neutral: it can be a file path, URL, object key, database
key, Git reference, or another locator meaningful to the application.

Detailed denial traces may reveal collection names or policy structure. Log
them to trusted telemetry and show them in authenticated developer/admin
tools. For an untrusted API client, return a generic forbidden response and a
correlation ID rather than the full `DeniedError` string.

Policies also expose side-effect-free `Decide`, and audit policies expose
`Classify`, making explanations easy to test without a database.

## Audit selection uses the same hierarchy

Audit policy effects are `audit` and `ignore-audit`; they do not allow or deny
data access and they do not write an audit record. They only classify whether
an application should emit an event.

```go
audit := access.MustAuditPolicy("sensitive-mutations",
	access.Collection("users", access.Audit(access.Write, "user-mutations")),
	access.Collection("transactions", access.Audit(access.Write, "transaction-mutations")),
	access.Collection("auditLog", access.IgnoreAudit(access.Write, "avoid-recursion")),
)
```

Persistence, delivery guarantees, and redaction remain the application's
responsibility.

## Query boundaries and future constraints

The current boundary authorizes the base collection and every joined source.
A filter cannot make an otherwise forbidden collection safe. Collection-group
queries use `access.CollectionGroupScope`; non-structured queries use the
deliberately broad `access.OpaqueQueryScope`.

Custom SQL text is always opaque. DALgo does not inspect or attempt to infer
tables from the SQL string, so ordinary path/collection rules can never
authorize it accidentally. Forbid it explicitly in a broad platform policy:

```go
access.OpaqueQueryScope(
	access.Deny(access.Query, "no-custom-sql"),
)
```

Conversely, a trusted database console must receive an explicit
`OpaqueQueryScope(Allow(Query, ...))` capability. Use database-native accounts,
read-only connections, and query limits as defense in depth for such tools.

The policy `Request` retains the original `dal.Query`, which is the extension
seam for a trusted SQL/DTQL analyzer. A future analyzer can turn supported SQL
into a canonical query shape and authorize every table, join, subquery, CTE,
predicate, and projection. This must be proof-based rather than best-effort:
unsupported syntax, a dialect mismatch, dynamic identifiers, stored
procedures, or incomplete analysis must fall back to the opaque-query decision.
An analyzed query should also retain its text-query provenance so a platform
policy can still forbid all custom SQL regardless of the sources it appears to
touch.

Keeping `Query` distinct from `Get` leaves a clean path for future query
conditions such as allowed filter fields, mandatory tenant predicates,
projections, indexes, row limits, and cost budgets. Those constraints are not
part of the first policy version.

## Security boundary

Access policies protect operations routed through the secured DALgo handle.
They are not a sandbox for hostile Go code: code that retains the raw adapter,
opens another database connection, or accesses the network or filesystem can
bypass the wrapper. Pass only secured handles to restricted components and use
database IAM, process isolation, or database-native row/security rules as
defense in depth where the threat model requires them.
