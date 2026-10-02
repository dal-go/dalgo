package dal

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

func TestIsNullConditionModel(t *testing.T) {
	isNull := NewIsNullCondition(Field("company"))
	if isNull.Negated() || isNull.Operand().String() != "company" || isNull.String() != "company IS NULL" {
		t.Fatalf("IS NULL = %#v %q", isNull, isNull)
	}
	notNull := NewIsNotNullCondition(NewFieldRef("c", "company"))
	if !notNull.Negated() || notNull.String() != "c.company IS NOT NULL" {
		t.Fatalf("IS NOT NULL = %#v %q", notNull, notNull)
	}
	if got := Field("x").IsNull(); got != NewIsNullCondition(Field("x")) {
		t.Fatalf("FieldRef.IsNull = %#v", got)
	}
	if got := Field("x").IsNotNull(); got != NewIsNotNullCondition(Field("x")) {
		t.Fatalf("FieldRef.IsNotNull = %#v", got)
	}
	if got := NewIsNullCondition(nil).String(); got != "{NO_OPERAND} IS NULL" {
		t.Fatalf("nil operand = %q", got)
	}
	group := NewGroupCondition(And, isNull, NewComparison(Field("a"), Equal, Constant{Value: 1}))
	if got := group.String(); got != "(company IS NULL AND a = 1)" {
		t.Fatalf("group = %q", got)
	}
}

// nullTestRow holds one parent row (a) joined to an optional child (b).
func nullTestRow(a, b map[string]any) joinRow {
	row := joinRow{base: "a", sources: map[string]map[string]any{"a": a}}
	if b != nil {
		row.sources["b"] = b
	}
	return row
}

func TestIsNullTruthInBothJoinEvaluators(t *testing.T) {
	row := nullTestRow(map[string]any{"id": 1, "company": nil}, map[string]any{"amount": 5})
	cases := []struct {
		name string
		cond Condition
		want bool
	}{
		{"explicit null is null", NewIsNullCondition(NewFieldRef("a", "company")), true},
		{"explicit null is not not-null", NewIsNotNullCondition(NewFieldRef("a", "company")), false},
		{"missing field is null", NewIsNullCondition(NewFieldRef("a", "absent")), true},
		{"missing field is not not-null", NewIsNotNullCondition(NewFieldRef("a", "absent")), false},
		{"value is not null", NewIsNullCondition(NewFieldRef("a", "id")), false},
		{"value is not-null", NewIsNotNullCondition(NewFieldRef("a", "id")), true},
		{"absent source is null", NewIsNullCondition(NewFieldRef("c", "id")), true},
		{"constant null", NewIsNullCondition(Constant{Value: nil}), true},
		{"constant value", NewIsNotNullCondition(Constant{Value: 0}), true},
		{"arithmetic on null is null", NewIsNullCondition(Binary(NewFieldRef("a", "company"), Add, Constant{Value: 1})), true},
		{"arithmetic on values is not null", NewIsNullCondition(Binary(NewFieldRef("a", "id"), Add, Constant{Value: 1})), false},
		{"and", NewGroupCondition(And, NewIsNullCondition(NewFieldRef("a", "company")), NewIsNotNullCondition(NewFieldRef("b", "amount"))), true},
		{"and false", NewGroupCondition(And, NewIsNullCondition(NewFieldRef("a", "company")), NewIsNullCondition(NewFieldRef("b", "amount"))), false},
		{"or", NewGroupCondition(Or, NewIsNotNullCondition(NewFieldRef("a", "company")), NewIsNotNullCondition(NewFieldRef("b", "amount"))), true},
		{"or false", NewGroupCondition(Or, NewIsNotNullCondition(NewFieldRef("a", "company")), NewIsNullCondition(NewFieldRef("b", "amount"))), false},
		// IS NULL is never UNKNOWN: it rescues an OR whose other branch is UNKNOWN,
		// which the plain == null comparison cannot do.
		{"or with unknown comparison", NewGroupCondition(Or, NewComparison(NewFieldRef("a", "company"), Equal, Constant{Value: nil}), NewIsNullCondition(NewFieldRef("a", "company"))), true},
		{"null comparison stays unknown", NewComparison(NewFieldRef("a", "company"), Equal, Constant{Value: nil}), false},
	}
	e := &joinExecution{recursive: true}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy, err := evalJoinCondition(tc.cond, row)
			if err != nil || legacy != tc.want {
				t.Fatalf("legacy evaluator = %v, %v; want %v", legacy, err, tc.want)
			}
			truth, err := e.evalTruthAt(tc.cond, row, "where")
			if err != nil || (truth == queryTrue) != tc.want {
				t.Fatalf("recursive evaluator = %v, %v; want %v", truth, err, tc.want)
			}
			if truth == queryUnknown && strings.Contains(tc.name, "null comparison") == false {
				t.Fatalf("IS NULL produced UNKNOWN for %s", tc.cond)
			}
		})
	}
	for _, cond := range []Condition{NewIsNullCondition(nil), NewIsNotNullCondition(nil)} {
		if _, err := evalJoinCondition(cond, row); err == nil {
			t.Fatal("legacy evaluator accepted a nil operand")
		}
		if _, err := e.evalTruthAt(cond, row, "where"); err == nil || !strings.Contains(err.Error(), "query_shape at where") {
			t.Fatalf("recursive evaluator nil operand error = %v", err)
		}
	}
	bad := NewIsNullCondition(Binary(Constant{Value: 1}, ArithmeticOperator("??"), Constant{Value: 1}))
	if _, err := evalJoinCondition(bad, row); err == nil {
		t.Fatal("legacy evaluator hid an operand error")
	}
	if _, err := e.evalTruthAt(bad, row, "where"); err == nil {
		t.Fatal("recursive evaluator hid an operand error")
	}
}

