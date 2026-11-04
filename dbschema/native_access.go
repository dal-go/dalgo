package dbschema

import (
	"context"
	"time"

	"github.com/dal-go/dalgo/dal"
)

// NativeAccessReader is an optional schema-introspection capability. It
// answers "which database principals can reach this collection natively, with
// which privileges, and why": access through the engine's own accounts and
// roles, outside any DALgo access policy. It is separate from SchemaReader so
// existing drivers need no API changes; a driver that cannot read its engine's
// authorization catalog, or whose engine has no principals (SQLite, for
// example), simply does not implement it. Callers resolve it with
// dal.As[NativeAccessReader](db) and MUST read its absence, or a
// *NotSupportedError from it, as "native access unknown", never as "nobody".
//
// An implementation MUST NOT report "nobody" when it cannot know. It either
// returns an error, or a *NativeAccess whose Coverage names every input it
// could not read and every kind of access its catalog cannot show. An answer
// with no principal MUST have Coverage.Level NativeCoverageUnknown. A
// collection that does not exist is an error, never an empty answer.
//
// The answer describes the catalog at NativeAccess.AsOf. It is an inventory,
// not an authorization decision: it never grants, widens or narrows DALgo
// access.
//
// The Native* types carry no serialization tags: a consumer that sends an
// answer over the wire maps it to its own format.
type NativeAccessReader interface {
	ReadNativeAccess(ctx context.Context, ref *dal.CollectionRef) (*NativeAccess, error)
}

// NativeAccess is the native access to one collection as the engine's catalog
// shows it.
type NativeAccess struct {
	// Collection is the collection the answer is about.
	Collection dal.CollectionRef
	// AsOf is the time the engine's catalog was read, on the engine's clock.
	AsOf time.Time
	// Principals are the principals that can reach the collection, in a
	// deterministic order. When the engine records an owner of the
	// collection, the owner is always listed.
	Principals []NativePrincipal
	// RowSecurity states whether the engine filters the collection's rows per
	// principal.
	RowSecurity NativeRowSecurity
	// Coverage states what the answer covers and what it cannot see.
	Coverage NativeAccessCoverage
}

// NativePrincipalKind classifies a principal. The kind also says whether the
// principal can open a session itself: only NativeLogin can.
type NativePrincipalKind string

const (
	// NativeLogin is an account or role that can open a session itself.
	NativeLogin NativePrincipalKind = "login"
	// NativeGroup is a role that cannot open a session; it is reached only
	// through membership.
	NativeGroup NativePrincipalKind = "group"
	// NativeEveryone is the pseudo-principal every account belongs to,
	// including accounts created later.
	NativeEveryone NativePrincipalKind = "everyone"
)

// NativePrincipal is one principal and the privileges it can exercise on the
// collection.
type NativePrincipal struct {
	// Name is the engine's name of the principal. Mapping it to a person is the
	// caller's business.
	Name string
	// Kind classifies the principal, including whether it can log in.
	Kind NativePrincipalKind
	// Superuser is true when the principal bypasses every privilege check.
	Superuser bool
	// Owner is true when the principal owns the collection.
	Owner bool
	// Privileges are the privileges the principal can exercise, each with why.
	Privileges []NativePrivilege
	// RowSecurity states whether row security filters this principal's rows.
	RowSecurity NativeRowSecurityEffect
	// NamespaceAccess states whether the principal can reach the namespace
	// (schema) holding the collection, without which its privileges cannot be
	// used.
	NamespaceAccess NativeNamespaceAccess
	// Confidence is the weakest confidence of the principal's privileges.
	Confidence NativeConfidence
}

// NativeOperation is a data operation a native privilege allows.
type NativeOperation string

const (
	// NativeRead reads rows (SELECT).
	NativeRead NativeOperation = "read"
	// NativeInsert adds rows.
	NativeInsert NativeOperation = "insert"
	// NativeUpdate changes rows.
	NativeUpdate NativeOperation = "update"
	// NativeDelete removes rows.
	NativeDelete NativeOperation = "delete"
	// NativeTruncate removes every row at once.
	NativeTruncate NativeOperation = "truncate"
)

// NativePrivilege is one operation a principal can exercise.
type NativePrivilege struct {
	Operation NativeOperation
	// Fields is empty for the whole collection; otherwise the privilege covers
	// only these fields (columns).
	Fields []string
	// Confidence is the strongest confidence among Grants.
	Confidence NativeConfidence
	// Grants are every reason the principal holds the privilege.
	Grants []NativeGrant
}

// NativeGrantSource is the kind of catalog fact a privilege comes from.
type NativeGrantSource string

const (
	// NativeFromDirect is a privilege granted to the principal itself.
	NativeFromDirect NativeGrantSource = "direct"
	// NativeFromMembership is a privilege granted to a role the principal is a
	// member of, possibly through a chain (see NativeGrant.Via).
	NativeFromMembership NativeGrantSource = "membership"
	// NativeFromEveryone is a privilege granted to every principal.
	NativeFromEveryone NativeGrantSource = "everyone"
	// NativeFromOwnership is a privilege the collection's owner holds.
	NativeFromOwnership NativeGrantSource = "ownership"
	// NativeFromSuperuser is a privilege a superuser holds on everything.
	NativeFromSuperuser NativeGrantSource = "superuser"
	// NativeFromBuiltinRole is a privilege an engine-defined role holds on
	// every collection.
	NativeFromBuiltinRole NativeGrantSource = "builtin-role"
)

