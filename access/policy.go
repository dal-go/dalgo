package access

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/dal-go/dalgo/dal"
)

var (
	// ErrAccessDenied is matched by all authorization denials.
	ErrAccessDenied = errors.New("dalgo access denied")
	// ErrNotSerializable indicates a policy uses a construct that cannot be
	// represented by a portable policy document.
	ErrNotSerializable = errors.New("dalgo access policy is not serializable")
)

// Request describes one DAL operation over one or more resources.
type Request struct {
	Operation Operations
	Resources []Resource
	// Query retains the structured or opaque query for custom policies and
	// future predicate, projection, index, and cost constraints. It is nil for
	// non-query operations; v1 declarative policies authorize query sources.
	Query dal.Query
}

// Decision explains an access-policy result.
type Decision struct {
	Allowed      bool
	Operation    Operations
	Resource     Resource
	Policy       string
	PolicySource string
	Rule         string
	Effect       string
	Explanation  string
}

// DeniedError is returned when a policy rejects an operation.
type DeniedError struct {
	Decision Decision
}

func (e *DeniedError) Error() string {
	policy := fmt.Sprintf("policy=%q", e.Decision.Policy)
	if e.Decision.PolicySource != "" {
		policy += fmt.Sprintf(" source=%q", e.Decision.PolicySource)
	}
	rule := ""
	if e.Decision.Rule != "" {
		rule = fmt.Sprintf(" rule=%q", e.Decision.Rule)
	}
	return fmt.Sprintf("%v: %s%s operation=%s resource=%s: %s", ErrAccessDenied,
		policy, rule, e.Decision.Operation, e.Decision.Resource, e.Decision.Explanation)
}

func (e *DeniedError) Unwrap() error { return ErrAccessDenied }

// Policy is a named access capability. Every Policy applied to a secured
// request must allow every target resource.
type Policy interface {
	Name() string
	Decide(context.Context, Request) Decision
	Authorize(context.Context, Request) error
}

// AccessPolicy is the declarative hierarchical Policy implementation.
type AccessPolicy struct {
	name     string
	source   string
	rules    []Rule
	compiled []compiledRule
}

// NewPolicy constructs a default-deny access policy.
func NewPolicy(name string, rules ...Rule) (*AccessPolicy, error) {
	if name == "" {
		return nil, fmt.Errorf("access: policy name is required")
	}
	compiled, err := compileRules(rules, map[effect]bool{effectAllow: true, effectDeny: true})
	if err != nil {
		return nil, err
	}
	return &AccessPolicy{name: name, rules: append([]Rule(nil), rules...), compiled: compiled}, nil
}

// MustPolicy constructs a policy and panics when its declaration is invalid.
func MustPolicy(name string, rules ...Rule) *AccessPolicy {
	policy, err := NewPolicy(name, rules...)
	if err != nil {
		panic(err)
	}
	return policy
}

func (p *AccessPolicy) Name() string { return p.name }

// Source returns an optional storage-neutral reference supplied while loading
// a policy document, such as an object key, URL, database key, or file path.
func (p *AccessPolicy) Source() string { return p.source }

func (p *AccessPolicy) Decide(_ context.Context, request Request) Decision {
	if !request.Operation.validLeaf() {
		return Decision{
			Operation:    request.Operation,
			Policy:       p.name,
			PolicySource: p.source,
			Effect:       effectDeny.String(),
			Explanation:  "operation is unknown or is not a single leaf operation",
		}
	}
	if len(request.Resources) == 0 {
		return Decision{
			Operation:    request.Operation,
			Policy:       p.name,
			PolicySource: p.source,
			Effect:       effectDeny.String(),
			Explanation:  "request has no resources",
		}
	}
	var last Decision
	for _, resource := range request.Resources {
		last = p.decideResource(request.Operation, resource)
		if !last.Allowed {
			return last
		}
	}
	return last
}

