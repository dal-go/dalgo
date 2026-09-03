package access

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
)

type effect uint8

const (
	effectAllow effect = iota + 1
	effectDeny
	effectAudit
	effectIgnoreAudit
)

func (e effect) String() string {
	switch e {
	case effectAllow:
		return "allow"
	case effectDeny:
		return "deny"
	case effectAudit:
		return "audit"
	case effectIgnoreAudit:
		return "ignore-audit"
	default:
		return "unknown"
	}
}

type ruleKind uint8

const (
	directiveRule ruleKind = iota + 1
	scopeRule
	collectionGroupRule
	opaqueQueryRule
)

// Rule is a declarative policy rule. Use Allow, Deny, Audit, IgnoreAudit,
// Scope, Collection, Under, Root, CollectionGroupScope, or OpaqueQueryScope.
type Rule struct {
	kind       ruleKind
	pattern    PathPattern
	name       string
	operations Operations
	effect     effect
	resource   string
	children   []Rule
	where      dal.Condition
	check      dal.Condition
}

// Where attaches a row condition to an allow rule: the rule applies only to
// records whose values satisfy condition. The condition is a dal.Condition over
// the record's fields (comparisons, In, And/Or groups) whose right-hand values
// may be parameters such as dal.NewParam("currentUser"), resolved from the
// operation context (see WithVariables and WithCurrentUser).
//
// Conditions are valid on allow rules under path scopes only. A conditional
// rule never authorises Truncate: naming it explicitly fails compilation, while
// the write and readwrite groups simply drop it.
func (r Rule) Where(condition dal.Condition) Rule {
	r.where = condition
	return r
}

// Check attaches a post-image condition to an allow rule: a row written under
// the rule must satisfy condition after the write (the new data of an Insert
// or Set, the updated row of an Update). A rule with Where but no Check uses
// Where as its check, so "rows where ownerID == $currentUser" also means "and
// they must still be mine afterwards". A rule with Check but no Where applies
// to every row on read and constrains only what a write may produce.
func (r Rule) Check(condition dal.Condition) Rule {
	r.check = condition
	return r
}

// Allow permits operations at the containing scope. The optional name appears
// in explanations; a deterministic name is generated when omitted.
func Allow(operations Operations, name ...string) Rule {
	return directive(effectAllow, operations, name)
}

// Deny rejects operations at the containing scope.
func Deny(operations Operations, name ...string) Rule {
	return directive(effectDeny, operations, name)
}

// Audit selects matching operations for an application's audit pipeline. It
// does not grant access and does not persist an audit record itself.
func Audit(operations Operations, name ...string) Rule {
	return directive(effectAudit, operations, name)
}

// IgnoreAudit excludes matching operations from audit selection.
func IgnoreAudit(operations Operations, name ...string) Rule {
	return directive(effectIgnoreAudit, operations, name)
}

func directive(ruleEffect effect, operations Operations, names []string) Rule {
	name := ""
	if len(names) > 0 {
		name = strings.TrimSpace(names[0])
	}
	return Rule{kind: directiveRule, effect: ruleEffect, operations: operations, name: name}
}

// Scope adds a collection-and-ID segment beneath its containing scope.
func Scope(collection string, id any, rules ...Rule) Rule {
	return Under(Path(collection, id), rules...)
}

// Collection adds a terminal collection segment beneath its containing scope.
func Collection(collection string, rules ...Rule) Rule {
	return Under(Path(collection), rules...)
}

// Under adds an arbitrary structural path prefix beneath its containing scope.
func Under(pattern PathPattern, rules ...Rule) Rule {
	return Rule{kind: scopeRule, pattern: pattern, children: append([]Rule(nil), rules...)}
}

// Root attaches rules at the root of all ordinary path resources.
func Root(rules ...Rule) Rule {
	return Under(PathPattern{}, rules...)
}

// CollectionGroupScope attaches rules to an explicit collection-group query.
func CollectionGroupScope(name string, rules ...Rule) Rule {
	return Rule{kind: collectionGroupRule, resource: name, children: append([]Rule(nil), rules...)}
}

// OpaqueQueryScope attaches rules to all non-structured queries. It is an
// intentionally explicit and potentially broad capability.
func OpaqueQueryScope(rules ...Rule) Rule {
	return Rule{kind: opaqueQueryRule, children: append([]Rule(nil), rules...)}
}

type compiledRule struct {
	kind       ResourceKind
	pattern    PathPattern
	resource   string
	name       string
	operations Operations
	effect     effect
	depth      int
	literals   int
	where      dal.Condition
	check      dal.Condition
	params     []string
}

