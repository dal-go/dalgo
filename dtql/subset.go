package dtql

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// checkInScope classifies a dal.StructuredQuery as in-scope for DTQL or returns
// a descriptive error naming the first unsupported construct it finds. It is the
// single enforcement point of the lossless guarantee: serialize gates on it, so
// no document is ever produced for an out-of-scope query.
func checkInScope(q dal.StructuredQuery) error {
	if q == nil {
		return fmt.Errorf("query is nil")
	}
	if err := checkFrom(q.From()); err != nil {
		return err
	}
	if len(q.GroupBy()) > 0 {
		return fmt.Errorf("GroupBy is not supported by DTQL")
	}
	if q.StartFrom() != "" {
		return fmt.Errorf("cursor (StartFrom) is not supported by DTQL")
	}
	for i, col := range q.Columns() {
		if err := checkExpr(col.Expression); err != nil {
			return fmt.Errorf("column #%d: %w", i, err)
		}
	}
	if err := checkCond(q.Where()); err != nil {
		return err
	}
	for i, o := range q.OrderBy() {
		if err := checkExpr(o.Expression()); err != nil {
			return fmt.Errorf("orderBy #%d: %w", i, err)
		}
	}
	return nil
}

// checkFrom verifies the From source is a single root CollectionRef with no joins.
func checkFrom(from dal.FromSource) error {
	if from == nil {
		return fmt.Errorf("query has no From source")
	}
	if len(from.Joins()) > 0 {
		return fmt.Errorf("joins are not supported by DTQL")
	}
	switch base := from.Base().(type) {
	case dal.CollectionRef:
		return checkRootCollection(base)
	case *dal.CollectionRef:
		if base == nil {
			return fmt.Errorf("query has no From source")
		}
		return checkRootCollection(*base)
	case dal.CollectionGroupRef, *dal.CollectionGroupRef:
		return fmt.Errorf("collection group references are not supported by DTQL")
	default:
		return fmt.Errorf("unsupported From source %T (only root collection references are supported)", base)
	}
}

func checkRootCollection(c dal.CollectionRef) error {
	if c.Parent() != nil {
		return fmt.Errorf("parented collection reference %q is not supported by DTQL (only root collections)", c.Path())
	}
	return nil
}

// checkExpr verifies an expression is an in-scope FieldRef, Constant or Array.
func checkExpr(expr dal.Expression) error {
	switch expr.(type) {
	case dal.FieldRef, *dal.FieldRef:
		return nil
	case dal.Constant, *dal.Constant:
		return nil
	case dal.Array, *dal.Array:
		return nil
	case nil:
		return fmt.Errorf("missing expression")
	default:
		return fmt.Errorf("unsupported expression %T (only field references, constants and arrays are supported)", expr)
	}
}

// checkCond verifies a condition tree uses only in-scope comparisons and groups.
func checkCond(cond dal.Condition) error {
	switch c := cond.(type) {
	case nil:
		return nil
	case dal.Comparison:
		return checkComparison(c)
	case *dal.Comparison:
		return checkComparison(*c)
	case dal.GroupCondition:
		return checkGroup(c)
	case *dal.GroupCondition:
		return checkGroup(*c)
	default:
		return fmt.Errorf("unsupported condition %T", cond)
	}
}

func checkComparison(c dal.Comparison) error {
	if !inScopeComparisonOps[c.Operator] {
		return fmt.Errorf("unsupported comparison operator %q", c.Operator)
	}
	if err := checkExpr(c.Left); err != nil {
		return fmt.Errorf("comparison left: %w", err)
	}
	if err := checkExpr(c.Right); err != nil {
		return fmt.Errorf("comparison right: %w", err)
	}
	return nil
}

func checkGroup(g dal.GroupCondition) error {
	if !inScopeGroupOps[g.Operator()] {
		return fmt.Errorf("unsupported group operator %q", g.Operator())
	}
	for i, sub := range g.Conditions() {
		if err := checkCond(sub); err != nil {
			return fmt.Errorf("group condition #%d: %w", i, err)
		}
	}
	return nil
}
