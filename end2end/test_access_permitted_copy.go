package end2end

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/end2end/models"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rule these cells prove: a read of one source through a secured database
// gives exactly the answer the same read gives, without a policy, on a copy of
// the data that holds only the rows and only the fields the caller may read.
//
// For each caller (a policy plus the context it is read under) the suite builds
// that copy by hand from the fixture rows (filtering them in Go and dropping the
// fields the caller may not read), works out the answer of every cell on it in
// Go, and compares that with what the adapter under test returns through the
// secured database, on both readers (records and recordset) and on both entry
// points (the database handle and a read transaction). A read that names a field
// the caller may not read is not answered at all: it is refused by the access
// layer, with the code of a hidden column, before the adapter is asked.
//
// The copy is a model in Go and not a second database because the shared suite
// runs on adapters that cannot create a collection the suite does not name.
//
// The fields of the copy are compared by the whole-row cell, which reads rows
// with no column named; every other cell reads Name, Country and AreaSqKm, which
// every field list allows, and the fields outside the lists are proved hidden by
// the denial cells. The fixture holds no NULL, so the null tests meet either every
// row or none.

const (
	permittedCopyRecords   = "records"
	permittedCopyRecordset = "recordset"

	permittedCopyProbeTx  = "access permitted copy: probe"
	permittedCopyReadTx   = "access permitted copy: read"
	permittedCopyDeniedTx = "access permitted copy: denied"
)

// copyRow is one row of an answer: the output column name and its value.
type copyRow map[string]any

// copyCaller is one caller of the secured database: the policies it is secured
// with, the context that carries its identity and variables, the rows of the
// fixture it may read and the fields of those rows it may read.
type copyCaller struct {
	name     string
	policies []access.Policy
	identify func(context.Context) context.Context
	readable func(models.City) bool
	fields   []string // nil: every field
	// wildcardList marks a field list that names a pattern: the access layer cannot
	// enumerate the columns of a read that names none, so what such a read returns
	// is the adapter's own. The denial cells hold the list; whole rows are not read.
	wildcardList bool
	// listRule marks a caller whose rule tests a field against a list. An adapter
	// that does not evaluate that test (the in-memory adapter in its Firestore
	// profile reads it as "the field is an array holding any of these") cannot
	// answer the caller's reads: the suite pins what it answers instead and skips
	// the caller's cells there (see pinListRuleAnswer).
	listRule bool
}

// hasFieldList reports whether the caller's policy restricts the fields.
func (c copyCaller) hasFieldList() bool { return c.fields != nil }

// cityRow is a city as the cities fixture stores it, by the names the suite
// spells its fields with.
func cityRow(city models.City) copyRow {
	return copyRow{
		"Name": city.Name, "State": city.State, "Country": city.Country,
		"Population": city.Population, "AreaSqKm": city.AreaSqKm,
		"IsCapital": city.IsCapital, "HasAirport": city.HasAirport,
	}
}

// permittedCopy is the copy of the fixture the caller may read: the rows it may
// read, each holding only the fields it may read.
func (c copyCaller) permittedCopy() []copyRow {
	var rows []copyRow
	for _, city := range models.Cities {
		if !c.readable(city) {
			continue
		}
		row := cityRow(city)
		if c.hasFieldList() {
			for name := range row {
				if !containsString(c.fields, name) {
					delete(row, name)
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// secure returns the database the caller reads through. The adapter under test
// sits behind a counter, so a cell can assert how many reads reached it.
func (c copyCaller) secure(db dal.DB, reads *atomic.Int64) dal.DB {
	return access.MustSecureDB(readCounter{DB: db, reads: reads}, access.WithDatabasePolicies(c.policies...))
}

// readCounter counts the reads that reach the adapter below the access layer: the
// queries and the reads by key (Get, GetMulti and Exists).
type readCounter struct {
	dal.DB
	reads *atomic.Int64
}

func (c readCounter) Get(ctx context.Context, rec record.Record) error {
	c.reads.Add(1)
	return c.DB.Get(ctx, rec)
}

func (c readCounter) GetMulti(ctx context.Context, records []record.Record) error {
	c.reads.Add(1)
	return c.DB.GetMulti(ctx, records)
}

func (c readCounter) Exists(ctx context.Context, key *record.Key) (bool, error) {
	c.reads.Add(1)
	return c.DB.Exists(ctx, key)
}

func (c readCounter) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	c.reads.Add(1)
	return c.DB.ExecuteQueryToRecordsReader(ctx, query)
}

func (c readCounter) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	c.reads.Add(1)
	return c.DB.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func (c readCounter) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	return c.DB.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return worker(ctx, readCountingTx{ReadTransaction: tx, reads: c.reads})
	}, options...)
}

type readCountingTx struct {
	dal.ReadTransaction
	reads *atomic.Int64
}

func (t readCountingTx) Get(ctx context.Context, rec record.Record) error {
	t.reads.Add(1)
	return t.ReadTransaction.Get(ctx, rec)
}

func (t readCountingTx) GetMulti(ctx context.Context, records []record.Record) error {
	t.reads.Add(1)
	return t.ReadTransaction.GetMulti(ctx, records)
}

func (t readCountingTx) Exists(ctx context.Context, key *record.Key) (bool, error) {
	t.reads.Add(1)
	return t.ReadTransaction.Exists(ctx, key)
}

func (t readCountingTx) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	t.reads.Add(1)
	return t.ReadTransaction.ExecuteQueryToRecordsReader(ctx, query)
}

