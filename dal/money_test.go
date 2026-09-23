package dal

import (
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
