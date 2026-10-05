package dal

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/dal-go/record"
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
	for _, value := range []any{json.Number("9007199254740992"), float64(1.25), float64(9007199254740992), math.Inf(1), true, "1e2", ""} {
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

func TestMoneyExactFiniteArithmetic(t *testing.T) {
	for _, tc := range []struct {
		op         ArithmeticOperator
		a, b, want any
	}{{Add, "0.1", "0.2", "0.3"}, {Subtract, "10", "9.99", "0.01"}, {Multiply, "0.1", "0.2", "0.02"}, {Multiply, "0.123456", "0.123456", "0.015241383936"}, {Multiply, "1234567890123456.12", "2", "2469135780246912.24"}, {Divide, "1", "8", "0.12"}} {
		got, err := moneyArithmetic(tc.op, tc.a, tc.b, 2)
		if err != nil || got != tc.want {
			t.Fatalf("%s %v %v = %v, %v; want %v", tc.op, tc.a, tc.b, got, err, tc.want)
		}
	}
	if _, err := moneyArithmetic(Add, float64(1.25), "1", 2); err == nil {
		t.Fatal("accepted fractional binary float")
	}
	if _, err := moneyArithmetic(Multiply, "99999999999999999999999999999999999999", "2", 2); err == nil {
		t.Fatal("accepted overflow")
	}
	if _, err := moneyNumber("999999999999999999999999999999999999999"); err == nil {
		t.Fatal("accepted 39 significant digits")
	}
	if _, err := moneyArithmetic(Divide, "99999999999999999999999999999999999999", "0.1", 2); err == nil {
		t.Fatal("accepted rounded division overflow")
	}
}

func TestMoneyNumericOrderAndDistinctAggregate(t *testing.T) {
	for _, tc := range []struct {
		a, b any
		want int
	}{{"9", "10", -1}, {"1.0", "1.00", 0}, {"10", "name", -1}, {"name", "zoo", -1}} {
		got, err := compareMoneyValues(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("compare %v %v=%d,%v want %d", tc.a, tc.b, got, err, tc.want)
		}
	}
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	agg := NewAggregate(SUM, true, Field("amount"))
	state := &aggregateState{expression: agg, distinct: map[string]struct{}{}}
	group := &localGroup{states: map[string]*aggregateState{agg.String(): state}}
	r := &localAggregationReader{money: config}
	for _, amount := range []string{"1.0", "1.00", "2"} {
		if err := r.updateGroup(group, map[string]any{"amount": amount}); err != nil {
			t.Fatal(err)
		}
	}
	if state.count != 2 || state.exactSum.Cmp(new(big.Rat).SetInt64(3)) != 0 {
		t.Fatalf("distinct exact aggregate count=%d sum=%v", state.count, state.exactSum)
	}
	group.keyValues = map[string]any{}
	got, err := r.resolveGroupExpression(agg, group, nil)
	if err != nil || got != "3" {
		t.Fatalf("sum=%v err=%v", got, err)
	}
}

func TestMoneyDecimalBoundsAndTextFallback(t *testing.T) {
	if _, err := parseMoneyDecimal("bad"); err == nil {
		t.Fatal("accepted invalid decimal parser input")
	}
	for _, text := range []string{"+.5", ".5", "001.20", "1."} {
		if _, err := moneyNumber(text); err != nil {
			t.Errorf("exact fixed-point %q rejected: %v", text, err)
		}
	}
	for _, text := range []string{"+" + strings.Repeat("0", 40) + "1", "-" + strings.Repeat("0", 40) + "1"} {
		if _, err := moneyNumber(text); err != nil {
			t.Errorf("signed leading-zero value %q rejected: %v", text, err)
		}
	}
	for _, text := range []string{" 1", "1e2", "1.01.2", strings.Repeat("1", 81)} {
		if _, err := moneyNumber(text); err == nil {
			t.Errorf("malformed/bounded input %q accepted", text)
		}
	}
	if err := validateMoneyRat(new(big.Rat).SetFrac64(1, 3)); err == nil {
		t.Fatal("accepted nonterminating decimal")
	}
	if err := validateMoneyRat(new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(38), nil))); err == nil {
		t.Fatal("accepted 39 digit result")
	}
	if _, err := moneyFiniteText(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := moneyFiniteText(new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(39), nil))); err == nil {
		t.Fatal("accepted excessive output scale")
	}
	if _, err := compareMoneyValues(true, false); err == nil {
		t.Fatal("accepted nondecimal nontext comparison")
	}
	if cmp, err := compareMoneyValues("not-number", "other"); err != nil || cmp >= 0 {
		t.Fatalf("lexical text order cmp=%d err=%v", cmp, err)
	}
}

