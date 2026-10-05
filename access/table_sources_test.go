package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// tableRulePolicy allows every name through a rule at the root and holds one table
// rule, so a source is refused only because a table rule makes it so.
func tableRulePolicy(extra ...Rule) Policy {
	rules := append([]Rule{Root(Allow(Query, "root")), TableScope(tableSalesNamed, Allow(Query, "sales-customers"))}, extra...)
	return MustPolicy("tables", rules...)
}

// requireRefusedBeforeAnyRead runs a query on one secured read path and requires an
// access denial with no read reaching the wrapped session.
func requireRefusedBeforeAnyRead(t *testing.T, run sourcePath, policy Policy, query dal.Query, message string) {
	t.Helper()
	wrapped := &countingSession{}
	err := run(context.Background(), wrapped, policy, query)
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("error = %v, want an access denial", err)
	}
	if message != "" && !strings.Contains(err.Error(), message) {
		t.Fatalf("error = %v, want it to say %q", err, message)
	}
	if wrapped.reads != 0 {
		t.Fatalf("%d reads reached the wrapped session, want 0", wrapped.reads)
	}
}

// Rules 3, 4 and 5 of the refusals: on every secured read path, wherever the source
// stands in the query, the source is refused and nothing is read.
func TestTableRulesRefuseTheSourcesTheyLeaveUndecidedOnEverySecuredReadPath(t *testing.T) {
	refused := map[string]dal.RecordsetSource{
		"a schema no table rule names":                         salesOrders,
		"the schema of a table rule, another table":            dal.NewQualifiedRootCollectionRef("sales", "customer", ""),
		"another schema":                                       publicCustomer,
		"the name of a table rule, with no schema":             plainCustomer,
		"the name of a table rule in another case, no schema":  dal.NewRootCollectionRef("CUSTOMER", ""),
		"the name of a table rule, with no schema, by pointer": &plainCustomer,
		"a database and a schema":                              appSalesCustomer,
		"a database and a schema, by pointer":                  &appSalesCustomer,
		"a database and no schema":                             appCustomer,
	}
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			for name, source := range refused {
				t.Run(name, func(t *testing.T) {
					for _, position := range matrixPositions() {
						requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), position.build(source), "")
					}
				})
			}
		})
	}
}

func TestTableRuleAllowsItsTableOnEverySecuredReadPath(t *testing.T) {
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			for _, position := range matrixPositions() {
				err := run(context.Background(), &countingSession{}, tableRulePolicy(), position.build(salesCustomer))
				if errors.Is(err, ErrAccessDenied) {
					t.Fatalf("%s: error = %v, want no access denial", position.name, err)
				}
			}
			// A name that no table rule has is allowed by the rule at the root.
			err := run(context.Background(), &countingSession{}, tableRulePolicy(), dal.From(plainOrders).NewQuery().SelectKeysOnly(reflect.String))
			if err != nil {
				t.Fatalf("a name no table rule has: error = %v", err)
			}
		})
	}
}

// Rule 6: the refusals that exist without table rules hold beside them.
func TestExistingRefusalsHoldBesideTableRulesOnEverySecuredReadPath(t *testing.T) {
	line := dal.NewRootCollectionRef("Line", "l")
	joined := func(source dal.RecordsetSource) dal.StructuredQuery {
		return dal.From(orderOrderSource()).Join(dal.NewJoinedSource(source, dal.JoinInner, joinOn("o", "l"))).NewQuery().SelectKeysOnly(reflect.String)
	}
	condition := dal.WhereField("country", dal.Equal, "GB")
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			// A field list on a joined source, whether a path rule or a table rule has it.
			requireRefusedBeforeAnyRead(t, run, tableRulePolicy(Collection("Line", Allow(Query, "line-fields").Fields("id"))), joined(line), "field rules on a joined source")
			fieldsOnTable := MustPolicy("p", Root(Allow(Query, "root")), TableScope(tableSalesNamed, Allow(Query, "t").Fields("id")))
			requireRefusedBeforeAnyRead(t, run, fieldsOnTable, joined(salesCustomer), "field rules on a joined source")
			// A row condition on a joined source, whether a path rule or a table rule has it.
			requireRefusedBeforeAnyRead(t, run, tableRulePolicy(Collection("Line", Allow(Query, "line-where").Where(condition))), joined(line), "joined source")
			whereOnTable := MustPolicy("p", Root(Allow(Query, "root")), TableScope(tableSalesNamed, Allow(Query, "t").Where(condition)))
			requireRefusedBeforeAnyRead(t, run, whereOnTable, joined(salesCustomer), "joined source")
			// A text query is opaque, and a rule for opaque queries is what allows it.
			requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), opaqueQ{}, "")
			opaqueAllowed := MustPolicy("p", OpaqueQueryScope(Allow(Query, "opaque")), TableScope(tableSalesNamed, Allow(Query, "t")))
			if err := run(context.Background(), &countingSession{}, opaqueAllowed, opaqueQ{}); err != nil {
				t.Fatalf("text query with a rule for opaque queries: error = %v", err)
			}
			// A part of the query the walk cannot read is opaque, and no table rule allows it.
			unreadable := dal.From(plainOrders).NewQuery().Where(strangeNode{}).SelectKeysOnly(reflect.String)
			requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), unreadable, "")
			// A collection mask denies a source before any rule is consulted.
			requireRefusedBeforeAnyRead(t, run, maskedSalesPolicy(), tableRead(salesCustomer), "collection mask")
			// A scan order that holds a query is refused after the sources are authorised.
			for name, build := range scanOrderQueryShapes() {
				decisions, reads, err := runBounded(t, run, tableRulePolicy(), build())
				var denied *DeniedError
				if !errors.As(err, &denied) || denied.Decision.Code != CodeEnforcementUnsupported || reads != 0 || decisions != 1 {
					t.Fatalf("%s: error = %v, reads = %d, decisions = %d; want an enforcement-unsupported denial after one decision and no read", name, err, reads, decisions)
				}
			}
		})
	}
}

// maskedSalesPolicy is a policy with a table rule for sales.Customer and a mask
// that admits only the names of the other tables.
func maskedSalesPolicy() Policy {
	policy := MustPolicy("p", Root(Allow(Query, "root")), TableScope(tableSalesNamed, Allow(Query, "t")))
	mask, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"Order", "Line"}}}}, NameMask)
	if err != nil {
		panic(err)
	}
	policy.collectionMask = mask
	return policy
}

// A table rule is also decided on the paths that read through a transaction of a
// database made by dal.NewDB: the one case of the table above that shows it.
func TestTableRuleDecidesInsideBothKindsOfTransaction(t *testing.T) {
	for _, path := range []string{"read transaction records reader", "read-write transaction records reader", "read transaction select", "read-write transaction select"} {
		run := sourcePaths[path]
		requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), tableRead(salesOrders), "")
		requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), tableRead(plainCustomer), "")
		wrapped := &countingSession{}
		if err := run(context.Background(), wrapped, tableRulePolicy(), tableRead(salesCustomer)); err != nil || wrapped.reads != 1 {
			t.Fatalf("%s: error = %v, reads = %d; want the table's own read", path, err, wrapped.reads)
		}
	}
}
