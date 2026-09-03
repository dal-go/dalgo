package access

import (
	"context"
	"fmt"
	"time"

	"github.com/dal-go/dalgo/dal"
)

type contextPoliciesKey struct{}

type contextVariablesKey struct{}

// WithVariables returns a child context carrying values for the parameters a
// conditional rule may reference: a rule conditioned on ownerID == $tenant reads
// the "tenant" entry. Variables inherited from the parent context are kept and
// same-named entries are replaced. $now resolves to the evaluation time unless
// a "now" variable is supplied.
func WithVariables(ctx context.Context, variables map[string]any) context.Context {
	if ctx == nil {
		panic("access: nil context")
	}
	combined := variablesFromContext(ctx)
	for name, value := range variables {
		if !dal.ValidParamName(name) {
			panic(fmt.Sprintf("access: invalid variable name %q", name))
		}
		combined[name] = value
	}
	return context.WithValue(ctx, contextVariablesKey{}, combined)
}

// WithCurrentUser sets the $currentUser variable.
func WithCurrentUser(ctx context.Context, userID any) context.Context {
	return WithVariables(ctx, map[string]any{"currentUser": userID})
}

func variablesFromContext(ctx context.Context) map[string]any {
	combined := map[string]any{}
	if ctx == nil {
		return combined
	}
	variables, _ := ctx.Value(contextVariablesKey{}).(map[string]any)
	for name, value := range variables {
		combined[name] = value
	}
	return combined
}

// variableResolver is one consistent snapshot of the variables for a request,
// so every condition of the request sees the same values and the same $now.
type variableResolver struct {
	variables map[string]any
	now       time.Time
}

func newVariableResolver(ctx context.Context) variableResolver {
	return variableResolver{variables: variablesFromContext(ctx), now: time.Now().UTC()}
}

func (r variableResolver) resolve(name string) (any, bool) {
	if value, ok := r.variables[name]; ok {
		return value, true
	}
	if name == "now" {
		return r.now, true
	}
	return nil, false
}

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

// authorize settles an operation that cannot carry a residual condition. In
// this version that is every write: a conditional rule on a write operation is
// refused rather than silently granted, until pre-image and post-image checks
// exist.
func (g guard) authorize(ctx context.Context, operation Operations, resources ...Resource) error {
	residuals, err := g.authorizeRequest(ctx, Request{Operation: operation, Resources: resources})
	if err != nil {
		return err
	}
	for _, perResource := range residuals {
		for _, residual := range perResource {
			return &DeniedError{Decision: Decision{
				Operation:    operation,
				Resource:     residual.resource,
				Policy:       residual.policy,
				PolicySource: residual.policySource,
				Rule:         residual.rule,
				Effect:       effectDeny.String(),
				Condition:    residual.text,
				Explanation:  fmt.Sprintf("conditional rule %q is not enforced for %s in this version (where: %s)", residual.rule, operation, residual.text),
			}}
		}
	}
	return nil
}

// authorizeRequest evaluates every applicable policy and returns, per request
// resource, the residual conditions the caller must still enforce. A denial by
// any policy is returned immediately.
func (g guard) authorizeRequest(ctx context.Context, request Request) ([][]residual, error) {
	dynamicPolicies := policiesFromContext(ctx)
	contextPolicyCount := len(g.boundPolicies) + len(dynamicPolicies)
	if err := g.requireContextPolicy(request.Operation, contextPolicyCount); err != nil {
		return nil, err
	}
	policies := make([]Policy, 0, len(g.databasePolicies)+contextPolicyCount)
	policies = append(policies, g.databasePolicies...)
	policies = append(policies, g.boundPolicies...)
	policies = append(policies, dynamicPolicies...)
	var residuals [][]residual
	for _, policy := range policies {
		decision := policy.Decide(ctx, request)
		if !decision.Allowed {
			return nil, &DeniedError{Decision: decision}
		}
		for i, condition := range decision.Residuals {
			if condition == nil || i >= len(request.Resources) {
				continue
			}
			if residuals == nil {
				residuals = make([][]residual, len(request.Resources))
			}
			residuals[i] = append(residuals[i], residual{
				policy:       decision.Policy,
				policySource: decision.PolicySource,
				rule:         decision.Rule,
				text:         decision.Condition,
				resource:     request.Resources[i],
				condition:    condition,
			})
		}
	}
	return residuals, nil
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