func TestMoneyConditionEvaluators(t *testing.T) {
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	row := map[string]any{"amount": "10.00", "name": "Bob"}
	conditions := []Condition{nil,
		NewComparison(Field("amount"), GreaterThen, String("9")),
		NewComparison(Field("name"), Equal, String("Bob")),
		NewIsNullCondition(Field("missing")),
		NewIsNotNullCondition(Field("name")),
		NewGroupCondition(And, NewComparison(Field("amount"), GreaterThen, String("9")), NewComparison(Field("name"), Equal, String("Bob"))),
		NewGroupCondition(Or, NewComparison(Field("amount"), LessThen, String("1")), NewComparison(Field("name"), Equal, String("Bob"))),
	}
	for _, condition := range conditions {
		got, err := evalMoneyMapCondition(condition, row, config)
		if err != nil || !got {
			t.Errorf("map condition %v got %v err=%v", condition, got, err)
		}
	}
	if matched, err := evalMoneyMapCondition(NewComparison(Field("missing"), Equal, String("1")), row, config); err != nil || matched {
		t.Fatalf("NULL map comparison matched=%v err=%v", matched, err)
	}
	if _, err := evalMoneyMapCondition(unsupportedMoneyCondition{}, row, config); err == nil {
		t.Fatal("accepted unknown map condition")
	}
	for _, condition := range []Condition{NewGroupCondition(And, NewComparison(Field("amount"), LessThen, String("1"))), NewGroupCondition(Or, NewComparison(Field("amount"), LessThen, String("1")))} {
		if got, err := evalMoneyMapCondition(condition, row, config); err != nil || got {
			t.Errorf("false condition %v got %v err=%v", condition, got, err)
		}
	}
	if got, err := evalMoneyMapCondition(NewGroupCondition(Operator("?")), row, config); err != nil || !got {
		t.Errorf("empty group got=%v err=%v", got, err)
	}
	for _, condition := range []Condition{NewComparison(Field("amount"), Operator("?"), String("0")), NewIsNullCondition(nil)} {
		if _, err := evalMoneyMapCondition(condition, row, config); err == nil {
			t.Errorf("expected condition error for %v", condition)
		}
	}
	for _, condition := range []Condition{NewComparison(unsupportedMoneyExpression{}, Equal, String("0")), NewComparison(Field("amount"), Equal, unsupportedMoneyExpression{}), NewComparison(Constant{Value: true}, Equal, Constant{Value: false})} {
		if _, err := evalMoneyMapCondition(condition, row, config); err == nil {
			t.Errorf("expected invalid map comparison %v", condition)
		}
	}
	for _, condition := range []Condition{NewComparison(Field("amount"), GreaterOrEqual, String("10")), NewComparison(Field("amount"), LessOrEqual, String("10"))} {
		keep, err := evalMoneyMapCondition(condition, row, config)
		if err != nil || !keep {
			t.Errorf("money comparison %v keep=%v err=%v", condition, keep, err)
		}
	}
	e := &joinExecution{money: true, moneyConfig: config}
	joined := joinRow{sources: map[string]map[string]any{"o": {"amount": "10.00", "name": "Bob"}}}
	amount := FieldRef{source: "o", name: "amount"}
	name := FieldRef{source: "o", name: "name"}
	none := FieldRef{source: "o", name: "none"}
	joinConditions := []Condition{NewComparison(amount, GreaterThen, String("9")), NewComparison(Binary(amount, Multiply, String("2")), Equal, String("20")), NewComparison(name, Equal, String("Bob")), NewIsNullCondition(none), NewGroupCondition(Or, NewComparison(amount, LessThen, String("1")), NewComparison(name, Equal, String("Bob")))}
	for _, condition := range joinConditions {
		ok, err := e.evalMoneyCondition(condition, joined)
		if err != nil || !ok {
			t.Errorf("join condition %v ok=%v err=%v", condition, ok, err)
		}
	}
	for _, condition := range []Condition{NewGroupCondition(And, NewComparison(amount, LessThen, String("1"))), NewGroupCondition(Or, NewComparison(amount, LessThen, String("1")))} {
		if got, err := e.evalMoneyCondition(condition, joined); err != nil || got {
			t.Errorf("false join condition %v got %v err=%v", condition, got, err)
		}
	}
	if got, err := e.evalMoneyCondition(NewGroupCondition(Operator("?")), joined); err != nil || !got {
		t.Errorf("empty join group got=%v err=%v", got, err)
	}
	if matched, err := e.evalMoneyCondition(NewComparison(amount, Equal, String("10")), joinRow{sources: map[string]map[string]any{"o": {"amount": nil}}}); err != nil || matched {
		t.Fatalf("NULL join comparison matched=%v err=%v", matched, err)
	}
	for _, condition := range []Condition{NewComparison(amount, Operator("?"), String("0")), NewIsNullCondition(nil)} {
		if _, err := e.evalMoneyCondition(condition, joined); err == nil {
			t.Errorf("expected join condition error for %v", condition)
		}
	}
	for _, condition := range []Condition{NewComparison(unsupportedMoneyExpression{}, Equal, String("0")), NewComparison(amount, Equal, unsupportedMoneyExpression{}), NewComparison(Constant{Value: true}, Equal, Constant{Value: false})} {
		if _, err := e.evalMoneyCondition(condition, joined); err == nil {
			t.Errorf("expected invalid join comparison %v", condition)
		}
	}
	for _, condition := range []Condition{NewComparison(amount, GreaterOrEqual, String("10")), NewComparison(amount, LessOrEqual, String("10"))} {
		ok, err := e.evalMoneyCondition(condition, joined)
		if err != nil || !ok {
			t.Errorf("join comparison %v ok=%v err=%v", condition, ok, err)
		}
	}
	if _, err := e.evalMoneyJoinScalar(unsupportedMoneyExpression{}, joined); err == nil {
		t.Fatal("accepted unknown join expression")
	}
	for _, expr := range []Expression{Binary(unsupportedMoneyExpression{}, Add, String("1")), Binary(amount, Add, unsupportedMoneyExpression{})} {
		if _, err := e.evalMoneyJoinScalar(expr, joined); err == nil {
			t.Errorf("accepted invalid money expression %v", expr)
		}
	}
	if _, err := e.evalMoneyJoinNumericScalar(FieldRef{source: "o", name: "none"}, joined); err != nil {
		t.Fatalf("nil numeric join operand: %v", err)
	}
	if _, err := e.evalMoneyJoinNumericScalar(Constant{Value: "bad"}, joined); err == nil {
		t.Fatal("accepted invalid numeric join operand")
	}
	if _, err := e.evalMoneyJoinNumericScalar(amount, joinRow{sources: map[string]map[string]any{"o": map[string]any{"amount": "1.001"}}}); err == nil {
		t.Fatal("accepted joined field above configured minor-unit scale")
	}
	if _, err := e.expressionAt(Binary(amount, Add, String("1")), joined, "test"); err != nil {
		t.Fatalf("Money expressionAt: %v", err)
	}
	e.recursive = true
	truth, err := e.evalTruthAt(NewComparison(amount, Equal, String("10.0")), joined, "test")
	if err != nil || truth != queryTrue {
		t.Fatalf("money recursive compare truth=%v err=%v", truth, err)
	}
	truth, err = e.evalTruthAt(NewComparison(amount, Equal, String("11")), joined, "test")
	if err != nil || truth != queryFalse {
		t.Fatalf("money recursive unequal compare truth=%v err=%v", truth, err)
	}
	if _, err := e.evalTruthAt(NewComparison(Constant{Value: true}, Equal, String("1")), joined, "test"); err == nil {
		t.Fatal("accepted invalid money comparator in recursive condition")
	}
	var nilFloat *float64
	if hasUnsafeMoneyFloat(nilFloat) || hasUnsafeMoneyFloat(nil) {
		t.Fatal("nil float value marked unsafe")
	}
	if !hasUnsafeMoneyFloat(map[string]any{"n": float64(1.5)}) || hasUnsafeMoneyFloat(map[string]any{"n": float64(2)}) {
		t.Fatal("unsafe money float classification failed")
	}
	if !hasUnsafeMoneyFloat(struct{ Values []any }{[]any{float32(0.25)}}) {
		t.Fatal("nested float32 was not detected")
	}
	if err := validateMoneyRat(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := evalMoneyNumericScalar(Field("amount"), map[string]any{"amount": "1.001"}, config); err == nil {
		t.Fatal("accepted field with excess minor-unit scale")
	}
	if _, err := evalMoneyNumericScalar(Constant{Value: "Bob"}, row, config); err == nil {
		t.Fatal("accepted text in numeric expression")
	}
	if _, err := evalMoneyScalar(unsupportedMoneyExpression{}, row, config); err == nil {
		t.Fatal("accepted unknown scalar expression")
	}
	if _, err := moneyArithmetic(ArithmeticOperator("?"), "1", "1", 2); err == nil {
		t.Fatal("accepted unknown arithmetic operator")
	}
	for _, tc := range []struct {
		op          ArithmeticOperator
		left, right any
	}{{Add, "bad", "1"}, {Add, "1", "bad"}, {Divide, "1", "0"}} {
		if _, err := moneyArithmetic(tc.op, tc.left, tc.right, 2); err == nil {
			t.Errorf("accepted invalid arithmetic operands %+v", tc)
		}
	}
	if got, err := moneyArithmetic(Add, nil, "1", 2); err != nil || got != nil {
		t.Fatalf("NULL arithmetic got=%v err=%v", got, err)
	}
	if _, err := evalMoneyScalar(Binary(String("bad"), Add, String("1")), map[string]any{}, config); err == nil {
		t.Fatal("accepted invalid binary left operand")
	}
	if _, err := evalMoneyScalar(Binary(String("1"), Add, String("bad")), map[string]any{}, config); err == nil {
		t.Fatal("accepted invalid binary right operand")
	}
	if _, err := evalMoneyNumericScalar(Field("missing"), map[string]any{}, config); err != nil {
		t.Fatalf("NULL numeric field: %v", err)
	}
	if _, err := evalMoneyNumericScalar(Constant{Value: "bad"}, map[string]any{}, config); err == nil {
		t.Fatal("accepted invalid numeric constant")
	}
	for _, pair := range [][2]any{{nil, "1"}, {nil, nil}, {"1", nil}} {
		if _, err := compareMoneyValues(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	if cmp, err := compareMoneyValues("name", "1"); err != nil || cmp != 1 {
		t.Fatalf("decimal/text order cmp=%d err=%v", cmp, err)
	}
	if _, err := compareMoneyValues("1", true); err == nil {
		t.Fatal("accepted nontext operand beside decimal")
	}
	if _, err := compareMoneyValues(true, "1"); err == nil {
		t.Fatal("accepted boolean in decimal comparison")
	}
}

func TestMoneyAverageRoundedPrecision(t *testing.T) {
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 6, Rounding: "halfEven"}
	aggregate := NewAggregate(AVERAGE, false, Field("amount"))
	state := &aggregateState{expression: aggregate, count: 2, exactSum: new(big.Rat).SetFrac64(3, 10)}
	group := &localGroup{states: map[string]*aggregateState{aggregate.String(): state}}
	reader := &localAggregationReader{money: config}
	got, err := reader.resolveGroupExpression(aggregate, group, nil)
	if err != nil || got != "0.15" {
		t.Fatalf("exact Money AVG=%v err=%v", got, err)
	}

	max38, ok := new(big.Int).SetString(strings.Repeat("9", 37)+"8", 10)
	if !ok {
		t.Fatal("could not construct 38-digit AVG input")
	}
	state.exactSum.SetInt(max38)
	state.count = 3
	if got, err := reader.resolveGroupExpression(aggregate, group, nil); err == nil {
		t.Fatalf("accepted rounded AVG result exceeding 38 coefficient digits: %v", got)
	}
}

func TestMoneyAggregationStreamingFilteringAndErrors(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	q := structuredQuery{from: From(sales), where: NewComparison(Field("amount"), GreaterThen, String("5")), columns: []Column{SumAs(Field("amount"), "total")}}
	raw := &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"amount": "1.00"}), aggregationCoverageRecord("2", map[string]any{"amount": "10.00"})}}
	reader := newLocalAggregationReader(context.Background(), q, raw, AggregationPlan{Strategy: AggregationStreaming})
	reader.money = config
	result, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Data().(map[string]any)["total"]; got != "10" {
		t.Fatalf("filtered sum=%v", got)
	}
	badRaw := &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"amount": float64(1.5)})}}
	bad := newLocalAggregationReader(context.Background(), q, badRaw, AggregationPlan{Strategy: AggregationStreaming})
	bad.money = config
	if _, err := bad.Next(); err == nil {
		t.Fatal("accepted fractional float in money stream")
	}
	if !badRaw.closed {
		t.Fatal("error did not close raw reader")
	}
	badQuery := structuredQuery{from: From(sales), where: unsupportedMoneyCondition{}, columns: []Column{SumAs(Field("amount"), "total")}}
	badCondition := newLocalAggregationReader(context.Background(), badQuery, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"amount": "1"})}}, AggregationPlan{Strategy: AggregationStreaming})
	badCondition.money = config
	if _, err := badCondition.Next(); err == nil {
		t.Fatal("accepted unsupported Money WHERE")
	}
}

