package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

var usersFrom = dal.From(dal.NewRootCollectionRef("users", ""))

func usersQuery() dal.IQueryBuilder { return dal.NewQueryBuilder(usersFrom) }

func fieldList(t *testing.T, fields ...string) fieldSets {
	t.Helper()
	set, err := parseFieldPatterns(fields)
	if err != nil {
		t.Fatal(err)
	}
	return fieldSets{set}
}

// customAggregate is an aggregate that is not DALgo's own type.
type customAggregate struct {
	name string
	args []dal.Expression
}

func (a customAggregate) String() string             { return a.name }
func (a customAggregate) FuncName() string           { return a.name }
func (a customAggregate) FuncArgs() []dal.Expression { return a.args }

func TestAggregatesAreCheckedOperandByOperand(t *testing.T) {
	sets := fieldList(t, "name", "age")
	name, age, secret := dal.Field("name"), dal.Field("age"), dal.Field("secret")
	sum := func(operand dal.Expression) dal.Column { return dal.SumAs(operand, "total") }
	having := func(aggregate dal.Expression) dal.IQueryBuilder {
		return usersQuery().GroupBy(name).Having(dal.NewComparison(aggregate, dal.GreaterThen, dal.Constant{Value: 1}))
	}
	selectName := dal.Column{Expression: name}

	allowed := map[string]dal.StructuredQuery{
		"COUNT(*)":                             usersQuery().SelectColumns(dal.Count()),
		"COUNT(allowed)":                       usersQuery().SelectColumns(dal.CountAs(name, "n")),
		"SUM(allowed)":                         usersQuery().SelectColumns(sum(age)),
		"COUNT(DISTINCT allowed)":              usersQuery().SelectColumns(dal.CountDistinctAs(name, "n")),
		"MAX(allowed + allowed)":               usersQuery().SelectColumns(dal.MaxAs(dal.Binary(age, dal.Add, age), "m")),
		"SUM(allowed * constant)":              usersQuery().SelectColumns(sum(dal.Binary(age, dal.Multiply, dal.Constant{Value: 2}))),
		"nested arithmetic":                    usersQuery().SelectColumns(sum(dal.Binary(dal.Binary(age, dal.Add, age), dal.Subtract, dal.Binary(age, dal.Divide, dal.Constant{Value: 2})))),
		"pointer operands":                     usersQuery().SelectColumns(sum(&dal.BinaryExpression{Left: &age, Operator: dal.Add, Right: &dal.Constant{Value: 1}})),
		"several aggregates":                   usersQuery().SelectColumns(dal.Count(), dal.AverageAs(age, "mean"), dal.MinAs(name, "first")),
		"grouped by an allowed field":          usersQuery().GroupBy(name).SelectColumns(selectName, dal.Count()),
		"HAVING over COUNT(*)":                 having(dal.Count().Expression).SelectColumns(selectName),
		"HAVING over SUM(allowed)":             having(sum(age).Expression).SelectColumns(selectName),
		"ORDER BY an allowed aggregate":        usersQuery().GroupBy(name).OrderBy(dal.Descending(sum(age).Expression)).SelectColumns(selectName),
		"null test of an aggregate":            usersQuery().GroupBy(name).Having(dal.NewIsNotNullCondition(sum(age).Expression)).SelectColumns(selectName),
		"aggregate beside a filter":            usersQuery().WhereField("name", dal.Equal, "Ann").SelectColumns(dal.Count()),
		"an aggregate inside an aggregate":     usersQuery().SelectColumns(sum(dal.NewAggregate(dal.MAX, false, age))),
		"aggregate with an alias":              usersQuery().SelectColumns(dal.Column{Alias: "n", Expression: dal.NewAggregate(dal.COUNT, false, name)}),
		"aggregate without an argument":        usersQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate(dal.COUNT, false)}),
		"COUNT(*) from another aggregate type": usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "count", args: []dal.Expression{dal.Star()}}}),
	}
	for name, query := range allowed {
		t.Run("allowed "+name, func(t *testing.T) {
			if err := validateRequestedQueryFields(query, sets); err != nil {
				t.Fatalf("denied: %v", err)
			}
		})
	}

	columnDenied := []struct {
		name   string
		query  dal.StructuredQuery
		slot   DecisionSlot
		column string
	}{
		{"SUM(hidden)", usersQuery().SelectColumns(sum(secret)), DecisionSlotFields, "secret"},
		{"COUNT(hidden)", usersQuery().SelectColumns(dal.CountAs(secret, "n")), DecisionSlotFields, "secret"},
		{"MAX(allowed + hidden)", usersQuery().SelectColumns(dal.MaxAs(dal.Binary(age, dal.Add, secret), "m")), DecisionSlotFields, "secret"},
		{"MAX(hidden + allowed)", usersQuery().SelectColumns(dal.MaxAs(dal.Binary(secret, dal.Add, age), "m")), DecisionSlotFields, "secret"},
		{"a hidden field deep in the arithmetic", usersQuery().SelectColumns(sum(dal.Binary(age, dal.Multiply, dal.Binary(dal.Constant{Value: 1}, dal.Add, secret)))), DecisionSlotFields, "secret"},
		{"a hidden field inside a nested aggregate", usersQuery().SelectColumns(sum(dal.NewAggregate(dal.MAX, false, secret))), DecisionSlotFields, "secret"},
		{"a hidden field behind a pointer", usersQuery().SelectColumns(sum(&secret)), DecisionSlotFields, "secret"},
		{"a hidden field in the second aggregate", usersQuery().SelectColumns(dal.Count(), sum(secret)), DecisionSlotFields, "secret"},
		{"HAVING SUM(hidden)", having(sum(secret).Expression).SelectColumns(selectName), DecisionSlotWhere, "secret"},
		{"ORDER BY SUM(hidden)", usersQuery().GroupBy(name).OrderBy(dal.Ascending(sum(secret).Expression)).SelectColumns(selectName), DecisionSlotFields, "secret"},
		{"null test of an aggregate over a hidden field", usersQuery().GroupBy(name).Having(dal.NewIsNullCondition(sum(secret).Expression)).SelectColumns(selectName), DecisionSlotWhere, "secret"},
	}
	for _, c := range columnDenied {
		t.Run("denied "+c.name, func(t *testing.T) {
			var denied *DeniedError
			err := validateRequestedQueryFields(c.query, sets)
			if !errors.As(err, &denied) {
				t.Fatalf("error = %v, want a denial", err)
			}
			if denied.Decision.Code != CodeColumnDenied || denied.Decision.Slot != c.slot || !reflect.DeepEqual(denied.Decision.Columns, [][]string{{c.column}}) {
				t.Fatalf("decision = code %s slot %s columns %v", denied.Decision.Code, denied.Decision.Slot, denied.Decision.Columns)
			}
		})
	}

	unsupported := map[string]dal.StructuredQuery{
		"a param operand":                          usersQuery().SelectColumns(sum(dal.NewParam("p"))),
		"a param in arithmetic":                    usersQuery().SelectColumns(sum(dal.Binary(age, dal.Add, dal.NewParam("p")))),
		"an array operand":                         usersQuery().SelectColumns(sum(dal.NewArray([]int{1}))),
		"a star under a function other than COUNT": usersQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate(dal.MAX, false, dal.Star())}),
		"a star inside arithmetic":                 usersQuery().SelectColumns(sum(dal.Binary(age, dal.Add, dal.Star()))),
		"a bare star column":                       usersQuery().SelectColumns(dal.Column{Expression: dal.Star()}),
		"an operand the check does not know":       usersQuery().SelectColumns(sum(strangeNode{})),
		"a subquery operand":                       usersQuery().SelectColumns(sum(dal.NewQueryExpression(secretRows(), "x"))),
		"arithmetic outside an aggregate":          usersQuery().SelectColumns(dal.Column{Expression: dal.Binary(age, dal.Add, age)}),
		"a bare constant column":                   usersQuery().SelectColumns(dal.Column{Expression: dal.Constant{Value: 1}}),
		"a nil operand":                            usersQuery().SelectColumns(sum(nil)),
		"a nil pointer operand":                    usersQuery().SelectColumns(sum((*dal.FieldRef)(nil))),
		"a nil pointer in arithmetic":              usersQuery().SelectColumns(sum((*dal.BinaryExpression)(nil))),
		"a star under a custom MAX":                usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "max", args: []dal.Expression{dal.Star()}}}),
		"an operand nested past the bound": usersQuery().SelectColumns(sum(func() dal.Expression {
			expression := dal.Expression(age)
			for i := 0; i <= maxQueryNesting; i++ {
				expression = dal.Binary(expression, dal.Add, dal.Constant{Value: 1})
			}
			return expression
		}())),
		"an operand that contains itself": usersQuery().SelectColumns(sum(func() dal.Expression {
			loop := &dal.BinaryExpression{Operator: dal.Add, Right: dal.Constant{Value: 1}}
			loop.Left = loop
			return loop
		}())),
	}
	for name, query := range unsupported {
		t.Run("unsupported "+name, func(t *testing.T) {
			var denied *DeniedError
			err := validateRequestedQueryFields(query, sets)
			if !errors.As(err, &denied) || denied.Decision.Code != CodeEnforcementUnsupported {
				t.Fatalf("error = %v, want an unsupported-expression denial", err)
			}
		})
	}
}

