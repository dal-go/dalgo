package dal

import (
	"context"
	"encoding/json"
	"math"
	"strings"
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

func TestOrderedAggregateMoneyOrdersDecimalKeysNumerically(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("rank")), Field("amount"))},
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("rank")), Field("amount"))},
	)
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	for _, tc := range []struct {
		name, lower, higher string
	}{
		{"whole numbers", "2", "10"},
		{"negative numbers", "-10", "-2"},
		{"fractions", "0.09", "0.1"},
		{"exponents", "2e1", "1e2"},
		{"beyond float precision", "9007199254740992", "9007199254740993"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lower := aggregationCoverageRecord("1", map[string]any{"rank": json.Number(tc.lower), "amount": json.Number("1.01")})
			higher := aggregationCoverageRecord("2", map[string]any{"rank": json.Number(tc.higher), "amount": json.Number("2.02")})
			for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
				for _, records := range [][]record.Record{{lower, higher}, {higher, lower}} {
					reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: records}, AggregationPlan{Strategy: strategy})
					reader.money = config
					row, err := reader.Next()
					if err != nil {
						t.Fatalf("strategy=%v: %v", strategy, err)
					}
					got := row.Data().(map[string]any)
					if got["first"] != json.Number("1.01") || got["last"] != json.Number("2.02") {
						t.Fatalf("strategy=%v lower=%s higher=%s row=%#v", strategy, tc.lower, tc.higher, got)
					}
				}
			}
		})
	}
	for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
		reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: []record.Record{
			aggregationCoverageRecord("3", map[string]any{"rank": 1.5, "amount": json.Number("3.03")}),
		}}, AggregationPlan{Strategy: strategy})
		reader.money = config
		if _, err := reader.Next(); err == nil {
			t.Fatalf("strategy=%v accepted fractional binary float order key", strategy)
		}
	}
}

func TestOrderedAggregateMoneyOrdersDecimalTextKeysNumerically(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("rank")), Field("amount"))},
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("rank")), Field("amount"))},
	)
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	for _, tc := range []struct {
		name         string
		lower, upper any
	}{
		{"text keys", "2", "10"},
		{"text then number", "2", json.Number("10")},
		{"number then text", json.Number("2"), "10"},
		{"fractional text keys", "0.09", "0.1"},
		{"precision beyond float64", "9007199254740992", "9007199254740993"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lower := aggregationCoverageRecord("1", map[string]any{"rank": tc.lower, "amount": json.Number("1.01")})
			upper := aggregationCoverageRecord("2", map[string]any{"rank": tc.upper, "amount": json.Number("2.02")})
			for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
				for _, records := range [][]record.Record{{lower, upper}, {upper, lower}} {
					reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: records}, AggregationPlan{Strategy: strategy})
					reader.money = config
					row, err := reader.Next()
					if err != nil {
						t.Fatalf("strategy=%v: %v", strategy, err)
					}
					got := row.Data().(map[string]any)
					if got["first"] != json.Number("1.01") || got["last"] != json.Number("2.02") {
						t.Fatalf("strategy=%v lower=%v upper=%v row=%#v", strategy, tc.lower, tc.upper, got)
					}
				}
			}
		})
	}
}

func TestOrderedAggregateMoneyOrdersDecimalTextTiesNumerically(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectColumns(
		Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField("rank")), Field("amount"))},
		Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("rank")), Field("amount"))},
	)
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	lower := aggregationCoverageRecord("1", map[string]any{"rank": "same", "amount": "2"})
	upper := aggregationCoverageRecord("2", map[string]any{"rank": "same", "amount": "10"})
	for _, strategy := range []AggregationStrategy{AggregationHash, AggregationStreaming} {
		for _, records := range [][]record.Record{{lower, upper}, {upper, lower}} {
			reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: records}, AggregationPlan{Strategy: strategy})
			reader.money = config
			row, err := reader.Next()
			if err != nil {
				t.Fatalf("strategy=%v: %v", strategy, err)
			}
			got := row.Data().(map[string]any)
			if got["first"] != "2" || got["last"] != "10" {
				t.Fatalf("strategy=%v row=%#v", strategy, got)
			}
		}
	}
}

func TestOrderedAggregateMoneyComparesKeysTiesAndAnswersExactly(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b any
		want int
	}{
		{"decimal strings", "2", "10", -1},
		{"money text forms", "+0002", "10.", -1},
		{"fractional text", ".09", "0.1", -1},
		{"mixed exact values", json.Number("9007199254740993"), "9007199254740992", 1},
		{"numeric before other text", "2", "word", -1},
		{"other text after numeric", "word", "2", 1},
		{"other text remains lexical", "word", "zebra", -1},
		{"exponent text remains lexical", "2e1", "10", 1},
		{"over-bound text remains lexical", strings.Repeat("9", 39), "1" + strings.Repeat("0", 39), 1},
		{"null remains first", nil, "2", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareOrderedValuesForMode(tc.a, tc.b, true); got != tc.want {
				t.Fatalf("Money comparison of %v and %v = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
	if got := compareOrderedValuesForMode("2", "10", false); got <= 0 {
		t.Fatalf("ordinary string order changed: %d", got)
	}
	if got := compareOrderedTuplesForMode(true, nil, nil, "same", "2", nil, "same", "10"); got >= 0 {
		t.Fatalf("Money answer comparison = %d, want negative", got)
	}
}

func TestOrderedDecimalComparisonKeepsPrecisionAndScale(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b any
		want int
	}{
		{"whole", json.Number("2"), json.Number("10"), -1},
		{"negative", json.Number("-10"), json.Number("-2"), -1},
		{"negative before positive", json.Number("-1"), json.Number("1"), -1},
		{"positive after negative", json.Number("1"), json.Number("-1"), 1},
		{"fraction", json.Number("0.09"), json.Number("0.1"), -1},
		{"fraction reverse", json.Number("1.03"), json.Number("1.02"), 1},
		{"trailing zero", json.Number("1.20"), json.Number("1.2"), 0},
		{"missing right digit", json.Number("1.201"), json.Number("1.2"), 1},
		{"missing left digit", json.Number("1.2"), json.Number("1.201"), -1},
		{"exponent", json.Number("2e1"), json.Number("1e2"), -1},
		{"negative exponent", json.Number("1e-3"), json.Number("9e-4"), 1},
		{"large exponent", json.Number("1e99999999999999999999"), json.Number("9e99999999999999999998"), 1},
		{"signed zero", json.Number("-0e10"), json.Number("0.0"), 0},
		{"mixed integer", json.Number("10"), 2, 1},
		{"mixed large integer equal", json.Number("9007199254740993"), int64(9007199254740993), 0},
		{"mixed large integer greater", json.Number("9007199254740993"), int64(9007199254740992), 1},
		{"mixed unsigned maximum", json.Number("18446744073709551615"), ^uint64(0), 0},
		{"mixed float", json.Number("0.1"), float64(0.1), 0},
		{"numeric and null", json.Number("1"), nil, 1},
		{"invalid number falls back", json.Number("bad"), json.Number("z"), -1},
		{"number with whitespace falls back", json.Number("1 "), json.Number("1"), 1},
		{"empty number falls back", json.Number(""), json.Number("1"), -1},
		{"JSON literal falls back", json.Number("true"), json.Number("1"), 1},
		{"non numeric value falls back", json.Number("1"), false, 1},
		{"nonfinite float falls back", json.Number("1"), math.NaN(), 1},
		{"infinite float falls back", json.Number("1"), math.Inf(1), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compareOrderedValues(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("compareOrderedValues(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