func (t readCountingTx) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	t.reads.Add(1)
	return t.ReadTransaction.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

// permittedEntry is a way into the secured database: its handle, or a read
// transaction. The read is handed the context to read with (the transaction's
// own, in a transaction) and the query executor.
type permittedEntry struct {
	name string
	run  func(ctx context.Context, db dal.DB, message string, read func(context.Context, dal.QueryExecutor) error) error
}

func permittedEntries() []permittedEntry {
	return []permittedEntry{
		{"handle", func(ctx context.Context, db dal.DB, _ string, read func(context.Context, dal.QueryExecutor) error) error {
			return read(ctx, db)
		}},
		{"transaction", func(ctx context.Context, db dal.DB, message string, read func(context.Context, dal.QueryExecutor) error) error {
			return db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
				return read(ctx, tx)
			}, dal.TxWithMessage(message))
		}},
	}
}

// readCopyRows runs a query on one reader and returns its rows by output column.
// Rows read through the records reader come back as the adapter gives them, every
// key of the map included; rows read through the recordset reader hold the named
// columns, and a recordset that holds any other number of columns is an error.
func readCopyRows(ctx context.Context, qe dal.QueryExecutor, reader string, q dal.Query, columns []string) ([]copyRow, error) {
	if reader == permittedCopyRecordset {
		rs, err := dal.ExecuteQueryAndReadAllToRecordset(ctx, q, qe)
		if err != nil {
			return nil, err
		}
		return recordsetRows(rs, columns)
	}
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, qe)
	if err != nil {
		return nil, err
	}
	rows := make([]copyRow, len(records))
	for i, rec := range records {
		// A record that holds no map is a row with no field.
		data, _ := rec.Data().(map[string]any)
		rows[i] = data
	}
	return rows, nil
}

// recordsetRows is the rows of a recordset by the named columns. A recordset that
// holds a column the read does not name (a hidden field, a generated alias) is
// refused, as the records reader's extra key is a mismatch.
func recordsetRows(rs recordset.Recordset, columns []string) ([]copyRow, error) {
	if rs.ColumnsCount() != len(columns) {
		held := make([]string, rs.ColumnsCount())
		for i, column := range rs.Columns() {
			held[i] = column.Name()
		}
		return nil, fmt.Errorf("the recordset holds the columns %v, the read names %v", held, columns)
	}
	rows := make([]copyRow, rs.RowsCount())
	for i := range rows {
		rows[i] = copyRow{}
		for _, column := range columns {
			value, err := rs.GetRow(i).GetValueByName(column, rs)
			if err != nil {
				return nil, fmt.Errorf("column %s of row %d: %w", column, i, err)
			}
			rows[i][column] = value
		}
	}
	return rows, nil
}

// answerValue is a value of an answer in a form the adapters agree on: every
// number as a float with six decimals, text and booleans as they are, nothing as
// a marker. A value of another kind keeps its type in its text, so that it is
// never equal to a number or to text.
func answerValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "<nothing>"
	case string:
		return strconv.Quote(v)
	case bool:
		return strconv.FormatBool(v)
	default:
		if number := reflect.ValueOf(v); number.CanFloat() || number.CanInt() || number.CanUint() {
			return strconv.FormatFloat(floatOf(v), 'f', 6, 64)
		}
		return fmt.Sprintf("%T %v", v, v)
	}
}

// floatOf is a number of any Go numeric type as a float64. It is for the values of
// the Go model and for values already known to be numbers; a value an adapter
// returned is read with numberOf, which fails the cell instead of panicking.
func floatOf(value any) float64 {
	return reflect.ValueOf(value).Convert(reflect.TypeOf(0.0)).Float()
}

func answerLine(row copyRow) string {
	names := make([]string, 0, len(row))
	for name := range row {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + answerValue(row[name])
	}
	return strings.Join(parts, ", ")
}

func answerLines(rows []copyRow, ordered bool) []string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = answerLine(row)
	}
	if !ordered {
		sort.Strings(lines)
	}
	return lines
}

// answerMismatch says how two answers differ, or returns "" when they are the
// same: the same columns with the same values in as many rows, in the same order
// when the query orders its rows and in any order when it does not.
func answerMismatch(got, want []copyRow, ordered bool) string {
	gotLines, wantLines := answerLines(got, ordered), answerLines(want, ordered)
	if strings.Join(gotLines, "\n") == strings.Join(wantLines, "\n") {
		return ""
	}
	return fmt.Sprintf("the secured read gave %d rows\n  %s\nthe permitted copy gives %d rows\n  %s",
		len(gotLines), strings.Join(gotLines, "\n  "), len(wantLines), strings.Join(wantLines, "\n  "))
}

// Answers on a copy, in Go. A number is a float64 and an aggregate over no value
// is nothing, as in SQL.

func numbersOf(rows []copyRow, field string) []float64 {
	var numbers []float64
	for _, row := range rows {
		if value, ok := row[field]; ok && value != nil {
			numbers = append(numbers, floatOf(value))
		}
	}
	return numbers
}

func sumOf(numbers []float64) any {
	if len(numbers) == 0 {
		return nil
	}
	sum := 0.0
	for _, number := range numbers {
		sum += number
	}
	return sum
}

func averageOf(numbers []float64) any {
	if len(numbers) == 0 {
		return nil
	}
	return sumOf(numbers).(float64) / float64(len(numbers))
}

