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
	if principal, ok := PrincipalFrom(ctx); ok {
		if _, set := combined["currentUser"]; !set && principal.ID != nil {
			combined["currentUser"] = principal.ID
		}
		if _, set := combined["principal.roles"]; !set {
			combined["principal.roles"] = append([]string{}, principal.Roles...)
		}
		if _, set := combined["principal.groups"]; !set {
			combined["principal.groups"] = append([]string{}, principal.Groups...)
		}
	}
	return combined
}

// variableResolver is one consistent snapshot of the variables for a request,
// so every condition of the request sees the same values and the same $now.
type variableResolver struct {
	variables map[string]any
	now       time.Time
}

func newVariableResolver(ctx context.Context, policyRealms ...string) variableResolver {
	variables := variablesFromContext(ctx)
	if principal, ok := PrincipalFrom(ctx); ok && principal.Subject != nil {
		delete(variables, "currentUser")
		variables["principal.roles"] = []string{}
		variables["principal.groups"] = []string{}
		realm := ""
		if len(policyRealms) > 0 {
			realm = policyRealms[0]
		}
		if realm != "" && principal.Subject.Realm == realm {
			variables["principal.roles"] = append([]string(nil), principal.Roles...)
			variables["principal.groups"] = append([]string(nil), principal.Groups...)
			if principal.Subject.Kind == PrincipalKindUser {
				variables["currentUser"] = principal.Subject.ID
			}
		}
	}
	return variableResolver{variables: variables, now: time.Now().UTC()}
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
	policyProvider   PolicyProvider
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
// enforce. Every mandatory policy is evaluated independently before a denial
// is returned so trusted callers can reduce complete internal diagnostics.
func (g guard) authorizeRequest(ctx context.Context, request Request) ([][]residual, [][]writeResidual, error) {
	var err error
	g, err = g.pinDatabasePolicies(ctx)
	if err != nil {
		return nil, nil, err
	}
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
	var decisions []Decision
	var firstDenial *Decision
	for _, policy := range policies {
		decision := policy.Decide(ctx, request)
		decisions = append(decisions, decision)
		if !decision.Allowed {
			if firstDenial == nil {
				copy := decision
				firstDenial = &copy
			}
			continue
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
	if firstDenial != nil {
		return nil, nil, &DeniedError{Decision: *firstDenial, Decisions: decisions}
	}
	return residuals, writes, nil
}

func (g guard) pinDatabasePolicies(ctx context.Context) (guard, error) {
	if g.policyProvider == nil {
		return g, nil
	}
	policies, err := g.policyProvider(ctx)
	if err != nil {
		return guard{}, &PolicyProviderError{Err: err}
	}
	if len(policies) == 0 {
		return guard{}, &PolicyProviderError{Err: fmt.Errorf("enabled provider returned no policies")}
	}
	for i, policy := range policies {
		if policy == nil {
			return guard{}, &PolicyProviderError{Err: fmt.Errorf("nil policy at index %d", i)}
		}
	}
	g.databasePolicies = append(append([]Policy(nil), g.databasePolicies...), policies...)
	g.policyProvider = nil
	return g, nil
}

// PolicyProviderError is a fail-closed dynamic policy snapshot failure.
type PolicyProviderError struct{ Err error }

func (e *PolicyProviderError) Error() string {
	return "dalgo access denied: database policy snapshot unavailable"
}
func (e *PolicyProviderError) Unwrap() error        { return e.Err }
func (e *PolicyProviderError) Is(target error) bool { return target == ErrAccessDenied }

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
