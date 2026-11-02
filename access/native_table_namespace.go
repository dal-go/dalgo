package access

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type nativeTableNamespaceKey struct{}

// WithNativeTableNamespace adds canonical collection-path constraints to the
// original authorization of qualified tables in namespace. It never replaces
// the original assessment or grants permission. The trusted host must verify
// the exact namespace and ordinary table names on the actual native source
// before data dispatch; query/client input must not configure this option.
//
// One immutable policy snapshot per authorized request is assessed against both
// resource forms. Each affected policy is assessed twice; AssessmentRestriction's
// PolicyIndex indexes Assessment.Policies, which includes canonical delegates.
// A canonical delegate sees a path-kind resource carrying the original table
// identity, so an explicit collection-path denial also constrains a table rule.
// BindDB and transaction handles retain the namespace when later operation
// contexts omit it, and refuse conflicting namespaces. Bind at the outermost
// secured handle or on the operation context: binding only an inner handle
// cannot constrain policies already evaluated by an outer handle.
// Every affected policy must declare InspectionPure. All original query,
// principal, execution and column information is retained, and all obligations
// from both assessments are enforced by the existing secured-read machinery.
// The canonical delegate is a complete path-kind assessment, not a deny-only
// probe. A missing path match denies. Matching table rules retain their usual
// precedence unless a collection-path denial settles the path; a path allow's
// row/field clauses are not additional constraints when a table rule wins.
// All obligations actually emitted by the two assessments are intersected.
// Other namespaces and opaque/unknown shapes retain their original policy
// semantics and must be refused by the host when native identity is unverified.
// Without this option all existing policy semantics remain unchanged.
// A nonnil context is required. An existing binding cannot be replaced.
func WithNativeTableNamespace(ctx context.Context, namespace string) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("access: native table namespace context required")
	}
	if namespace == "" || strings.TrimSpace(namespace) != namespace || !utf8.ValidString(namespace) || strings.ContainsRune(namespace, 0) {
		return nil, fmt.Errorf("access: native table namespace is invalid")
	}
	if prior, ok := ctx.Value(nativeTableNamespaceKey{}).(string); ok {
		if prior != namespace {
			return nil, fmt.Errorf("access: native table namespace is already bound")
		}
		return ctx, nil
	}
	return context.WithValue(ctx, nativeTableNamespaceKey{}, namespace), nil
}

// nativeTablePolicies only augments the already-pinned policy slice. It does
// not resolve providers, walk queries, evaluate permissions or mutate requests.
func nativeTablePolicies(ctx context.Context, request Request, policies []Policy) ([]Policy, bool) {
	if ctx == nil {
		return policies, false
	}
	namespace, ok := ctx.Value(nativeTableNamespaceKey{}).(string)
	if !ok || request.Operation != Query {
		return policies, false
	}
	var resources []Resource
	for i, resource := range request.Resources {
		identity, named := resource.Table()
		if !named || identity.Database != "" || identity.Schema != namespace || identity.Name == "" {
			continue
		}
		if resources == nil {
			resources = append([]Resource(nil), request.Resources...)
		}
		resources[i] = CollectionResourceFor(nil, identity.Name)
		resources[i].table = &identity
	}
	if resources == nil {
		return policies, false
	}
	// Keep the original decisions, including no-match/tableNoRule/opaque
	// denials. A path fallback can only add restrictions, never override them.
	augmented := append([]Policy(nil), policies...)
	for _, policy := range policies {
		if !isNilNode(policy) {
			augmented = append(augmented, nativeTablePolicy{Policy: policy, resources: resources})
		}
	}
	return augmented, true
}

type nativeTablePolicy struct {
	Policy
	resources []Resource
}

func (p nativeTablePolicy) mapped(request Request) Request {
	request.Resources = p.resources
	return request
}
func (p nativeTablePolicy) Decide(ctx context.Context, request Request) Decision {
	return p.Policy.Decide(ctx, p.mapped(request))
}
func (p nativeTablePolicy) Authorize(ctx context.Context, request Request) error {
	return p.Policy.Authorize(ctx, p.mapped(request))
}
func (p nativeTablePolicy) PolicyMetadata() PolicyMetadata { return DescribePolicy(p.Policy) }
func (p nativeTablePolicy) InspectionPure() bool           { return CanInspectPolicy(p.Policy) }