func leastOf(numbers []float64) any {
	if len(numbers) == 0 {
		return nil
	}
	least := numbers[0]
	for _, number := range numbers {
		least = min(least, number)
	}
	return least
}

func mostOf(numbers []float64) any {
	if len(numbers) == 0 {
		return nil
	}
	most := numbers[0]
	for _, number := range numbers {
		most = max(most, number)
	}
	return most
}

func distinctCount(rows []copyRow, field string) int {
	seen := map[any]bool{}
	for _, row := range rows {
		if value, ok := row[field]; ok && value != nil {
			seen[value] = true
		}
	}
	return len(seen)
}

func keepRows(rows []copyRow, keep func(copyRow) bool) []copyRow {
	var kept []copyRow
	for _, row := range rows {
		if keep(row) {
			kept = append(kept, row)
		}
	}
	return kept
}

// groupRows groups rows by the text of one field, in the order of that text.
func groupRows(rows []copyRow, field string) (keys []string, groups map[string][]copyRow) {
	groups = map[string][]copyRow{}
	for _, row := range rows {
		key := row[field].(string)
		if _, seen := groups[key]; !seen {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], row)
	}
	sort.Strings(keys)
	return keys, groups
}

// orderRows sorts rows by a number field, ascending or descending.
func orderRows(rows []copyRow, field string, descending bool) []copyRow {
	ordered := append([]copyRow(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := floatOf(ordered[i][field]), floatOf(ordered[j][field])
		if descending {
			return left > right
		}
		return left < right
	})
	return ordered
}

// pageOf is the rows from offset, at most limit of them.
func pageOf[T any](rows []T, offset, limit int) []T {
	rows = rows[min(offset, len(rows)):]
	return rows[:min(limit, len(rows))]
}

// permittedCopyCell is one read of one source and what it must answer on the
// permitted copy of any caller.
type permittedCopyCell struct {
	name    string
	columns []string
	ordered bool
	// aliasRefs marks a read that names an aggregate by its alias in HAVING and
	// ORDER BY: not every adapter runs one, and under a field list the recordset
	// reader sends the aggregate under the caller's own alias and refuses to
	// pass such a name (see access_field_lists, having_over_the_alias_is_refused_on_the_recordset_path).
	aliasRefs bool
	// recordsOnly marks a read of whole rows, which only the records reader serves.
	recordsOnly bool
	// fieldListOnly marks a read that an adapter runs only because the access layer
	// turns it into a read of named columns: a caller with a list of named fields.
	fieldListOnly bool
	query         func() dal.Query
	answer        func(copy []copyRow) []copyRow
}

