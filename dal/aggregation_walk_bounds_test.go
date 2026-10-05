package dal

import (
	"context"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

// walkGraph is a query whose condition or aggregate holds itself. The walks of the
// aggregates of a query end on it as the inspection of the query tree does: a node
// held by pointer that is already on the path is not walked again, and a path of
// more than maxQueryTreeDepth nodes is not followed.
type walkGraph struct {
	name  string
	query StructuredQuery
	// deep marks a graph that holds itself by value: it has no end, so the walk stops
	// at the bound and the query is refused. A graph held by pointer ends where it
	// meets itself, and is read as the query it is.
	deep bool
	// oneSource marks a query over one source.
	oneSource bool
	// validatedOnly marks an aggregate held by value in the columns of a join: validation refuses
	// it as a nested aggregate, and only validation is run on it, as the readers hand it to the scope
	// validation, which follows a graph held by value to no end.
	validatedOnly bool
}

// walkGraphs lists the graphs of TestHasSubqueryStopsAtACycle where an aggregate
// walk reads them: a condition group that holds itself, by pointer and by value, in
// where; the group held by pointer in the having of a join; and an aggregate that
// holds itself, in where and in the columns of a join.
func walkGraphs(visits *int, customers, orders CollectionRef) []walkGraph {
	on := joinOn(customers.Alias(), "id", orders.Alias(), "ref")
	joined := func() FromSource { return From(customers).Join(NewJoinedSource(orders, JoinInner, on)) }
	count := Column{Alias: "n", Expression: Count().Expression}

	byPointer := make([]Condition, 2)
	pointerGroup := NewGroupCondition(Or, byPointer...)
	byPointer[0], byPointer[1] = &pointerGroup, WhereField("a", Equal, 1)

	byValue := make([]Condition, 2)
	valueGroup := NewGroupCondition(Or, byValue...)
	byValue[0], byValue[1] = valueGroup, WhereField("a", Equal, 1)

	selfPointer := &countingAggregate{visits: visits}
	selfPointer.args = []Expression{selfPointer}
	args := make([]Expression, 1)
	selfValue := valueAggregate{args: args, visits: visits}
	args[0] = selfValue

	holding := func(aggregate Expression) Condition { return NewComparison(aggregate, GreaterThen, NewConstant(1)) }
	return []walkGraph{
		{"a condition group that holds itself by pointer, in where", From(customers).NewQuery().Where(pointerGroup).SelectKeysOnly(0), false, true, false},
		{"a condition group that holds itself by value, in where", From(customers).NewQuery().Where(valueGroup).SelectKeysOnly(0), true, true, false},
		{"an aggregate that holds itself by pointer, in where", From(customers).NewQuery().Where(holding(selfPointer)).SelectKeysOnly(0), false, true, false},
		{"an aggregate that holds itself by value, in where", From(customers).NewQuery().Where(holding(selfValue)).SelectKeysOnly(0), true, true, false},
		{"the group held by pointer, in the having of a join", joined().NewQuery().Having(pointerGroup).SelectColumns(count), false, false, false},
		{"an aggregate that holds itself by pointer, in the columns of a join", joined().NewQuery().SelectColumns(Column{Alias: "n", Expression: selfPointer}), false, false, false},
		{"an aggregate that holds itself by value, in the columns of a join", joined().NewQuery().SelectColumns(Column{Alias: "n", Expression: selfValue}), false, false, true},
	}
}

const nestedTooDeep = "too deep to be checked"

// Every call returns on a graph that holds itself. A query that holds itself by value is
// refused; one that holds itself by pointer is read as the query it is.
func TestAggregateWalkEndsOnAGraphThatHoldsItself(t *testing.T) {
	ctx := context.Background()
	visits := 0
	for _, graph := range walkGraphs(&visits, NewRootCollectionRef("Customer", "c"), NewRootCollectionRef("Order", "o")) {
		t.Run(graph.name, func(t *testing.T) {
			visits = 0
			validateErr := ValidateAggregation(graph.query)
			visits = 0
			_, planErr := PlanAggregation(graph.query, orderedCapabilities)
			if graph.validatedOnly {
				// A graph the validation follows to no end is refused as a nested aggregate.
				if validateErr == nil || planErr == nil {
					t.Fatalf("validation errors = %v, %v", validateErr, planErr)
				}
				return
			}
			if graph.deep {
				for _, err := range []error{validateErr, planErr} {
					if err == nil || !strings.Contains(err.Error(), nestedTooDeep) {
						t.Fatalf("validation error = %v", err)
					}
				}
			}
			for label, caps := range map[string]QueryCapabilities{"no capabilities": {}, "capabilities": orderedCapabilities} {
				rows := map[string][]record.Record{"Customer": customerRecords(), "Order": nil}
				stub := &orderedStub{caps: caps, rows: rows}
				db := NewDB(stub)
				visits = 0
				_, recordsErr := db.ExecuteQueryToRecordsReader(ctx, graph.query)
				readsOfRecords := stub.plain
				visits = 0
				_, recordsetErr := db.ExecuteQueryToRecordsetReader(ctx, graph.query)
				if graph.deep {
					for _, err := range []error{recordsErr, recordsetErr} {
						if err == nil || !strings.Contains(err.Error(), nestedTooDeep) {
							t.Fatalf("%s: error = %v", label, err)
						}
					}
					if stub.native+stub.plain+stub.nativeSets != 0 {
						t.Fatalf("%s: %d reads reached the provider, want 0", label, stub.native+stub.plain+stub.nativeSets)
					}
				}
				// A query over one source whose where holds itself by pointer is handed to the provider.
				if graph.oneSource && !graph.deep && (recordsErr != nil || readsOfRecords != 1) {
					t.Fatalf("%s: records reader error = %v, the provider was read %d times, want 1", label, recordsErr, readsOfRecords)
				}
			}
		})
	}
}

// A federated query is held to the same rule before any database is resolved.
func TestFederatedQueryEndsOnAGraphThatHoldsItself(t *testing.T) {
	visits := 0
	for _, graph := range walkGraphs(&visits, NewDatabaseCollectionRef("customers", "", "Customer", "c"), NewDatabaseCollectionRef("orders", "", "Order", "o")) {
		if graph.validatedOnly {
			continue
		}
		t.Run(graph.name, func(t *testing.T) {
			reads, resolved := 0, 0
			resolve := func(context.Context, string) (QueryExecutor, error) {
				resolved++
				return &readCountingExecutor{reads: &reads}, nil
			}
			visits = 0
			_, err := ExecuteFederatedQuery(context.Background(), graph.query, resolve)
			if graph.deep {
				if err == nil || !strings.Contains(err.Error(), nestedTooDeep) || reads != 0 || resolved != 0 {
					t.Fatalf("error = %v, reads = %d, databases resolved = %d", err, reads, resolved)
				}
			} else if graph.oneSource && (err != nil || reads != 1) {
				t.Fatalf("error = %v, reads = %d, want one read", err, reads)
			}
		})
	}
}

// valueFrom is a source tree held by value that holds a copy of itself, through the
// slice of joins its copies share. It has no end, and no address to be known by.
type valueFrom struct {
	base  RecordsetSource
	joins []JoinedSource
}

func (f valueFrom) Base() RecordsetSource        { return f.base }
func (f valueFrom) Join(JoinedSource) FromSource { return f }
func (f valueFrom) NewQuery() *QueryBuilder      { return NewQueryBuilder(f) }
func (f valueFrom) Joins() []JoinedSource        { return f.joins }

// The walks of the sources of a query end on a tree that joins itself. One held by pointer ends
// where it meets itself, and the query is read as the query it is; one held by value is followed
// to the bound of the walk and refused, by every check that walks it.
func TestSourceTreeWalksEndOnATreeThatJoinsItself(t *testing.T) {
	ctx := context.Background()
	customers := NewRootCollectionRef("Customer", "c")
	onSelf := joinOn("c", "id", "c", "id")
	ordered := NewOrderedAggregate(LAST, orderedBy(AscendingField("id")), Field("id"))
	columns := Column{Alias: "v", Expression: ordered}

	t.Run("by pointer", func(t *testing.T) {
		visits := 0
		tree := &countingFrom{base: customers, visits: &visits}
		tree.joins = []JoinedSource{NewNestedJoinedSource(tree, JoinInner, onSelf)}
		q := tree.NewQuery().SelectColumns(columns)
		scope, complete := newOrderedAggregateScope(tree)
		if !complete || len(scope.stored) != 1 || !scope.stored["c"] {
			t.Fatalf("scope = %v, complete = %v", scope, complete)
		}
		if err := validateOrderedAggregateSources(q); err != nil {
			t.Fatal(err)
		}
		if err := validateOrderedAggregatePlacement(q); err != nil {
			t.Fatal(err)
		}
		// A scan order of the source it meets again is found once, and refused.
		scanned := &countingFrom{base: customers.WithScan(5, Ascending(placementOrdered())), visits: &visits}
		scanned.joins = []JoinedSource{NewNestedJoinedSource(scanned, JoinInner, onSelf)}
		if err := validateOrderedAggregatePlacement(scanned.NewQuery().SelectColumns(columns)); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("by value", func(t *testing.T) {
		joins := make([]JoinedSource, 1)
		tree := valueFrom{base: customers, joins: joins}
		joins[0] = NewNestedJoinedSource(tree, JoinInner, onSelf)
		q := tree.NewQuery().SelectColumns(columns)
		if _, complete := newOrderedAggregateScope(tree); complete {
			t.Fatal("the walk of a tree that holds itself by value was complete")
		}
		for name, err := range map[string]error{
			"the scope of the sources":     validateOrderedAggregateSources(q),
			"the placement":                validateOrderedAggregatePlacement(q),
			"ValidateAggregation":          ValidateAggregation(q),
			"the query placement":          validateQueryPlacement(q),
			"the placement of a plain one": validateOrderedAggregatePlacement(tree.NewQuery().SelectKeysOnly(0)),
		} {
			if err == nil || !strings.Contains(err.Error(), nestedTooDeep) {
				t.Fatalf("%s: error = %v", name, err)
			}
		}
		stub := &orderedStub{rows: map[string][]record.Record{"Customer": customerRecords()}}
		db := NewDB(stub)
		_, recordsErr := db.ExecuteQueryToRecordsReader(ctx, q)
		_, recordsetErr := db.ExecuteQueryToRecordsetReader(ctx, q)
		_, planErr := PlanAggregation(q, orderedCapabilities)
		for name, err := range map[string]error{"records reader": recordsErr, "recordset reader": recordsetErr, "PlanAggregation": planErr} {
			if err == nil || !strings.Contains(err.Error(), nestedTooDeep) {
				t.Fatalf("%s: error = %v", name, err)
			}
		}
		if stub.readsReached() != 0 {
			t.Fatalf("reads = %d, want none", stub.readsReached())
		}
		// A federated query is refused before any database is resolved.
		federated := NewDatabaseCollectionRef("customers", "", "Customer", "c")
		federatedJoins := make([]JoinedSource, 1)
		federatedTree := valueFrom{base: federated, joins: federatedJoins}
		federatedJoins[0] = NewNestedJoinedSource(federatedTree, JoinInner, onSelf)
		resolved := 0
		resolve := func(context.Context, string) (QueryExecutor, error) {
			resolved++
			return &readCountingExecutor{reads: new(int)}, nil
		}
		if _, err := ExecuteFederatedQuery(ctx, federatedTree.NewQuery().SelectColumns(columns), resolve); err == nil || !strings.Contains(err.Error(), nestedTooDeep) || resolved != 0 {
			t.Fatalf("federated: error = %v, databases resolved = %d", err, resolved)
		}
	})
}

// A query whose aggregates the walk could not follow to their end is not run natively by a
// provider, and is not handed to one.
func TestAggregateWalkThatIsCutDecidesAgainstARunByTheProvider(t *testing.T) {
	visits := 0
	args := make([]Expression, 1)
	deepAggregate := valueAggregate{args: args, visits: &visits}
	args[0] = deepAggregate
	q := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Alias: "n", Expression: deepAggregate})
	// Each walk counts its own visits, as the walk of one query does.
	if visits = 0; walkAggregates(q, func(AggregateFunc) {}) {
		t.Fatal("the walk of an aggregate that holds itself by value was complete")
	}
	visits = 0
	if err := validateOrderedAggregatesForProvider(q); err == nil || !strings.Contains(err.Error(), nestedTooDeep) {
		t.Fatalf("a query the walk could not follow to its end was handed to a provider: error = %v", err)
	}
	if visits = 0; providerRunsOrderedAggregates(q, orderedCapabilities) {
		t.Fatal("a provider was said to run the aggregates of a query the walk could not follow to its end")
	}
	if visits = 0; nativeAggregationSupported(q, QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}) {
		t.Fatal("a provider was said to run the aggregation of a query the walk could not follow to its end")
	}
	// The same aggregate, finite, is run.
	finite := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Alias: "n", Expression: Count().Expression})
	if holdsOrderedAggregate(finite) || !providerRunsOrderedAggregates(finite, QueryCapabilities{}) || !nativeAggregationSupported(finite, QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}) {
		t.Fatal("a finite query was decided against")
	}
}
