package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
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
		"every function DALgo defines": usersQuery().SelectColumns(
			dal.Column{Expression: dal.NewAggregate(dal.COUNT, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.SUM, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.AVERAGE, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.MIN, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.MAX, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.FIRST, false, age)},
			dal.Column{Expression: dal.NewAggregate(dal.LAST, false, age)}),
		"a function name in lower case": usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "sum", args: []dal.Expression{age}}}),
		"every operator DALgo defines":  usersQuery().SelectColumns(sum(dal.Binary(dal.Binary(age, dal.Add, age), dal.Subtract, dal.Binary(dal.Binary(age, dal.Multiply, age), dal.Divide, age)))),
		"an aggregate in HAVING beside one in ORDER BY": usersQuery().GroupBy(name).
			Having(dal.NewComparison(sum(age).Expression, dal.GreaterThen, dal.Constant{Value: 1})).
			OrderBy(dal.Ascending(sum(age).Expression)).SelectColumns(selectName),
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
		"a param operand":                            usersQuery().SelectColumns(sum(dal.NewParam("p"))),
		"a param in arithmetic":                      usersQuery().SelectColumns(sum(dal.Binary(age, dal.Add, dal.NewParam("p")))),
		"an array operand":                           usersQuery().SelectColumns(sum(dal.NewArray([]int{1}))),
		"a star under a function other than COUNT":   usersQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate(dal.MAX, false, dal.Star())}),
		"a star inside arithmetic":                   usersQuery().SelectColumns(sum(dal.Binary(age, dal.Add, dal.Star()))),
		"a bare star column":                         usersQuery().SelectColumns(dal.Column{Expression: dal.Star()}),
		"an operand the check does not know":         usersQuery().SelectColumns(sum(strangeNode{})),
		"a subquery operand":                         usersQuery().SelectColumns(sum(dal.NewQueryExpression(secretRows(), "x"))),
		"arithmetic outside an aggregate":            usersQuery().SelectColumns(dal.Column{Expression: dal.Binary(age, dal.Add, age)}),
		"a bare constant column":                     usersQuery().SelectColumns(dal.Column{Expression: dal.Constant{Value: 1}}),
		"a nil operand":                              usersQuery().SelectColumns(sum(nil)),
		"a function DALgo does not define":           usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "EVIL", args: []dal.Expression{age}}}),
		"an empty function name":                     usersQuery().SelectColumns(dal.Column{Expression: customAggregate{args: []dal.Expression{age}}}),
		"a function that is not an aggregate":        usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "LOWER", args: []dal.Expression{age}}}),
		"an unknown function nested in an aggregate": usersQuery().SelectColumns(sum(customAggregate{name: "EVIL", args: []dal.Expression{age}})),
		"an operator DALgo does not define":          usersQuery().SelectColumns(sum(dal.Binary(age, "%", age))),
		"an empty operator":                          usersQuery().SelectColumns(sum(dal.Binary(age, "", age))),
		"an operator that holds SQL":                 usersQuery().SelectColumns(sum(dal.Binary(age, "+ 1) FROM secrets --", age))),
		"an unknown operator deep in the operand":    usersQuery().SelectColumns(sum(dal.Binary(age, dal.Add, dal.Binary(age, "^", age)))),
		"an aggregate in WHERE":                      usersQuery().Where(dal.NewComparison(sum(age).Expression, dal.GreaterThen, dal.Constant{Value: 1})).SelectColumns(selectName),
		"an aggregate on the right of a WHERE":       usersQuery().Where(dal.NewComparison(dal.Constant{Value: 1}, dal.LessThen, sum(age).Expression)).SelectColumns(selectName),
		"an aggregate in a grouped WHERE":            usersQuery().Where(dal.NewGroupCondition(dal.And, dal.WhereField("name", dal.Equal, "Ann"), dal.NewComparison(sum(age).Expression, dal.GreaterThen, dal.Constant{Value: 1}))).SelectColumns(selectName),
		"a null test of an aggregate in WHERE":       usersQuery().Where(dal.NewIsNotNullCondition(sum(age).Expression)).SelectColumns(selectName),
		"an aggregate over a hidden field in WHERE":  usersQuery().Where(dal.NewComparison(sum(secret).Expression, dal.GreaterThen, dal.Constant{Value: 1})).SelectColumns(selectName),
		"an aggregate in GROUP BY":                   usersQuery().GroupBy(sum(age).Expression).SelectColumns(selectName),
		"a nil pointer operand":                      usersQuery().SelectColumns(sum((*dal.FieldRef)(nil))),
		"a nil pointer in arithmetic":                usersQuery().SelectColumns(sum((*dal.BinaryExpression)(nil))),
		"a star under a custom MAX":                  usersQuery().SelectColumns(dal.Column{Expression: customAggregate{name: "max", args: []dal.Expression{dal.Star()}}}),
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

