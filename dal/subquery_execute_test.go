package dal

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type cappedTestReader struct {
	records []record.Record
	next    int
	closed  bool
}

func (r *cappedTestReader) Next() (record.Record, error) {
	if r.next >= len(r.records) {
		return nil, ErrNoMoreRecords
	}
	value := r.records[r.next]
	r.next++
	return value, nil
}
func (*cappedTestReader) Cursor() (string, error) { return "", nil }
func (r *cappedTestReader) Close() error          { r.closed = true; return nil }

type cappedTestBackend struct {
	data    map[string][]record.Record
	readers map[string]*cappedTestReader
}

func (b *cappedTestBackend) ExecuteQueryToRecordsReader(_ context.Context, q Query) (RecordsReader, error) {
	name := q.(StructuredQuery).From().Base().Name()
	r := &cappedTestReader{records: b.data[name]}
	b.readers[name] = r
	return r, nil
}
func (*cappedTestBackend) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, errors.New("recordset unsupported")
}

func TestGenericRecursiveExistsAndScalar(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1}), joinTestRecord("Customer", "2", map[string]any{"id": 2})},
		"Invoice":  {joinTestRecord("Invoice", "a", map[string]any{"customer": 1, "total": 9})},
	}, reads: map[string]int{}}
	invoice := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "customer"), Equal, NewFieldRef("c", "id"))).SelectColumns(Column{Expression: NewFieldRef("i", "total")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(invoice)).SelectColumns(
		Column{Expression: NewFieldRef("c", "id")},
		Column{Expression: NewQueryExpression(invoice, "total")},
	)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	records, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	data := records[0].Data().(map[string]any)
	if data["id"] != float64(1) || data["total"] != float64(9) {
		t.Fatalf("row = %#v", data)
	}
	if backend.reads["Invoice"] < 2 {
		t.Fatalf("nested leaf scans = %d", backend.reads["Invoice"])
	}
}

func TestRecursiveSimpleChildCapClosesAfterSecondScalarRow(t *testing.T) {
	backend := &cappedTestBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1}), joinTestRecord("Invoice", "2", map[string]any{"id": 2}), joinTestRecord("Invoice", "3", map[string]any{"id": 3})},
	}, readers: map[string]*cappedTestReader{}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: NewQueryExpression(child, "invoice")})
	_, err := ExecuteRecursiveQuery(context.Background(), backend, query)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "cardinality" {
		t.Fatalf("scalar error = %v", err)
	}
	invoice := backend.readers["Invoice"]
	if invoice == nil || invoice.next != 2 || !invoice.closed {
		t.Fatalf("invoice reader = %#v", invoice)
	}
}

func TestRecursiveSimpleScalarRespectsChildLimit(t *testing.T) {
	backend := &cappedTestBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1}), joinTestRecord("Invoice", "2", map[string]any{"id": 2})},
	}, readers: map[string]*cappedTestReader{}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Limit(1).SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().SelectColumns(Column{Expression: NewQueryExpression(child, "invoice")})
	reader, err := ExecuteRecursiveQuery(context.Background(), backend, query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAllToRecords(context.Background(), reader); err != nil {
		t.Fatal(err)
	}
	if backend.readers["Invoice"].next != 1 {
		t.Fatalf("child reads = %d", backend.readers["Invoice"].next)
	}
}

func TestRecursiveExistsSkipsChildProjection(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1}), joinTestRecord("Invoice", "2", map[string]any{"id": 2})},
	}, reads: map[string]int{}}
	badScalar := From(NewRootCollectionRef("Invoice", "s")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("s", "id")})
	exists := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewQueryExpression(badScalar, "bad")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(exists)).SelectIntoRecord(nil)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(rows) != 1 {
		t.Fatalf("exists rows=%#v err=%v", rows, err)
	}
}

func TestRecursiveCappedPathDoesNotSendDerivedSourceToLeaf(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	derived := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	child := From(NewQuerySource(derived, "d")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("d", "id")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(child)).SelectIntoRecord(nil)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(rows) != 1 || backend.reads["Invoice"] != 1 {
		t.Fatalf("derived exists rows=%#v reads=%v err=%v", rows, backend.reads, err)
	}
}

