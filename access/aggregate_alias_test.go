package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// groupedTotals selects each name with the sum of its ages under the alias
// "total", and filters and orders the groups by that alias.
func groupedTotals() dal.IQueryBuilder {
	return usersQuery().GroupBy(dal.Field("name"))
}

func totalColumns() []dal.Column {
	return []dal.Column{{Expression: dal.Field("name")}, dal.SumAs(dal.Field("age"), "total")}
}

func TestAliasOfAnAllowedAggregateIsAllowedInHavingAndOrderBy(t *testing.T) {
	sets := fieldList(t, "name", "age")
	total := dal.Field("total")
	over := dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})
	pointerTotal := &total
	cases := []struct {
		name    string
		query   dal.StructuredQuery
		allowed bool
	}{
		{"HAVING over the alias", groupedTotals().Having(over).SelectColumns(totalColumns()...), true},
		{"HAVING over the alias through a pointer", groupedTotals().Having(dal.NewComparison(pointerTotal, dal.GreaterThen, dal.Constant{Value: 1})).SelectColumns(totalColumns()...), true},
		{"HAVING over the alias in a condition group", groupedTotals().Having(dal.NewGroupCondition(dal.And, over, over)).SelectColumns(totalColumns()...), true},
		{"HAVING null test of the alias", groupedTotals().Having(dal.NewIsNotNullCondition(total)).SelectColumns(totalColumns()...), true},
		{"ORDER BY the alias", groupedTotals().OrderBy(dal.Descending(total)).SelectColumns(totalColumns()...), true},
		{"ORDER BY the alias through a pointer", groupedTotals().OrderBy(dal.Descending(pointerTotal)).SelectColumns(totalColumns()...), true},
		{"the alias of a plain field", groupedTotals().Having(dal.NewComparison(dal.Field("n"), dal.GreaterThen, dal.Constant{Value: 1})).
			SelectColumns(dal.Column{Alias: "n", Expression: dal.Field("name")}), false},
		{"the alias of an aggregate over a hidden field", groupedTotals().Having(over).
			SelectColumns(dal.Column{Expression: dal.Field("name")}, dal.SumAs(dal.Field("secret"), "total")), false},
		{"the alias of an aggregate no list can check", groupedTotals().Having(over).
			SelectColumns(dal.Column{Expression: dal.Field("name")}, dal.Column{Alias: "total", Expression: customAggregate{name: "MEDIAN", args: []dal.Expression{dal.Field("age")}}}), false},
		{"a name that is no alias", groupedTotals().Having(dal.NewComparison(dal.Field("sum"), dal.GreaterThen, dal.Constant{Value: 1})).SelectColumns(totalColumns()...), false},
		{"a name two columns come back under", groupedTotals().Having(over).
			SelectColumns(dal.SumAs(dal.Field("age"), "total"), dal.CountAs(dal.Field("name"), "total")), false},
		{"a name an aggregate and a field come back under", groupedTotals().Having(over).
			SelectColumns(dal.SumAs(dal.Field("age"), "total"), dal.Column{Expression: dal.Field("total")}), false},
		{"the alias qualified by a source", groupedTotals().Having(dal.NewComparison(dal.NewFieldRef("users", "total"), dal.GreaterThen, dal.Constant{Value: 1})).SelectColumns(totalColumns()...), false},
		{"the alias in WHERE", groupedTotals().Where(over).SelectColumns(totalColumns()...), false},
		{"the alias in GROUP BY", usersQuery().GroupBy(total).SelectColumns(totalColumns()...), false},
		{"the alias selected as a field", usersQuery().SelectColumns(dal.SumAs(dal.Field("age"), "total"), dal.Column{Expression: dal.Field("total")}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateRequestedQueryFieldsWithAliases(c.query, sets)
			if c.allowed && err != nil {
				t.Fatalf("error = %v, want none", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("want a denial")
			}
			// Held to the list for a reader that cannot rename what it sends, no
			// alias is a name HAVING or ORDER BY may use.
			if err := validateRequestedQueryFields(c.query, sets); err == nil {
				t.Fatal("the same query is allowed when no alias may be named")
			}
		})
	}
}