// rowsSession answers every query with the rows it was given. A row may carry
// fields the query did not ask for, as an adapter that ignores a projection
// would return them.
type rowsSession struct {
	countingSession
	rows []map[string]any
}

func (s *rowsSession) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return rowsReader(s.rows), nil
}

func rowsReader(rows []map[string]any) dal.RecordsReader {
	records := make([]record.Record, len(rows))
	for i, row := range rows {
		records[i] = record.NewRecordWithData(record.NewKeyWithID("users", i), row)
		records[i].SetError(nil)
	}
	return dal.NewRecordsReader(records)
}

// projectingSession answers a query as an adapter that honours projections
// does: one row with one key per selected column, named by the alias the query
// carries, otherwise by the field or the text of the expression. A plain field
// holds its stored value and an aggregate holds 2. A wildcard column adds every
// stored field. seen records each query that reached it.
type projectingSession struct {
	countingSession
	stored map[string]any
	seen   []dal.StructuredQuery
}

func (s *projectingSession) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	structured := query.(dal.StructuredQuery)
	s.seen = append(s.seen, structured)
	row := map[string]any{}
	for _, column := range structured.Columns() {
		if column.Wildcard != nil {
			for key, value := range s.stored {
				if !column.Wildcard.Excludes(key) {
					row[key] = value
				}
			}
			continue
		}
		field, plain := column.Expression.(dal.FieldRef)
		if pointer, isPointer := column.Expression.(*dal.FieldRef); isPointer {
			field, plain = *pointer, true
		}
		key := column.Alias
		switch {
		case plain && key == "":
			key = field.Name()
		case key == "":
			key = column.Expression.String()
		}
		if plain {
			row[key] = s.stored[field.Name()]
		} else {
			row[key] = 2
		}
	}
	return rowsReader([]map[string]any{row}), nil
}

// aliasesSent lists the alias of every selected column of the last query that
// reached the session, in column order.
func (s *projectingSession) aliasesSent(t *testing.T) []string {
	t.Helper()
	if len(s.seen) == 0 {
		t.Fatal("no query reached the wrapped session")
	}
	var aliases []string
	for _, column := range s.seen[len(s.seen)-1].Columns() {
		aliases = append(aliases, column.Alias)
	}
	return aliases
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

var storedUser = map[string]any{"name": "Ann", "age": 3, "secret": "s"}

// Every list refuses the names "secret" and "city".
var outputNameLists = map[string][]string{
	"enumerable list": {"name", "age"},
	"wildcard list":   {"name", "a*"},
}

// Every column the caller selected comes back under the name the caller gave
// it, with the value its own expression produced.
func TestSelectedOutputNamesComeBackUnderTheCallersNames(t *testing.T) {
	name, age := dal.Field("name"), dal.Field("age")
	cases := map[string]struct {
		query dal.StructuredQuery
		want  map[string]any
	}{
		"allowed field under an alias": {
			usersQuery().SelectColumns(dal.Column{Alias: "city", Expression: name}),
			map[string]any{"city": "Ann"},
		},
		"allowed field under a refused name": {
			usersQuery().SelectColumns(dal.Column{Alias: "secret", Expression: name}),
			map[string]any{"secret": "Ann"},
		},
		"allowed field and an aliased one": {
			usersQuery().SelectColumns(dal.Column{Expression: name}, dal.Column{Alias: "years", Expression: age}),
			map[string]any{"name": "Ann", "years": 3},
		},
		"allowed field under an allowed name": {
			usersQuery().SelectColumns(dal.Column{Alias: "age", Expression: name}),
			map[string]any{"age": "Ann"},
		},
		"aggregate with an alias": {
			usersQuery().SelectColumns(dal.CountAs(name, "n")),
			map[string]any{"n": 2},
		},
		"aggregate under a refused name": {
			usersQuery().SelectColumns(dal.CountAs(name, "secret")),
			map[string]any{"secret": 2},
		},
		"aggregate without an alias": {
			usersQuery().SelectColumns(dal.Count()),
			map[string]any{"COUNT(*)": 2},
		},
		"grouped aggregate": {
			usersQuery().GroupBy(name).SelectColumns(dal.Column{Expression: name}, dal.SumAs(age, "total")),
			map[string]any{"name": "Ann", "total": 2},
		},
		"pointer to a field under an alias": {
			usersQuery().SelectColumns(dal.Column{Alias: "city", Expression: &name}),
			map[string]any{"city": "Ann"},
		},
		"pointer to a field without an alias": {
			usersQuery().SelectColumns(dal.Column{Expression: &name}),
			map[string]any{"name": "Ann"},
		},
	}
	for list, fields := range outputNameLists {
		policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields(fields...)))
		for label, c := range cases {
			t.Run(list+" "+label, func(t *testing.T) {
				wrapped := &projectingSession{stored: storedUser}
				rows := readRows(t, SecureReadSession(wrapped, policy), c.query)
				if len(rows) != 1 || !reflect.DeepEqual(rows[0], c.want) {
					t.Fatalf("rows = %v, want [%v]", rows, c.want)
				}
			})
		}
	}
}

