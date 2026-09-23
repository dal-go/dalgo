package dal

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"testing"
)

func TestMoneyGoldenCases(t *testing.T) {
	sum := new(big.Int)
	for _, input := range []any{"0.10", "0.20", "9007199254740993"} {
		minor, err := moneyInput(input, 2)
		if err != nil {
			t.Fatal(err)
		}
		sum.Add(sum, minor)
	}
	if got := moneyMinorText(sum, 2); got != "9007199254740993.3" {
		t.Fatalf("sum=%s", got)
	}
	for _, invalid := range []any{"0.001", 0.1} {
		if _, err := moneyInput(invalid, 2); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
	}
	for _, test := range []struct {
		name        string
		left, right any
		op          ArithmeticOperator
		scale       int
		want        any
		wantError   bool
	}{
		{"per capita", "1", 3, Divide, 4, "0.3333", false},
		{"half even down", "1", 8, Divide, 2, "0.12", false},
		{"half even up", "3", 8, Divide, 2, "0.38", false},
		{"null", nil, 3, Divide, 4, nil, false},
		{"zero divisor", "1", 0, Divide, 4, nil, true},
		{"unsafe float", 0.1, "1", Divide, 4, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := moneyPerCapita(test.op, test.left, test.right, test.scale)
			if (err != nil) != test.wantError || !test.wantError && got != test.want {
				t.Fatalf("got=%v err=%v want=%v", got, err, test.want)
			}
		})
	}
}

func TestMoneyInputAndOperandValidation(t *testing.T) {
	for _, value := range []any{json.Number("2"), 2, int8(2), int64(2), uint64(2), float64(2), "2.00", "-0.10"} {
		if _, err := moneyInput(value, 2); err != nil {
			t.Fatalf("input %T %v: %v", value, value, err)
		}
		if _, err := moneyNumber(value); err != nil {
			t.Fatalf("operand %T %v: %v", value, value, err)
		}
	}
	for _, value := range []any{json.Number("9007199254740992"), int64(9007199254740992), float64(9007199254740992), math.NaN(), true, "1e2", "", "0.001", json.Number("0.1")} {
		if _, err := moneyInput(value, 2); err == nil {
			t.Fatalf("accepted input %T %v", value, value)
		}
	}
	for _, value := range []any{json.Number("9007199254740992"), float64(9007199254740992), math.Inf(1), true, "1e2", ""} {
		if _, err := moneyNumber(value); err == nil {
			t.Fatalf("accepted operand %T %v", value, value)
		}
	}
	for _, config := range []*MoneyConfig{nil, {MinorUnitScale: 2, DivisionScale: 4, Rounding: "halfEven"}} {
		if err := validateMoney(config); err != nil {
			t.Fatal(err)
		}
	}
	for _, config := range []*MoneyConfig{{MinorUnitScale: -1, DivisionScale: 4, Rounding: "halfEven"}, {MinorUnitScale: 2, DivisionScale: 19, Rounding: "halfEven"}, {MinorUnitScale: 2, DivisionScale: 4, Rounding: "halfUp"}} {
		if err := validateMoney(config); err == nil {
			t.Fatalf("accepted config %+v", config)
		}
	}
	for _, test := range []struct {
		operator    ArithmeticOperator
		left, right any
	}{
		{Add, "1", "1"}, {Divide, "bad", "1"}, {Divide, "1", "bad"}, {Divide, "1", true},
	} {
		if _, err := moneyPerCapita(test.operator, test.left, test.right, 4); err == nil {
			t.Fatalf("accepted operands %+v", test)
		}
	}
	if got := moneyText(new(big.Rat).SetFrac64(-3, 8), 2); got != "-0.38" {
		t.Fatalf("negative rounding = %s", got)
	}
	if got := moneyText(new(big.Rat).SetInt64(2), 0); got != "2" {
		t.Fatalf("zero scale = %s", got)
	}
}

func TestMoneyExecutionErrorPaths(t *testing.T) {
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 4, Rounding: "halfEven"}
	aggregate := NewAggregate(SUM, false, Field("amount"))
	r := &localAggregationReader{money: config}
	state := &aggregateState{expression: aggregate}
	group := &localGroup{states: map[string]*aggregateState{aggregate.String(): state}}
	if err := r.updateGroup(group, map[string]any{"amount": "0.001"}); err == nil {
		t.Fatal("accepted excess amount precision")
	}
	state.count = math.MaxInt64
	if err := r.updateGroup(group, map[string]any{"amount": "1"}); err == nil {
		t.Fatal("accepted count overflow")
	}
	if _, err := joinValueKey(json.Number("not-a-number"), "key"); err == nil {
		t.Fatal("accepted invalid JSON join key")
	}
	if _, err := joinValueKey(json.Number("9007199254740992"), "key"); err == nil {
		t.Fatal("accepted unsafe JSON join key")
	}
	query := From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectIntoRecordset()
	resolver := func(context.Context, string) (QueryExecutor, error) { return nil, nil }
	if _, err := ExecuteFederatedQueryWithOptions(context.Background(), query, resolver, FederatedQueryOptions{Money: &MoneyConfig{Rounding: "bad"}}); err == nil {
		t.Fatal("accepted invalid money config")
	}
	if _, err := ExecuteFederatedQueryWithOptions(context.Background(), query, resolver, FederatedQueryOptions{Money: config}); err == nil {
		t.Fatal("accepted nonstream money query")
	}
}