func TestRecursiveCappedPathRejectsChildCursorBeforeLeafScan(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().StartAfter("cursor").SelectIntoRecord(nil)
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(child)).SelectIntoRecord(nil)
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err == nil || !strings.Contains(err.Error(), "cursor") || backend.reads["Invoice"] != 0 {
		t.Fatalf("cursor error=%v reads=%v", err, backend.reads)
	}
}

func TestRecursiveRootBudgetCounters(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "1", map[string]any{"id": "b1", "aid": 1, "cid": 9})},
		"C": {joinTestRecord("C", "1", map[string]any{"id": 9})},
	}, reads: map[string]int{}}
	flat := From(NewRootCollectionRef("A", "a")).NewQuery().SelectIntoRecord(nil)
	cases := []struct {
		name, counter, path string
		query               StructuredQuery
		budget              recursiveBudget
	}{
		{name: "fetched", counter: "fetched_rows", path: "from", query: flat, budget: recursiveBudget{fetched: maxJoinRows}},
		{name: "result", counter: "result_rows", path: "columns", query: flat, budget: recursiveBudget{output: maxJoinRows}},
		{name: "bytes", counter: "retained_bytes", path: "from", query: flat, budget: recursiveBudget{bytes: maxJoinBytes}},
		{name: "candidates", counter: "candidate_evaluations", path: "from.joins[0].from.joins[0]", query: joinTestQuery(), budget: recursiveBudget{candidates: maxJoinRows * 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := executeGenericRecursiveBudget(context.Background(), backend, tc.query, nil, &tc.budget)
			var diagnostic *QueryValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" || diagnostic.Path != tc.path || diagnostic.Message != tc.counter {
				t.Fatalf("budget error = %#v, want query_limit at %s: %s", err, tc.path, tc.counter)
			}
		})
	}
}

func TestRecursiveMemoReuseChargesCandidateWork(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Invoice": {joinTestRecord("Invoice", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	budget := &recursiveBudget{memo: map[string][]memoizedQuery{}}
	execution := &joinExecution{ctx: context.Background(), executor: backend, budget: budget, memo: budget.memo}
	query := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	if _, err := execution.queryRecordsCapped(query, nil, 1, false); err != nil {
		t.Fatal(err)
	}
	if backend.reads["Invoice"] != 1 {
		t.Fatalf("first use leaf scans = %d", backend.reads["Invoice"])
	}
	fetched, bytes := budget.fetched, budget.bytes
	budget.candidates = maxJoinRows * 10
	_, err := execution.queryRecordsCapped(query, nil, 1, false)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" || diagnostic.Message != "candidate_evaluations" {
		t.Fatalf("cache work diagnostic = %v", err)
	}
	if backend.reads["Invoice"] != 1 || budget.fetched != fetched || budget.bytes != bytes {
		t.Fatalf("cache repeated leaf work: scans=%d fetched=%d bytes=%d", backend.reads["Invoice"], budget.fetched, budget.bytes)
	}
}

func TestRecursiveMemoReuseChargesOriginalJoinCandidates(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "1", map[string]any{"id": 1})},
		"B": {
			joinTestRecord("B", "1", map[string]any{"id": 1, "aid": 1}),
			joinTestRecord("B", "2", map[string]any{"id": 2, "aid": 1}),
			joinTestRecord("B", "3", map[string]any{"id": 3, "aid": 1}),
			joinTestRecord("B", "4", map[string]any{"id": 4, "aid": 1}),
			joinTestRecord("B", "5", map[string]any{"id": 5, "aid": 1}),
		},
	}, reads: map[string]int{}}
	budget := &recursiveBudget{memo: map[string][]memoizedQuery{}}
	execution := &joinExecution{ctx: context.Background(), executor: backend, budget: budget, memo: budget.memo}
	from := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner,
		NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	query := from.NewQuery().Where(NewComparison(NewFieldRef("b", "id"), Equal, NewConstant(1))).SelectColumns(Column{Expression: NewFieldRef("a", "id")})
	rows, err := execution.queryRecords(query, nil)
	if err != nil || len(rows) != 1 || budget.candidates < 5 {
		t.Fatalf("first execution rows=%d candidates=%d err=%v", len(rows), budget.candidates, err)
	}
	reads := backend.reads["B"]
	budget.candidates = maxJoinRows*10 - 4
	_, err = execution.queryRecords(query, nil)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" || diagnostic.Message != "candidate_evaluations" || backend.reads["B"] != reads {
		t.Fatalf("memo work error=%v B scans=%d, want cached work limit and %d scans", err, backend.reads["B"], reads)
	}
}

