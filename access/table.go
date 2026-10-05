package access

import (
	"fmt"
	"strings"
)

// TableName says which stored table a source denotes: the database, the schema
// and the name of the table, each as the policy author wrote it, with no SQL
// quoting. A rule that names a table through TableScope, or through the table
// key of a policy document, writes one. The identity of the table a query source
// denotes is read from a resource with Resource.Table.
//
// Name is required. A table rule names a schema, a database, or both; a table
// with neither is an ordinary collection, which Collection and a path scope
// name. A name is a plain name: a dot inside it is part of the name, so the
// schema "a" and the name "b.c" are not the schema "a.b" and the name "c".
type TableName struct {
	Database string `json:"database,omitempty" yaml:"database,omitempty"`
	Schema   string `json:"schema,omitempty" yaml:"schema,omitempty"`
	Name     string `json:"name" yaml:"name"`
}

// Table returns the identity of the table the resource's source denotes, and
// false when the resource carries none. A source that writes a schema or a
// database carries one: the parts it wrote, as written. The kind and the text of
// the resource are the same as they are for a source that carries no identity, so
// a policy that does not read the identity decides as it always did. A resource
// built by RecordResourceForKey, CollectionResourceFor, CollectionGroup or
// OpaqueQuery, a collection source with neither a schema nor a database, a
// collection-group source and a part of a query that cannot be read carry none.
func (r Resource) Table() (TableName, bool) {
	if r.table == nil {
		return TableName{}, false
	}
	return *r.table, true
}

// TableScope attaches rules to one table, named by its schema, by its database,
// or by both. It is a top-level scope that holds directives only; the rules it
// holds may carry Where, Check, Fields and a field mask, as the rules of a path
// scope do. A condition that uses $path.x is refused, because a table scope
// captures nothing.
//
// A policy that holds a table rule decides a source that writes a schema by its
// table rules alone: a rule for opaque queries is not consulted for it, and a
// source that writes a schema which no table rule names is denied. A policy that
// holds no table rule decides every source as it always did.
func TableScope(table TableName, rules ...Rule) Rule {
	return Rule{kind: tableScopeRule, table: table, children: append([]Rule(nil), rules...)}
}

// tableRuleResource is the kind of a compiled table rule. No resource has it, so
// a table rule never matches a resource by kind; table rules are matched against
// the identity a resource carries.
const tableRuleResource ResourceKind = "table"

func (t TableName) validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("access: table name is required")
	}
	if t.Database == "" && t.Schema == "" {
		return fmt.Errorf("access: a table rule names a schema, a database, or both; a table with neither is named by a path rule")
	}
	for _, part := range []struct{ label, value string }{{"database", t.Database}, {"schema", t.Schema}} {
		if part.value != "" && strings.TrimSpace(part.value) == "" {
			return fmt.Errorf("access: table %s must not be blank", part.label)
		}
	}
	return nil
}

// describe writes the table the way a rule name and an explanation show it. Each
// part is quoted and labelled, so two tables never read alike.
func (t TableName) describe() string {
	var parts []string
	if t.Database != "" {
		parts = append(parts, fmt.Sprintf("database=%q", t.Database))
	}
	if t.Schema != "" {
		parts = append(parts, fmt.Sprintf("schema=%q", t.Schema))
	}
	parts = append(parts, fmt.Sprintf("name=%q", t.Name))
	return strings.Join(parts, " ")
}

// parts is the number of parts a rule names. A rule that names more parts is the
// more specific one.
func (t TableName) parts() int {
	count := 1
	if t.Database != "" {
		count++
	}
	if t.Schema != "" {
		count++
	}
	return count
}

// sameTable reports whether a rule's table is the identity of a source, as a
// session that resolves no name compares them: schema and name are equal byte for
// byte, and neither names a database. A database cannot be checked without help
// from the session, so a rule or a source that names one never matches here.
func (t TableName) sameTable(identity TableName) bool {
	return t.Database == "" && identity.Database == "" && t.Schema == identity.Schema && t.Name == identity.Name
}