func TestMoneyAggregationMaterializedHavingAndOrder(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	sum := SumAs(Field("amount"), "total")
	q := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}, columns: []Column{{Expression: Field("category")}, sum}, having: NewComparison(Field("total"), GreaterThen, String("1")), orderBy: []OrderExpression{Ascending(Field("category"))}}
	reader := newLocalAggregationReader(context.Background(), q, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"category": "b", "amount": "2.00"}), aggregationCoverageRecord("2", map[string]any{"category": "a", "amount": "0.50"}), aggregationCoverageRecord("3", map[string]any{"category": "a", "amount": "1.00"})}}, AggregationPlan{Strategy: AggregationHash})
	reader.money = &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	rows := aggregationCoverageRows(t, reader)
	if len(rows) != 2 || rows[0]["category"] != "a" || rows[1]["category"] != "b" {
		t.Fatalf("money HAVING/ORDER BY rows=%#v", rows)
	}
}

func TestMoneyAggregationErrorAndHavingBranches(t *testing.T) {
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	aggregate := SumAs(Field("amount"), "total").Expression.(AggregateFunc)
	r := &localAggregationReader{money: config}
	state := &aggregateState{expression: aggregate}
	group := &localGroup{states: map[string]*aggregateState{aggregate.String(): state}}
	if err := r.updateGroup(group, map[string]any{"amount": "bad"}); err == nil {
		t.Fatal("accepted invalid sum input")
	}
	constantSum := NewAggregate(SUM, false, Constant{Value: "bad"})
	constantState := &aggregateState{expression: constantSum}
	constantGroup := &localGroup{states: map[string]*aggregateState{constantSum.String(): constantState}}
	if err := r.updateGroup(constantGroup, map[string]any{}); err == nil {
		t.Fatal("accepted invalid numeric aggregate argument")
	}
	state.exactSum = new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(38), nil))
	state.count = 0
	if err := r.updateGroup(group, map[string]any{"amount": "99999999999999999999999999999999999999"}); err == nil {
		t.Fatal("accepted exact SUM overflow")
	}
	state.exactSum = nil
	state.count = 0
	state.hasValue = true
	state.value = true
	min := NewAggregate(MIN, false, Field("name"))
	r2 := &localAggregationReader{money: config}
	s2 := &aggregateState{expression: min, hasValue: true, value: true}
	g2 := &localGroup{states: map[string]*aggregateState{min.String(): s2}}
	if err := r2.updateGroup(g2, map[string]any{"name": false}); err == nil {
		t.Fatal("accepted incomparable MIN values")
	}
	r.money = config
	group.out = map[string]any{"total": "2"}
	for _, op := range []Operator{Equal, GreaterThen, GreaterOrEqual, LessThen, LessOrEqual} {
		keep, err := r.evalHaving(NewComparison(Field("total"), op, String("2")), group)
		if err != nil {
			t.Errorf("HAVING %s: %v", op, err)
		} else if op == Equal || op == GreaterOrEqual || op == LessOrEqual {
			if !keep {
				t.Errorf("HAVING %s should keep", op)
			}
		} else if keep {
			t.Errorf("HAVING %s should reject", op)
		}
	}
	if _, err := r.evalHaving(NewComparison(Field("total"), Operator("?"), String("2")), group); err == nil {
		t.Fatal("accepted invalid HAVING operator")
	}
	if _, err := r.evalHaving(NewIsNullCondition(nil), group); err == nil {
		t.Fatal("accepted empty HAVING IS NULL")
	}
	if _, err := r.evalHaving(NewComparison(Field("total"), GreaterThen, NewConstant(false)), &localGroup{out: map[string]any{"total": true}}); err == nil {
		t.Fatal("accepted incomparable HAVING values")
	}
}