func TestRecursivePointerDerivedSourceUsesGenericLeafScan(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Invoice": {joinTestRecord("Invoice", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	inner := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	source := NewQuerySource(inner, "d")
	query := From(&source).NewQuery().SelectColumns(Column{Expression: NewFieldRef("d", "id")})
	if !HasSubquery(query) {
		t.Fatal("pointer derived source missed recursive routing")
	}
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(rows) != 1 || backend.reads["Invoice"] != 1 || backend.reads["d"] != 0 {
		t.Fatalf("pointer derived rows=%d leaf scans=%v err=%v", len(rows), backend.reads, err)
	}
}

func TestRecursiveCappedExistsChargesAllNonqualifyingRowsAcrossRoot(t *testing.T) {
	invoices := make([]record.Record, maxJoinRows)
	for i := range invoices {
		invoices[i] = joinTestRecord("Invoice", strconv.Itoa(i), map[string]any{"id": i})
	}
	backend := &cappedTestBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  invoices,
	}, readers: map[string]*cappedTestReader{}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "id"), Equal, NewConstant(-1))).SelectIntoRecord(nil)
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(child)).SelectIntoRecord(nil)
	_, err := ExecuteRecursiveQuery(context.Background(), backend, query)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" || diagnostic.Path != "where.query.from" || diagnostic.Message != "fetched_rows" {
		t.Fatalf("budget error = %#v", err)
	}
	if invoice := backend.readers["Invoice"]; invoice == nil || !invoice.closed {
		t.Fatalf("invoice reader not closed: %#v", invoice)
	}
}

func TestRecursiveNestedConditionLimitReportsActualPath(t *testing.T) {
	backend := &cappedTestBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice":  {joinTestRecord("Invoice", "1", map[string]any{"id": 1})},
	}, readers: map[string]*cappedTestReader{}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewGroupCondition(And,
		NewExistsCondition(child))).SelectIntoRecord(nil)
	budget := &recursiveBudget{fetched: maxJoinRows - 1}
	_, err := executeGenericRecursiveBudget(context.Background(), backend, query, nil, budget)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" || diagnostic.Path != "where.conditions[0].query.from" || diagnostic.Message != "fetched_rows" {
		t.Fatalf("nested limit path = %v", err)
	}
}

func TestGenericRecursiveMemoizesUncorrelatedNestedQuery(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1}), joinTestRecord("Customer", "2", map[string]any{"id": 2})},
		"Invoice":  {joinTestRecord("Invoice", "a", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	uncorrelated := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(uncorrelated)).SelectIntoRecord(nil)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAllToRecords(context.Background(), reader); err != nil {
		t.Fatal(err)
	}
	if backend.reads["Invoice"] != 1 {
		t.Fatalf("uncorrelated reads = %d, want 1", backend.reads["Invoice"])
	}
}

