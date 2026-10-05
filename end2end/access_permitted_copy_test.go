package end2end

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermittedCopyRefusal(t *testing.T) {
	tests := []struct {
		name         string
		caller       string
		err          error
		adapterCalls int64
		ok           bool
	}{
		{"field list server authorization refusal", "row_rule_and_field_list", fmt.Errorf("%w: authorization_unsupported", dal.ErrNotSupported), 1, true},
		{"multiple policies server authorization refusal", "two_policies_on_one_source", fmt.Errorf("%w: authorization_unsupported", dal.ErrNotSupported), 1, true},
		{"OR adapter refusal", "two_alternatives", fmt.Errorf("%w: query OR group conditions", dal.ErrNotSupported), 1, true},
		{"unlisted caller", "rule_over_a_list_of_many", fmt.Errorf("%w: authorization_unsupported", dal.ErrNotSupported), 0, false},
		{"unexpected authorization error", "row_rule_and_field_list", fmt.Errorf("%w: authorization denied", dal.ErrNotSupported), 1, false},
		{"wrong OR refusal", "two_alternatives", fmt.Errorf("%w: authorization_unsupported", dal.ErrNotSupported), 1, false},
		{"ordinary error", "two_alternatives", fmt.Errorf("query OR group conditions"), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, ok := permittedCopyRefusal(tt.caller, tt.err)
			if calls != tt.adapterCalls || ok != tt.ok {
				t.Fatalf("permittedCopyRefusal() = (%d, %t), want (%d, %t)", calls, ok, tt.adapterCalls, tt.ok)
			}
		})
	}
}

// This adapter runs the unprotected capability probe, then refuses the OR
// predicate introduced by the secured caller. Exercise the full conformance
// path rather than treating the unprotected probe as proof of policy support.
type permittedCopyRefusingDB struct {
	dal.DB
	queries int
}

func (d *permittedCopyRefusingDB) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	d.queries++
	return nil, fmt.Errorf("%w: query OR group conditions", dal.ErrNotSupported)
}

func TestPermittedCopySecuredRefusalAfterSupportedProbe(t *testing.T) {
	db := &permittedCopyRefusingDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	var caller copyCaller
	for _, candidate := range permittedCopyCallers() {
		if candidate.name == "two_alternatives" {
			caller = candidate
			break
		}
	}
	require.Equal(t, "two_alternatives", caller.name)
	permittedCopyCells()[0].verify(context.Background(), t, db, caller, permittedCopyRecords, permittedEntries()[0])
	assert.Equal(t, 1, db.queries, "the unprotected probe uses the supported transaction; the secured handle calls the refusing adapter once")
}

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

func TestRecordsetRows(t *testing.T) {
	build := func(names ...string) recordset.Recordset {
		columns := make([]recordset.Column[any], len(names))
		for i, name := range names {
			columns[i] = recordset.NewColumn[int](name, 0)
		}
		rs := recordset.NewColumnarRecordset("rows", columns...)
		row := rs.NewRow()
		for i := range names {
			require.NoError(t, row.SetValueByIndex(i, 7+i, rs))
		}
		return rs
	}
	t.Run("the_named_columns", func(t *testing.T) {
		rows, err := recordsetRows(build("country", "n"), []string{"country", "n"})
		require.NoError(t, err)
		assert.Equal(t, []copyRow{{"country": 7, "n": 8}}, rows)
	})
	t.Run("an_extra_column_is_refused", func(t *testing.T) {
		_, err := recordsetRows(build("n", "population"), []string{"n"})
		assert.ErrorContains(t, err, "the recordset holds the columns [n population], the read names [n]")
	})
	t.Run("a_missing_column_is_refused", func(t *testing.T) {
		_, err := recordsetRows(build("n"), []string{"total"})
		assert.ErrorContains(t, err, "column total of row 0")
	})
}

func TestReadCounterCountsEveryRead(t *testing.T) {
	ctx := context.Background()
	key := record.NewKeyWithID("Cities", "one")
	query := dal.From(dal.NewRootCollectionRef("Cities", "")).NewQuery().SelectKeysOnly(0)
	reads := &atomic.Int64{}
	db := readCounter{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile()), reads: reads}
	calls := map[string]func(qe dal.QueryExecutor, getter dal.ReadSession) error{
		"get": func(_ dal.QueryExecutor, getter dal.ReadSession) error {
			return getter.Get(ctx, record.NewRecordWithData(key, map[string]any{}))
		},
		"get_multi": func(_ dal.QueryExecutor, getter dal.ReadSession) error {
			return getter.GetMulti(ctx, []record.Record{record.NewRecordWithData(key, map[string]any{})})
		},
		"exists": func(_ dal.QueryExecutor, getter dal.ReadSession) error {
			_, err := getter.Exists(ctx, key)
			return err
		},
		"records_reader": func(qe dal.QueryExecutor, _ dal.ReadSession) error {
			_, err := qe.ExecuteQueryToRecordsReader(ctx, query)
			return err
		},
		"recordset_reader": func(qe dal.QueryExecutor, _ dal.ReadSession) error {
			_, _ = qe.ExecuteQueryToRecordsetReader(ctx, query)
			return nil
		},
	}
	for name, call := range calls {
		t.Run(name+"_on_the_handle", func(t *testing.T) {
			reads.Store(0)
			_ = call(db, db)
			assert.EqualValues(t, 1, reads.Load())
		})
		t.Run(name+"_in_a_transaction", func(t *testing.T) {
			reads.Store(0)
			require.NoError(t, db.RunReadonlyTransaction(ctx, func(_ context.Context, tx dal.ReadTransaction) error {
				_ = call(tx, tx)
				return nil
			}))
			assert.EqualValues(t, 1, reads.Load())
		})
	}
}
