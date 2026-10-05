package access

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// maxNestingDepth bounds how many times a query with nested queries is routed
// from a secured session to the query engine along one chain of calls. The
// engine runs such a query one source at a time and puts each scan back to the
// session, so a chain is one level deep; the bound allows a few more for
// executors that nest.
const maxNestingDepth = 4

// ErrNestingTooDeep is matched by the error a secured session returns for a
// query with nested queries put to it from inside more than a small number of
// nested routes: the session and the query engine do not re-enter each other
// without bound. The error also matches ErrAccessDenied.
var ErrNestingTooDeep = errors.New("dalgo access: queries nested too deeply")

type nestingKey struct{}

// nestingDepth is the number of nested routes the context has been through.
func nestingDepth(ctx context.Context) int {
	depth, _ := ctx.Value(nestingKey{}).(int)
	return depth
}

// enterNesting returns the context a nested route runs in: the one it was given,
// one level deeper. At the bound it refuses, with an error that matches both
// ErrNestingTooDeep and ErrAccessDenied.
func enterNesting(ctx context.Context, query dal.StructuredQuery) (context.Context, error) {
	depth := nestingDepth(ctx)
	if depth >= maxNestingDepth {
		return ctx, fmt.Errorf("%w: %w", ErrNestingTooDeep, &DeniedError{Decision: Decision{
			Operation:   Query,
			Resource:    resourcesForQuery(query)[0],
			Policy:      "nesting",
			Effect:      effectDeny.String(),
			Code:        CodeEnforcementUnsupported,
			Scope:       DecisionScopeRequest,
			Explanation: fmt.Sprintf("queries nested more than %d levels deep between the secured session and the query engine are not executed", maxNestingDepth),
		}})
	}
	return context.WithValue(ctx, nestingKey{}, depth+1), nil
}

// refuseScanOrderQueries refuses a query on the nested route whose source has a
// scan order that holds a query. The engine scans a source with its scan order as
// the ORDER BY of the scan, so such a scan holds a nested query itself; the
// nested route does not run it.
func refuseScanOrderQueries(query dal.StructuredQuery) error {
	if !scanOrdersHoldQuery(query) {
		return nil
	}
	return &DeniedError{Decision: Decision{
		Operation:   Query,
		Resource:    resourcesForQuery(query)[0],
		Policy:      "scan-orders",
		Effect:      effectDeny.String(),
		Code:        CodeEnforcementUnsupported,
		Scope:       DecisionScopeRequest,
		Explanation: "a scan order that holds a query cannot be executed together with nested queries",
	}}
}