func nullJoinBackend() *ignoringJoinBackend {
	return &ignoringJoinBackend{data: map[string][]record.Record{
		"Chat": {
			joinTestRecord("Chat", "1", map[string]any{"id": 1, "company": nil}),
			joinTestRecord("Chat", "2", map[string]any{"id": 2, "company": "acme"}),
			joinTestRecord("Chat", "3", map[string]any{"id": 3}),
		},
		"Invoice": {
			joinTestRecord("Invoice", "i1", map[string]any{"chat": 1, "total": 10}),
			joinTestRecord("Invoice", "i2", map[string]any{"chat": 1, "total": nil}),
			joinTestRecord("Invoice", "i3", map[string]any{"chat": 2, "total": 7}),
		},
	}, reads: map[string]int{}}
}

func runNullJoin(t *testing.T, where Condition) []int {
	t.Helper()
	chat := NewRootCollectionRef("Chat", "c")
	invoice := NewRootCollectionRef("Invoice", "i")
	root := From(chat).Join(NewJoinedSource(invoice, JoinLeft, NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("i", "chat"))))
	q := root.NewQuery().Where(where).SelectColumns(Column{Expression: NewFieldRef("c", "id"), Alias: "id"})
	reader, err := NewDB(nullJoinBackend()).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, row := range rows {
		ids = append(ids, int(row.Data().(map[string]any)["id"].(float64)))
	}
	sort.Ints(ids)
	return ids
}

func TestIsNullInJoinedWhere(t *testing.T) {
	company := NewFieldRef("c", "company")
	total := NewFieldRef("i", "total")
	cases := []struct {
		name  string
		where Condition
		want  []int
	}{
		// The reported regression: == null never matches in a joined query ...
		{"equal null matches nothing", NewComparison(company, Equal, Constant{Value: nil}), nil},
		// ... and isNull selects the null and missing parents instead.
		{"parent is null or missing", NewIsNullCondition(company), []int{1, 1, 3}},
		{"parent is not null", NewIsNotNullCondition(company), []int{2}},
		{"anti join via null-extended child", NewIsNullCondition(NewFieldRef("i", "chat")), []int{3}},
		{"null total", NewIsNullCondition(total), []int{1, 3}},
		{"group", NewGroupCondition(And, NewIsNullCondition(company), NewIsNotNullCondition(total)), []int{1}},
		{"or group", NewGroupCondition(Or, NewIsNotNullCondition(company), NewIsNullCondition(total)), []int{1, 2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runNullJoin(t, tc.where)
			if len(got) != len(tc.want) {
				t.Fatalf("ids = %v; want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v; want %v", got, tc.want)
				}
			}
		})
	}
}

