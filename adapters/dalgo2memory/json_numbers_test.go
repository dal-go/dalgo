package dalgo2memory

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNumber(t *testing.T) {
	t.Parallel()

	// Integer types
	for _, val := range []any{
		int(42), int8(42), int16(42), int32(42), int64(42),
		int64(math.MaxInt64), int64(math.MaxInt64 - 1),
		int64(math.MinInt64), int64(math.MinInt64 + 1),
	} {
		pn, ok := parseNumber(val)
		require.True(t, ok, "val: %v (%T)", val, val)
		require.Equal(t, numInt, pn.kind)
	}

	// Unsigned integer types
	for _, val := range []any{
		uint(42), uint8(42), uint16(42), uint32(42), uint64(42), uintptr(42),
		uint64(math.MaxInt64),
	} {
		pn, ok := parseNumber(val)
		require.True(t, ok, "val: %v (%T)", val, val)
		require.Equal(t, numInt, pn.kind) // normalized to numInt because <= MaxInt64
	}

	// Large unsigned integer (> MaxInt64)
	pn, ok := parseNumber(uint64(math.MaxUint64))
	require.True(t, ok)
	require.Equal(t, numUint, pn.kind)
	require.Equal(t, uint64(math.MaxUint64), pn.u)

	// Floating point types
	for _, val := range []any{float32(1.5), float64(1.5), float64(-0.25)} {
		pn, ok := parseNumber(val)
		require.True(t, ok)
		require.Equal(t, numFloat, pn.kind)
	}

	// json.Number exact integer representations
	for _, s := range []string{
		"9223372036854775807",  // MaxInt64
		"9223372036854775806",  // MaxInt64 - 1
		"-9223372036854775808", // MinInt64
		"-9223372036854775807", // MinInt64 + 1
		"0", "-1", "1", "42",
	} {
		pn, ok := parseNumber(json.Number(s))
		require.True(t, ok, "s: %s", s)
		require.Equal(t, numInt, pn.kind)
	}

	// json.Number large uint (> MaxInt64)
	pn, ok = parseNumber(json.Number("18446744073709551615"))
	require.True(t, ok)
	require.Equal(t, numUint, pn.kind)
	require.Equal(t, uint64(math.MaxUint64), pn.u)

	// json.Number fractional / scientific
	for _, s := range []string{"1.5", "-0.25", "1e5", "2.5e-3"} {
		pn, ok := parseNumber(json.Number(s))
		require.True(t, ok, "s: %s", s)
		require.Equal(t, numFloat, pn.kind)
	}

	// Non-numbers must return false
	for _, val := range []any{
		nil,
		"123",
		"9223372036854775807",
		true,
		false,
		struct{}{},
		make(chan int),
		[]int{1},
		map[string]any{},
	} {
		_, ok := parseNumber(val)
		require.False(t, ok, "expected non-number for %v (%T)", val, val)
	}
}

