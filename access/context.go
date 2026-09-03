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

// withCaptures returns a resolver that also knows the values a matched path
// pattern bound to its captures; captures take precedence over variables of
// the same name.
func (r variableResolver) withCaptures(captures map[string]any) variableResolver {
	if len(captures) == 0 {
		return r
	}
	merged := make(map[string]any, len(r.variables)+len(captures))
	for name, value := range r.variables {
		merged[name] = value
	}
	for name, value := range captures {
		merged[name] = value
	}
	return variableResolver{variables: merged, now: r.now}
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

// authorize settles an operation by policy decision alone; residual row
// conditions, if any, are ignored. Reads use authorizeRequest and writes use
// authorizeWrite so residuals are enforced.
func (g guard) authorize(ctx context.Context, operation Operations, resources ...Resource) error {
	_, _, err := g.authorizeRequest(ctx, Request{Operation: operation, Resources: resources})
	return err
}

// authorizeWrite returns, per resource, the write residuals every applicable
// policy requires the caller to enforce before delegating the write.
func (g guard) authorizeWrite(ctx context.Context, operation Operations, resources ...Resource) ([][]writeResidual, error) {
	_, writes, err := g.authorizeRequest(ctx, Request{Operation: operation, Resources: resources})
	return writes, err
}

// authorizeRequest evaluates every applicable policy and returns, per request
// resource, the read residuals and the write residuals the caller must still
// enforce. A denial by any policy is returned immediately.
func (g guard) authorizeRequest(ctx context.Context, request Request) ([][]residual, [][]writeResidual, error) {
	dynamicPolicies := policiesFromContext(ctx)
	contextPolicyCount := len(g.boundPolicies) + len(dynamicPolicies)
	if err := g.requireContextPolicy(request.Operation, contextPolicyCount); err != nil {
		return nil, nil, err
	}
	policies := make([]Policy, 0, len(g.databasePolicies)+contextPolicyCount)
	policies = append(policies, g.databasePolicies...)
	policies = append(policies, g.boundPolicies...)
	policies = append(policies, dynamicPolicies...)
	var residuals [][]residual
	var writes [][]writeResidual
	for _, policy := range policies {
		decision := policy.Decide(ctx, request)
		if !decision.Allowed {
			return nil, nil, &DeniedError{Decision: decision}
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
		for i, write := range decision.Writes {
			if write == nil || i >= len(request.Resources) {
				continue
			}
			if writes == nil {
				writes = make([][]writeResidual, len(request.Resources))
			}
			writes[i] = append(writes[i], writeResidual{
				policy:       decision.Policy,
				policySource: decision.PolicySource,
				resource:     request.Resources[i],
				residual:     write,
			})
		}
	}
	return residuals, writes, nil
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
