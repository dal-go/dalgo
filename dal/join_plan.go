package dal

import (
	"context"
	"fmt"
)

// NativeJoinProvider is an optional, query-specific promise that the adapter
// can execute the entire relation tree with DALgo's JOIN semantics. The query
// retains every ordered per-edge JoinedSource.Algorithms hint unchanged; a
// native dialect documents which applicable preferences it honors. A non-nil
// error declines pushdown; DALgo then attempts a bounded generic plan.
// Human reason from the approved brief: “Not every adapter supports native
// JOINs—especially Firestore, document/key-value stores, HTTP/API sources,
// etc. JOIN support cannot simply be implemented in SQL adapters.”
type NativeJoinProvider interface {
	CanExecuteJoin(context.Context, StructuredQuery) error
}

// JoinFieldsProvider optionally supplies schema-ordered fields for wildcard
// expansion. A generic JOIN rejects a wildcard when this metadata is absent.
// The approved Feature requires wildcard expansion in schema order and an
// explicit join_plan error when a source cannot supply that schema.
type JoinFieldsProvider interface {
	JoinFields(ctx context.Context, source RecordsetSource) ([]string, error)
}

type JoinStrategy string

const (
	JoinNative  JoinStrategy = "native"
	JoinGeneric JoinStrategy = "dalgo-generic"
)

type JoinPlan struct {
	Strategy JoinStrategy
	Reason   string
}

// PlanJoin validates the tree and selects complete native pushdown or generic
// execution. Mixed plans are reserved; no subtree is sent to an adapter after
// generic execution has begun.
func PlanJoin(ctx context.Context, q StructuredQuery, provider NativeJoinProvider) (JoinPlan, error) {
	if q == nil || q.From() == nil {
		return JoinPlan{}, joinError("join_shape", "from", "from is required")
	}
	if err := ValidateJoinTree(q.From()); err != nil {
		return JoinPlan{}, err
	}
	if len(q.From().Joins()) == 0 {
		return JoinPlan{Strategy: JoinNative, Reason: "single source"}, nil
	}
	if provider != nil {
		if err := provider.CanExecuteJoin(ctx, q); err == nil {
			return JoinPlan{Strategy: JoinNative, Reason: "adapter accepted complete JOIN tree"}, nil
		} else {
			return JoinPlan{Strategy: JoinGeneric, Reason: fmt.Sprintf("native JOIN declined: %v", err)}, nil
		}
	}
	return JoinPlan{Strategy: JoinGeneric, Reason: "adapter has no native JOIN capability"}, nil
}

func hasJoin(query Query) bool {
	q, ok := query.(StructuredQuery)
	return ok && q.From() != nil && len(q.From().Joins()) > 0
}