func TestAllowedAggregateAliasesNamesOnlyAllowedAggregates(t *testing.T) {
	sets := fieldList(t, "name", "age")
	columns := []dal.Column{
		dal.SumAs(dal.Field("age"), "allowed"),
		dal.SumAs(dal.Field("secret"), "hidden"),
		{Alias: "plain", Expression: dal.Field("name")},
		{Expression: dal.Field("age")},
		{},
		dal.CountAs(dal.Field("name"), "twice"),
		dal.MinAs(dal.Field("age"), "twice"),
	}
	got := allowedAggregateAliases(columns, sets)
	if want := map[string]bool{"allowed": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
}

// A column the field list allows but whose alias it does not is sent under an
// alias of the access layer's own. A HAVING or ORDER BY that names the caller's
// alias names the generated one in what is sent, so the query keeps its meaning
// and the caller's name never reaches the wrapped session.
func TestReferencesToARenamedAliasFollowTheRename(t *testing.T) {
	ctx := context.Background()
	total := dal.Field("total")
	query := groupedTotals().
		Having(dal.NewGroupCondition(dal.And,
			dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1}),
			dal.NewIsNotNullCondition(&total),
			&dal.Comparison{Left: dal.Field("name"), Operator: dal.LessThen, Right: dal.Constant{Value: "x"}},
			dal.NewComparison(dal.Count().Expression, dal.GreaterThen, total))).
		OrderBy(dal.Descending(total), dal.AscendingField("name"), dal.Descending(dal.Count().Expression)).
		SelectColumns(totalColumns()...)
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))

	t.Run("records reader", func(t *testing.T) {
		wrapped := &projectingSession{stored: storedUser}
		reader, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsReader(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		row, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := row.Data(), map[string]any{"name": "Ann", "total": 2}; !reflect.DeepEqual(got, want) {
			t.Fatalf("row = %v, want %v", got, want)
		}
		sent := wrapped.seen[0]
		generated := wrapped.aliasesSent(t)[1]
		if generated == "" || generated == "total" {
			t.Fatalf("the aggregate was sent under %q, want an alias of the access layer's own", generated)
		}
		ref := dal.Field(generated)
		wantHaving := dal.NewGroupCondition(dal.And,
			dal.NewComparison(ref, dal.GreaterThen, dal.Constant{Value: 1}),
			dal.NewIsNotNullCondition(ref),
			dal.Comparison{Left: dal.Field("name"), Operator: dal.LessThen, Right: dal.Constant{Value: "x"}},
			dal.NewComparison(dal.Count().Expression, dal.GreaterThen, ref))
		if !reflect.DeepEqual(sent.Having(), wantHaving) {
			t.Fatalf("HAVING sent = %v, want %v", sent.Having(), wantHaving)
		}
		var sentOrder []string
		for _, order := range sent.OrderBy() {
			sentOrder = append(sentOrder, order.String())
		}
		if want := []string{dal.Descending(ref).String(), dal.AscendingField("name").String(), dal.Descending(dal.Count().Expression).String()}; !reflect.DeepEqual(sentOrder, want) {
			t.Fatalf("ORDER BY sent = %v, want %v", sentOrder, want)
		}
		if err := dal.ValidateAggregation(sent); err != nil {
			t.Fatalf("the query that reached the session is not a valid aggregation: %v", err)
		}
		// The caller's own query is unchanged.
		if !reflect.DeepEqual(query.Having().(dal.GroupCondition).Conditions()[0], dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})) {
			t.Fatal("the caller's HAVING was rewritten")
		}
	})
	// The recordset reader sends the aggregate under the caller's own alias and
	// renames nothing, so an alias the list does not allow is not a name HAVING or
	// ORDER BY may use there; naming the aggregate itself is.
	t.Run("recordset reader refuses the alias, with no read", func(t *testing.T) {
		plain := groupedTotals().
			Having(dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})).
			SelectColumns(totalColumns()...)
		wrapped := &countingSession{}
		_, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, plain)
		var denied *DeniedError
		if !errors.As(err, &denied) || denied.Decision.Code != CodeColumnDenied || !reflect.DeepEqual(denied.Decision.Columns, [][]string{{"total"}}) || wrapped.reads != 0 {
			t.Fatalf("error = %v, %d reads, want a column denial of total and 0", err, wrapped.reads)
		}
	})
	t.Run("recordset reader allows the aggregate itself", func(t *testing.T) {
		byAggregate := groupedTotals().
			Having(dal.NewComparison(dal.SumAs(dal.Field("age"), "").Expression, dal.GreaterThen, dal.Constant{Value: 1})).
			SelectColumns(totalColumns()...)
		wrapped := &recordsetRecorder{}
		if _, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, byAggregate); err != nil {
			t.Fatal(err)
		}
		if err := dal.ValidateAggregation(wrapped.seen[0].(dal.StructuredQuery)); err != nil {
			t.Fatalf("the query that reached the session is not a valid aggregation: %v", err)
		}
	})
	t.Run("recordset reader allows an alias the list allows", func(t *testing.T) {
		listed := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age", "total")))
		plain := groupedTotals().
			Having(dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})).
			OrderBy(dal.Descending(total)).
			SelectColumns(totalColumns()...)
		wrapped := &recordsetRecorder{}
		if _, err := SecureReadSession(wrapped, listed).ExecuteQueryToRecordsetReader(ctx, plain); err != nil {
			t.Fatal(err)
		}
		sent := wrapped.seen[0].(dal.StructuredQuery)
		if sent.Columns()[1].Alias != "total" || dal.ValidateAggregation(sent) != nil {
			t.Fatalf("columns sent = %v", sent.Columns())
		}
	})
}