func permittedCopyCells() []permittedCopyCell {
	cities := func() dal.IQueryBuilder {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery()
	}
	countAs := func(alias string) dal.Column {
		count := dal.Count()
		count.Alias = alias
		return count
	}
	area := dal.Field("AreaSqKm")
	country := dal.Column{Alias: "country", Expression: dal.Field("Country")}
	one := func(row copyRow) []copyRow { return []copyRow{row} }
	areaOf := func(rows []copyRow) []float64 { return numbersOf(rows, "AreaSqKm") }
	greaterThan := func(expression dal.Expression, value int) dal.Condition {
		return dal.NewComparison(expression, dal.GreaterThen, dal.NewConstant(value))
	}
	// perCountry answers a grouped read: one row per country of the copy that the
	// summary keeps, in the order of the country codes.
	perCountry := func(rows []copyRow, summary func(country string, group []copyRow) copyRow) []copyRow {
		var answer []copyRow
		keys, groups := groupRows(rows, "Country")
		for _, key := range keys {
			if row := summary(key, groups[key]); row != nil {
				answer = append(answer, row)
			}
		}
		return answer
	}
	countAndArea := func(key string, group []copyRow) copyRow {
		return copyRow{"country": key, "n": len(group), "area": sumOf(areaOf(group))}
	}
	cityAndArea := []dal.Column{{Alias: "city", Expression: dal.Field("Name")}, {Alias: "area", Expression: area}}
	cityAndAreaRow := func(row copyRow) copyRow { return copyRow{"city": row["Name"], "area": row["AreaSqKm"]} }
	cityAndAreaRows := func(rows []copyRow) []copyRow {
		answer := make([]copyRow, len(rows))
		for i, row := range rows {
			answer[i] = cityAndAreaRow(row)
		}
		return answer
	}
	big := func(row copyRow) bool { return floatOf(row["AreaSqKm"]) > 1500 }

	return []permittedCopyCell{
		{name: "count_star", columns: []string{"n"},
			query:  func() dal.Query { return cities().SelectColumns(countAs("n")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"n": len(rows)}) }},
		{name: "count_of_a_field", columns: []string{"named"},
			query:  func() dal.Query { return cities().SelectColumns(dal.CountAs(dal.Field("Name"), "named")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"named": len(textsOf(rows, "Name"))}) }},
		{name: "count_distinct", columns: []string{"countries"},
			query: func() dal.Query {
				return cities().SelectColumns(dal.CountDistinctAs(dal.Field("Country"), "countries"))
			},
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"countries": distinctCount(rows, "Country")}) }},
		{name: "sum", columns: []string{"total"},
			query:  func() dal.Query { return cities().SelectColumns(dal.SumAs(area, "total")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"total": sumOf(areaOf(rows))}) }},
		{name: "average", columns: []string{"mean"},
			query:  func() dal.Query { return cities().SelectColumns(dal.AverageAs(area, "mean")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"mean": averageOf(areaOf(rows))}) }},
		{name: "minimum", columns: []string{"least"},
			query:  func() dal.Query { return cities().SelectColumns(dal.MinAs(area, "least")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"least": leastOf(areaOf(rows))}) }},
		{name: "maximum", columns: []string{"most"},
			query:  func() dal.Query { return cities().SelectColumns(dal.MaxAs(area, "most")) },
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"most": mostOf(areaOf(rows))}) }},
		{name: "every_aggregate_at_once", columns: []string{"n", "named", "total", "mean", "least", "most"},
			query: func() dal.Query {
				return cities().SelectColumns(countAs("n"), dal.CountAs(dal.Field("Name"), "named"), dal.SumAs(area, "total"),
					dal.AverageAs(area, "mean"), dal.MinAs(area, "least"), dal.MaxAs(area, "most"))
			},
			answer: func(rows []copyRow) []copyRow {
				return one(copyRow{"n": len(rows), "named": len(textsOf(rows, "Name")), "total": sumOf(areaOf(rows)),
					"mean": averageOf(areaOf(rows)), "least": leastOf(areaOf(rows)), "most": mostOf(areaOf(rows))})
			}},
		{name: "group_by", columns: []string{"country", "n", "area"},
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).SelectColumns(country, countAs("n"), dal.SumAs(area, "area"))
			},
			answer: func(rows []copyRow) []copyRow { return perCountry(rows, countAndArea) }},
		{name: "group_by_having_count", columns: []string{"country", "n"},
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).Having(greaterThan(dal.Count().Expression, 1)).
					SelectColumns(country, countAs("n"))
			},
			answer: func(rows []copyRow) []copyRow {
				return perCountry(rows, func(key string, group []copyRow) copyRow {
					if len(group) <= 1 {
						return nil
					}
					return copyRow{"country": key, "n": len(group)}
				})
			}},
		{name: "group_by_having_sum", columns: []string{"country", "area"},
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).Having(greaterThan(dal.SumAs(area, "").Expression, 2000)).
					SelectColumns(country, dal.SumAs(area, "area"))
			},
			answer: func(rows []copyRow) []copyRow {
				return perCountry(rows, func(key string, group []copyRow) copyRow {
					if sum := sumOf(areaOf(group)).(float64); sum <= 2000 {
						return nil
					}
					return copyRow{"country": key, "area": sumOf(areaOf(group))}
				})
			}},
		{name: "order_by", columns: []string{"city", "area"}, ordered: true,
			query: func() dal.Query {
				return cities().OrderBy(dal.DescendingField("AreaSqKm")).SelectColumns(cityAndArea...)
			},
			answer: func(rows []copyRow) []copyRow { return cityAndAreaRows(orderRows(rows, "AreaSqKm", true)) }},
		{name: "group_by_order_by", columns: []string{"country", "n"}, ordered: true,
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).OrderBy(dal.AscendingField("Country")).SelectColumns(country, countAs("n"))
			},
			answer: func(rows []copyRow) []copyRow {
				return perCountry(rows, func(key string, group []copyRow) copyRow { return copyRow{"country": key, "n": len(group)} })
			}},
		{name: "alias_in_having_and_order_by", columns: []string{"country", "total"}, ordered: true, aliasRefs: true,
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).
					Having(greaterThan(dal.Field("total"), 1000)).OrderBy(dal.DescendingField("total")).
					SelectColumns(country, dal.SumAs(area, "total"))
			},
			answer: func(rows []copyRow) []copyRow {
				kept := perCountry(rows, func(key string, group []copyRow) copyRow {
					if sum := sumOf(areaOf(group)).(float64); sum <= 1000 {
						return nil
					}
					return copyRow{"country": key, "total": sumOf(areaOf(group))}
				})
				return orderRows(kept, "total", true)
			}},
		{name: "null_test_keeps_every_row", columns: []string{"n"},
			query: func() dal.Query {
				return cities().Where(dal.NewIsNotNullCondition(dal.Field("Country"))).SelectColumns(countAs("n"))
			},
			answer: func(rows []copyRow) []copyRow { return one(copyRow{"n": len(rows)}) }},
		{name: "null_test_keeps_no_row", columns: []string{"n"},
			query: func() dal.Query {
				return cities().Where(dal.NewIsNullCondition(dal.Field("Country"))).SelectColumns(countAs("n"))
			},
			answer: func([]copyRow) []copyRow { return one(copyRow{"n": 0}) }},
		{name: "limit_and_offset", columns: []string{"city", "area"}, ordered: true,
			query: func() dal.Query {
				return cities().OrderBy(dal.AscendingField("AreaSqKm")).Offset(1).Limit(2).SelectColumns(cityAndArea...)
			},
			answer: func(rows []copyRow) []copyRow {
				return cityAndAreaRows(pageOf(orderRows(rows, "AreaSqKm", false), 1, 2))
			}},
		{name: "group_by_limit_and_offset", columns: []string{"country", "n"}, ordered: true,
			query: func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).OrderBy(dal.AscendingField("Country")).Offset(1).Limit(2).
					SelectColumns(country, countAs("n"))
			},
			answer: func(rows []copyRow) []copyRow {
				return pageOf(perCountry(rows, func(key string, group []copyRow) copyRow { return copyRow{"country": key, "n": len(group)} }), 1, 2)
			}},
		{name: "callers_own_condition_on_rows", columns: []string{"city", "area"},
			query: func() dal.Query {
				return cities().Where(greaterThan(area, 1500)).SelectColumns(cityAndArea...)
			},
			answer: func(rows []copyRow) []copyRow { return cityAndAreaRows(keepRows(rows, big)) }},
		{name: "callers_own_condition_on_an_aggregate", columns: []string{"n", "total"},
			query: func() dal.Query {
				return cities().Where(greaterThan(area, 1500)).SelectColumns(countAs("n"), dal.SumAs(area, "total"))
			},
			answer: func(rows []copyRow) []copyRow {
				kept := keepRows(rows, big)
				return one(copyRow{"n": len(kept), "total": sumOf(areaOf(kept))})
			}},
		// A read that names no column: the rows the caller may read, with the fields the
		// caller may read and no other. Under no field list an adapter reads the keys
		// alone, so the cell is read for the callers with one, where the access layer
		// turns it into a read of the fields the list allows.
		{name: "whole_rows", recordsOnly: true, fieldListOnly: true,
			query:  func() dal.Query { return cities().SelectKeysOnly(reflect.String) },
			answer: func(rows []copyRow) []copyRow { return rows }},
	}
}