func TestRecursiveMemoSeparatesDistinctJoinTrees(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "1", map[string]any{"aid": 1})},
		"C": {joinTestRecord("C", "1", map[string]any{"aid": 1})},
	}, reads: map[string]int{}}
	join := func(name, alias string) StructuredQuery {
		return From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef(name, alias), JoinInner,
			NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef(alias, "aid")))).NewQuery().SelectIntoRecord(nil)
	}
	budget := &recursiveBudget{memo: map[string][]memoizedQuery{}}
	e := &joinExecution{ctx: context.Background(), executor: backend, budget: budget, memo: budget.memo}
	if _, err := e.queryRecords(join("B", "b"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.queryRecords(join("C", "c"), nil); err != nil {
		t.Fatal(err)
	}
	if backend.reads["C"] == 0 {
		t.Fatal("distinct JOIN tree reused cached B result")
	}
}

func TestGenericRecursiveScalarDiagnosticsAndEmptyRecordsetShape(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice": {
			joinTestRecord("Invoice", "a", map[string]any{"id": 1, "total": 9}),
			joinTestRecord("Invoice", "b", map[string]any{"id": 2, "total": 8}),
		},
	}, reads: map[string]int{}}
	outer := From(NewRootCollectionRef("Customer", "c")).NewQuery()
	rows := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	manyRows := outer.SelectColumns(Column{Expression: NewQueryExpression(rows, "invoice")})
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), manyRows)
	var validation *QueryValidationError
	if !errors.As(err, &validation) || validation.Category != "cardinality" || validation.Path != "columns[0].query" {
		t.Fatalf("many-row scalar error = %#v", err)
	}
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), outer.SelectColumns(
		Column{Expression: NewFieldRef("c", "id")}, Column{Expression: NewQueryExpression(rows, "invoice")},
	))
	if !errors.As(err, &validation) || validation.Category != "cardinality" || validation.Path != "columns[1].query" {
		t.Fatalf("second-column scalar error = %#v", err)
	}
	manyColumns := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "id"), Equal, Constant{Value: 1})).SelectColumns(
		Column{Expression: NewFieldRef("i", "id")},
		Column{Expression: NewFieldRef("i", "total")},
	)
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), outer.SelectColumns(Column{Expression: NewQueryExpression(manyColumns, "invoice")}))
	if !errors.As(err, &validation) || validation.Category != "shape" || validation.Path != "columns[0].query.columns" {
		t.Fatalf("many-column scalar error = %#v", err)
	}
	empty := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewNotExistsCondition(rows)).SelectColumns(
		Column{Expression: NewFieldRef("c", "id")},
		Column{Expression: NewQueryExpression(rows, "invoice")},
	)
	recordsetReader, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	defer recordsetReader.Close()
	var names []string
	for _, column := range recordsetReader.Recordset().Columns() {
		names = append(names, column.Name())
	}
	if !reflect.DeepEqual(names, []string{"id", "invoice"}) {
		t.Fatalf("empty recordset columns = %v", names)
	}
}

func TestGenericRecursiveValidatesScopeBeforeLeafScan(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	invalid := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(
		From(NewRootCollectionRef("Customer", "inner")).NewQuery().Where(NewComparison(NewFieldRef("inner", "id"), Equal, NewFieldRef("missing", "id"))).SelectIntoRecord(nil),
	)).SelectIntoRecord(nil)
	if _, err := ExecuteRecursiveQuery(context.Background(), backend, invalid); err == nil {
		t.Fatal("invalid recursive scope succeeded")
	}
	if len(backend.reads) != 0 {
		t.Fatalf("invalid scope performed leaf scans: %#v", backend.reads)
	}
}

func TestPlanRecursiveQueryIsConservative(t *testing.T) {
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(
		From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil),
	)).SelectIntoRecord(nil)
	plan, err := PlanRecursiveQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != RecursiveQueryGeneric || plan.Reason == "" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestGenericRecursiveBindsAmbiguousUnqualifiedFieldWithSchema(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"CustomerId": 1})},
		"Ledger":   {joinTestRecord("Ledger", "1", map[string]any{"CustomerId": 1})},
	}, fields: map[string][]string{
		"Customer": {"CustomerId"},
		"Ledger":   {"CustomerId"},
	}, reads: map[string]int{}}
	ledger := NewRootCollectionRef("Ledger", "l")
	from := From(NewRootCollectionRef("Customer", "c")).Join(NewJoinedSource(ledger, JoinInner,
		NewComparison(NewFieldRef("c", "CustomerId"), Equal, NewFieldRef("l", "CustomerId"))))
	// The EXISTS node selects the recursive evaluator; the unqualified output
	// must then be resolved from supplied schema rather than guessed as c.
	query := from.NewQuery().Where(NewExistsCondition(From(NewRootCollectionRef("Customer", "e")).NewQuery().SelectIntoRecord(nil))).SelectColumns(
		Column{Expression: NewFieldRef("", "CustomerId")},
	)
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "scope" || diagnostic.Path != "columns[0]" {
		t.Fatalf("ambiguous field diagnostic = %v", err)
	}
}