func TestReferencesToAnAllowedAliasAreLeftAlone(t *testing.T) {
	total := dal.Field("total")
	query := groupedTotals().
		Having(dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})).
		OrderBy(dal.Descending(total)).
		SelectColumns(totalColumns()...)
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age", "total")))
	wrapped := &projectingSession{stored: storedUser}
	rows := readRows(t, SecureReadSession(wrapped, policy), query)
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"name": "Ann", "total": 2}) {
		t.Fatalf("rows = %v", rows)
	}
	sent := wrapped.seen[0]
	if !reflect.DeepEqual(sent.Having(), query.Having()) || !reflect.DeepEqual(sent.OrderBy(), query.OrderBy()) {
		t.Fatalf("HAVING = %v, ORDER BY = %v, want the caller's", sent.Having(), sent.OrderBy())
	}
}

// Two secured sessions, one over the other: each renames the aggregate in turn
// and each rewrites the references to the name it replaced, so the query that
// reaches the wrapped session is a valid aggregation whose HAVING and ORDER BY
// name its aggregate.
func TestReferencesToARenamedAliasFollowEveryRename(t *testing.T) {
	total := dal.Field("total")
	query := groupedTotals().
		Having(dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1})).
		OrderBy(dal.Descending(total)).
		SelectColumns(totalColumns()...)
	policy := func(name string) Policy {
		return MustPolicy(name, Collection("users", Allow(Query, "list").Fields("name", "age")))
	}
	wrapped := &projectingSession{stored: storedUser}
	rows := readRows(t, SecureReadSession(SecureReadSession(wrapped, policy("inner")), policy("outer")), query)
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"name": "Ann", "total": 2}) {
		t.Fatalf("rows = %v", rows)
	}
	sent := wrapped.seen[0]
	generated := wrapped.aliasesSent(t)[1]
	if generated == "" || generated == "total" {
		t.Fatalf("the aggregate was sent under %q", generated)
	}
	if !reflect.DeepEqual(sent.Having(), dal.NewComparison(dal.Field(generated), dal.GreaterThen, dal.Constant{Value: 1})) {
		t.Fatalf("HAVING sent = %v", sent.Having())
	}
	if len(sent.OrderBy()) != 1 || sent.OrderBy()[0].String() != dal.Descending(dal.Field(generated)).String() {
		t.Fatalf("ORDER BY sent = %v", sent.OrderBy())
	}
	if err := dal.ValidateAggregation(sent); err != nil {
		t.Fatalf("the query that reached the session is not a valid aggregation: %v", err)
	}
}

// A query that carries renamed aliases hands itself, not the query it wraps, to
// the executor, and renders with its own HAVING and ORDER BY.
func TestRenamedAliasQueryHandsItselfToTheExecutor(t *testing.T) {
	total := dal.Field("total")
	base := groupedTotals().SelectColumns(totalColumns()...)
	query := renamedAliasQuery{
		StructuredQuery: base,
		having:          dal.NewComparison(total, dal.GreaterThen, dal.Constant{Value: 1}),
		orderBy:         []dal.OrderExpression{dal.Descending(total)},
	}
	if text := query.String(); !strings.Contains(text, "total") || text != dal.QueryString(query) {
		t.Fatalf("String() = %q", text)
	}
	wrapped := &projectingSession{stored: storedUser}
	if _, err := query.GetRecordsReader(context.Background(), wrapped); err != nil {
		t.Fatal(err)
	}
	recorder := &recordsetRecorder{}
	if _, err := query.GetRecordsetReader(context.Background(), recorder); err != nil {
		t.Fatal(err)
	}
	if len(wrapped.seen) != 1 || len(recorder.seen) != 1 {
		t.Fatalf("%d records reads and %d recordset reads reached the executors", len(wrapped.seen), len(recorder.seen))
	}
	for _, reached := range []dal.Query{wrapped.seen[0], recorder.seen[0]} {
		if _, ok := reached.(renamedAliasQuery); !ok {
			t.Fatalf("the executor was handed a %T, want the wrapper itself", reached)
		}
	}
}

