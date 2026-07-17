package access

import (
	"context"
	"fmt"
)

type contextPoliciesKey struct{}

// WithPolicy returns a child context carrying additional restrictive policies.
// Policies inherited from the parent context are preserved.
func WithPolicy(ctx context.Context, policies ...Policy) context.Context {
	if ctx == nil {
		panic("access: nil context")
	}
	combined := append(policiesFromContext(ctx), policies...)
	for i, policy := range combined {
		if policy == nil {
			panic(fmt.Sprintf("access: nil context policy at index %d", i))
		}
	}
	return context.WithValue(ctx, contextPoliciesKey{}, combined)
}

func policiesFromContext(ctx context.Context) []Policy {
	if ctx == nil {
		return nil
	}
	policies, _ := ctx.Value(contextPoliciesKey{}).([]Policy)
	return append([]Policy(nil), policies...)
}

type guard struct {
	databasePolicies []Policy
	boundPolicies    []Policy
	requireContext   bool
}

func (g guard) bind(ctx context.Context) guard {
	bound := policiesFromContext(ctx)
	g.boundPolicies = append(append([]Policy(nil), g.boundPolicies...), bound...)
	return g
}

func (g guard) authorize(ctx context.Context, operation Operations, resources ...Resource) error {
	return g.authorizeRequest(ctx, Request{Operation: operation, Resources: resources})
}

func (g guard) authorizeRequest(ctx context.Context, request Request) error {
	dynamicPolicies := policiesFromContext(ctx)
	contextPolicyCount := len(g.boundPolicies) + len(dynamicPolicies)
	if err := g.requireContextPolicy(request.Operation, contextPolicyCount); err != nil {
		return err
	}
	policies := make([]Policy, 0, len(g.databasePolicies)+contextPolicyCount)
	policies = append(policies, g.databasePolicies...)
	policies = append(policies, g.boundPolicies...)
	policies = append(policies, dynamicPolicies...)
	for _, policy := range policies {
		if err := policy.Authorize(ctx, request); err != nil {
			return err
		}
	}
	return nil
}

func (g guard) checkContext(ctx context.Context) error {
	return g.requireContextPolicy(0, len(g.boundPolicies)+len(policiesFromContext(ctx)))
}

func (g guard) requireContextPolicy(operation Operations, contextPolicyCount int) error {
	if !g.requireContext || contextPolicyCount > 0 {
		return nil
	}
	return &DeniedError{Decision: Decision{
		Operation:   operation,
		Policy:      "context",
		Effect:      effectDeny.String(),
		Explanation: "a context-bound access policy is required",
	}}
}
