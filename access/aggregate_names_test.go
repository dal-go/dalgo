package access

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestKnownAggregateNamesKeepOrdinaryReadCompatibility(t *testing.T) {
	for _, name := range []string{dal.COUNT, dal.SUM, dal.AVERAGE, dal.MIN, dal.MAX, dal.FIRST, dal.LAST} {
		for _, aggregate := range []dal.Expression{
			dal.NewAggregate(name, false, dal.Field("id")),
			customAggregate{name: strings.ToLower(name), args: []dal.Expression{dal.Field("id")}},
			&customAggregate{name: name[:1] + strings.ToLower(name[1:]), args: []dal.Expression{dal.Field("id")}},
		} {
			query := customerQuery().SelectColumns(dal.Column{Expression: aggregate})
			if got := resourceNames(resourcesForQuery(query)); !slices.Equal(got, []string{"/Customer"}) {
				t.Fatalf("known %s: resources = %v", name, got)
			}
			for _, path := range []string{"session records reader", "session recordset reader"} {
				wrapped := &countingSession{}
				if err := sourcePaths[path](context.Background(), wrapped, allowEverything(), query); err != nil || wrapped.reads != 1 {
					t.Fatalf("%s, known %s: error=%v reads=%d", path, name, err, wrapped.reads)
				}
			}
		}
	}
}

func TestUnknownAggregateNamesNeedExplicitOpaqueGrantBeforeEveryReadPath(t *testing.T) {
	for _, name := range []string{"UNKNOWN_PROOF_FUNCTION", "CuStOm", "SUM ", ""} {
		for _, aggregate := range []dal.Expression{
			dal.NewAggregate(name, false, dal.Field("id")),
			customAggregate{name: name, args: []dal.Expression{dal.Field("id")}},
			&customAggregate{name: name, args: []dal.Expression{dal.Field("id")}},
		} {
			query := customerQuery().SelectColumns(dal.Column{Expression: aggregate})
			for path, run := range sourcePaths {
				t.Run(name+"/"+path, func(t *testing.T) {
					wrapped := &countingSession{}
					err := run(context.Background(), wrapped, allowEverything(), query)
					if !errors.Is(err, ErrAccessDenied) || wrapped.reads != 0 {
						t.Fatalf("without opaque grant: error=%v reads=%d", err, wrapped.reads)
					}
					opaque := MustPolicy("explicit-opaque", Root(Allow(Query, "ordinary")), OpaqueQueryScope(Allow(Query, "opaque")))
					if err := run(context.Background(), wrapped, opaque, query); err != nil || wrapped.reads != 1 {
						t.Fatalf("explicit opaque grant: error=%v reads=%d", err, wrapped.reads)
					}
				})
			}
		}
	}
	// A qualified table scope cannot accidentally grant the unknown function.
	query := dal.From(salesCustomer).NewQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate("CUSTOM", false, dal.Field("id"))})
	for path, run := range sourcePaths {
		t.Run("qualified/"+path, func(t *testing.T) {
			requireRefusedBeforeAnyRead(t, run, tableRulePolicy(), query, "")
		})
	}
}

func TestUnknownAggregateStillWalksNestedArgumentsAndOrderedKeys(t *testing.T) {
	scalar := scalarSecret()
	unknown := dal.NewOrderedAggregate("CUSTOM", []dal.OrderExpression{dal.Ascending(scalar)}, scalar)
	query := customerQuery().SelectColumns(dal.Column{Expression: unknown})
	want := []string{"/Customer", "opaque-query:unrecognised aggregate function", "/Secret", "/Secret"}
	if got := resourceNames(resourcesForQuery(query)); !slices.Equal(got, want) {
		t.Fatalf("unknown aggregate hid its argument/order sources: resources=%v, want=%v", got, want)
	}
	// Even explicit opaque permission cannot bypass a source inside an unknown
	// aggregate. Separate argument/order cases prevent one walk hiding another.
	for name, expression := range map[string]dal.Expression{
		"argument": dal.NewAggregate("CUSTOM", false, scalar),
		"order":    dal.NewOrderedAggregate("CUSTOM", []dal.OrderExpression{dal.Ascending(scalar)}, dal.Field("id")),
		"nested":   dal.NewAggregate(dal.SUM, false, dal.NewAggregate("CUSTOM", false, scalar)),
	} {
		query := customerQuery().SelectColumns(dal.Column{Expression: expression})
		policy := MustPolicy("opaque-but-not-secret", Root(Allow(Query, "ordinary")),
			OpaqueQueryScope(Allow(Query, "opaque")), Collection(hiddenCollection, Deny(Query, "secret")))
		for _, path := range []string{"session records reader", "session recordset reader"} {
			t.Run(name+"/"+path, func(t *testing.T) {
				requireRefusedBeforeAnyRead(t, sourcePaths[path], policy, query, "")
			})
		}
	}
}