// textsOf is the values of a text field that are set.
func textsOf(rows []copyRow, field string) []string {
	var texts []string
	for _, row := range rows {
		if value, ok := row[field]; ok && value != nil {
			texts = append(texts, value.(string))
		}
	}
	return texts
}

// permittedCopyCallers are the callers the cells run for: no restriction as the
// control, every kind of policy on one source, and the shapes of a rule: one that
// names a variable, the caller, a list of any length, two alternatives, and a
// conditional rule with an unconditional one behind it.
func permittedCopyCallers() []copyCaller {
	listed := []string{"Name", "Country", "AreaSqKm"}
	var everyone = func(models.City) bool { return true }
	inCountry := func(codes ...string) func(models.City) bool {
		return func(city models.City) bool { return containsString(codes, city.Country) }
	}
	identity := func(ctx context.Context) context.Context { return ctx }
	forCountry := func(code string) func(context.Context) context.Context {
		return func(ctx context.Context) context.Context {
			return access.WithVariables(ctx, map[string]any{"country": code})
		}
	}
	withPrincipal := func(id any, roles ...string) func(context.Context) context.Context {
		return func(ctx context.Context) context.Context {
			return access.WithPrincipal(ctx, access.Principal{ID: id, Roles: roles})
		}
	}
	allow := func(name string) access.Rule { return access.Allow(access.Query, name) }
	policy := func(name string, rules ...access.Rule) access.Policy {
		return access.MustPolicy(name, access.Collection(models.CitiesCollection, rules...))
	}
	countryIs := dal.WhereField("Country", dal.Equal, dal.NewParam("country"))
	capital := dal.WhereField("IsCapital", dal.Equal, true)

	return []copyCaller{
		{name: "control_no_restriction", policies: []access.Policy{policy("control", allow("read-all"))},
			identify: identity, readable: everyone},
		{name: "row_rule_one_row", policies: []access.Policy{policy("rows-by-country", allow("own-country").Where(countryIs))},
			identify: forCountry("JP"), readable: inCountry("JP")},
		{name: "row_rule_two_rows", policies: []access.Policy{policy("rows-by-country", allow("own-country").Where(countryIs))},
			identify: forCountry("IN"), readable: inCountry("IN")},
		{name: "row_rule_no_row", policies: []access.Policy{policy("rows-by-country", allow("own-country").Where(countryIs))},
			identify: forCountry("ZZ"), readable: inCountry("ZZ")},
		{name: "field_list", policies: []access.Policy{policy("fields", allow("listed-fields").Fields(listed...))},
			identify: identity, readable: everyone, fields: listed},
		{name: "field_list_with_a_wildcard", policies: []access.Policy{policy("fields", allow("listed-fields").Fields("Name", "Count*", "AreaSqKm"))},
			identify: identity, readable: everyone, fields: listed, wildcardList: true},
		{name: "row_rule_and_field_list", policies: []access.Policy{policy("capitals", allow("listed-capitals").Where(capital).Fields(listed...))},
			identify: identity, readable: func(city models.City) bool { return city.IsCapital }, fields: listed},
		{name: "two_policies_on_one_source", policies: []access.Policy{
			policy("capitals", allow("listed-capitals").Where(capital).Fields(listed...)),
			policy("by-area", allow("large").Where(dal.WhereField("AreaSqKm", dal.GreaterThen, 1000)).Fields("Name", "Country", "AreaSqKm", "IsCapital")),
		}, identify: identity,
			readable: func(city models.City) bool { return city.IsCapital && city.AreaSqKm > 1000 }, fields: listed},
		{name: "rule_naming_the_caller", policies: []access.Policy{policy("callers-country",
			allow("own-country").Where(dal.WhereField("Country", dal.Equal, dal.NewParam("currentUser"))))},
			identify: withPrincipal("IN"), readable: inCountry("IN")},
		{name: "rule_over_a_list_of_many", policies: []access.Policy{policy("roles",
			allow("by-role").Where(dal.WhereField("Country", dal.In, dal.NewParam("principal.roles"))))},
			identify: withPrincipal("someone", "IN", "CN", "BR"), readable: inCountry("IN", "CN", "BR"), listRule: true},
		{name: "rule_over_a_list_of_one", policies: []access.Policy{policy("roles",
			allow("by-role").Where(dal.WhereField("Country", dal.In, dal.NewParam("principal.roles"))))},
			identify: withPrincipal("someone", "TR"), readable: inCountry("TR"), listRule: true},
		{name: "rule_over_an_empty_list", policies: []access.Policy{policy("roles",
			allow("by-role").Where(dal.WhereField("Country", dal.In, dal.NewParam("principal.roles"))))},
			identify: withPrincipal("someone"), readable: inCountry(), listRule: true},
		{name: "two_alternatives", policies: []access.Policy{policy("alternatives",
			allow("own-country").Where(dal.WhereField("Country", dal.Equal, "JP")), allow("capitals").Where(capital))},
			identify: identity, readable: func(city models.City) bool { return city.Country == "JP" || city.IsCapital }},
		{name: "conditional_rule_with_an_unconditional_allow_behind_it", policies: []access.Policy{policy("everything",
			allow("own-country").Where(dal.WhereField("Country", dal.Equal, "JP")), allow("everyone"))},
			identify: identity, readable: everyone},
	}
}