func TestMoneyStreamingSingleSourceSetupErrors(t *testing.T) {
	source := NewDatabaseCollectionRef("sales", "", "Sale", "s")
	query := From(source).NewQuery().SelectColumns(SumAs(Field("amount"), "total"))
	wrapped := moneyAggregationSourceQuery{aggregationSourceQuery: aggregationSourceQuery{StructuredQuery: query}}
	if wrapped.Where() != nil {
		t.Fatal("money source query pushed the local filter down")
	}
	boom := errors.New("source failed")
	routed := federatedQueryExecutor{resolve: func(context.Context, string) (QueryExecutor, error) {
		return federatedStub{err: boom}, nil
	}}
	if _, err := executeStreamingFederatedAggregate(context.Background(), query, routed, FederatedQueryOptions{}); !errors.Is(err, boom) {
		t.Fatalf("source error=%v", err)
	}
	badPlan := From(source).NewQuery().SelectColumns(SumAs(unsupportedMoneyExpression{}, "total"))
	routed.resolve = func(context.Context, string) (QueryExecutor, error) {
		return federatedStub{}, nil
	}
	if _, err := executeStreamingFederatedAggregate(context.Background(), badPlan, routed, FederatedQueryOptions{}); err == nil {
		t.Fatal("accepted invalid single-source aggregation plan")
	}
}

