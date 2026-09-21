package dal

import (
	"fmt"
)

// QuerySource is a derived relation whose rows are produced by Query. Its name
// and alias are the query result name, so it can be used anywhere a
// RecordsetSource is accepted without extending that interface.
type QuerySource struct {
	query StructuredQuery
	as    string
}

var _ RecordsetSource = QuerySource{}

// NewQuerySource creates a derived relation named as. The query is retained as
// supplied so planners can bind and execute it in the surrounding scope.
func NewQuerySource(query StructuredQuery, as string) QuerySource {
	return QuerySource{query: query, as: as}
}

func (s QuerySource) Name() string     { return s.as }
func (s QuerySource) Alias() string    { return s.as }
func (s QuerySource) recordsetSource() {}

// Query returns the query that produces this relation.
func (s QuerySource) Query() StructuredQuery { return s.query }

// QueryExpression is a scalar subquery expression. Execution validates its
// one-column, at-most-one-row cardinality; the model intentionally does not
// choose a value for multi-row results.
type QueryExpression struct {
	query StructuredQuery
	as    string
}

var _ Expression = QueryExpression{}

// NewQueryExpression creates a scalar subquery expression with result name as.
func NewQueryExpression(query StructuredQuery, as string) QueryExpression {
	return QueryExpression{query: query, as: as}
}

func (e QueryExpression) Query() StructuredQuery { return e.query }
func (e QueryExpression) As() string             { return e.as }
func (e QueryExpression) String() string {
	if e.as == "" {
		return "(SUBQUERY)"
	}
	return "(SUBQUERY AS " + e.as + ")"
}

// ExistsCondition tests whether a nested query produces any row. Negated
// distinguishes NOT EXISTS while keeping Condition's existing method set.
type ExistsCondition struct {
	query   StructuredQuery
	negated bool
}

var _ Condition = ExistsCondition{}

// NewExistsCondition creates an EXISTS predicate.
func NewExistsCondition(query StructuredQuery) ExistsCondition {
	return ExistsCondition{query: query}
}

// NewNotExistsCondition creates a NOT EXISTS predicate.
func NewNotExistsCondition(query StructuredQuery) ExistsCondition {
	return ExistsCondition{query: query, negated: true}
}

func (c ExistsCondition) Query() StructuredQuery { return c.query }
func (c ExistsCondition) Negated() bool          { return c.negated }
func (c ExistsCondition) String() string {
	if c.negated {
		return "NOT EXISTS (SUBQUERY)"
	}
	return "EXISTS (SUBQUERY)"
}

// HasSubquery reports whether q contains any derived source, scalar query,
// EXISTS predicate, or query-valued comparison operand. It is safe for the
// pointer-backed recursive graphs callers can construct with these nodes.
func HasSubquery(q StructuredQuery) bool {
	return inspectQueryTree(q, func(StructuredQuery) bool { return true })
}

// QueryValidationError identifies a structural or lexical problem in a
// recursive query. Field existence and unqualified-field ambiguity require
// schema metadata and are deliberately left to schema-aware execution.
type QueryValidationError struct {
	Category string
	Path     string
	Message  string
}

// RecursiveQueryStrategy records the execution boundary chosen for a query
// containing recursive nodes. Providers do not receive recursive DTQL as a
// native query unless a future capability explicitly supports it.
type RecursiveQueryStrategy string

const RecursiveQueryGeneric RecursiveQueryStrategy = "generic"

// RecursiveQueryPlan is the conservative plan exposed before recursive
// execution begins. Generic execution obtains each leaf through QueryExecutor.
type RecursiveQueryPlan struct {
	Strategy RecursiveQueryStrategy
	Reason   string
}

// PlanRecursiveQuery validates the portable graph and selects the only
// currently supported strategy for nested queries.
func PlanRecursiveQuery(q StructuredQuery) (RecursiveQueryPlan, error) {
	if err := ValidateQueryScope(q); err != nil {
		return RecursiveQueryPlan{}, err
	}
	return RecursiveQueryPlan{Strategy: RecursiveQueryGeneric, Reason: "nested queries are evaluated through bounded ordinary leaf scans"}, nil
}

func (e *QueryValidationError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
	return fmt.Sprintf("%s at %s: %s", e.Category, e.Path, e.Message)
}

// ValidateQueryScope validates the recursive query graph and lexical source
// qualifiers. It does not infer fields from absent schema metadata.
func ValidateQueryScope(q StructuredQuery) error {
	return validateQueryScope(q, nil, "", map[uintptr]bool{})
}