// evaluatesMembership reports whether the adapter evaluates a test of a field
// against a list, as a rule over $principal.roles needs it to.
func evaluatesMembership(ctx context.Context, t *testing.T, db dal.DB) bool {
	t.Helper()
	count := dal.Count()
	count.Alias = "n"
	probe := permittedCopyCell{columns: []string{"n"}, query: func() dal.Query {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().
			Where(dal.NewComparison(dal.Field("Country"), dal.In, dal.NewArray([]string{"IN", "CN"}))).SelectColumns(count)
	}}
	rows, err := probe.read(ctx, db, permittedCopyRecords, permittedEntries()[1], permittedCopyProbeTx)
	return err == nil && len(rows) == 1 && answerValue(rows[0]["n"]) == answerValue(4)
}

// listRuleStatement names the read that a caller whose rule tests a field against
// a list is pinned with on an adapter that does not evaluate such a test.
const listRuleStatement = "SELECT COUNT(*) FROM Cities, secured by Country IN $principal.roles"

// listRuleAnswers reads COUNT(*) of the source through the secured database of a
// caller whose rule tests a field against a list, on each entry, and returns the
// answers. An adapter that does not evaluate the test cannot answer the caller's
// cells; this is what it answers instead.
func listRuleAnswers(ctx context.Context, t *testing.T, db dal.DB, caller copyCaller) ([]float64, error) {
	t.Helper()
	count := permittedCopyCells()[0]
	require.Equal(t, "count_star", count.name)
	// The same read without a policy tells whether the adapter runs it at all.
	if _, err := count.read(caller.identify(ctx), db, permittedCopyRecords, permittedEntries()[1], permittedCopyProbeTx); err != nil {
		return nil, err
	}
	var answers []float64
	for _, entry := range permittedEntries() {
		rows, err := count.read(caller.identify(ctx), caller.secure(db, &atomic.Int64{}), permittedCopyRecords, entry, permittedCopyReadTx)
		if err != nil {
			return nil, err
		}
		require.Len(t, rows, 1, "%s on the %s", listRuleStatement, entry.name)
		answers = append(answers, numberOf(t, rows[0]["n"]))
	}
	return answers, nil
}

// pinListRuleAnswer is what the suite holds an adapter to when it does not
// evaluate a test of a field against a list: the rule must not serve a row the
// caller may not read. The answer it gives is named in the skip message.
func pinListRuleAnswer(ctx context.Context, t *testing.T, db dal.DB, caller copyCaller) {
	t.Helper()
	answers, err := listRuleAnswers(ctx, t, db, caller)
	if errors.Is(err, dal.ErrNotSupported) {
		t.Skip("adapter does not run this read:", err)
	}
	require.NoError(t, err)
	for _, answer := range answers {
		assert.LessOrEqual(t, answer, float64(len(caller.permittedCopy())), "%s serves more rows than the caller may read", listRuleStatement)
	}
	t.Skipf("adapter does not evaluate a test of a field against a list: %s answers %v, the permitted copy holds %d rows",
		listRuleStatement, answers, len(caller.permittedCopy()))
}

// runsOn reports whether the cell is read through the reader. A whole-row read
// has no column names to read a recordset by.
func (cell permittedCopyCell) runsOn(reader string) bool {
	return !cell.recordsOnly || reader == permittedCopyRecords
}

// runsFor reports whether the cell is read for the caller.
func (cell permittedCopyCell) runsFor(caller copyCaller) bool {
	return !cell.fieldListOnly || (caller.hasFieldList() && !caller.wildcardList)
}

// verify runs the cell for one caller on one reader through one entry and
// compares the secured answer with the answer on the caller's permitted copy.
func (cell permittedCopyCell) verify(ctx context.Context, t *testing.T, db dal.DB, caller copyCaller, reader string, entry permittedEntry) {
	t.Helper()
	if cell.aliasRefs && caller.hasFieldList() && reader == permittedCopyRecordset {
		// Under a field list the recordset reader sends the aggregate under the
		// caller's own alias and refuses a read that names the alias in HAVING or
		// ORDER BY (see access_field_lists, having_over_the_alias_is_refused_on_the_recordset_path).
		verifyDenied(ctx, t, db, caller, cell.query(), reader, entry, access.CodeColumnDenied)
		return
	}
	ctx = caller.identify(ctx)
	// The same read without a policy tells whether the adapter runs it at all.
	_, probe := cell.read(ctx, db, reader, permittedEntries()[1], permittedCopyProbeTx)
	if errors.Is(probe, dal.ErrNotSupported) || (cell.aliasRefs && probe != nil) {
		t.Skip("adapter does not run this read:", probe)
	}
	require.NoError(t, probe)

	reads := &atomic.Int64{}
	got, err := cell.read(ctx, caller.secure(db, reads), reader, entry, permittedCopyReadTx)
	require.NoError(t, err)
	assert.Empty(t, answerMismatch(got, cell.answer(caller.permittedCopy()), cell.ordered), "the secured read against the permitted copy")
	assert.EqualValues(t, 1, reads.Load(), "the adapter is asked once")
}