func TestIsNullInHaving(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	maxAmount := MaxAs(Field("amount"), "top")
	build := func(having Condition) StructuredQuery {
		return From(sales).NewQuery().GroupBy(Field("category")).Having(having).
			SelectColumns(Column{Expression: Field("category")}, maxAmount)
	}
	raw := func() *aggregationCoverageReader {
		return &aggregationCoverageReader{records: []record.Record{
			aggregationCoverageRecord("1", map[string]any{"category": "a", "amount": nil}),
			aggregationCoverageRecord("2", map[string]any{"category": "b", "amount": 4}),
			aggregationCoverageRecord("3", map[string]any{"category": "c"}),
			aggregationCoverageRecord("4", map[string]any{"category": nil, "amount": 1}),
		}}
	}
	categories := func(having Condition) []string {
		reader := newLocalAggregationReader(context.Background(), build(having), raw(), AggregationPlan{Strategy: AggregationHash})
		var out []string
		for _, row := range aggregationCoverageRows(t, reader) {
			if v, ok := row["category"].(string); ok {
				out = append(out, v)
			} else {
				out = append(out, "<null>")
			}
		}
		sort.Strings(out)
		return out
	}
	join := func(items []string) string { return strings.Join(items, ",") }
	if got := join(categories(NewIsNullCondition(Field("top")))); got != "a,c" {
		t.Fatalf("HAVING max IS NULL = %q", got)
	}
	if got := join(categories(NewIsNotNullCondition(Field("top")))); got != "<null>,b" {
		t.Fatalf("HAVING max IS NOT NULL = %q", got)
	}
	if got := join(categories(NewIsNullCondition(Field("category")))); got != "<null>" {
		t.Fatalf("HAVING key IS NULL = %q", got)
	}
	if got := join(categories(NewGroupCondition(Or, NewIsNullCondition(Field("category")), NewIsNotNullCondition(Field("top"))))); got != "<null>,b" {
		t.Fatalf("HAVING group = %q", got)
	}
	if got := join(categories(NewIsNullCondition(NewAggregate(MAX, false, Field("amount"))))); got != "a,c" {
		t.Fatalf("HAVING aggregate expression IS NULL = %q", got)
	}
	r := newLocalAggregationReader(context.Background(), build(nil), raw(), AggregationPlan{Strategy: AggregationHash})
	group := &localGroup{keyValues: map[string]any{}, out: map[string]any{}}
	if _, err := r.evalHaving(NewIsNullCondition(Field("missing")), group); err == nil {
		t.Fatal("HAVING IS NULL over an unknown name succeeded")
	}
	if _, err := r.evalHaving(NewIsNullCondition(nil), group); err == nil {
		t.Fatal("HAVING IS NULL with a nil operand succeeded")
	}
}

func TestIsNullAggregationPlanning(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	q := From(sales).NewQuery().GroupBy(Field("category")).
		Having(NewIsNullCondition(Field("amount"))).
		SelectColumns(Column{Expression: Field("category")}, CountAs(Star(), "n"))
	if err := ValidateAggregation(q); err == nil || !strings.Contains(err.Error(), "HAVING") {
		t.Fatalf("ungrouped IS NULL operand in HAVING = %v", err)
	}
	ok := From(sales).NewQuery().GroupBy(Field("category")).
		Having(NewIsNullCondition(NewAggregate(MAX, false, Field("amount")))).
		SelectColumns(Column{Expression: Field("category")})
	if err := ValidateAggregation(ok); err != nil {
		t.Fatalf("aggregate operand in HAVING rejected: %v", err)
	}
	if got := uniqueAggregates(ok); len(got) != 1 || got[0].String() != "MAX(amount)" {
		t.Fatalf("aggregates hidden in IS NULL operand = %#v", got)
	}
	if got := collectSourceFields(ok); len(got) != 2 {
		t.Fatalf("fields hidden in IS NULL operand = %#v", got)
	}
	if err := ValidateAggregation(From(sales).NewQuery().GroupBy(Field("category")).Having(NewIsNullCondition(nil)).SelectColumns(Column{Expression: Field("category")})); err == nil {
		t.Fatal("nil IS NULL operand accepted")
	}
}