// A name the field list refuses is never sent to the wrapped session as the
// name of a column. The column goes under an alias of the access layer's own,
// different for every query, and a name the list allows is sent as it is.
func TestRefusedOutputNamesAreSentUnderAliasesOfTheirOwn(t *testing.T) {
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))
	name, age := dal.Field("name"), dal.Field("age")
	query := usersQuery().SelectColumns(
		dal.Column{Expression: name},
		dal.Column{Alias: "city", Expression: name},
		dal.Column{Alias: "age", Expression: name},
		dal.Column{Alias: "secret", Expression: age},
		dal.CountAs(name, "n"),
		dal.Count(),
	)
	var runs [2][]string
	for i := range runs {
		wrapped := &projectingSession{stored: storedUser}
		readRows(t, SecureReadSession(wrapped, policy), query)
		runs[i] = wrapped.aliasesSent(t)
	}
	for i, aliases := range runs {
		if len(aliases) != 6 || aliases[0] != "" || aliases[2] != "age" {
			t.Fatalf("run %d aliases = %q, want no alias on the plain field and the allowed name unchanged", i, aliases)
		}
		for _, j := range []int{1, 3, 4, 5} {
			if !strings.HasPrefix(aliases[j], "access_") || aliases[j] != strings.ToLower(aliases[j]) {
				t.Fatalf("run %d column %d alias = %q, want a lower-case access_ alias", i, j, aliases[j])
			}
			for _, caller := range []string{"city", "secret", "n", "COUNT(*)"} {
				if aliases[j] == caller {
					t.Fatalf("run %d column %d alias = %q, a name the caller chose", i, j, aliases[j])
				}
			}
		}
	}
	for _, j := range []int{1, 3, 4, 5} {
		if runs[0][j] == runs[1][j] {
			t.Fatalf("column %d got the alias %q in two queries", j, runs[0][j])
		}
	}
}