// read runs the cell's query through one entry of db.
func (cell permittedCopyCell) read(ctx context.Context, db dal.DB, reader string, entry permittedEntry, message string) (rows []copyRow, err error) {
	err = entry.run(ctx, db, message, func(ctx context.Context, qe dal.QueryExecutor) error {
		var readErr error
		rows, readErr = readCopyRows(ctx, qe, reader, cell.query(), cell.columns)
		return readErr
	})
	return rows, err
}

// hiddenRead is a read of one source that names a field the callers with a field
// list may not read, in the clause the name says. The fields are Population and
// State; IsCapital is read by the rule of one caller and by no field list.
type hiddenRead struct {
	name  string
	query func() dal.Query
}

func hiddenReads() []hiddenRead {
	cities := func() dal.IQueryBuilder {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery()
	}
	name := dal.Column{Alias: "city", Expression: dal.Field("Name")}
	count := dal.Count()
	count.Alias = "n"
	populationOver := func(value int) dal.Condition {
		return dal.NewComparison(dal.Field("Population"), dal.GreaterThen, dal.NewConstant(value))
	}
	return []hiddenRead{
		{"in_the_select_list", func() dal.Query {
			return cities().SelectColumns(dal.Column{Alias: "p", Expression: dal.Field("Population")})
		}},
		{"under_an_allowed_alias", func() dal.Query {
			return cities().SelectColumns(dal.Column{Alias: "Name", Expression: dal.Field("Population")})
		}},
		{"every_selected_column_refused", func() dal.Query {
			return cities().SelectColumns(dal.Column{Alias: "p", Expression: dal.Field("Population")}, dal.Column{Alias: "s", Expression: dal.Field("State")})
		}},
		{"in_where", func() dal.Query { return cities().Where(populationOver(1)).SelectColumns(name) }},
		{"in_where_of_an_aggregate", func() dal.Query { return cities().Where(populationOver(1)).SelectColumns(count) }},
		{"in_a_null_test", func() dal.Query {
			return cities().Where(dal.NewIsNullCondition(dal.Field("State"))).SelectColumns(count)
		}},
		{"in_group_by", func() dal.Query { return cities().GroupBy(dal.Field("State")).SelectColumns(count) }},
		{"in_having", func() dal.Query {
			return cities().GroupBy(dal.Field("Country")).Having(populationOver(1)).SelectColumns(count)
		}},
		{"in_having_through_an_aggregate", func() dal.Query {
			return cities().GroupBy(dal.Field("Country")).
				Having(dal.NewComparison(dal.SumAs(dal.Field("Population"), "").Expression, dal.GreaterThen, dal.NewConstant(1))).
				SelectColumns(count)
		}},
		{"in_order_by", func() dal.Query { return cities().OrderBy(dal.DescendingField("Population")).SelectColumns(name) }},
		{"in_order_by_with_limit_and_offset", func() dal.Query {
			return cities().OrderBy(dal.AscendingField("State")).Offset(1).Limit(2).SelectColumns(name)
		}},
		{"as_the_operand_of_sum", func() dal.Query { return cities().SelectColumns(dal.SumAs(dal.Field("Population"), "total")) }},
		{"as_the_operand_of_average", func() dal.Query { return cities().SelectColumns(dal.AverageAs(dal.Field("Population"), "mean")) }},
		{"as_the_operand_of_minimum", func() dal.Query { return cities().SelectColumns(dal.MinAs(dal.Field("State"), "least")) }},
		{"as_the_operand_of_maximum", func() dal.Query { return cities().SelectColumns(dal.MaxAs(dal.Field("Population"), "most")) }},
		{"as_the_operand_of_count", func() dal.Query { return cities().SelectColumns(dal.CountAs(dal.Field("State"), "n")) }},
		{"as_the_operand_of_count_distinct", func() dal.Query {
			return cities().SelectColumns(dal.CountDistinctAs(dal.Field("State"), "n"))
		}},
		{"inside_arithmetic", func() dal.Query {
			return cities().SelectColumns(dal.MaxAs(dal.Binary(dal.Field("AreaSqKm"), dal.Add, dal.Field("Population")), "most"))
		}},
		{"in_the_field_only_a_rule_reads", func() dal.Query {
			return cities().Where(dal.WhereField("IsCapital", dal.Equal, true)).SelectColumns(count)
		}},
	}
}