func (p *AccessPolicy) decideResource(operation Operations, resource Resource) Decision {
	matching := matchingRules(p.compiled, operation, resource)
	if len(matching) == 0 {
		return Decision{
			Operation:    operation,
			Resource:     resource,
			Policy:       p.name,
			PolicySource: p.source,
			Effect:       effectDeny.String(),
			Explanation:  "no matching allow rule",
		}
	}
	winner := matching[0]
	allowed := winner.effect == effectAllow
	explanation := fmt.Sprintf("matched rule %q (%s)", winner.name, winner.effect)
	return Decision{
		Allowed:      allowed,
		Operation:    operation,
		Resource:     resource,
		Policy:       p.name,
		PolicySource: p.source,
		Rule:         winner.name,
		Effect:       winner.effect.String(),
		Explanation:  explanation,
	}
}

func (p *AccessPolicy) Authorize(ctx context.Context, request Request) error {
	decision := p.Decide(ctx, request)
	if decision.Allowed {
		return nil
	}
	return &DeniedError{Decision: decision}
}

func matchingRules(rules []compiledRule, operation Operations, resource Resource) []compiledRule {
	matches := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.operations.contains(operation) || !ruleMatchesResource(rule, resource) {
			continue
		}
		matches = append(matches, rule)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		if left.depth != right.depth {
			return left.depth > right.depth
		}
		if left.literals != right.literals {
			return left.literals > right.literals
		}
		if left.effect != right.effect {
			return effectIsRestrictive(left.effect)
		}
		return left.name < right.name
	})
	return matches
}

func ruleMatchesResource(rule compiledRule, resource Resource) bool {
	if rule.kind != resource.kind {
		return false
	}
	switch rule.kind {
	case PathResource:
		return patternsMatch(rule.pattern, resource)
	case CollectionGroupResource:
		return rule.resource == resource.name
	case OpaqueQueryResource:
		return true
	default:
		return false
	}
}

func effectIsRestrictive(ruleEffect effect) bool {
	return ruleEffect == effectDeny || ruleEffect == effectIgnoreAudit
}

// AuditDecision explains whether an operation should be emitted to an audit
// pipeline. It never changes authorization.
type AuditDecision struct {
	Audit        bool
	Operation    Operations
	Resource     Resource
	Policy       string
	PolicySource string
	Rule         string
	Effect       string
	Explanation  string
}

// AuditPolicy classifies operations with the same hierarchy as AccessPolicy.
// Its default is IgnoreAudit.
type AuditPolicy struct {
	name     string
	source   string
	rules    []Rule
	compiled []compiledRule
}

func NewAuditPolicy(name string, rules ...Rule) (*AuditPolicy, error) {
	if name == "" {
		return nil, fmt.Errorf("access: audit policy name is required")
	}
	compiled, err := compileRules(rules, map[effect]bool{effectAudit: true, effectIgnoreAudit: true})
	if err != nil {
		return nil, err
	}
	return &AuditPolicy{name: name, rules: append([]Rule(nil), rules...), compiled: compiled}, nil
}

func MustAuditPolicy(name string, rules ...Rule) *AuditPolicy {
	policy, err := NewAuditPolicy(name, rules...)
	if err != nil {
		panic(err)
	}
	return policy
}

func (p *AuditPolicy) Name() string { return p.name }

// Source returns the storage-neutral reference supplied while loading the
// policy document.
func (p *AuditPolicy) Source() string { return p.source }

func (p *AuditPolicy) Classify(_ context.Context, request Request) AuditDecision {
	if !request.Operation.validLeaf() || len(request.Resources) == 0 {
		return AuditDecision{Operation: request.Operation, Policy: p.name, PolicySource: p.source, Effect: effectIgnoreAudit.String(), Explanation: "invalid operation or no resources"}
	}
	var last AuditDecision
	for _, resource := range request.Resources {
		matching := matchingRules(p.compiled, request.Operation, resource)
		if len(matching) == 0 {
			last = AuditDecision{Operation: request.Operation, Resource: resource, Policy: p.name, PolicySource: p.source, Effect: effectIgnoreAudit.String(), Explanation: "no matching audit rule"}
			continue
		}
		winner := matching[0]
		last = AuditDecision{
			Audit:        winner.effect == effectAudit,
			Operation:    request.Operation,
			Resource:     resource,
			Policy:       p.name,
			PolicySource: p.source,
			Rule:         winner.name,
			Effect:       winner.effect.String(),
			Explanation:  fmt.Sprintf("matched rule %q (%s)", winner.name, winner.effect),
		}
		if last.Audit {
			return last
		}
	}
	return last
}