// A session that returns more than the query asked for, or ignores its
// projection, cannot bring a field back by the name of a column: a stored field
// the list refuses stays out whatever name the caller gave a column.
func TestNoCallerChosenNameSurvivesRedaction(t *testing.T) {
	name := dal.Field("name")
	cases := map[string]struct {
		query dal.StructuredQuery
		row   map[string]any
		want  map[string]any
	}{
		"allowed field under an alias": {
			usersQuery().SelectColumns(dal.Column{Alias: "city", Expression: name}),
			map[string]any{"city": "Cork", "name": "Ann", "secret": "s"},
			map[string]any{"name": "Ann"},
		},
		"allowed field under a refused name": {
			usersQuery().SelectColumns(dal.Column{Alias: "secret", Expression: name}),
			map[string]any{"name": "Ann", "secret": "s"},
			map[string]any{"name": "Ann"},
		},
		"aggregate under a refused name": {
			usersQuery().SelectColumns(dal.CountAs(name, "secret")),
			map[string]any{"name": "Ann", "secret": 99},
			map[string]any{"name": "Ann"},
		},
		"aggregate without an alias": {
			usersQuery().SelectColumns(dal.Count()),
			map[string]any{"COUNT(*)": 2, "name": "Ann", "secret": "s"},
			map[string]any{"name": "Ann"},
		},
		"aggregate named by its own text": {
			usersQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate(dal.COUNT, false, name)}),
			map[string]any{"COUNT(name)": 2, "name": "Ann"},
			map[string]any{"name": "Ann"},
		},
		"field under a name the list allows": {
			usersQuery().SelectColumns(dal.Column{Alias: "age", Expression: name}),
			map[string]any{"age": 3, "secret": "s"},
			map[string]any{"age": 3},
		},
		"no columns selected": {
			usersQuery().SelectKeysOnly(reflect.String),
			map[string]any{"name": "Ann", "secret": "s", "city": "Cork"},
			map[string]any{"name": "Ann"},
		},
	}
	for list, fields := range outputNameLists {
		policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields(fields...)))
		for label, c := range cases {
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

// An alias that names a stored object with a leaf the list refuses brings back
// only the leaves the list allows, never the refused one.
func TestRefusedOutputNameHoldingAnObjectIsRedactedByLeaf(t *testing.T) {
	row := map[string]any{"name": "Ann", "profile": map[string]any{"public": "p", "secret": "x"}}
	want := map[string]any{"name": "Ann", "profile": map[string]any{"public": "p"}}
	for list, fields := range map[string][]string{
		"enumerable list": {"name", "profile.public"},
		"wildcard list":   {"name*", "profile.public"},
	} {
		policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields(fields...)))
		t.Run(list, func(t *testing.T) {
			query := usersQuery().SelectColumns(dal.Column{Alias: "profile", Expression: dal.Field("name")})
			rows := readRows(t, SecureReadSession(&rowsSession{rows: []map[string]any{row}}, policy), query)
			if len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
				t.Fatalf("rows = %v, want [%v]", rows, want)
			}
		})
	}
}

// Two secured sessions, one over the other, each give the column an alias of its
// own and give back the name it was sent under, so the caller still gets its name.
func TestOutputNamesSurviveNestedSecuredSessions(t *testing.T) {
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))
	wrapped := &projectingSession{stored: storedUser}
	session := SecureReadSession(SecureReadSession(wrapped, policy), policy)
	rows := readRows(t, session, usersQuery().SelectColumns(dal.Column{Alias: "city", Expression: dal.Field("name")}))
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"city": "Ann"}) {
		t.Fatalf("rows = %v, want [map[city:Ann]]", rows)
	}
	if aliases := wrapped.aliasesSent(t); len(aliases) != 1 || !strings.HasPrefix(aliases[0], "access_") {
		t.Fatalf("aliases sent = %q, want one access_ alias", aliases)
	}
}

// Columns that name no output are left as they are: a wildcard, a column with
// no expression, and a wildcard that also carries an expression.
func TestAliasRefusedOutputsLeavesUnnamedColumnsAlone(t *testing.T) {
	sets := fieldList(t, "name")
	query := usersQuery().SelectColumns(
		dal.AllColumnsExcept("secret"),
		dal.Column{},
		dal.Column{Expression: dal.Field("name")},
		dal.Column{Expression: dal.Field("name"), Wildcard: &dal.WildcardProjection{Exclude: []string{"secret"}}},
	)
	rewritten, renames := aliasRefusedOutputs(query, sets)
	if len(renames) != 0 || !reflect.DeepEqual(rewritten.Columns(), query.Columns()) {
		t.Fatalf("renames = %v, columns = %v, want the query unchanged", renames, rewritten.Columns())
	}
}

