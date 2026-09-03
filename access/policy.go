package access

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/condeval"
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
	// Condition is the source text of the row condition behind a conditional
	// decision, with parameter names rather than resolved values.
	Condition string
	// Residuals holds, per request resource (same index as Request.Resources),
	// the resolved row condition the caller must still enforce before the
	// operation is complete: after the read for point operations, by query
	// rewrite for Query. A nil entry means the resource is allowed outright.
	// Residuals is nil for an unconditional decision.
	Residuals []dal.Condition
	// Writes holds, per request resource, the ordered alternatives that decide
	// a write on that resource (see WriteResidual). A nil entry means the write
	// is allowed outright, with no row or post-image constraint.
	Writes []*WriteResidual
}

// WriteAlternative is one allow rule as it applies to a write. Where selects
// the rows (by pre-image) the rule applies to; nil means every row. Check is
// what the row must satisfy after the write; nil means no constraint. Both are
// resolved conditions; the Text fields keep the source form with parameter
// names for explanations.
type WriteAlternative struct {
	Rule      string
	Where     dal.Condition
	Check     dal.Condition
	WhereText string
	CheckText string
	// Fields is the rule's allow-list of field patterns; nil means every field.
	Fields []string
	fields *fieldSet
}

// WriteResidual is what the secured wrapper enforces on a write: the
// conditional alternatives in precedence order, then Terminal — the first
// unconditional rule when it is an allow (possibly with a Check) — or nil when
// the walk ends in a deny or in no rule at all. The first alternative whose
// Where holds on the pre-image decides the write; a new row (Insert, or Set of
// a missing row) is admitted by the first alternative whose Check it satisfies.
type WriteResidual struct {
	Alternatives []WriteAlternative
	Terminal     *WriteAlternative
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

func (p *AccessPolicy) Decide(ctx context.Context, request Request) Decision {
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
	var residuals []dal.Condition
	var writes []*WriteResidual
	resolver := newVariableResolver(ctx)
	for i, resource := range request.Resources {
		last = p.decideResource(resolver, request.Operation, resource)
		if !last.Allowed {
			return last
		}
		if last.Residuals != nil {
			if residuals == nil {
				residuals = make([]dal.Condition, len(request.Resources))
			}
			residuals[i] = last.Residuals[0]
		}
		if last.Writes != nil {
			if writes == nil {
				writes = make([]*WriteResidual, len(request.Resources))
			}
			writes[i] = last.Writes[0]
		}
	}
	last.Residuals = residuals
	last.Writes = writes
	return last
}

func (p *AccessPolicy) decideResource(resolver variableResolver, operation Operations, resource Resource) Decision {
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
	// Walk matches in precedence order: conditional allows met before the first
	// unconditional rule apply to the rows their conditions select; the first
	// unconditional rule settles everything else. An unconditional allow makes
	// the collected conditions moot; a deny (or no unconditional rule at all)
	// leaves their disjunction as the residual the caller must enforce.
	var conditional []compiledRule
	var terminal *compiledRule
	for i := range matching {
		if matching[i].where == nil {
			terminal = &matching[i]
			break
		}
		conditional = append(conditional, matching[i])
	}
	if len(conditional) == 0 && (terminal.effect != effectAllow || (terminal.check == nil && terminal.fields == nil)) {
		allowed := terminal.effect == effectAllow
		return Decision{
			Allowed:      allowed,
			Operation:    operation,
			Resource:     resource,
			Policy:       p.name,
			PolicySource: p.source,
			Rule:         terminal.name,
			Effect:       terminal.effect.String(),
			Explanation:  fmt.Sprintf("matched rule %q (%s)", terminal.name, terminal.effect),
		}
	}
	// Resolve every alternative once; an unresolved parameter denies.
	write := &WriteResidual{}
	for _, rule := range conditional {
		alternative, err := p.resolveAlternative(resolver.withCaptures(captureValues(rule.pattern, resource)), rule)
		if err != nil {
			return p.denyUnresolved(operation, resource, rule, err)
		}
		write.Alternatives = append(write.Alternatives, alternative)
	}
	if terminal != nil && terminal.effect == effectAllow {
		alternative, err := p.resolveAlternative(resolver.withCaptures(captureValues(terminal.pattern, resource)), *terminal)
		if err != nil {
			return p.denyUnresolved(operation, resource, *terminal, err)
		}
		write.Terminal = &alternative
	}
	decision := Decision{
		Allowed:      true,
		Operation:    operation,
		Resource:     resource,
		Policy:       p.name,
		PolicySource: p.source,
		Effect:       effectAllow.String(),
		Writes:       []*WriteResidual{write},
	}
	if len(conditional) == 0 {
		// A terminal allow with only a post-image check or a fields list:
		// reads are unconditional on rows.
		decision.Rule = terminal.name
		decision.Explanation = fmt.Sprintf("matched rule %q (allow)", terminal.name)
		if terminal.check != nil {
			decision.Condition = terminal.check.String()
			decision.Explanation = fmt.Sprintf("matched rule %q (allow; check: %s)", terminal.name, decision.Condition)
		}
		return decision
	}
	names := make([]string, 0, len(conditional))
	texts := make([]string, 0, len(conditional))
	parts := make([]dal.Condition, 0, len(conditional))
	for _, alternative := range write.Alternatives {
		names = append(names, alternative.Rule)
		texts = append(texts, alternative.WhereText)
		parts = append(parts, alternative.Where)
	}
	if write.Terminal != nil {
		// An unconditional allow makes the row conditions moot for reads.
		decision.Rule = write.Terminal.Rule
		decision.Explanation = fmt.Sprintf("matched rule %q (allow)", write.Terminal.Rule)
		return decision
	}
	residual, text := parts[0], texts[0]
	if len(parts) > 1 {
		residual = dal.NewGroupCondition(dal.Or, parts...)
		text = "(" + strings.Join(texts, " OR ") + ")"
	}
	decision.Rule = strings.Join(names, ", ")
	decision.Condition = text
	decision.Explanation = fmt.Sprintf("matched conditional rule(s) %s (where: %s)", quoteAll(names), text)
	decision.Residuals = []dal.Condition{residual}
	return decision
}

func (p *AccessPolicy) resolveAlternative(resolver variableResolver, rule compiledRule) (WriteAlternative, error) {
	alternative := WriteAlternative{Rule: rule.name, fields: rule.fields}
	if rule.fields != nil {
		alternative.Fields = append([]string(nil), rule.fields.sources...)
	}
	if rule.where != nil {
		resolved, err := condeval.Substitute(rule.where, resolver.resolve)
		if err != nil {
			return WriteAlternative{}, err
		}
		alternative.Where = resolved
		alternative.WhereText = rule.where.String()
	}
	if rule.check != nil {
		resolved, err := condeval.Substitute(rule.check, resolver.resolve)
		if err != nil {
			return WriteAlternative{}, err
		}
		alternative.Check = resolved
		alternative.CheckText = rule.check.String()
	}
	return alternative, nil
}

func (p *AccessPolicy) denyUnresolved(operation Operations, resource Resource, rule compiledRule, err error) Decision {
	condition := ""
	if rule.where != nil {
		condition = rule.where.String()
	} else if rule.check != nil {
		condition = rule.check.String()
	}
	return Decision{
		Operation:    operation,
		Resource:     resource,
		Policy:       p.name,
		PolicySource: p.source,
		Rule:         rule.name,
		Effect:       effectDeny.String(),
		Condition:    condition,
		Explanation:  fmt.Sprintf("cannot evaluate rule %q: %v", rule.name, err),
	}
}

func quoteAll(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	return strings.Join(quoted, ", ")
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