// verifyDenied runs a read that must be refused: the access layer denies it as a
// hidden column, or as no access at all when code is empty of columns, and the
// adapter is never asked.
func verifyDenied(ctx context.Context, t *testing.T, db dal.DB, caller copyCaller, q dal.Query, reader string, entry permittedEntry, code access.ReasonCode) {
	t.Helper()
	ctx = caller.identify(ctx)
	reads := &atomic.Int64{}
	secured := caller.secure(db, reads)
	err := entry.run(ctx, secured, permittedCopyDeniedTx, func(ctx context.Context, qe dal.QueryExecutor) error {
		_, readErr := readCopyRows(ctx, qe, reader, q, []string{"n"})
		return readErr
	})
	assert.ErrorIs(t, err, access.ErrAccessDenied)
	var denied *access.DeniedError
	if assert.ErrorAs(t, err, &denied) {
		assert.Equal(t, code, denied.Decision.Code)
	}
	assert.Zero(t, reads.Load(), "no query and no key read reaches the adapter")
}

// accessPermittedCopyTest proves that a secured read of one source equals the
// same read on the copy of the data the caller may read, and that a read naming
// a field the caller may not read is refused before the adapter is asked.
func accessPermittedCopyTest(ctx context.Context, t *testing.T, db dal.DB) {
	readers := []string{permittedCopyRecords, permittedCopyRecordset}
	callers := permittedCopyCallers()
	membership := evaluatesMembership(ctx, t, db)
	var control copyCaller
	for _, caller := range callers {
		if caller.name == "control_no_restriction" {
			control = caller
		}
	}

	t.Run("equals_the_read_on_the_permitted_copy", func(t *testing.T) {
		for _, caller := range callers {
			t.Run(caller.name, func(t *testing.T) {
				if caller.listRule && !membership {
					pinListRuleAnswer(ctx, t, db, caller)
				}
				for _, cell := range permittedCopyCells() {
					if !cell.runsFor(caller) {
						continue
					}
					t.Run(cell.name, func(t *testing.T) {
						for _, reader := range readers {
							if !cell.runsOn(reader) {
								continue
							}
							for _, entry := range permittedEntries() {
								t.Run(reader+"_"+entry.name, func(t *testing.T) {
									cell.verify(ctx, t, db, caller, reader, entry)
								})
							}
						}
					})
				}
			})
		}
	})

	t.Run("a_field_the_caller_may_not_read_is_refused", func(t *testing.T) {
		for _, caller := range callers {
			if !caller.hasFieldList() {
				continue
			}
			t.Run(caller.name, func(t *testing.T) {
				for _, hidden := range hiddenReads() {
					t.Run(hidden.name, func(t *testing.T) {
						for _, reader := range readers {
							for _, entry := range permittedEntries() {
								t.Run(reader+"_"+entry.name, func(t *testing.T) {
									verifyDenied(ctx, t, db, caller, hidden.query(), reader, entry, access.CodeColumnDenied)
								})
							}
						}
					})
				}
			})
		}
	})

	t.Run("no_policy_refuses_every_read", func(t *testing.T) {
		nobody := copyCaller{name: "no_policy", identify: func(ctx context.Context) context.Context { return ctx }}
		for _, cell := range permittedCopyCells() {
			t.Run(cell.name, func(t *testing.T) {
				for _, reader := range readers {
					if !cell.runsOn(reader) {
						continue
					}
					for _, entry := range permittedEntries() {
						t.Run(reader+"_"+entry.name, func(t *testing.T) {
							verifyDenied(ctx, t, db, nobody, cell.query(), reader, entry, access.CodeConfigurationInvalid)
						})
					}
				}
			})
		}
	})

	// The checks of the checks: the cells above can fail. Without the row rule the
	// answer of a caller who reads some of the rows differs from its copy, and
	// without the field list a read of a hidden field is served.
	t.Run("self_check_without_the_row_rule_the_answer_differs", func(t *testing.T) {
		count := permittedCopyCells()[0]
		require.Equal(t, "count_star", count.name)
		reads := &atomic.Int64{}
		got, err := count.read(ctx, control.secure(db, reads), permittedCopyRecords, permittedEntries()[1], permittedCopyReadTx)
		if errors.Is(err, dal.ErrNotSupported) {
			t.Skip("adapter does not run this read:", err)
		}
		require.NoError(t, err)
		checked := 0
		for _, caller := range callers {
			if strings.HasPrefix(caller.name, "row_rule_") {
				checked++
				assert.NotEmpty(t, answerMismatch(got, count.answer(caller.permittedCopy()), count.ordered), caller.name)
			}
		}
		assert.GreaterOrEqual(t, checked, 3, "the callers whose row rule the self-check removes")
		assert.Empty(t, answerMismatch(got, count.answer(control.permittedCopy()), count.ordered), "the control reads every row")
	})
	t.Run("self_check_without_the_field_list_a_hidden_field_is_served", func(t *testing.T) {
		sum := hiddenReads()[11]
		require.Equal(t, "as_the_operand_of_sum", sum.name)
		reads := &atomic.Int64{}
		var served []copyRow
		err := permittedEntries()[1].run(ctx, control.secure(db, reads), permittedCopyReadTx, func(ctx context.Context, qe dal.QueryExecutor) (readErr error) {
			served, readErr = readCopyRows(ctx, qe, permittedCopyRecords, sum.query(), []string{"total"})
			return readErr
		})
		if errors.Is(err, dal.ErrNotSupported) {
			t.Skip("adapter does not run this read:", err)
		}
		require.NoError(t, err)
		require.Len(t, served, 1)
		var total int
		for _, city := range models.Cities {
			total += city.Population
		}
		assert.InDelta(t, float64(total), numberOf(t, served[0]["total"]), 1, "the hidden field is summed")
		assert.EqualValues(t, 1, reads.Load())
	})
}
