package access

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// unboundedNesting is what boundedPolicy panics with.
type unboundedNesting struct{ decisions int }

// boundedPolicy counts the requests a secured session puts to it and panics past
// a bound, so a re-entry between the secured session and the query engine that
// never ends is reported as a failure of the test, with the number of requests
// counted.
type boundedPolicy struct {
	Policy
	decisions *int
}

const decisionBound = 200

func (p boundedPolicy) Decide(ctx context.Context, request Request) Decision {
	*p.decisions++
	if *p.decisions > decisionBound {
		panic(unboundedNesting{*p.decisions})
	}
	return p.Policy.Decide(ctx, request)
}

// runBounded runs a query on a secured path over a wrapped session that counts
// reads, and reports the number of requests the policy decided. A path that does
// not stop fails the test.
func runBounded(t *testing.T, run sourcePath, policy Policy, query dal.Query) (decisions, reads int, err error) {
	t.Helper()
	wrapped := &countingSession{}
	defer func() {
		if recovered := recover(); recovered != nil {
			if unbounded, ok := recovered.(unboundedNesting); ok {
				t.Fatalf("the secured session and the query engine re-entered each other %d times without stopping", unbounded.decisions)
			}
			panic(recovered)
		}
	}()
	err = run(context.Background(), wrapped, boundedPolicy{Policy: policy, decisions: &decisions}, query)
	return decisions, wrapped.reads, err
}

// scanOrderQueryShapes are queries on the nested route that hold a query in the
// scan order of a source.
func scanOrderQueryShapes() map[string]func() dal.StructuredQuery {
	scalar := scalarSecret()
	scanned := customers.WithScan(10, dal.Ascending(scalar))
	return map[string]func() dal.StructuredQuery{
		"EXISTS and a scan ordered by a scalar subquery": func() dal.StructuredQuery {
			return dal.From(scanned).NewQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
		},
		"a scan ordered by a scalar subquery": func() dal.StructuredQuery {
			return dal.From(scanned).NewQuery().SelectKeysOnly(reflect.String)
		},
		"a scan ordered by a scalar subquery through a pointer": func() dal.StructuredQuery {
			return dal.From(&scanned).NewQuery().SelectKeysOnly(reflect.String)
		},
		"a scan ordered by arithmetic over a scalar subquery": func() dal.StructuredQuery {
			return dal.From(customers.WithScan(10, dal.Ascending(dal.Binary(dal.Field("a"), dal.Add, scalar)))).NewQuery().SelectKeysOnly(reflect.String)
		},
		"a joined source with a scan ordered by a scalar subquery": func() dal.StructuredQuery {
			return dal.From(customers).Join(dal.NewJoinedSource(orders.WithScan(10, dal.Descending(scalar)), dal.JoinInner, joinOn("c", "o"))).NewQuery().SelectKeysOnly(reflect.String)
		},
		"a derived source over a scan ordered by a scalar subquery": func() dal.StructuredQuery {
			inner := dal.From(scanned).NewQuery().SelectKeysOnly(reflect.String)
			return dal.From(dal.NewQuerySource(inner, "d")).NewQuery().SelectKeysOnly(reflect.String)
		},
		"an EXISTS over a scan ordered by a scalar subquery": func() dal.StructuredQuery {
			inner := dal.From(scanned).NewQuery().SelectKeysOnly(reflect.String)
			return customerQuery().Where(dal.NewExistsCondition(inner)).SelectKeysOnly(reflect.String)
		},
	}
}

func TestScanOrderHoldingAQueryIsRefusedBeforeAnyReadOnEverySecuredReadPath(t *testing.T) {
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			for name, build := range scanOrderQueryShapes() {
				t.Run(name, func(t *testing.T) {
					decisions, reads, err := runBounded(t, run, allowEverything(), build())
					var denied *DeniedError
					if !errors.As(err, &denied) || denied.Decision.Code != CodeEnforcementUnsupported {
						t.Fatalf("error = %v, want an enforcement-unsupported denial", err)
					}
					if reads != 0 {
						t.Fatalf("%d reads reached the wrapped session, want 0", reads)
					}
					if decisions != 1 {
						t.Fatalf("%d requests were put to the policy, want 1: the sources are authorised once and nothing is run", decisions)
					}
				})
				t.Run(name+", a denied collection comes first", func(t *testing.T) {
					_, reads, err := runBounded(t, run, hiddenOnly(), build())
					var denied *DeniedError
					if !errors.As(err, &denied) || denied.Decision.Code == CodeEnforcementUnsupported || denied.Decision.Resource.String() != "/Secret" {
						t.Fatalf("error = %v, want the denial of /Secret", err)
					}
					if reads != 0 {
						t.Fatalf("%d reads reached the wrapped session, want 0", reads)
					}
				})
			}
		})
	}
}

func TestScanOrderThatHoldsNoQueryStaysOnTheDirectRoute(t *testing.T) {
	wrapped := &countingSession{}
	query := dal.From(customers.WithScan(10, dal.AscendingField("a"))).NewQuery().SelectKeysOnly(reflect.String)
	if _, err := SecureReadSession(wrapped, allowEverything()).ExecuteQueryToRecordsReader(context.Background(), query); err != nil {
		t.Fatalf("error = %v", err)
	}
	if wrapped.reads != 1 {
		t.Fatalf("%d reads, want 1", wrapped.reads)
	}
}