func TestGenericRecursiveUsesBoundUnqualifiedProjectionSource(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"CustomerId": 1})},
		"Ledger":   {joinTestRecord("Ledger", "1", map[string]any{"CustomerId": 1, "total": 7})},
	}, fields: map[string][]string{
		"Customer": {"CustomerId"},
		"Ledger":   {"CustomerId", "total"},
	}, reads: map[string]int{}}
	from := From(NewRootCollectionRef("Customer", "c")).Join(NewJoinedSource(NewRootCollectionRef("Ledger", "l"), JoinInner,
		NewComparison(NewFieldRef("c", "CustomerId"), Equal, NewFieldRef("l", "CustomerId"))))
	query := from.NewQuery().Where(NewExistsCondition(From(NewRootCollectionRef("Customer", "e")).NewQuery().SelectIntoRecord(nil))).SelectColumns(
		Column{Expression: NewFieldRef("", "total")},
	)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(rows) != 1 || rows[0].Data().(map[string]any)["total"] != float64(7) {
		t.Fatalf("bound projection rows=%#v err=%v", rows, err)
	}
}

func TestGenericRecursiveCorrelatesDerivedJoinRightPerLeftRow(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {
			joinTestRecord("Customer", "1", map[string]any{"id": 1}),
			joinTestRecord("Customer", "2", map[string]any{"id": 2}),
		},
		"Invoice": {joinTestRecord("Invoice", "i", map[string]any{"customer": 1, "total": 9})},
	}, reads: map[string]int{}}
	derived := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(
		NewComparison(NewFieldRef("i", "customer"), Equal, NewFieldRef("c", "id")),
	).SelectColumns(Column{Expression: NewFieldRef("i", "customer")}, Column{Expression: NewFieldRef("i", "total")})
	from := From(NewRootCollectionRef("Customer", "c")).Join(NewJoinedSource(NewQuerySource(derived, "d"), JoinLeft,
		NewComparison(NewFieldRef("c", "id"), Equal, NewFieldRef("d", "customer"))))
	query := from.NewQuery().Where(NewExistsCondition(From(NewRootCollectionRef("Customer", "e")).NewQuery().SelectIntoRecord(nil))).OrderBy(Ascending(NewFieldRef("c", "id"))).SelectColumns(
		Column{Expression: NewFieldRef("c", "id")}, Column{Expression: NewFieldRef("d", "total")},
	)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil || len(rows) != 2 {
		t.Fatalf("derived rows=%#v err=%v", rows, err)
	}
	first, second := rows[0].Data().(map[string]any), rows[1].Data().(map[string]any)
	if first["total"] != float64(9) || second["total"] != nil {
		t.Fatalf("derived values = %#v, %#v", first, second)
	}
}

func TestRecursiveTruthTablesRetainUnknownUntilFilterBoundary(t *testing.T) {
	e := &joinExecution{}
	row := joinRow{base: "t", sources: map[string]map[string]any{"t": {"value": nil}}}
	unknown := NewComparison(NewFieldRef("t", "value"), Equal, Constant{Value: 1})
	falseCondition := NewComparison(Constant{Value: 1}, Equal, Constant{Value: 2})
	trueCondition := NewComparison(Constant{Value: 1}, Equal, Constant{Value: 1})
	for name, testCase := range map[string]struct {
		condition Condition
		want      queryTruth
	}{
		"comparison NULL":   {unknown, queryUnknown},
		"unknown AND false": {NewGroupCondition(And, unknown, falseCondition), queryFalse},
		"unknown OR false":  {NewGroupCondition(Or, unknown, falseCondition), queryUnknown},
		"unknown OR true":   {NewGroupCondition(Or, unknown, trueCondition), queryTrue},
		"NULL NOT IN empty": {NewComparison(NewFieldRef("t", "value"), NotIn, Array{Value: []int{}}), queryTrue},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := e.evalTruth(testCase.condition, row)
			if err != nil || got != testCase.want {
				t.Fatalf("truth = %v, %v; want %v", got, err, testCase.want)
			}
		})
	}
}
