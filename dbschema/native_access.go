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
// authorization catalog simply does not implement it. Callers resolve it with
// dal.As[NativeAccessReader](db) and MUST read its absence, or a
// *NotSupportedError from it, as "native access unknown", never as "nobody".
//
// An implementation MUST NOT report "nobody" when it cannot know. It either
// returns an error, or a *NativeAccess whose Coverage names every input it
// could not read and every kind of access its catalog cannot show. A
// collection that does not exist is an error, never an empty answer.
//
// The answer describes the catalog at NativeAccess.AsOf. It is an inventory,
// not an authorization decision: it never grants, widens or narrows DALgo
// access.
type NativeAccessReader interface {
	ReadNativeAccess(ctx context.Context, ref *dal.CollectionRef) (*NativeAccess, error)
}

// NativeAccess is the native access to one collection as the engine's catalog
// shows it.
type NativeAccess struct {
	// Collection is the collection the answer is about.
	Collection dal.CollectionRef `json:"collection"`
	// AsOf is the time the engine's catalog was read, on the engine's clock.
	AsOf time.Time `json:"asOf"`
	// Principals are the principals that can reach the collection, in a
	// deterministic order. The collection's owner is always listed.
	Principals []NativePrincipal `json:"principals"`
	// RowSecurity states whether the engine filters the collection's rows per
	// principal.
	RowSecurity NativeRowSecurity `json:"rowSecurity"`
	// Coverage states what the answer covers and what it cannot see.
	Coverage NativeAccessCoverage `json:"coverage"`
}

// NativePrincipalKind classifies a principal.
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
	Name string              `json:"name"`
	Kind NativePrincipalKind `json:"kind"`
	// CanLogin is true when the principal can open a session itself.
	CanLogin bool `json:"canLogin"`
	// Superuser is true when the principal bypasses every privilege check.
	Superuser bool `json:"superuser,omitempty"`
	// Owner is true when the principal owns the collection.
	Owner bool `json:"owner,omitempty"`
	// Privileges are the privileges the principal can exercise, each with why.
	Privileges []NativePrivilege `json:"privileges"`
	// RowSecurity states whether row security filters this principal's rows.
	RowSecurity NativeRowSecurityEffect `json:"rowSecurity"`
	// NamespaceAccess states whether the principal can reach the namespace
	// (schema) holding the collection, without which its privileges cannot be
	// used.
	NamespaceAccess NativeNamespaceAccess `json:"namespaceAccess"`
	// Confidence is the weakest confidence of the principal's privileges.
	Confidence NativeConfidence `json:"confidence"`
}

// NativeOperation is a data operation a native privilege allows.
type NativeOperation string

const (
	NativeRead     NativeOperation = "read"
	NativeInsert   NativeOperation = "insert"
	NativeUpdate   NativeOperation = "update"
	NativeDelete   NativeOperation = "delete"
	NativeTruncate NativeOperation = "truncate"
)

// NativePrivilege is one operation a principal can exercise.
type NativePrivilege struct {
	Operation NativeOperation `json:"operation"`
	// Fields is empty for the whole collection; otherwise the privilege covers
	// only these fields (columns).
	Fields []string `json:"fields,omitempty"`
	// Confidence is the strongest confidence among Grants.
	Confidence NativeConfidence `json:"confidence"`
	// Grants are every reason the principal holds the privilege.
	Grants []NativeGrant `json:"grants"`
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
	// NativeRouteSelf: the principal's own privilege.
	NativeRouteSelf NativeRoute = "self"
	// NativeRouteInherited: used automatically through membership.
	NativeRouteInherited NativeRoute = "inherited"
	// NativeRouteSwitchRole: usable after the principal switches its session
	// to another role it is a member of.
	NativeRouteSwitchRole NativeRoute = "switch-role"
	// NativeRouteSelfGrant: usable after the principal grants itself
	// membership it administers. A potential, not a current, privilege.
	NativeRouteSelfGrant NativeRoute = "self-grant"
)

// NativeGrant is one reason a principal holds a privilege.
type NativeGrant struct {
	Source NativeGrantSource `json:"source"`
	Route  NativeRoute       `json:"route"`
	// Via is the chain of role names from the principal (excluded) to the role
	// holding the privilege (included). Empty for the principal's own.
	Via        []string         `json:"via,omitempty"`
	Confidence NativeConfidence `json:"confidence"`
}

// NativeConfidence states how a privilege is known.
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
	Enabled bool `json:"enabled"`
	// AppliesToOwner is true when the owner is filtered too.
	AppliesToOwner bool `json:"appliesToOwner,omitempty"`
}

// NativeRowSecurityEffect is how row security affects one principal.
type NativeRowSecurityEffect string

const (
	NativeRowSecurityOff      NativeRowSecurityEffect = "off"
	NativeRowSecurityApplies  NativeRowSecurityEffect = "applies"
	NativeRowSecurityBypassed NativeRowSecurityEffect = "bypassed"
)

// NativeNamespaceAccess is whether a principal can reach the collection's
// namespace.
type NativeNamespaceAccess string

const (
	NativeNamespaceGranted NativeNamespaceAccess = "granted"
	NativeNamespaceMissing NativeNamespaceAccess = "missing"
	NativeNamespaceUnknown NativeNamespaceAccess = "unknown"
)

// NativeCoverageLevel is how complete a native answer is.
type NativeCoverageLevel string

const (
	// NativeCoveragePartial: the catalog was read; access it cannot show is
	// named in BlindSpots. A native answer is never complete.
	NativeCoveragePartial NativeCoverageLevel = "partial"
	// NativeCoverageUnknown: an input the answer depends on could not be read.
	NativeCoverageUnknown NativeCoverageLevel = "unknown"
)

// Blind spots shared by engines. Adapters may add their own codes.
const (
	BlindSpotConnectionRules  = "connection-rules"
	BlindSpotExternalIdentity = "external-identity-logins"
	BlindSpotReplicasBackups  = "replicas-and-backups"
	BlindSpotSharedCredential = "shared-credentials"
	BlindSpotDefinerCode      = "definer-rights-code"
	BlindSpotRowPredicates    = "row-security-predicates"
)

// NativeAccessCoverage states what an answer covers and what it cannot see.
type NativeAccessCoverage struct {
	Level NativeCoverageLevel `json:"level"`
	// Covered names the catalog facts the answer read.
	Covered []string `json:"covered"`
	// BlindSpots names access the engine's catalog cannot show at all.
	BlindSpots []string `json:"blindSpots"`
	// Gaps are inputs that could not be read or did not resolve; each makes
	// some privileges unknown.
	Gaps []NativeAccessGap `json:"gaps,omitempty"`
}

// NativeAccessGap is one input that could not be read or resolved.
type NativeAccessGap struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}
