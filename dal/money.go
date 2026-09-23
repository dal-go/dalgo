package dal

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
)

// MoneyConfig enables decimal-text results for federated streaming arithmetic.
// Fractional inputs must be decimal strings. Binary floating-point inputs are rejected.
type MoneyConfig struct {
	MinorUnitScale int
	DivisionScale  int
	Rounding       string
}

var decimalText = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
var maxSafeMoneyInteger = big.NewInt(9007199254740991)

func checkSafeMoneyInteger(text string) error {
	integer := new(big.Int)
	if _, ok := integer.SetString(text, 10); !ok || new(big.Int).Abs(integer).Cmp(maxSafeMoneyInteger) > 0 {
		return fmt.Errorf("money integer input must be within the portable safe range")
	}
	return nil
}

func validateMoney(config *MoneyConfig) error {
	if config == nil {
		return nil
	}
	if config.MinorUnitScale < 0 || config.MinorUnitScale > 18 || config.DivisionScale < 0 || config.DivisionScale > 18 || config.Rounding != "halfEven" {
		return fmt.Errorf("money requires minorUnitScale and divisionScale 0..18 and rounding halfEven")
	}
	return nil
}

func moneyInput(value any, scale int) (*big.Int, error) {
	var text string
	numeric := true
	switch v := value.(type) {
	case string:
		text = v
		numeric = false
	case json.Number:
		text = string(v)
	case int:
		text = fmt.Sprint(v)
	case int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		text = fmt.Sprint(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || math.Abs(v) > 9007199254740991 {
			return nil, fmt.Errorf("money input must be a safe whole integer, got %v", v)
		}
		text = fmt.Sprintf("%.0f", v)
	default:
		return nil, fmt.Errorf("money input must be a base-10 string or integer, got %T", value)
	}
	if !decimalText.MatchString(text) {
		return nil, fmt.Errorf("invalid money input %q", text)
	}
	if numeric {
		if err := checkSafeMoneyInteger(text); err != nil {
			return nil, err
		}
	}
	parts := strings.Split(text, ".")
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > scale {
		return nil, fmt.Errorf("money input %q exceeds minorUnitScale %d", text, scale)
	}
	minor := new(big.Int)
	minorText := parts[0] + fraction + strings.Repeat("0", scale-len(fraction))
	if _, ok := minor.SetString(minorText, 10); !ok {
		return nil, fmt.Errorf("invalid money input %q", text)
	}
	return minor, nil
}

func moneyMinorText(value *big.Int, scale int) string {
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return moneyText(new(big.Rat).SetFrac(value, denominator), scale)
}

func moneyNumber(value any) (*big.Rat, error) {
	var text string
	numeric := true
	switch v := value.(type) {
	case string:
		text = v
		numeric = false
	case json.Number:
		text = v.String()
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		text = fmt.Sprint(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || math.Abs(v) > 9007199254740991 {
			return nil, fmt.Errorf("money operand must be a safe integer, got %v", v)
		}
		text = fmt.Sprintf("%.0f", v)
	default:
		return nil, fmt.Errorf("money operand must be a decimal string or integer, got %T", value)
	}
	if !decimalText.MatchString(text) {
		return nil, fmt.Errorf("invalid money operand %q", text)
	}
	if numeric {
		if err := checkSafeMoneyInteger(text); err != nil {
			return nil, err
		}
	}
	result := new(big.Rat)
	result.SetString(text)
	return result, nil
}

func moneyText(value *big.Rat, scale int) string {
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	numerator := new(big.Int).Mul(value.Num(), factor)
	denominator := value.Denom()
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	doubled := new(big.Int).Mul(new(big.Int).Abs(remainder), big.NewInt(2))
	if doubled.Cmp(denominator) > 0 || doubled.Cmp(denominator) == 0 && quotient.Bit(0) == 1 {
		if numerator.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	negative := quotient.Sign() < 0
	digits := new(big.Int).Abs(quotient).String()
	for len(digits) <= scale {
		digits = "0" + digits
	}
	if scale > 0 {
		digits = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
		digits = strings.TrimRight(strings.TrimRight(digits, "0"), ".")
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func moneyPerCapita(operator ArithmeticOperator, left, right any, scale int) (any, error) {
	if left == nil || right == nil {
		return nil, nil
	}
	if operator != Divide {
		return nil, fmt.Errorf("money arithmetic supports per-capita division only")
	}
	a, err := moneyNumber(left)
	if err != nil {
		return nil, fmt.Errorf("money left operand: %w", err)
	}
	b, err := moneyNumber(right)
	if err != nil {
		return nil, fmt.Errorf("money right operand: %w", err)
	}
	if b.Sign() == 0 {
		return nil, fmt.Errorf("money division by zero")
	}
	return moneyText(new(big.Rat).Quo(a, b), scale), nil
}
