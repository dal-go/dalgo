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
- A secured handle with no policy in force denies every request: a session built
  with no policy, with a nil policy or with a list that holds a nil one, and a
  database with no database, bound or context policy.
- `Get`, `Exists`, and `Query` are separate read capabilities.
- `Insert`, `Set`, `Update`, `Delete`, and reserved `Truncate` are separate
  write capabilities. Write does not imply read.
- A path rule applies to its descendants. Within one policy, the most-specific
  rule wins; deny wins an equally specific tie.
- Multiple policies intersect. Every applicable database, bound-context, and
  operation-context policy must allow every target resource.
- Batches and joined queries are preflighted in full before execution.
- Collection-group and opaque queries require explicit rules.
- A table in a schema or a database is named with a table rule
  (`access.TableScope`); see [Tables in schemas and databases](#tables-in-schemas-and-databases).

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

## Tables in schemas and databases

A source that writes a schema (`dal.NewQualifiedRootCollectionRef`) or a database
(`dal.NewDatabaseCollectionRef`) can have rules of its own, written with a table
rule. A table rule names the table by its schema, its database, or both, and by
its name. In Go:

```go
policy := access.MustPolicy("sales-readers",
	access.Collection("Customer",
		access.Allow(access.Query, "customers-basic").Fields("CustomerId", "FirstName", "Country")),
	access.TableScope(access.TableName{Schema: "sales", Name: "Customer"},
		access.Allow(access.Query, "sales-customers-gb").
			Fields("id", "company", "country").
			Where(dal.WhereField("country", dal.Equal, "GB"))),
)
```

In a policy document `table` is a fourth selector beside `path`,
`collectionGroup` and `opaqueQuery`; a scope selects exactly one of them:

```yaml
scopes:
  - path: /Customer
    rules:
      - {id: customers-basic, effect: allow, operations: [query], fields: [CustomerId, FirstName, Country]}
  - table: {schema: sales, name: Customer}   # database: is optional
    rules:
      - id: sales-customers-gb
        effect: allow
        operations: [query]
        fields: [id, company, country]
        where: {op: "==", left: {field: country}, right: {value: GB}}
```

A table scope is top-level, or directly under the root scope, and holds rules, not
scopes. The `name` is required and
at least one of `schema` and `database` is required; a table with neither is an
ordinary collection, which a path rule names. A rule under a table scope may carry
`fields`, a field mask, a row condition and a post-image check, as a rule under a
path scope does. A condition that uses `$path.x` is refused, because a table scope
captures nothing. A name is compared as written, with no SQL quoting: a dot is part
of the name, so the schema `a` and the name `b.c` are not the schema `a.b` and the
name `c`. The portable file format of the DTQL policy loader keeps path scopes only.

A rule written for a collection (`/Customer`) keeps its meaning: it governs the
source that writes no schema, and nothing else. It never applies to
`sales.Customer`. How an access policy that holds table rules decides a request:

- A source that writes a schema is decided by the table rules that match it, and
  by them alone. When none does, the request is denied (`ACL_NO_MATCH`). A rule for
  opaque queries, whether it allows or denies, is not consulted for it; it still
  decides custom SQL text and a part of a query that cannot be read.
- A resource that carries the identity of a table with a schema, though its source
  writes none, is decided by the table rules that match it, and by the path rules
  when none does. A deny rule written for the collection's bare name is never
  overridden by a table rule. A deny at the root does not name the collection, so
  a table rule allows beside it. No adapter of this repository says which schema
  an unqualified name is read from, so in this version a source that writes neither
  a schema nor a database carries no identity, and one that writes a database and no
  schema carries an identity with no schema.
- A source that writes no schema, a key and a root collection are decided by the
  path rules as before, with one exception that only refuses: when the policy holds
  a table rule for a table of the same name, compared without regard to case, the
  name could be that table, and the request is denied
  (`ACL_ENFORCEMENT_UNSUPPORTED`) because the source does not say which schema it
  is read from. A key with a parent is decided by the path rules.
- A table rule matches a source when schema and name are equal byte for byte and
  neither the rule nor the source names a database. A database cannot be checked,
  so a rule or a source that names one never matches. A source that writes a schema
  is then denied (`ACL_NO_MATCH`); one that writes none is denied when a table rule
  has its name, and is otherwise decided by its path rules.
- The collection mask is checked first, as it is for every resource, and it denies a
  schema-qualified source.

A policy that holds no table rule is decided exactly as before. Several policies
intersect, so every policy in force must allow every resource: a table in another
schema is readable only when each applied policy allows it, by a table rule in a
policy that holds table rules and by a rule for opaque queries in one that holds
none.

An audit policy takes table scopes too, with audit effects. It refuses nothing, so
a name that a table rule makes ambiguous is classified by its path rules. A source
that writes a schema is classified by the table rule that names it, an
ignore-audit rule included; when no table rule names it, the policy's rule for
opaque queries classifies it, as it did before the policy held a table rule.

## Query boundaries and future constraints

The current boundary authorizes every source a structured query reads before
any of them is read: the base collection, every joined source at any depth,
every source inside a derived source, and every source of a subquery (EXISTS,
NOT EXISTS or scalar) in any clause. A filter cannot make an otherwise
forbidden collection safe. Collection-group queries use
`access.CollectionGroupScope`; non-structured queries use the deliberately
broad `access.OpaqueQueryScope`. A part of a structured query that cannot be
analysed (an unrecognised node, a query that refers to itself, nesting deeper
than 64 levels) is an opaque query and needs the same explicit rule.

Under a field allow-list, an aggregate is held to the list by its operands:
`COUNT(*)` and an aggregate over allowed fields run, and an aggregate over a
hidden field, alone or inside arithmetic, is denied as a column denial. Only
the aggregate functions DALgo defines (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`,
`FIRST`, `LAST`) and the operators `+ - * /` are checked this way; any other
function or operator is refused as unsupported. An aggregate belongs in the
select list, `HAVING` and `ORDER BY`; in `WHERE` or `GROUP BY` it is refused as
unsupported.

A selected field or aggregate comes back under the name the caller gave it. When
the field list allows the column's expression but not that name, the access
layer asks the adapter for the column under an alias of its own, different for
every query, and renames it on the way back; a field the adapter returns under
the caller's name is redacted like any other field. An adapter that ignores the
column projection therefore returns no value for such a column. A column whose
expression the list does not allow is sent under its own name and removed from
the result by that name; this is what bounds a secured session over another
secured session to the fields both lists allow, for the columns the outer
session adds to a query that names none or uses a wildcard.

On the recordset path nothing can be redacted, so the inner session of two
nested secured sessions refuses a query it cannot project onto columns its own
list allows, before anything is read. With an inner list narrower than the outer
one the query is denied; with an outer list narrower than the inner one, or the
same, it is read once.

A grouped query that names no columns selects its group keys, as DALgo defines
it, and is sent with those keys as its columns. A query that aggregates with no
`GROUP BY` and no columns (only a `HAVING`, or an `ORDER BY` over an aggregate)
has no group key to select and is not projected to the allowed fields: the
records reader sends it as it is and redacts the result, and the recordset reader
refuses it.

`HAVING` and `ORDER BY` may name an aggregate of the select list by its alias.
On the records reader, under a field list, that name is accepted when the alias
belongs to one column of the query and that column is an aggregate over allowed
fields. The same name for a plain field, for an aggregate over a hidden field,
for a name two columns come back under, or for nothing is refused. When the
access layer sends the aggregate under an alias of its own, it names that alias
in `HAVING` and `ORDER BY` as well. The recordset reader renames nothing, so
there an alias the field list does not allow is not a name `HAVING` or
`ORDER BY` may use; the aggregate itself is. A plan (`access.AssessPlan`) is
judged by the recordset rule, because it does not know which reader will run the
query: it reports a denial for an alias the field list does not allow, including
the alias of an allowed aggregate that the records reader would run.

A field list applies to the base source of a query, and to every clause that
reads its fields: the select list, `WHERE`, `GROUP BY`, `HAVING`, `ORDER BY`, the
`ON` conditions of every join at any depth, and the scan order of every source.
In a join condition a field belongs to the source its qualifier names. A field
that names the base (by its name or its alias), or that has no qualifier, is held
to the list; a field qualified with a joined source is that source's own and is
not; a field whose qualifier names no source of the query, or names the base and a
joined source alike, is refused. A qualifier is matched to the names and aliases
of the sources of the query without regard to case, because an engine may match
it that way: a qualifier that matches the base and a joined source alike names
neither. A scan orders its own source alone, whatever qualifier a field of the
scan order carries, so every field of the scan order of the base source is held
to the list. In the scan order of a joined source a field is attributed as in a
join condition, with an unqualified field belonging to that joined source. A
joined source carries no field list of its own: a rule that lists fields for one
is refused. A condition nested more than 64 levels deep cannot be checked, and
a secured session refuses it as an unsupported enforcement. A condition that
holds itself by value is not followed to its end by `dal.HasSubquery`, which
reports it as a query with nested queries, so a secured session takes it to the
nested route, where a policy with no rule for opaque queries denies it as a
source it cannot analyse. Both are denied before anything is read.

A query with nested queries is executed one source at a time through the same
secured session, which authorises and narrows each scan. The nesting between the
session and the query engine is bounded: a query with nested queries that is put
to a session from inside more than four such routes is refused with an error that
matches both `access.ErrNestingTooDeep` and `access.ErrAccessDenied`. A scan
order that holds a query cannot be run together with nested queries, and is
refused as an unsupported enforcement (`ACL_ENFORCEMENT_UNSUPPORTED`) before
anything is read, after the sources of the query have been authorised.

What stays of the three published limits.

A source is matched to a path rule by the exact spelling of its collection name.
On an engine that folds identifier case, one table can be named in several
spellings, and a rule written for one spelling does not apply to another. A
policy for such an engine should therefore be an allow-list written with the
names the engine stores, not an allow-all with denials for particular names. The
names of a table rule are compared exactly as well.

A schema-qualified source (`schema.table`) is one opaque resource in a policy that
holds no table rule: only a rule for opaque queries (`access.OpaqueQueryScope`)
can allow it, that rule applies to every opaque resource, custom SQL text
included, and a rule for opaque queries carries no field list and no row
condition. In an access policy that holds table rules a schema-qualified source is decided
by the table rules, which carry a field list and a row condition (see
[Tables in schemas and databases](#tables-in-schemas-and-databases)), and a rule
for opaque queries is not consulted for it.

A source that names a database (`CollectionRef.Database`, used by a federated
executor) and no schema is matched to path rules by its collection name alone: the
database is not part of its resource, so one policy cannot tell the same
collection name in two databases apart, and a rule for a collection applies to it
in every database the wrapped executor reaches. Where an executor routes a query
by the database a source names, write the policy for every database it reaches
as one. A table rule that names a database never matches a source in this version,
because a session cannot say which database it is; a policy that holds a table
rule for the name of such a source denies it.

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
