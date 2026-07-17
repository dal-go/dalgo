// Package access provides adapter-independent capability policies for DALgo.
//
// Access policies are default-deny and distinguish point reads, existence
// checks, queries, individual mutation kinds, and the reserved truncate
// operation. Rules inherit through structural DAL paths; the most-specific
// rule wins within one policy, while multiple policies compose by intersection.
//
// Secured sessions and databases enforce policies before delegating to an
// adapter. Policies may be attached globally, carried by context.Context, or
// captured in a bound database handle. YAML and JSON codecs load the same
// versioned policy model from any io.Reader.
//
// A denial returns a DeniedError matching ErrAccessDenied. The decision
// includes the operation, resource, policy name and source, winning rule, and
// explanation for trusted logs, tests, and administrative tooling.
package access