// An aggregate passes the field check on both read paths and the wrapped
// session sees one read; one over a hidden field never reaches it.
func TestAggregatesUnderAFieldListReachTheWrappedSessionOnce(t *testing.T) {
	ctx := context.Background()
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))
	name, age, secret := dal.Field("name"), dal.Field("age"), dal.Field("secret")
	paths := map[string]func(dal.ReadSession, dal.Query) error{
		"records reader": func(session dal.ReadSession, query dal.Query) error {
			_, err := session.ExecuteQueryToRecordsReader(ctx, query)
			return err
		},
		"recordset reader": func(session dal.ReadSession, query dal.Query) error {
			_, err := session.ExecuteQueryToRecordsetReader(ctx, query)
			return err
		},
	}
	for path, run := range paths {
		t.Run(path, func(t *testing.T) {
			for name, query := range map[string]dal.StructuredQuery{
				"COUNT(*)":       usersQuery().SelectColumns(dal.Count()),
				"COUNT(allowed)": usersQuery().SelectColumns(dal.CountAs(name, "n")),
				"SUM(allowed)":   usersQuery().SelectColumns(dal.SumAs(age, "total")),
				"grouped": usersQuery().GroupBy(name).SelectColumns(
					dal.Column{Expression: name}, dal.Count()),
			} {
				wrapped := &countingSession{}
				if err := run(SecureReadSession(wrapped, policy), query); err != nil || wrapped.reads != 1 {
					t.Errorf("%s: error = %v, %d reads, want none and 1", name, err, wrapped.reads)
				}
			}
			for name, query := range map[string]dal.StructuredQuery{
				"SUM(hidden)":           usersQuery().SelectColumns(dal.SumAs(secret, "total")),
				"MAX(allowed + hidden)": usersQuery().SelectColumns(dal.MaxAs(dal.Binary(age, dal.Add, secret), "m")),
			} {
				wrapped := &countingSession{}
				if err := run(SecureReadSession(wrapped, policy), query); !errors.Is(err, ErrAccessDenied) || wrapped.reads != 0 {
					t.Errorf("%s: error = %v, %d reads, want a denial and 0", name, err, wrapped.reads)
				}
			}
		})
	}
}

