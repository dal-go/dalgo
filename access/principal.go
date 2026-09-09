package access

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type PrincipalKind string

const (
	PrincipalKindUser        PrincipalKind = "user"
	PrincipalKindService     PrincipalKind = "service"
	PrincipalKindApplication PrincipalKind = "application"
	PrincipalKindAgent       PrincipalKind = "agent"
)

// PrincipalRef is a stable identity in an owner-configured realm.
type PrincipalRef struct {
	Realm string        `json:"realm" yaml:"realm"`
	Kind  PrincipalKind `json:"kind" yaml:"kind"`
	ID    string        `json:"id" yaml:"id"`
}

func (reference PrincipalRef) Validate() error {
	if !validPrincipalPart(reference.Realm) {
		return fmt.Errorf("access: principal realm must be a non-empty canonical string of at most 256 bytes")
	}
	if !validPrincipalPart(reference.ID) {
		return fmt.Errorf("access: principal id must be a non-empty canonical string of at most 256 bytes")
	}
	switch reference.Kind {
	case PrincipalKindUser, PrincipalKindService, PrincipalKindApplication, PrincipalKindAgent:
		return nil
	default:
		return fmt.Errorf("access: unknown principal kind %q", reference.Kind)
	}
}

func validPrincipalPart(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Principal identifies the caller for principal bindings and for the
// $currentUser, $principal.roles and $principal.groups variables. Roles and
// groups are opaque strings the host assigns; DALgo never looks them up.
type Principal struct {
	Subject            *PrincipalRef
	Actor              *PrincipalRef
	ID                 any // legacy untyped user identity, used only when Subject is nil
	Roles              []string
	Groups             []string
	MembershipRevision string
}

func NewPrincipal(subject PrincipalRef, roles, groups []string) (Principal, error) {
	if err := subject.Validate(); err != nil {
		return Principal{}, err
	}
	copy := subject
	return Principal{Subject: &copy, Roles: append([]string(nil), roles...), Groups: append([]string(nil), groups...)}, nil
}

type contextPrincipalKey struct{}

// WithPrincipal returns a child context carrying the caller's principal. It
// replaces any principal inherited from the parent context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	if ctx == nil {
		panic("access: nil context")
	}
	copy := clonePrincipal(principal)
	if copy.Subject != nil {
		if err := copy.Subject.Validate(); err != nil {
			panic(err)
		}
		copy.ID = nil
	}
	if copy.Actor != nil {
		if err := copy.Actor.Validate(); err != nil {
			panic(err)
		}
	}
	return context.WithValue(ctx, contextPrincipalKey{}, copy)
}

// PrincipalFrom returns the principal carried by ctx, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	principal, ok := ctx.Value(contextPrincipalKey{}).(Principal)
	return clonePrincipal(principal), ok
}

func clonePrincipal(principal Principal) Principal {
	principal.Roles = append([]string(nil), principal.Roles...)
	principal.Groups = append([]string(nil), principal.Groups...)
	if principal.Subject != nil {
		copy := *principal.Subject
		principal.Subject = &copy
	}
	if principal.Actor != nil {
		copy := *principal.Actor
		principal.Actor = &copy
	}
	return principal
}

// Bindings map principals to named rule sets. A binding value is a list of
// rule-set names; Everyone applies to every principal, including an absent one.
type Bindings struct {
	Roles    map[string][]string
	Groups   map[string][]string
	Users    map[string][]string
	Everyone []string
}

// PrincipalPolicySet is a Policy that derives the caller's authority from
// who they are: the rule sets bound to the principal's ID, roles and groups
// (plus Everyone) are unioned and compiled as one AccessPolicy, so the parent
// feature's precedence resolves overlaps between bindings; that single policy
// then takes part in the ordinary intersection with database and context
// policies, so a binding can never widen what another policy denies.
type PrincipalPolicySet struct {
	name           string
	source         string
	visibility     string
	revision       string
	realm          string
	collectionMask *CompiledMask
	execution      *compiledExecutionGate
	ruleSets       map[string][]Rule
	bindings       Bindings

	mu    sync.Mutex
	cache map[string]*AccessPolicy
}