func compileRules(rules []Rule, allowedEffects map[effect]bool) ([]compiledRule, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		if err := compileRule(rule, PathPattern{}, PathResource, "", allowedEffects, &compiled); err != nil {
			return nil, err
		}
	}
	names := make(map[string]struct{}, len(compiled))
	for _, rule := range compiled {
		if _, exists := names[rule.name]; exists {
			return nil, fmt.Errorf("access: duplicate rule name %q", rule.name)
		}
		names[rule.name] = struct{}{}
	}
	return compiled, nil
}

func compileRule(
	rule Rule,
	prefix PathPattern,
	resourceKind ResourceKind,
	resourceName string,
	allowedEffects map[effect]bool,
	compiled *[]compiledRule,
) error {
	if (rule.where != nil || rule.check != nil) && rule.kind != directiveRule {
		return fmt.Errorf("access: conditions are only valid on allow rules, not on scopes")
	}
	switch rule.kind {
	case directiveRule:
		if !allowedEffects[rule.effect] {
			return fmt.Errorf("access: effect %q is not valid in this policy", rule.effect)
		}
		if !rule.operations.validSet() {
			return fmt.Errorf("access: rule %q has an invalid operation set", rule.name)
		}
		operations := rule.operations
		var params []string
		if rule.where != nil || rule.check != nil {
			if rule.effect != effectAllow {
				return fmt.Errorf("access: rule %q: conditional %s rules are not supported; conditions apply to allow rules only", rule.name, rule.effect)
			}
			if resourceKind != PathResource {
				return fmt.Errorf("access: rule %q: conditions are not valid on %s rules", rule.name, resourceKind)
			}
			if operations&Truncate != 0 {
				if operations != Write && operations != ReadWrite {
					return fmt.Errorf("access: rule %q: a conditional rule cannot authorise truncate", rule.name)
				}
				operations &^= Truncate
			}
			seen := map[string]struct{}{}
			for slot, condition := range map[string]dal.Condition{"where": rule.where, "check": rule.check} {
				if condition == nil {
					continue
				}
				info, err := condeval.Validate(condition)
				if err != nil {
					return fmt.Errorf("access: rule %q: invalid %s condition: %w", rule.name, slot, err)
				}
				for _, param := range info.Params {
					if _, dup := seen[param]; !dup {
						seen[param] = struct{}{}
						params = append(params, param)
					}
				}
			}
			sort.Strings(params)
			declared := prefix.captures()
			for _, param := range params {
				if !strings.HasPrefix(param, "path.") {
					continue
				}
				if !slices.Contains(declared, strings.TrimPrefix(param, "path.")) {
					return fmt.Errorf("access: rule %q references $%s but its path %s captures no such segment", rule.name, param, prefix)
				}
			}
		}
		name := rule.name
		if name == "" {
			name = fmt.Sprintf("%s %s at %s", rule.effect, operations, resourceDescription(resourceKind, resourceName, prefix))
			if rule.where != nil {
				name += " where " + rule.where.String()
			}
			if rule.check != nil {
				name += " check " + rule.check.String()
			}
		}
		*compiled = append(*compiled, compiledRule{
			kind:       resourceKind,
			pattern:    prefix,
			resource:   resourceName,
			name:       name,
			operations: operations,
			effect:     rule.effect,
			depth:      len(prefix.segments),
			literals:   literalCount(prefix),
			where:      rule.where,
			check:      rule.check,
			params:     params,
		})
		return nil
	case scopeRule:
		if resourceKind != PathResource {
			return fmt.Errorf("access: path scopes cannot be nested inside %s", resourceKind)
		}
		joined := prefix.append(rule.pattern)
		for _, child := range rule.children {
			if err := compileRule(child, joined, PathResource, "", allowedEffects, compiled); err != nil {
				return err
			}
		}
		return nil
	case collectionGroupRule:
		if resourceKind != PathResource || len(prefix.segments) != 0 {
			return fmt.Errorf("access: collection-group rules must be top-level")
		}
		if strings.TrimSpace(rule.resource) == "" {
			return fmt.Errorf("access: collection-group name is required")
		}
		for _, child := range rule.children {
			if err := compileRule(child, PathPattern{}, CollectionGroupResource, rule.resource, allowedEffects, compiled); err != nil {
				return err
			}
		}
		return nil
	case opaqueQueryRule:
		if resourceKind != PathResource || len(prefix.segments) != 0 {
			return fmt.Errorf("access: opaque-query rules must be top-level")
		}
		for _, child := range rule.children {
			if err := compileRule(child, PathPattern{}, OpaqueQueryResource, "", allowedEffects, compiled); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("access: unknown rule kind %d", rule.kind)
	}
}

func literalCount(pattern PathPattern) int {
	count := 0
	for _, segment := range pattern.segments {
		if !segment.anyID {
			count++
		}
	}
	return count
}

func resourceDescription(kind ResourceKind, name string, pattern PathPattern) string {
	switch kind {
	case CollectionGroupResource:
		return "collection-group:" + name
	case OpaqueQueryResource:
		return "opaque-query"
	default:
		return pattern.String()
	}
}