// NativeRoute is how a principal exercises a privilege held by another role.
type NativeRoute string

const (
	// NativeRouteSelf is the principal's own privilege.
	NativeRouteSelf NativeRoute = "self"
	// NativeRouteInherited is used automatically through membership.
	NativeRouteInherited NativeRoute = "inherited"
	// NativeRouteSwitchRole is usable after the principal switches its session
	// to another role it is a member of.
	NativeRouteSwitchRole NativeRoute = "switch-role"
	// NativeRouteSelfGrant is usable after the principal grants itself
	// membership it administers. A potential, not a current, privilege.
	NativeRouteSelfGrant NativeRoute = "self-grant"
)

// NativeGrant is one reason a principal holds a privilege.
type NativeGrant struct {
	Source NativeGrantSource
	Route  NativeRoute
	// Via is the chain of role names from the principal (excluded) to the role
	// holding the privilege (included). Empty for the principal's own.
	Via        []string
	Confidence NativeConfidence
}

// NativeConfidence states how a privilege is known. From strongest to
// weakest: NativeVerified, NativeInferred, NativeUnknown.
type NativeConfidence string

const (
	// NativeVerified is read directly from an explicit catalog entry.
	NativeVerified NativeConfidence = "verified"
	// NativeInferred is derived by the adapter, for example through a
	// membership chain.
	NativeInferred NativeConfidence = "inferred"
	// NativeUnknown means an input needed to decide was missing.
	NativeUnknown NativeConfidence = "unknown"
)

// NativeRowSecurity is the collection's row-security state.
type NativeRowSecurity struct {
	// Enabled is true when rows are filtered per principal.
	Enabled bool
	// AppliesToOwner is true when the owner is filtered too; false where the
	// engine makes no such distinction.
	AppliesToOwner bool
}

// NativeRowSecurityEffect is how row security affects one principal.
type NativeRowSecurityEffect string

const (
	// NativeRowSecurityOff means the collection has no row security.
	NativeRowSecurityOff NativeRowSecurityEffect = "off"
	// NativeRowSecurityApplies means the principal sees only the rows the
	// engine's row-security rules let it see.
	NativeRowSecurityApplies NativeRowSecurityEffect = "applies"
	// NativeRowSecurityBypassed means the principal sees every row.
	NativeRowSecurityBypassed NativeRowSecurityEffect = "bypassed"
)

// NativeNamespaceAccess is whether a principal can reach the collection's
// namespace.
type NativeNamespaceAccess string

const (
	// NativeNamespaceGranted means the principal can reach the namespace.
	NativeNamespaceGranted NativeNamespaceAccess = "granted"
	// NativeNamespaceMissing means the principal holds privileges on the
	// collection but cannot use them, because it cannot reach the namespace.
	NativeNamespaceMissing NativeNamespaceAccess = "missing"
	// NativeNamespaceUnknown means the namespace's access could not be read.
	NativeNamespaceUnknown NativeNamespaceAccess = "unknown"
	// NativeNamespaceNotApplicable means the engine has no separate gate on
	// reaching a namespace (a namespace grant, where one exists, is reported
	// as a grant instead).
	NativeNamespaceNotApplicable NativeNamespaceAccess = "not-applicable"
)

// NativeCoverageLevel is how complete a native answer is.
type NativeCoverageLevel string

const (
	// NativeCoveragePartial means the catalog was read; access it cannot show
	// is named in BlindSpots. A native answer is never complete.
	NativeCoveragePartial NativeCoverageLevel = "partial"
	// NativeCoverageUnknown means an input the answer depends on could not be
	// read or resolved.
	NativeCoverageUnknown NativeCoverageLevel = "unknown"
)

// Blind spots shared by engines. Adapters may add their own codes.
const (
	// BlindSpotConnectionRules is the engine's rules on who may connect from
	// where and how (for example pg_hba.conf).
	BlindSpotConnectionRules = "connection-rules"
	// BlindSpotExternalIdentity is logins mapped from an external identity
	// provider, such as cloud IAM.
	BlindSpotExternalIdentity = "external-identity-logins"
	// BlindSpotReplicasBackups is copies of the data outside this database.
	BlindSpotReplicasBackups = "replicas-and-backups"
	// BlindSpotSharedCredential is people sharing one principal's credential.
	BlindSpotSharedCredential = "shared-credentials"
	// BlindSpotDefinerCode is code that runs with its owner's rights, such as
	// definer-rights functions and views owned by other principals.
	BlindSpotDefinerCode = "definer-rights-code"
	// BlindSpotRowPredicates is the row-security rules themselves, which the
	// answer does not read.
	BlindSpotRowPredicates = "row-security-predicates"
)

// NativeAccessCoverage states what an answer covers and what it cannot see.
type NativeAccessCoverage struct {
	Level NativeCoverageLevel
	// Covered names the catalog facts the answer read.
	Covered []string
	// BlindSpots names access the engine's catalog cannot show at all.
	BlindSpots []string
	// Gaps are inputs that could not be read or did not resolve; each makes
	// some privileges unknown.
	Gaps []NativeAccessGap
}

// NativeAccessGap is one input that could not be read or resolved.
type NativeAccessGap struct {
	Code   string
	Detail string
}