// NewPrincipalPolicySet validates that every binding names an existing rule
// set and that every rule set compiles on its own.
func NewPrincipalPolicySet(name string, ruleSets map[string][]Rule, bindings Bindings) (*PrincipalPolicySet, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("access: principal policy set name is required")
	}
	if len(ruleSets) == 0 {
		return nil, fmt.Errorf("access: principal policy set %q has no rule sets", name)
	}
	for setName, rules := range ruleSets {
		if strings.TrimSpace(setName) == "" || strings.Contains(setName, "/") {
			return nil, fmt.Errorf("access: principal policy set %q: invalid rule set name %q", name, setName)
		}
		if _, err := NewPolicy(name+"/"+setName, prefixRuleNames(setName, rules)...); err != nil {
			return nil, fmt.Errorf("access: rule set %q: %w", setName, err)
		}
	}
	check := func(kind string, references map[string][]string) error {
		for subject, sets := range references {
			for _, setName := range sets {
				if _, ok := ruleSets[setName]; !ok {
					return fmt.Errorf("access: principal policy set %q: %s %q references unknown rule set %q", name, kind, subject, setName)
				}
			}
		}
		return nil
	}
	if err := check("role", bindings.Roles); err != nil {
		return nil, err
	}
	if err := check("group", bindings.Groups); err != nil {
		return nil, err
	}
	if err := check("user", bindings.Users); err != nil {
		return nil, err
	}
	if err := check("everyone", map[string][]string{"everyone": bindings.Everyone}); err != nil {
		return nil, err
	}
	return &PrincipalPolicySet{name: name, ruleSets: ruleSets, bindings: bindings, cache: map[string]*AccessPolicy{}}, nil
}

// MustPrincipalPolicySet constructs a set and panics when it is invalid.
func MustPrincipalPolicySet(name string, ruleSets map[string][]Rule, bindings Bindings) *PrincipalPolicySet {
	set, err := NewPrincipalPolicySet(name, ruleSets, bindings)
	if err != nil {
		panic(err)
	}
	return set
}

func (p *PrincipalPolicySet) Name() string { return p.name }

// Source returns the storage-neutral reference supplied while loading the
// document, if any.
func (p *PrincipalPolicySet) Source() string { return p.source }

func (p *PrincipalPolicySet) PolicyMetadata() PolicyMetadata {
	visibility := PolicyVisibility(p.visibility)
	if visibility == "" {
		visibility = PolicyVisibilityPrivate
	}
	return PolicyMetadata{ID: p.name, Revision: p.revision, Visibility: visibility, Source: p.source}
}

// Decide unions the rule sets bound to the principal on ctx and evaluates the
// request against them as one policy. Without any applicable binding the
// request is denied.
func (p *PrincipalPolicySet) Decide(ctx context.Context, request Request) Decision {
	if !policyRealmAllows(ctx, p.realm) {
		return principalRealmDenied(request, p.name, p.source)
	}
	if !p.execution.allows(request) {
		return executionDenied(request, p.name, p.source)
	}
	for _, resource := range request.Resources {
		if !collectionMaskAllows(p.collectionMask, resource) {
			return Decision{Operation: request.Operation, Resource: resource, Policy: p.name, PolicySource: p.source, Effect: effectDeny.String(), Code: CodeCollectionDenied, Scope: DecisionScopeTable, Explanation: "collection mask denies resource"}
		}
	}

	principal, present := PrincipalFrom(ctx)
	sets, attribution := p.boundSets(principal, present)
	if len(sets) == 0 {
		return Decision{
			Operation:    request.Operation,
			Policy:       p.name,
			PolicySource: p.source,
			Effect:       effectDeny.String(),
			Code:         CodeNoMatch,
			Scope:        DecisionScopePrincipal,
			Explanation:  fmt.Sprintf("no binding applies to principal %v", principalLabel(principal, present)),
		}
	}
	decision := p.effective(sets).Decide(ctx, request)
	decision.Policy = p.name
	decision.PolicySource = p.source
	if decision.Rule != "" {
		labels := map[string]struct{}{}
		for _, rule := range strings.Split(decision.Rule, ", ") {
			setName, _, _ := strings.Cut(rule, "/")
			for _, label := range attribution[setName] {
				labels[label] = struct{}{}
			}
		}
		decision.Explanation += " via " + strings.Join(sortedKeys(labels), ", ")
	}
	return decision
}

func (p *PrincipalPolicySet) Authorize(ctx context.Context, request Request) error {
	decision := p.Decide(ctx, request)
	if decision.Allowed {
		return nil
	}
	return &DeniedError{Decision: decision}
}