// A column is sent under a generated alias only when this session's list allows
// its expression: the list is what holds an expression to a name the list
// refuses. Every other column keeps the name it has, so redaction judges it by
// that name.
func TestAliasRefusedOutputsAliasesOnlyColumnsTheListAllows(t *testing.T) {
	sets := fieldList(t, "name")
	name, age := dal.Field("name"), dal.Field("age")
	var nilField *dal.FieldRef
	columns := []dal.Column{
		{Expression: age},
		{Alias: "city", Expression: age},
		{Expression: &age},
		{Alias: "city", Expression: &age},
		dal.SumAs(age, "total"),
		dal.CountAs(dal.Binary(name, dal.Add, age), "n"),
		{Alias: "unknown", Expression: customAggregate{name: "MEDIAN", args: []dal.Expression{name}}},
		{Alias: "constant", Expression: dal.Constant{Value: 1}},
		{Alias: "nothing", Expression: nilField},
		{Alias: "city", Expression: name},
		{Alias: "town", Expression: &name},
		dal.CountAs(name, "n"),
		{Expression: name},
	}
	rewritten, renames := aliasRefusedOutputs(usersQuery().SelectColumns(columns...), sets)
	generated := map[int]bool{9: true, 10: true, 11: true}
	if len(renames) != len(generated) {
		t.Fatalf("renames = %v, want %d", renames, len(generated))
	}
	for i, column := range rewritten.Columns() {
		if generated[i] {
			if !strings.HasPrefix(column.Alias, "access_") {
				t.Errorf("column %d alias = %q, want a generated alias", i, column.Alias)
			}
			continue
		}
		if !reflect.DeepEqual(column, columns[i]) {
			t.Errorf("column %d = %#v, want it unchanged: %#v", i, column, columns[i])
		}
	}
}

// A selected column holds what the list allows only if the list allows its
// expression, whether the field is given by value or by pointer.
func TestReadsOnlyAllowedFieldsCoversThePointerForm(t *testing.T) {
	sets := fieldList(t, "name")
	name, age := dal.Field("name"), dal.Field("age")
	var nilField *dal.FieldRef
	for label, c := range map[string]struct {
		expression dal.Expression
		want       bool
	}{
		"allowed field":                   {name, true},
		"pointer to an allowed field":     {&name, true},
		"hidden field":                    {age, false},
		"pointer to a hidden field":       {&age, false},
		"nil pointer":                     {nilField, false},
		"no expression":                   {nil, false},
		"constant":                        {dal.Constant{Value: 1}, false},
		"aggregate over an allowed field": {dal.CountAs(name, "n").Expression, true},
		"aggregate over a hidden field":   {dal.SumAs(age, "n").Expression, false},
	} {
		if got := sets.readsOnlyAllowedFields(c.expression); got != c.want {
			t.Errorf("%s: readsOnlyAllowedFields = %v, want %v", label, got, c.want)
		}
	}
}

// Two secured sessions, one over the other, with lists that differ: what comes
// back is bounded by the fields both lists allow. A column the outer session's
// projection adds to the query is one the inner session never held to its list,
// so the inner session sends it under its own name and redacts it by that name,
// and no generated alias is sent for it.
func TestNestedSecuredSessionsReturnTheFieldsBothListsAllow(t *testing.T) {
	nameOnly, nameAndAge := map[string]any{"name": "Ann"}, map[string]any{"name": "Ann", "age": 3}
	cases := map[string]struct {
		outer, inner []string
		want         map[string]any
	}{
		"inner list narrower":      {[]string{"name", "age"}, []string{"name"}, nameOnly},
		"outer list narrower":      {[]string{"name"}, []string{"name", "age"}, nameOnly},
		"the same fields":          {[]string{"name", "age"}, []string{"name", "age"}, nameAndAge},
		"outer list with wildcard": {[]string{"name", "a*"}, []string{"name"}, nameOnly},
		"inner list with wildcard": {[]string{"name"}, []string{"name", "a*"}, nameOnly},
	}
	queries := map[string]dal.StructuredQuery{
		"no columns": usersQuery().SelectKeysOnly(reflect.String),
		"wildcard":   usersQuery().SelectColumns(dal.AllColumnsExcept("secret")),
	}
	policy := func(name string, fields []string) Policy {
		return MustPolicy(name, Collection("users", Allow(Query, "list").Fields(fields...)))
	}
	for label, c := range cases {
		for shape, query := range queries {
			t.Run(label+" "+shape, func(t *testing.T) {
				wrapped := &projectingSession{stored: storedUser}
				session := SecureReadSession(SecureReadSession(wrapped, policy("inner", c.inner)), policy("outer", c.outer))
				rows := readRows(t, session, query)
				if len(rows) != 1 || !reflect.DeepEqual(rows[0], c.want) {
					t.Fatalf("rows = %v, want [%v]", rows, c.want)
				}
				for _, alias := range wrapped.aliasesSent(t) {
					if alias != "" {
						t.Fatalf("aliases sent = %q, want none", wrapped.aliasesSent(t))
					}
				}
			})
		}
	}
}