func TestCompareParsed(t *testing.T) {
	t.Parallel()

	// MaxInt64 vs MaxInt64 - 1
	pnMax, _ := parseNumber(int64(math.MaxInt64))
	pnMaxMinus1, _ := parseNumber(int64(math.MaxInt64 - 1))
	require.Equal(t, 1, compareParsed(pnMax, pnMaxMinus1))
	require.Equal(t, -1, compareParsed(pnMaxMinus1, pnMax))
	require.Equal(t, 0, compareParsed(pnMax, pnMax))

	// MinInt64 vs MinInt64 + 1
	pnMin, _ := parseNumber(int64(math.MinInt64))
	pnMinPlus1, _ := parseNumber(int64(math.MinInt64 + 1))
	require.Equal(t, -1, compareParsed(pnMin, pnMinPlus1))
	require.Equal(t, 1, compareParsed(pnMinPlus1, pnMin))
	require.Equal(t, 0, compareParsed(pnMin, pnMin))

	// json.Number vs int64
	jnMax, _ := parseNumber(json.Number("9223372036854775807"))
	require.Equal(t, 0, compareParsed(jnMax, pnMax))
	require.Equal(t, 1, compareParsed(jnMax, pnMaxMinus1))

	// Signed vs Unsigned
	pnNeg, _ := parseNumber(int64(-5))
	pnUint, _ := parseNumber(uint64(math.MaxUint64))
	require.Equal(t, -1, compareParsed(pnNeg, pnUint))
	require.Equal(t, 1, compareParsed(pnUint, pnNeg))

	pnPos, _ := parseNumber(int64(100))
	pnUintSmall, _ := parseNumber(uint(100))
	require.Equal(t, 0, compareParsed(pnPos, pnUintSmall))

	// Floats
	pnF1, _ := parseNumber(1.5)
	pnF2, _ := parseNumber(2.5)
	require.Equal(t, -1, compareParsed(pnF1, pnF2))
	require.Equal(t, 1, compareParsed(pnF2, pnF1))
	require.Equal(t, 0, compareParsed(pnF1, pnF1))

	// Float vs Int
	pnF7, _ := parseNumber(float64(7))
	pnI7, _ := parseNumber(int(7))
	require.Equal(t, 0, compareParsed(pnF7, pnI7))

	pnF75, _ := parseNumber(float64(7.5))
	require.Equal(t, 1, compareParsed(pnF75, pnI7))
	require.Equal(t, -1, compareParsed(pnI7, pnF75))

	// Mixed integer-vs-float at MaxInt64 / MinInt64 / MaxUint64 boundaries
	pnFMaxPow2, _ := parseNumber(float64(1 << 63)) // 9223372036854775808.0 = 2^63
	require.Equal(t, -1, compareParsed(pnMax, pnFMaxPow2), "MaxInt64 (2^63 - 1) must be strictly less than float64(2^63)")
	require.Equal(t, 1, compareParsed(pnFMaxPow2, pnMax))

	pnFMaxBelow, _ := parseNumber(float64(math.MaxInt64 - 2048)) // 9223372036854773760.0
	require.Equal(t, 1, compareParsed(pnMax, pnFMaxBelow))
	require.Equal(t, -1, compareParsed(pnFMaxBelow, pnMax))

	pnFMinPow2, _ := parseNumber(-float64(1 << 63)) // -9223372036854775808.0 = -2^63
	require.Equal(t, 0, compareParsed(pnMin, pnFMinPow2), "MinInt64 (-2^63) must be exactly equal to -float64(2^63)")
	require.Equal(t, 0, compareParsed(pnFMinPow2, pnMin))

	require.Equal(t, 1, compareParsed(pnMinPlus1, pnFMinPow2), "MinInt64 + 1 must be strictly greater than -float64(2^63)")
	require.Equal(t, -1, compareParsed(pnFMinPow2, pnMinPlus1))

	pnFUintPow2, _ := parseNumber(float64(18446744073709551616.0)) // 2^64
	require.Equal(t, -1, compareParsed(pnUint, pnFUintPow2), "MaxUint64 must be strictly less than float64(2^64)")

	// Mixed integer-vs-float around 2^53 (9007199254740992)
	pn53, _ := parseNumber(int64(1 << 53))
	pn53Plus1, _ := parseNumber(int64((1 << 53) + 1))
	pn53Minus1, _ := parseNumber(int64((1 << 53) - 1))
	pnF53, _ := parseNumber(float64(1 << 53))

	require.Equal(t, 0, compareParsed(pn53, pnF53), "2^53 must be equal across int64 and float64")
	require.Equal(t, 1, compareParsed(pn53Plus1, pnF53), "2^53 + 1 must be strictly greater than float64(2^53)")
	require.Equal(t, -1, compareParsed(pnF53, pn53Plus1))
	require.Equal(t, -1, compareParsed(pn53Minus1, pnF53), "2^53 - 1 must be strictly less than float64(2^53)")
	require.Equal(t, 1, compareParsed(pnF53, pn53Minus1))

	// Positive and negative fractional neighbors
	pnF7PlusEps, _ := parseNumber(7.000000000000001)
	pnF7MinusEps, _ := parseNumber(6.999999999999999)
	require.Equal(t, -1, compareParsed(pnI7, pnF7PlusEps))
	require.Equal(t, 1, compareParsed(pnI7, pnF7MinusEps))

	pnINeg7, _ := parseNumber(int(-7))
	pnFNeg7, _ := parseNumber(float64(-7.0))
	pnFNeg7PlusEps, _ := parseNumber(-6.999999999999999)
	pnFNeg7MinusEps, _ := parseNumber(-7.000000000000001)
	require.Equal(t, 0, compareParsed(pnINeg7, pnFNeg7))
	require.Equal(t, -1, compareParsed(pnINeg7, pnFNeg7PlusEps))
	require.Equal(t, 1, compareParsed(pnINeg7, pnFNeg7MinusEps))

	// Infinities
	pnInfPos, _ := parseNumber(math.Inf(1))
	pnInfNeg, _ := parseNumber(math.Inf(-1))
	require.Equal(t, -1, compareParsed(pnMax, pnInfPos))
	require.Equal(t, 1, compareParsed(pnInfPos, pnMax))
	require.Equal(t, 1, compareParsed(pnMin, pnInfNeg))
	require.Equal(t, -1, compareParsed(pnInfNeg, pnMin))

	// NaNs in comparator
	pnNaN, _ := parseNumber(math.NaN())
	require.Equal(t, 0, compareParsed(pnNaN, pnNaN))
	require.Equal(t, -1, compareParsed(pnNaN, pnI7))
	require.Equal(t, 1, compareParsed(pnI7, pnNaN))
}