// tableVerdict is what the selection of the rules of a policy says besides the
// rules themselves.
type tableVerdict uint8

const (
	// tableDecided: the rules returned decide the resource.
	tableDecided tableVerdict = iota
	// tableNoRule: the source writes a schema, the policy holds table rules and none
	// of them names this table. No rule decides it.
	tableNoRule
	// tableAmbiguous: the resource names a collection that a table rule also names a
	// table of, and says no schema, so one name could denote either table.
	tableAmbiguous
)

// selectRules finds the rules that decide a resource, in precedence order, and
// says whether the policy's table rules leave it undecided. It is the one place
// path rules and table rules meet, for access policies and audit policies alike.
//
//   - A policy that holds no table rule: the rules that match the resource, as
//     they always were.
//   - A resource that carries the identity of a table with a schema: if the rule
//     that would win among the path rules is a deny that names the collection, the
//     path rules are returned, so the deny stands. Otherwise the table rules that
//     match decide; a source that writes a schema is decided by them alone, and a
//     path resource falls back to the path rules when no table rule matches.
//   - Any other resource, when a table rule names a table of the same name as the
//     collection it reads, without regard to case: tableAmbiguous. The path rules
//     are returned with it, for a caller that has nothing to refuse.
func selectRules(rules []compiledRule, operation Operations, resource Resource) ([]compiledRule, tableVerdict) {
	path := matchingRules(rules, operation, resource)
	tables := tableRulesOf(rules)
	if len(tables) == 0 {
		return path, tableDecided
	}
	identity, named := resource.Table()
	if !named || identity.Schema == "" {
		if namesATableOf(tables, resource) {
			return path, tableAmbiguous
		}
		return path, tableDecided
	}
	if resource.kind == PathResource && len(path) > 0 && path[0].depth > 0 && effectIsRestrictive(path[0].effect) {
		return path, tableDecided
	}
	if matches := matchingTableRules(tables, operation, identity); len(matches) > 0 {
		return matches, tableDecided
	}
	if resource.kind == PathResource {
		return path, tableDecided
	}
	return nil, tableNoRule
}

func tableRulesOf(rules []compiledRule) []compiledRule {
	var tables []compiledRule
	for _, rule := range rules {
		if rule.kind == tableRuleResource {
			tables = append(tables, rule)
		}
	}
	return tables
}

func matchingTableRules(tables []compiledRule, operation Operations, identity TableName) []compiledRule {
	matches := make([]compiledRule, 0, len(tables))
	for _, rule := range tables {
		if rule.operations.contains(operation) && rule.table.sameTable(identity) {
			matches = append(matches, rule)
		}
	}
	return sortRules(matches)
}

// namesATableOf reports whether the resource is a root collection, or a record of
// one, that a table rule has a table of the same name as, without regard to case.
func namesATableOf(tables []compiledRule, resource Resource) bool {
	if resource.kind != PathResource || (len(resource.path) != 1 && len(resource.path) != 2) {
		return false
	}
	collection := fmt.Sprint(resource.path[0].value)
	for _, rule := range tables {
		if strings.EqualFold(rule.table.Name, collection) {
			return true
		}
	}
	return false
}

// denyAmbiguousTable is the decision for a resource a table rule makes ambiguous.
func (p *AccessPolicy) denyAmbiguousTable(operation Operations, resource Resource) Decision {
	return Decision{
		Operation:    operation,
		Resource:     resource,
		Policy:       p.name,
		PolicySource: p.source,
		Effect:       effectDeny.String(),
		Code:         CodeEnforcementUnsupported,
		Scope:        DecisionScopeTable,
		Explanation:  "the policy holds a table rule for a table of this name, and this source does not say which schema the name is read from",
	}
}