func TestQueryOutputNames(t *testing.T) {
	name := dal.Field("name")
	pointer := &name
	query := usersQuery().SelectColumns(
		dal.Column{Alias: "city", Expression: name},
		dal.Column{Expression: name},
		dal.Column{Expression: pointer},
		dal.CountAs(name, "n"),
		dal.Count(),
		dal.Column{},
		dal.AllColumnsExcept("secret"),
	)
	want := []string{"city", "name", "n", "COUNT(*)"}
	if got := outputNames(query); !reflect.DeepEqual(got, want) {
		t.Fatalf("output names = %v, want %v", got, want)
	}
	if got := outputNames(usersQuery().SelectKeysOnly(reflect.String)); len(got) != 0 {
		t.Fatalf("output names of a query without columns = %v", got)
	}
	if got := outputNames(nil); len(got) != 0 {
		t.Fatalf("output names of no query = %v", got)
	}
}

// rowsSession answers every query with the rows it was given. A row may carry
// fields the query did not ask for, as an adapter that ignores a projection
// would return them.
type rowsSession struct {
	countingSession
	rows []map[string]any
}

func (s *rowsSession) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	records := make([]record.Record, len(s.rows))
	for i, row := range s.rows {
		records[i] = record.NewRecordWithData(record.NewKeyWithID("users", i), row)
		records[i].SetError(nil)
	}
	return dal.NewRecordsReader(records), nil
}