func TestScanOrdersHoldQuery(t *testing.T) {
	for name, build := range scanOrderQueryShapes() {
		t.Run(name, func(t *testing.T) {
			if !scanOrdersHoldQuery(build()) {
				t.Fatal("scanOrdersHoldQuery = false, want true")
			}
		})
	}
	holdsNone := map[string]dal.StructuredQuery{
		"no scan order":                     customerQuery().SelectKeysOnly(reflect.String),
		"a scan order over a field":         dal.From(customers.WithScan(5, dal.AscendingField("a"))).NewQuery().SelectKeysOnly(reflect.String),
		"a subquery outside a scan order":   customerQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String),
		"a scalar subquery in the ORDER BY": customerQuery().OrderBy(dal.Ascending(scalarSecret())).SelectKeysOnly(reflect.String),
		"a derived source":                  dal.From(dal.NewQuerySource(secretRows(), "d")).NewQuery().SelectKeysOnly(reflect.String),
	}
	for name, query := range holdsNone {
		t.Run(name, func(t *testing.T) {
			if scanOrdersHoldQuery(query) {
				t.Fatal("scanOrdersHoldQuery = true, want false")
			}
		})
	}
}

// reentrantSession is a wrapped session that answers every read by putting a
// query with a nested query to the secured session again, as an adapter that
// re-enters its caller would. It gives up, with an error of its own, after it has
// been read from more times than the secured session can allow.
type reentrantSession struct {
	countingSession
	secured func() dal.ReadSession
	query   func() dal.StructuredQuery
}

const readGiveUp = 50

func (s *reentrantSession) ExecuteQueryToRecordsReader(ctx context.Context, _ dal.Query) (dal.RecordsReader, error) {
	s.reads++
	if s.reads > readGiveUp {
		return nil, fmt.Errorf("re-entered %d times without stopping", s.reads)
	}
	return s.secured().ExecuteQueryToRecordsReader(ctx, s.query())
}

func TestReentryBetweenTheSecuredSessionAndTheEngineIsBounded(t *testing.T) {
	existsQuery := func() dal.StructuredQuery {
		return customerQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
	}
	paths := map[string]func(context.Context, dal.ReadSession, dal.Query) error{
		"records reader": func(ctx context.Context, session dal.ReadSession, query dal.Query) error {
			_, err := session.ExecuteQueryToRecordsReader(ctx, query)
			return err
		},
		"recordset reader": func(ctx context.Context, session dal.ReadSession, query dal.Query) error {
			_, err := session.ExecuteQueryToRecordsetReader(ctx, query)
			return err
		},
	}
	for path, run := range paths {
		t.Run(path, func(t *testing.T) {
			wrapped := &reentrantSession{query: existsQuery}
			session := SecureReadSession(wrapped, allowEverything())
			wrapped.secured = func() dal.ReadSession { return session }
			err := run(context.Background(), session, existsQuery())
			if !errors.Is(err, ErrNestingTooDeep) || !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, want ErrNestingTooDeep and ErrAccessDenied", err)
			}
			var denied *DeniedError
			if !errors.As(err, &denied) || denied.Decision.Code != CodeEnforcementUnsupported {
				t.Fatalf("error = %v, want an enforcement-unsupported denial", err)
			}
			if wrapped.reads != maxNestingDepth {
				t.Fatalf("%d reads reached the wrapped session, want %d: one for each level the secured session allows", wrapped.reads, maxNestingDepth)
			}
		})
	}
}

func TestNestedRouteIsRefusedBeyondTheBoundBeforeAnyRead(t *testing.T) {
	query := customerQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			for depth, refused := range map[int]bool{0: false, maxNestingDepth - 1: false, maxNestingDepth: true, maxNestingDepth + 3: true} {
				wrapped := &countingSession{}
				ctx := withNestingDepth(context.Background(), depth)
				err := run(ctx, wrapped, allowEverything(), query)
				if refused != errors.Is(err, ErrNestingTooDeep) || errors.Is(err, ErrAccessDenied) != refused {
					t.Fatalf("depth %d: error = %v, want refused = %v", depth, err, refused)
				}
				if refused && wrapped.reads != 0 {
					t.Fatalf("depth %d: %d reads reached the wrapped session, want 0", depth, wrapped.reads)
				}
				if !refused && wrapped.reads == 0 {
					t.Fatalf("depth %d: the query did not run", depth)
				}
			}
		})
	}
}

// withNestingDepth is a context that has already been through depth nested routes.
func withNestingDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, nestingKey{}, depth)
}

func TestEnterNestingCountsEveryLevel(t *testing.T) {
	ctx := context.Background()
	query := secretRows()
	for level := 1; level <= maxNestingDepth; level++ {
		var err error
		ctx, err = enterNesting(ctx, query)
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		if got := nestingDepth(ctx); got != level {
			t.Fatalf("depth after level %d = %d", level, got)
		}
	}
	if _, err := enterNesting(ctx, query); !errors.Is(err, ErrNestingTooDeep) {
		t.Fatalf("error = %v, want ErrNestingTooDeep", err)
	}
	if got := nestingDepth(context.Background()); got != 0 {
		t.Fatalf("depth of a context that was never entered = %d", got)
	}
}