func TestRewriteAliasReferences(t *testing.T) {
	renamed := map[string]string{"total": "generated"}
	total, other := dal.Field("total"), dal.Field("other")
	var nilComparison *dal.Comparison
	var nilGroup *dal.GroupCondition
	var nilIsNull *dal.IsNullCondition
	cases := []struct {
		name      string
		condition dal.Condition
		want      dal.Condition
	}{
		{"a missing condition", nil, nil},
		{"a nil comparison", nilComparison, nilComparison},
		{"a nil null test", nilIsNull, nilIsNull},
		{"a nil condition group", nilGroup, nilGroup},
		{"a comparison", dal.NewComparison(total, dal.Equal, other), dal.NewComparison(dal.Field("generated"), dal.Equal, other)},
		{"a comparison through a pointer", &dal.Comparison{Left: other, Operator: dal.Equal, Right: &total}, dal.Comparison{Left: other, Operator: dal.Equal, Right: dal.Field("generated")}},
		{"a null test", dal.NewIsNullCondition(total), dal.NewIsNullCondition(dal.Field("generated"))},
		{"an IS NOT NULL test", dal.NewIsNotNullCondition(total), dal.NewIsNotNullCondition(dal.Field("generated"))},
		{"a null test through a pointer", func() dal.Condition { c := dal.NewIsNullCondition(total); return &c }(), dal.NewIsNullCondition(dal.Field("generated"))},
		{"a condition group through a pointer", func() dal.Condition {
			g := dal.NewGroupCondition(dal.Or, dal.NewIsNullCondition(total))
			return &g
		}(), dal.NewGroupCondition(dal.Or, dal.NewIsNullCondition(dal.Field("generated")))},
		{"a qualified field is no reference", dal.NewComparison(dal.NewFieldRef("users", "total"), dal.Equal, dal.Constant{Value: 1}), dal.NewComparison(dal.NewFieldRef("users", "total"), dal.Equal, dal.Constant{Value: 1})},
		{"a condition of another kind", dal.NewExistsCondition(nil), dal.NewExistsCondition(nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rewriteAliasReferences(c.condition, renamed); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("condition = %#v, want %#v", got, c.want)
			}
		})
	}
	t.Run("an order", func(t *testing.T) {
		orders := []dal.OrderExpression{dal.Descending(total), nil, dal.Ascending(&total), dal.AscendingField("other")}
		got := rewriteAliasReferencesInOrders(orders, renamed)
		if len(got) != 4 || got[0].String() != dal.Descending(dal.Field("generated")).String() || got[1] != nil ||
			got[2].String() != dal.Ascending(dal.Field("generated")).String() || got[3].String() != orders[3].String() {
			t.Fatalf("orders = %v", got)
		}
		if !got[0].Descending() || got[2].Descending() {
			t.Fatal("the direction of an order changed")
		}
	})
}

// A plan is judged by the recordset rule: the records reader runs HAVING and
// ORDER BY over the alias of an allowed aggregate, and a plan of the same query
// reports a denial for that alias, as it does for an alias the list does not
// allow. The aggregate itself, which every reader accepts, is not denied.
func TestAssessPlanJudgesAnAggregateAliasByTheRecordsetRule(t *testing.T) {
	ctx := context.Background()
	policy := MustPolicy("fields", Collection("users", Allow(Query, "list").Fields("name", "age")))
	byAlias := groupedTotals().
		Having(dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.Constant{Value: 1})).
		SelectColumns(totalColumns()...)
	byAggregate := groupedTotals().
		Having(dal.NewComparison(dal.SumAs(dal.Field("age"), "").Expression, dal.GreaterThen, dal.Constant{Value: 1})).
		SelectColumns(totalColumns()...)
	plan := func(query dal.StructuredQuery) Assessment {
		return AssessPlan(ctx, Request{Operation: Query, Resources: resourcesForQuery(query), Query: query}, []Policy{policy})
	}

	wrapped := &countingSession{}
	if _, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsReader(ctx, byAlias); err != nil || wrapped.reads != 1 {
		t.Fatalf("the records reader: error = %v, %d reads, want none and 1", err, wrapped.reads)
	}
	assessment := plan(byAlias)
	if assessment.Outcome != AssessmentDeny {
		t.Fatalf("outcome = %s, want deny", assessment.Outcome)
	}
	var columns [][]string
	for _, assessed := range assessment.Policies {
		if assessed.Decision.Code == CodeColumnDenied {
			columns = append(columns, assessed.Decision.Columns...)
		}
	}
	if !reflect.DeepEqual(columns, [][]string{{"total"}}) {
		t.Fatalf("denied columns = %v, want total", columns)
	}
	if assessment := plan(byAggregate); assessment.Outcome == AssessmentDeny {
		t.Fatalf("outcome = %s, want the aggregate itself to be allowed", assessment.Outcome)
	}
}