func TestNumbersEqual(t *testing.T) {
	t.Parallel()

	// Large integers
	eq, ok := numbersEqual(int64(math.MaxInt64), json.Number("9223372036854775807"))
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = numbersEqual(int64(math.MaxInt64), json.Number("9223372036854775806"))
	require.True(t, ok)
	require.False(t, eq)

	eq, ok = numbersEqual(int64(math.MinInt64), json.Number("-9223372036854775808"))
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = numbersEqual(int64(math.MinInt64), json.Number("-9223372036854775807"))
	require.True(t, ok)
	require.False(t, eq)

	// Boundary integer-vs-float equality
	eq, ok = numbersEqual(int64(math.MaxInt64), float64(1<<63))
	require.True(t, ok)
	require.False(t, eq, "MaxInt64 must not be equal to float64(2^63)")

	eq, ok = numbersEqual(int64(math.MinInt64), -float64(1<<63))
	require.True(t, ok)
	require.True(t, eq, "MinInt64 is exactly equal to -float64(2^63)")

	eq, ok = numbersEqual(int64((1<<53)+1), float64(1<<53))
	require.True(t, ok)
	require.False(t, eq, "2^53 + 1 must not be equal to float64(2^53)")

	eq, ok = numbersEqual(int64(1<<53), float64(1<<53))
	require.True(t, ok)
	require.True(t, eq, "2^53 must be equal across int64 and float64")

	// Cross-type equality for normal numbers
	eq, ok = numbersEqual(float64(7), int(7))
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = numbersEqual(json.Number("7"), float64(7))
	require.True(t, ok)
	require.True(t, eq)

	// Non-finite equality rules: NaN never compares equal to anything
	eq, ok = numbersEqual(math.NaN(), int(1))
	require.True(t, ok)
	require.False(t, eq, "NaN == 1 must be false")

	eq, ok = numbersEqual(int(1), math.NaN())
	require.True(t, ok)
	require.False(t, eq, "1 == NaN must be false")

	eq, ok = numbersEqual(math.NaN(), math.NaN())
	require.True(t, ok)
	require.False(t, eq, "NaN == NaN must be false")

	// Infinities
	eq, ok = numbersEqual(math.Inf(1), math.Inf(1))
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = numbersEqual(math.Inf(-1), math.Inf(-1))
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = numbersEqual(math.Inf(1), math.Inf(-1))
	require.True(t, ok)
	require.False(t, eq)

	// String vs Number must not be equal and must report non-numeric
	_, ok = numbersEqual("7", int(7))
	require.False(t, ok)

	_, ok = numbersEqual("9223372036854775807", int64(math.MaxInt64))
	require.False(t, ok)
}