// A grouped query with no columns selects its group keys, which is what DALgo
// defines for it. It is sent with those keys as its columns, whatever fields the
// list allows besides, so the engine's own grouping rules accept it.
func TestGroupedQueryWithNoColumnsProjectsItsGroupKeys(t *testing.T) {
	name, secret := dal.Field("name"), dal.Field("secret")
	moreThanOne := dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.Constant{Value: 1})
	grouped := func(key dal.Expression) dal.StructuredQuery {
		return usersQuery().GroupBy(key).Having(moreThanOne).SelectKeysOnly(reflect.String)
	}
	lists := map[string][]string{
		"enumerable list": {"name", "age"},
		"wildcard list":   {"name", "a*"},
	}
	for label, fields := range lists {
		sets := fieldList(t, fields...)
		for shape, key := range map[string]dal.Expression{"field": name, "pointer to a field": &name} {
			t.Run(label+" "+shape, func(t *testing.T) {
				projection := projectQuery(grouped(key), sets)
				columns := projection.query.Columns()
				if projection.status != queryProjectionApplied || len(columns) != 1 || columns[0].Expression != key {
					t.Fatalf("status = %v, columns = %v, want applied with the group key", projection.status, columns)
				}
				// DALgo's grouping rules accept a group key given by value only.
				if _, byValue := key.(dal.FieldRef); byValue {
					if err := dal.ValidateAggregation(projection.query); err != nil {
						t.Fatalf("the projected query is not a valid aggregation: %v", err)
					}
				}
			})
		}
		t.Run(label+" hidden group key", func(t *testing.T) {
			if projection := projectQuery(grouped(secret), sets); projection.status != queryProjectionUnavailable {
				t.Fatalf("status = %v, want unavailable for a group key the list refuses", projection.status)
			}
		})
	}
}

// recordsetRecorder remembers the query of each recordset read that reaches it.
type recordsetRecorder struct {
	countingSession
	seen []dal.Query
}

func (s *recordsetRecorder) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	s.seen = append(s.seen, query)
	return s.countingSession.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

// On both read paths a grouped query with no columns reaches the wrapped session
// as a valid aggregation over its group keys, with a HAVING over an aggregate.
func TestGroupedQueryWithNoColumnsReachesTheWrappedSessionAsItsGroupKeys(t *testing.T) {
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))
	name := dal.Field("name")
	query := usersQuery().GroupBy(name).
		Having(dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.Constant{Value: 1})).
		SelectKeysOnly(reflect.String)
	assertGroupKeys := func(t *testing.T, reached dal.Query) {
		t.Helper()
		structured, ok := reached.(dal.StructuredQuery)
		if !ok {
			t.Fatalf("query %T is not structured", reached)
		}
		columns := structured.Columns()
		if len(columns) != 1 || columns[0].Expression != dal.Expression(name) {
			t.Fatalf("columns = %v, want the group key", columns)
		}
		if err := dal.ValidateAggregation(structured); err != nil {
			t.Fatalf("the query that reached the session is not a valid aggregation: %v", err)
		}
	}
	t.Run("records reader", func(t *testing.T) {
		wrapped := &projectingSession{stored: storedUser}
		rows := readRows(t, SecureReadSession(wrapped, policy), query)
		if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"name": "Ann"}) {
			t.Fatalf("rows = %v, want [map[name:Ann]]", rows)
		}
		assertGroupKeys(t, wrapped.seen[0])
	})
	t.Run("recordset reader", func(t *testing.T) {
		wrapped := &recordsetRecorder{}
		if _, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsetReader(context.Background(), query); err != nil {
			t.Fatal(err)
		}
		if len(wrapped.seen) != 1 {
			t.Fatalf("%d queries reached the session, want 1", len(wrapped.seen))
		}
		assertGroupKeys(t, wrapped.seen[0])
	})
}