// boundSets returns the sorted, de-duplicated rule-set names bound to the
// principal and, per set, the binding labels that pulled it in.
func (p *PrincipalPolicySet) boundSets(principal Principal, present bool) ([]string, map[string][]string) {
	attribution := map[string][]string{}
	add := func(label string, sets []string) {
		for _, setName := range sets {
			attribution[setName] = append(attribution[setName], label)
		}
	}
	add("everyone", p.bindings.Everyone)
	if present {
		if principal.Subject != nil {
			if p.realm != "" && principal.Subject.Realm == p.realm {
				for _, role := range principal.Roles {
					add("role:"+role, p.bindings.Roles[role])
				}
				for _, group := range principal.Groups {
					add("group:"+group, p.bindings.Groups[group])
				}
				if principal.Subject.Kind == PrincipalKindUser {
					add("user:"+principal.Subject.ID, p.bindings.Users[principal.Subject.ID])
				}
			}
		} else {
			if principal.ID != nil {
				add("user:"+fmt.Sprint(principal.ID), p.bindings.Users[fmt.Sprint(principal.ID)])
			}
			for _, role := range principal.Roles {
				add("role:"+role, p.bindings.Roles[role])
			}
			for _, group := range principal.Groups {
				add("group:"+group, p.bindings.Groups[group])
			}
		}
	}
	for setName := range attribution {
		sort.Strings(attribution[setName])
	}
	return sortedKeys(attribution), attribution
}

func policyRealmAllows(ctx context.Context, realm string) bool {
	principal, ok := PrincipalFrom(ctx)
	if realm == "" {
		return !ok || principal.Subject == nil
	}
	return ok && principal.Subject != nil && principal.Subject.Realm == realm
}

func principalRealmDenied(request Request, policy, source string) Decision {
	decision := Decision{Operation: request.Operation, Policy: policy, PolicySource: source, Effect: effectDeny.String(), Code: CodePrincipalUnresolved, Scope: DecisionScopePrincipal, Explanation: "principal is not valid in the policy realm"}
	if len(request.Resources) > 0 {
		decision.Resource = request.Resources[0]
	}
	return decision
}

// effective returns the compiled union of the given rule sets, cached by the
// set combination so repeated requests by the same kind of principal reuse it.
func (p *PrincipalPolicySet) effective(sets []string) *AccessPolicy {
	key := strings.Join(sets, "\x00")
	p.mu.Lock()
	defer p.mu.Unlock()
	if policy, ok := p.cache[key]; ok {
		return policy
	}
	var rules []Rule
	for _, setName := range sets {
		rules = append(rules, prefixRuleNames(setName, p.ruleSets[setName])...)
	}
	// Every rule set compiled alone at construction and names are prefixed
	// per set, so the union compiles.
	policy := MustPolicy(p.name, rules...)
	policy.realm = p.realm
	p.cache[key] = policy
	return policy
}

// prefixRuleNames returns a copy of rules whose directive names are prefixed
// with the rule-set name ("content-write/read"), giving every rule a unique,
// attributable identity across the union. Unnamed directives are numbered.
func prefixRuleNames(setName string, rules []Rule) []Rule {
	counter := 0
	var walk func(rules []Rule) []Rule
	walk = func(rules []Rule) []Rule {
		out := make([]Rule, len(rules))
		for i, rule := range rules {
			out[i] = rule
			switch rule.kind {
			case directiveRule:
				name := rule.name
				if name == "" {
					counter++
					name = fmt.Sprintf("rule-%d", counter)
				}
				out[i].name = setName + "/" + name
			default:
				out[i].children = walk(rule.children)
			}
		}
		return out
	}
	return walk(rules)
}

func principalLabel(principal Principal, present bool) string {
	if !present {
		return "<none>"
	}
	if principal.Subject != nil {
		return fmt.Sprintf("%s:%s:%q", principal.Subject.Realm, principal.Subject.Kind, principal.Subject.ID)
	}
	return fmt.Sprintf("%q", fmt.Sprint(principal.ID))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var _ Policy = (*PrincipalPolicySet)(nil)

// NewPolicyForRealm creates an application-owned declarative policy using
// typed principals in realm. Legacy NewPolicy remains unqualified.
func NewPolicyForRealm(realm, name string, rules ...Rule) (*AccessPolicy, error) {
	if !validPrincipalPart(realm) {
		return nil, fmt.Errorf("access: invalid policy realm")
	}
	policy, err := NewPolicy(name, rules...)
	if err != nil {
		return nil, err
	}
	policy.realm = realm
	return policy, nil
}

// NewPrincipalPolicySetForRealm binds application-owned rule sets to typed
// principals in realm using the existing user/role/group composition rules.
func NewPrincipalPolicySetForRealm(realm, name string, ruleSets map[string][]Rule, bindings Bindings) (*PrincipalPolicySet, error) {
	if !validPrincipalPart(realm) {
		return nil, fmt.Errorf("access: invalid policy realm")
	}
	policy, err := NewPrincipalPolicySet(name, ruleSets, bindings)
	if err != nil {
		return nil, err
	}
	policy.realm = realm
	return policy, nil
}