func TestCanonicalGroupValue(t *testing.T) {
	t.Parallel()

	// Canonical integer grouping: 1, 1.0, json.Number("1"), json.Number("1.0")
	// must produce identical canonical tokens.
	tokInt := canonicalGroupValue(int(1))
	tokInt64 := canonicalGroupValue(int64(1))
	tokFloat := canonicalGroupValue(float64(1.0))
	tokJnInt := canonicalGroupValue(json.Number("1"))
	tokJnFloat := canonicalGroupValue(json.Number("1.0"))

	require.Equal(t, "num:int:1", tokInt)
	require.Equal(t, tokInt, tokInt64)
	require.Equal(t, tokInt, tokFloat)
	require.Equal(t, tokInt, tokJnInt)
	require.Equal(t, tokInt, tokJnFloat)

	// Distinct large integers must NOT collapse
	tokMax := canonicalGroupValue(int64(math.MaxInt64))
	tokMaxMinus1 := canonicalGroupValue(int64(math.MaxInt64 - 1))
	require.Equal(t, "num:int:9223372036854775807", tokMax)
	require.Equal(t, "num:int:9223372036854775806", tokMaxMinus1)
	require.NotEqual(t, tokMax, tokMaxMinus1)

	// Canonical unsigned grouping for 2^63 (9223372036854775808):
	// uint64, json.Number("9223372036854775808"), json.Number("9223372036854775808.0"), and float64(2^63)
	// must produce identical canonical tokens.
	tokUint64 := canonicalGroupValue(uint64(1 << 63))
	tokJnUint64 := canonicalGroupValue(json.Number("9223372036854775808"))
	tokJnFloat64 := canonicalGroupValue(json.Number("9223372036854775808.0"))
	tokFloat64 := canonicalGroupValue(float64(1 << 63))
	require.Equal(t, "num:uint:9223372036854775808", tokUint64)
	require.Equal(t, tokUint64, tokJnUint64)
	require.Equal(t, tokUint64, tokJnFloat64)
	require.Equal(t, tokUint64, tokFloat64)

	// Canonical unsigned grouping for 2^63 + 2048 (9223372036854777856)
	tokUint64_2048 := canonicalGroupValue(uint64((1 << 63) + 2048))
	tokJnFloat64_2048 := canonicalGroupValue(json.Number("9223372036854777856.0"))
	require.Equal(t, "num:uint:9223372036854777856", tokUint64_2048)
	require.Equal(t, tokUint64_2048, tokJnFloat64_2048)

	// Neighboring unequal integer 2^63 + 1 (9223372036854775809) must stay distinct
	tokUint64_plus1 := canonicalGroupValue(uint64((1 << 63) + 1))
	require.Equal(t, "num:uint:9223372036854775809", tokUint64_plus1)
	require.NotEqual(t, tokUint64, tokUint64_plus1)
	require.NotEqual(t, tokUint64_2048, tokUint64_plus1)

	// Out-of-range float (> 2^64) must not convert to uint64
	tokOutOfRange := canonicalGroupValue(float64(18446744073709551616.0))
	require.Equal(t, "num:float:1.8446744073709552e+19", tokOutOfRange)

	// Floats with fractional parts
	require.Equal(t, "num:float:1.5", canonicalGroupValue(1.5))
	require.Equal(t, "num:float:1.5", canonicalGroupValue(json.Number("1.5")))

	// Non-finite
	require.Equal(t, "num:nan", canonicalGroupValue(math.NaN()))
	require.Equal(t, "num:+inf", canonicalGroupValue(math.Inf(1)))
	require.Equal(t, "num:-inf", canonicalGroupValue(math.Inf(-1)))

	// Non-numbers preserve distinction
	require.Equal(t, "str:1", canonicalGroupValue("1"))
	require.NotEqual(t, tokInt, canonicalGroupValue("1"))
	require.Equal(t, "bool:true", canonicalGroupValue(true))
	require.Equal(t, "nil", canonicalGroupValue(nil))
}