func TestIsNullQueryScopeAndFreeReferences(t *testing.T) {
	scalar := From(NewRootCollectionRef("Invoice", "i")).NewQuery().
		Where(NewComparison(NewFieldRef("i", "customer"), Equal, NewFieldRef("c", "id"))).
		SelectColumns(Column{Expression: NewFieldRef("i", "total")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().
		Where(NewIsNullCondition(NewQueryExpression(scalar, "t"))).SelectIntoRecordset()
	if !HasSubquery(query) {
		t.Fatal("subquery inside an IS NULL operand was not detected")
	}
	if err := ValidateQueryScope(query); err != nil {
		t.Fatalf("outer correlation inside IS NULL rejected: %v", err)
	}
	bad := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewIsNotNullCondition(NewFieldRef("missing", "id"))).SelectIntoRecordset()
	if err := ValidateQueryScope(bad); err == nil || !strings.Contains(err.Error(), "query_scope at where.operand.source") {
		t.Fatalf("unknown qualifier in IS NULL = %v", err)
	}
	if !queryHasOuterReference(From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewIsNullCondition(NewFieldRef("c", "id"))).SelectIntoRecordset()) {
		t.Fatal("outer reference inside IS NULL was not detected")
	}
}

func TestIsNullRecursiveQueryEndToEnd(t *testing.T) {
	// A correlated EXISTS whose own WHERE uses IS NULL: the recursive executor
	// must evaluate IS NULL inside the nested query and the validation walkers
	// must see through it.
	chat := NewRootCollectionRef("Chat", "c")
	invoice := NewRootCollectionRef("Invoice", "i")
	nested := From(invoice).NewQuery().Where(NewGroupCondition(And,
		NewComparison(NewFieldRef("i", "chat"), Equal, NewFieldRef("c", "id")),
		NewIsNullCondition(NewFieldRef("i", "total")),
	)).SelectIntoRecordset()
	q := From(chat).NewQuery().Where(NewGroupCondition(Or,
		NewExistsCondition(nested),
		NewIsNullCondition(NewFieldRef("c", "company")),
	)).SelectColumns(Column{Expression: NewFieldRef("c", "id"), Alias: "id"})
	reader, err := NewDB(nullJoinBackend()).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, row := range rows {
		ids = append(ids, int(row.Data().(map[string]any)["id"].(float64)))
	}
	sort.Ints(ids)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("ids = %v; want [1 3]", ids)
	}
}

func TestIsNullRejectedInJoinOn(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner,
		NewIsNullCondition(NewFieldRef("b", "id"))))
	if err := ValidateJoinTree(root); err == nil || !strings.Contains(err.Error(), "ON predicate must be a comparison") {
		t.Fatalf("IS NULL in ON = %v", err)
	}
}

func TestIsNullHavingAggregatesReachPlanning(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	first := From(sales).NewQuery().GroupBy(Field("category")).
		Having(NewIsNotNullCondition(NewAggregate(FIRST, false, Field("amount")))).
		SelectColumns(Column{Expression: Field("category")})
	if !needsStableOrder(first) {
		t.Fatal("FIRST hidden inside HAVING ... IS NOT NULL was not seen by planning")
	}
	maxQuery := From(sales).NewQuery().GroupBy(Field("category")).
		Having(NewIsNullCondition(NewAggregate(MAX, false, Field("amount")))).
		SelectColumns(Column{Expression: Field("category")})
	if nativeAggregationSupported(maxQuery, QueryCapabilities{GroupBy: true, Having: true}) {
		t.Fatal("native aggregation accepted MAX hidden inside HAVING ... IS NULL without MAX support")
	}
	if !nativeAggregationSupported(maxQuery, QueryCapabilities{GroupBy: true, Having: true, Aggregate: AggregateCapabilities{Max: true}}) {
		t.Fatal("native aggregation rejected supported MAX inside HAVING ... IS NULL")
	}
}
