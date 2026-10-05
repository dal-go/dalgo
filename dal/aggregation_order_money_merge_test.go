package dal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dal-go/record"
)

func TestOrderedAggregateMoneyKeepsExactValuesAndTimestampOrder(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("at")), Field("amount"))},
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("at")), Field("amount"))},
	)
	early := aggregationCoverageRecord("1", map[string]any{
		"at":     time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("plus2", 2*3600)),
		"amount": json.Number("9007199254740993.01"),
	})
	late := aggregationCoverageRecord("2", map[string]any{
		"at":     time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		"amount": json.Number("9007199254740992.01"),
	})
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
		for _, records := range [][]record.Record{{late, early}, {early, late}} {
			reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: records}, AggregationPlan{Strategy: strategy})
			reader.money = config
			row, err := reader.Next()
			if err != nil {
				t.Fatalf("strategy=%v: %v", strategy, err)
			}
			got := row.Data().(map[string]any)
			if got["first"] != json.Number("9007199254740993.01") || got["last"] != json.Number("9007199254740992.01") {
				t.Fatalf("strategy=%v row=%#v", strategy, got)
			}
		}
	}

	for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
		reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: []record.Record{
			aggregationCoverageRecord("3", map[string]any{"at": time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC), "amount": 1.5}),
		}}, AggregationPlan{Strategy: strategy})
		reader.money = config
		if _, err := reader.Next(); err == nil {
			t.Fatalf("strategy=%v accepted fractional binary float", strategy)
		}
	}
}
