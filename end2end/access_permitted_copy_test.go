package end2end

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnswerValue(t *testing.T) {
	assert.Equal(t, "<nothing>", answerValue(nil))
	assert.Equal(t, `"JP"`, answerValue("JP"))
	assert.Equal(t, "true", answerValue(true))
	assert.Equal(t, "3.000000", answerValue(3))
	assert.Equal(t, answerValue(int64(3)), answerValue(float64(3)), "a count is the same whatever type the adapter returns it in")
	assert.Equal(t, answerValue(uint8(3)), answerValue(float32(3)))
	assert.Equal(t, "[]uint8 [52 46 53]", answerValue([]byte("4.5")), "a value of another kind is not read as a number")
}

func TestAnswerMismatch(t *testing.T) {
	rows := func(counts ...int) []copyRow {
		answer := make([]copyRow, len(counts))
		for i, count := range counts {
			answer[i] = copyRow{"country": "IN", "n": count}
		}
		return answer
	}
	t.Run("the_same_rows", func(t *testing.T) {
		assert.Empty(t, answerMismatch(rows(1, 2), rows(1, 2), true))
		assert.Empty(t, answerMismatch(rows(1, 2), rows(1, 2), false))
	})
	t.Run("the_same_rows_in_another_order", func(t *testing.T) {
		assert.Empty(t, answerMismatch(rows(2, 1), rows(1, 2), false), "no order is asked for")
		assert.NotEmpty(t, answerMismatch(rows(2, 1), rows(1, 2), true), "the order is asked for")
	})
	t.Run("another_value", func(t *testing.T) {
		assert.Contains(t, answerMismatch(rows(1), rows(2), false), "the secured read gave 1 rows")
	})
	t.Run("another_number_of_rows", func(t *testing.T) {
		assert.NotEmpty(t, answerMismatch(rows(1), rows(1, 1), false))
		assert.NotEmpty(t, answerMismatch(nil, rows(1), false))
	})
	t.Run("another_column", func(t *testing.T) {
		got := []copyRow{{"country": "IN", "n": 1, "population": 5}}
		assert.NotEmpty(t, answerMismatch(got, rows(1), false), "a column the permitted copy does not hold")
	})
	t.Run("a_value_where_the_copy_has_none", func(t *testing.T) {
		assert.NotEmpty(t, answerMismatch([]copyRow{{"total": 0}}, []copyRow{{"total": nil}}, false))
	})
}

func TestAnswersOnACopy(t *testing.T) {
	copyOf := []copyRow{
		{"Country": "IN", "AreaSqKm": 10, "Name": "a"},
		{"Country": "IN", "AreaSqKm": 30, "Name": nil},
		{"Country": "JP", "AreaSqKm": nil},
	}
	numbers := numbersOf(copyOf, "AreaSqKm")
	assert.Equal(t, []float64{10, 30}, numbers, "a field that is not set is not counted")
	assert.EqualValues(t, 40, sumOf(numbers))
	assert.EqualValues(t, 20, averageOf(numbers))
	assert.EqualValues(t, 10, leastOf(numbers))
	assert.EqualValues(t, 30, mostOf(numbers))
	for name, aggregate := range map[string]func([]float64) any{"sum": sumOf, "average": averageOf, "least": leastOf, "most": mostOf} {
		assert.Nil(t, aggregate(nil), "%s of no value", name)
	}
	assert.Equal(t, 2, distinctCount(copyOf, "Country"))
	assert.Equal(t, []string{"a"}, textsOf(copyOf, "Name"))
	keys, groups := groupRows(copyOf, "Country")
	assert.Equal(t, []string{"IN", "JP"}, keys)
	assert.Len(t, groups["IN"], 2)
	assert.Len(t, keepRows(copyOf, func(row copyRow) bool { return row["Country"] == "JP" }), 1)
	ordered := orderRows(copyOf[:2], "AreaSqKm", true)
	assert.EqualValues(t, 30, ordered[0]["AreaSqKm"])
	assert.EqualValues(t, 10, orderRows(ordered, "AreaSqKm", false)[0]["AreaSqKm"])
	assert.Equal(t, []int{2, 3}, pageOf([]int{1, 2, 3, 4}, 1, 2))
	assert.Equal(t, []int{4}, pageOf([]int{1, 2, 3, 4}, 3, 2))
	assert.Empty(t, pageOf([]int{1}, 5, 2))
}