func TestMoneySourceQueryFloatAndInvalidOrderErrors(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	query := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}, columns: []Column{{Expression: Field("category")}, SumAs(Field("amount"), "total")}, orderBy: []OrderExpression{Ascending(Field("category"))}}
	reader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"category": true, "amount": "1"})}}, AggregationPlan{Strategy: AggregationHash})
	reader.money = &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	if _, err := reader.Next(); err == nil {
		t.Fatal("accepted invalid Money ORDER BY class")
	}
	unsafeReader := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"category": "x", "amount": float64(1.5)})}}, AggregationPlan{Strategy: AggregationHash})
	unsafeReader.money = &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	if _, err := unsafeReader.Next(); err == nil {
		t.Fatal("accepted fractional float in materialized money scan")
	}
	whereQuery := structuredQuery{from: From(sales), where: unsupportedMoneyCondition{}, columns: []Column{SumAs(Field("amount"), "total")}, orderBy: []OrderExpression{Ascending(Field("total"))}}
	whereReader := newLocalAggregationReader(context.Background(), whereQuery, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"amount": "1"})}}, AggregationPlan{Strategy: AggregationHash})
	whereReader.money = &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	if _, err := whereReader.Next(); err == nil {
		t.Fatal("accepted invalid materialized money WHERE")
	}
	filterQuery := structuredQuery{from: From(sales), where: NewComparison(Field("amount"), GreaterThen, String("9")), columns: []Column{SumAs(Field("amount"), "total")}, orderBy: []OrderExpression{Ascending(Field("total"))}}
	filterReader := newLocalAggregationReader(context.Background(), filterQuery, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"amount": "1"})}}, AggregationPlan{Strategy: AggregationHash})
	filterReader.money = &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	filtered, err := filterReader.Next()
	if err != nil || filtered.Data().(map[string]any)["total"] != nil {
		t.Fatalf("materialized Money filter result=%v err=%v", filtered, err)
	}
	base := testFederatedAggregate()
	joined := structuredQuery{from: base.From(), groupBy: []Expression{NewFieldRef("c", "id")}, columns: []Column{SumAs(NewFieldRef("o", "amount"), "total")}}
	resolve := func(_ context.Context, db string) (QueryExecutor, error) {
		if db == "orders" {
			return federatedStub{rows: []record.Record{aggregationCoverageRecord("1", map[string]any{"country_id": 1, "amount": float64(1.25)})}}, nil
		}
		return federatedStub{rows: []record.Record{aggregationCoverageRecord("1", map[string]any{"id": 1})}}, nil
	}
	ctx := context.Background()
	config := &MoneyConfig{MinorUnitScale: 2, DivisionScale: 2, Rounding: "halfEven"}
	e := &joinExecution{ctx: ctx, q: joined, executor: federatedQueryExecutor{resolve: resolve}, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}, fields: map[string][]string{}, keyRefs: map[string][]joinKeyReference{}, money: true, moneyConfig: config}
	e.collectKeyRefs(joined.From(), "from")
	err = e.scanTree(joined.From(), "from")
	if err == nil {
		t.Fatal("accepted fractional float in joined source")
	}
}

type unsupportedMoneyCondition struct{}

func (unsupportedMoneyCondition) String() string { return "unsupported" }

type unsupportedMoneyExpression struct{}

func (unsupportedMoneyExpression) String() string { return "unsupported" }

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