func readRows(t *testing.T, session dal.ReadSession, query dal.Query) []map[string]any {
	t.Helper()
	reader, err := session.ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			return rows
		}
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, rec.Data().(map[string]any))
	}
}

// Every column the caller selected comes back under the name the caller gave
// it; any other field the adapter returns is still redacted.
func TestSelectedOutputNamesSurviveRedaction(t *testing.T) {
	name := dal.Field("name")
	lists := map[string][]string{
		"enumerable list": {"name", "age"},
		"wildcard list":   {"name", "a*"},
	}
	queries := map[string]struct {
		query dal.StructuredQuery
		row   map[string]any
		want  map[string]any
	}{
		"allowed field under an alias": {
			usersQuery().SelectColumns(dal.Column{Alias: "city", Expression: name}),
			map[string]any{"city": "Cork", "secret": "s"},
			map[string]any{"city": "Cork"},
		},
		"allowed field and an aliased one": {
			usersQuery().SelectColumns(dal.Column{Expression: name}, dal.Column{Alias: "years", Expression: dal.Field("age")}),
			map[string]any{"name": "Ann", "years": 3, "secret": "s"},
			map[string]any{"name": "Ann", "years": 3},
		},
		"aggregate with an alias": {
			usersQuery().SelectColumns(dal.CountAs(name, "n")),
			map[string]any{"n": 2, "secret": "s"},
			map[string]any{"n": 2},
		},
		"aggregate without an alias": {
			usersQuery().SelectColumns(dal.Count()),
			map[string]any{"COUNT(*)": 2, "secret": "s"},
			map[string]any{"COUNT(*)": 2},
		},
		"grouped aggregate": {
			usersQuery().GroupBy(name).SelectColumns(dal.Column{Expression: name}, dal.SumAs(dal.Field("age"), "total")),
			map[string]any{"name": "Ann", "total": 7, "secret": "s"},
			map[string]any{"name": "Ann", "total": 7},
		},
		"an alias that holds an object": {
			usersQuery().SelectColumns(dal.Column{Alias: "who", Expression: name}),
			map[string]any{"who": map[string]any{"first": "Ann"}, "secret": map[string]any{"x": 1}},
			map[string]any{"who": map[string]any{"first": "Ann"}},
		},
		"no columns selected": {
			usersQuery().SelectKeysOnly(reflect.String),
			map[string]any{"name": "Ann", "secret": "s", "city": "Cork"},
			map[string]any{"name": "Ann"},
		},
	}
	for list, fields := range lists {
		policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields(fields...)))
		for label, c := range queries {
			t.Run(list+" "+label, func(t *testing.T) {
				session := SecureReadSession(&rowsSession{rows: []map[string]any{c.row}}, policy)
				rows := readRows(t, session, c.query)
				if len(rows) != 1 || !reflect.DeepEqual(rows[0], c.want) {
					t.Fatalf("rows = %v, want [%v]", rows, c.want)
				}
			})
		}
	}
}