func TestUnmarshalGeneric(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"max":9223372036854775807,"min":-9223372036854775808,"float":1.25,"nested":{"seq":9223372036854775806},"list":[1,9223372036854775805]}`)
	var out map[string]any
	err := unmarshalGeneric(raw, &out)
	require.NoError(t, err)

	require.Equal(t, json.Number("9223372036854775807"), out["max"])
	require.Equal(t, json.Number("-9223372036854775808"), out["min"])
	require.Equal(t, json.Number("1.25"), out["float"])

	nested := out["nested"].(map[string]any)
	require.Equal(t, json.Number("9223372036854775806"), nested["seq"])

	list := out["list"].([]any)
	require.Equal(t, json.Number("1"), list[0])
	require.Equal(t, json.Number("9223372036854775805"), list[1])
}

func TestToPublicValue(t *testing.T) {
	t.Parallel()

	require.Equal(t, float64(42), toPublicValue(json.Number("42")))
	require.Equal(t, float64(1.5), toPublicValue(json.Number("1.5")))
	require.Equal(t, "hello", toPublicValue("hello"))
	require.Equal(t, true, toPublicValue(true))
	require.Nil(t, toPublicValue(nil))

	m := map[string]any{
		"num":    json.Number("10"),
		"text":   "abc",
		"nested": map[string]any{"inner": json.Number("20")},
		"list":   []any{json.Number("30"), "xyz"},
	}
	pub := toPublicValue(m).(map[string]any)
	require.Equal(t, float64(10), pub["num"])
	require.Equal(t, "abc", pub["text"])
	require.Equal(t, float64(20), pub["nested"].(map[string]any)["inner"])
	require.Equal(t, float64(30), pub["list"].([]any)[0])
	require.Equal(t, "xyz", pub["list"].([]any)[1])

	// Non-numeric json.Number fallback to string
	require.Equal(t, "invalid_num", toPublicValue(json.Number("invalid_num")))

	// internalGenericData direct normalization
	genData := internalGenericData{
		"seq": json.Number("100"),
	}
	pubGen := toPublicValue(genData).(map[string]any)
	require.Equal(t, float64(100), pubGen["seq"])

	// nil map and nil slice
	require.Nil(t, toPublicMap(nil))
	require.Nil(t, toPublicSlice(nil))
}

func TestInternalGenericData_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	// Valid JSON decodes with UseNumber preserving numeric token
	var d internalGenericData
	err := d.UnmarshalJSON([]byte(`{"count": 9223372036854775807}`))
	require.NoError(t, err)
	require.Equal(t, json.Number("9223372036854775807"), d["count"])

	// Invalid JSON returns error
	var dErr internalGenericData
	err = dErr.UnmarshalJSON([]byte(`{not valid json`))
	require.Error(t, err)
}

func TestParsedNumber_ToFloat(t *testing.T) {
	t.Parallel()

	pnU, ok := parseNumber(uint64(math.MaxInt64 + 1))
	require.True(t, ok)
	require.Equal(t, float64(uint64(math.MaxInt64+1)), pnU.toFloat())
	require.Equal(t, float64(0), parsedNumber{kind: numInvalid}.toFloat())
}

func TestParseNumber_Invalid(t *testing.T) {
	t.Parallel()

	// Non-numeric string json.Number
	_, ok := parseNumber(json.Number("invalid"))
	require.False(t, ok)
}

func TestCompareParsed_Combinations(t *testing.T) {
	t.Parallel()

	// Canonical numUint vs numUint (greater, less, and equal)
	// All canonical numUint values satisfy u > math.MaxInt64.
	pnU1, ok1 := parseNumber(uint64(math.MaxInt64 + 1))
	require.True(t, ok1)
	require.Equal(t, numUint, pnU1.kind)

	pnU2, ok2 := parseNumber(uint64(math.MaxInt64 + 2))
	require.True(t, ok2)
	require.Equal(t, numUint, pnU2.kind)

	require.Equal(t, -1, compareParsed(pnU1, pnU2))
	require.Equal(t, 1, compareParsed(pnU2, pnU1))
	require.Equal(t, 0, compareParsed(pnU1, pnU1))

	// Canonical numInt vs numUint and numUint vs numInt
	// By parseNumber invariant, any canonical numInt satisfies i <= math.MaxInt64 < u.
	// Cross-kind int-vs-uint comparison is always -1, and uint-vs-int is always 1.
	pnIMax, okIMax := parseNumber(int64(math.MaxInt64))
	require.True(t, okIMax)
	require.Equal(t, numInt, pnIMax.kind)

	pnINeg, okINeg := parseNumber(int64(-100))
	require.True(t, okINeg)
	require.Equal(t, numInt, pnINeg.kind)

	// numInt vs numUint: strictly -1
	require.Equal(t, -1, compareParsed(pnIMax, pnU1))
	require.Equal(t, -1, compareParsed(pnINeg, pnU1))

	// numUint vs numInt: strictly 1
	require.Equal(t, 1, compareParsed(pnU1, pnIMax))
	require.Equal(t, 1, compareParsed(pnU1, pnINeg))

	// Canonical json.Number boundary cases: MaxInt64 vs MaxInt64+1 vs MaxUint64
	pnJSONInt, okJSONInt := parseNumber(json.Number("9223372036854775807"))
	require.True(t, okJSONInt)
	require.Equal(t, numInt, pnJSONInt.kind)

	pnJSONUint, okJSONUint := parseNumber(json.Number("9223372036854775808"))
	require.True(t, okJSONUint)
	require.Equal(t, numUint, pnJSONUint.kind)

	pnJSONMaxUint, okJSONMaxUint := parseNumber(json.Number("18446744073709551615"))
	require.True(t, okJSONMaxUint)
	require.Equal(t, numUint, pnJSONMaxUint.kind)

	require.Equal(t, -1, compareParsed(pnJSONInt, pnJSONUint))
	require.Equal(t, 1, compareParsed(pnJSONUint, pnJSONInt))
	require.Equal(t, -1, compareParsed(pnJSONUint, pnJSONMaxUint))
	require.Equal(t, 1, compareParsed(pnJSONMaxUint, pnJSONUint))
	require.Equal(t, 0, compareParsed(pnJSONUint, pnJSONUint))
}

func TestCanonicalGroupValue_EdgeCases(t *testing.T) {
	t.Parallel()

	require.Equal(t, "bool:false", canonicalGroupValue(false))
	require.Equal(t, "struct {}:{}", canonicalGroupValue(struct{}{}))
}

func TestIsUnsafeFloatNumber(t *testing.T) {
	t.Parallel()

	// Non-numbers
	require.False(t, isUnsafeFloatNumber("abc"))
	require.False(t, isUnsafeFloatNumber(nil))

	// uint64 safe vs unsafe
	require.True(t, isUnsafeFloatNumber(uint64(math.MaxUint64)))
	require.False(t, isUnsafeFloatNumber(uint64(100)))

	// float64 safe vs unsafe / non-finite
	require.True(t, isUnsafeFloatNumber(math.NaN()))
	require.True(t, isUnsafeFloatNumber(math.Inf(1)))
	require.True(t, isUnsafeFloatNumber(math.Inf(-1)))
	require.True(t, isUnsafeFloatNumber(1e20))
	require.False(t, isUnsafeFloatNumber(1.5))
	require.False(t, isUnsafeFloatNumber(-1.5))
}
