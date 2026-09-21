package dal

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type aggregationCoverageExpression struct{ text string }

func (e aggregationCoverageExpression) String() string { return e.text }

type aggregationCoverageCondition struct{}

func (aggregationCoverageCondition) String() string { return "unsupported" }

type aggregationCoverageBackend struct {
	Backend
	records      RecordsReader
	recordsErr   error
	recordset    RecordsetReader
	recordsetErr error
	capabilities QueryCapabilities
	seen         Query
	readonlyTx   ReadTransaction
}

func (b *aggregationCoverageBackend) ExecuteQueryToRecordsReader(_ context.Context, q Query) (RecordsReader, error) {
	b.seen = q
	if b.recordsErr != nil {
		return nil, b.recordsErr
	}
	return b.records, nil
}

func (b *aggregationCoverageBackend) ExecuteQueryToRecordsetReader(_ context.Context, q Query, _ ...recordset.Option) (RecordsetReader, error) {
	b.seen = q
	if b.recordsetErr != nil {
		return nil, b.recordsetErr
	}
	return b.recordset, nil
}

func (b *aggregationCoverageBackend) QueryCapabilities() QueryCapabilities { return b.capabilities }

func (b *aggregationCoverageBackend) RunReadonlyTransaction(ctx context.Context, worker ROTxWorker, _ ...TransactionOption) error {
	return worker(ctx, b.readonlyTx)
}

type aggregationCoverageReader struct {
	records   []record.Record
	index     int
	err       error
	closeErr  error
	closed    bool
	cursorErr error
}

type aggregationCoverageSequenceReader struct {
	total int
	next  int
}

func (r *aggregationCoverageSequenceReader) Next() (record.Record, error) {
	if r.next >= r.total {
		return nil, ErrNoMoreRecords
	}
	id := strconv.Itoa(r.next)
	r.next++
	return aggregationCoverageRecord(id, map[string]any{"category": id}), nil
}

func (*aggregationCoverageSequenceReader) Cursor() (string, error) { return "", nil }
func (*aggregationCoverageSequenceReader) Close() error            { return nil }

type aggregationCoverageTx struct {
	ReadTransaction
	backend *aggregationCoverageBackend
}

type aggregationCoverageSelectingTx struct {
	aggregationCoverageTx
	selected Reader
}

func (tx aggregationCoverageSelectingTx) Select(context.Context, Query) (Reader, error) {
	return tx.selected, nil
}

func (tx aggregationCoverageTx) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	return tx.backend.ExecuteQueryToRecordsReader(ctx, q)
}

func (tx aggregationCoverageTx) ExecuteQueryToRecordsetReader(ctx context.Context, q Query, options ...recordset.Option) (RecordsetReader, error) {
	return tx.backend.ExecuteQueryToRecordsetReader(ctx, q, options...)
}

func (r *aggregationCoverageReader) Next() (record.Record, error) {
	if r.index < len(r.records) {
		rec := r.records[r.index]
		r.index++
		return rec, nil
	}
	if r.err != nil {
		return nil, r.err
	}
	return nil, ErrNoMoreRecords
}

func (r *aggregationCoverageReader) Cursor() (string, error) { return "raw", r.cursorErr }
func (r *aggregationCoverageReader) Close() error {
	r.closed = true
	return r.closeErr
}

func aggregationCoverageRecord(id string, data any) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID("sales", id), data)
}

func aggregationCoverageRows(t *testing.T, reader RecordsReader) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for {
		rec, err := reader.Next()
		if err == ErrNoMoreRecords {
			return rows
		}
		if err != nil {
			t.Fatal(err)
		}
		row, err := normalizedRecordMap(rec)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
}

func TestAggregationExpressionCoverage(t *testing.T) {
	binary := Binary(Field("left"), Add, Field("right"))
	if got := binary.String(); got != "(left + right)" {
		t.Fatalf("Binary.String() = %q", got)
	}
	if binary.Operator != Add {
		t.Fatalf("Binary operator = %q", binary.Operator)
	}

	star, ok := Star().(StarExpression)
	if !ok || !star.IsStar() {
		t.Fatal("Star() does not expose StarExpression")
	}
	if got := NewAggregate("sum", true, Field("amount")).String(); got != "SUM(DISTINCT amount)" {
		t.Fatalf("distinct aggregate = %q", got)
	}
	columns := []Column{
		CountDistinctAs(Field("x"), "cd"),
		SumDistinctAs(Field("x"), "sd"),
		AverageDistinctAs(Field("x"), "ad"),
		LastAs(Field("x"), "last"),
	}
	for _, column := range columns {
		if column.Expression.String() == "" || column.Alias == "" {
			t.Fatalf("invalid helper result: %#v", column)
		}
	}
}

func TestAggregationPlanAndValidationCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	plain := From(sales).NewQuery().SelectColumns(Column{Expression: Field("amount")})
	if plan, err := PlanAggregation(plain, QueryCapabilities{}); err != nil || plan.Strategy != AggregationNative {
		t.Fatalf("plain plan=%#v err=%v", plan, err)
	}
	if HasAggregation(nil) || HasAggregation(plain) {
		t.Fatal("plain query reported as aggregate")
	}
	if err := ValidateAggregation(nil); err != nil {
		t.Fatal(err)
	}

	count := Count()
	count.Alias = "n"
	grouped := From(sales).NewQuery().
		GroupBy(Field("category")).
		Having(NewComparison(Field("n"), GreaterOrEqual, NewConstant(1))).
		OrderBy(Descending(Field("n"))).
		SelectColumns(Column{Expression: Field("category")}, count)
	fullCaps := QueryCapabilities{
		GroupBy: true, Having: true, OrderBy: true,
		Aggregate: AggregateCapabilities{Count: true},
	}
	if plan, err := PlanAggregation(grouped, fullCaps); err != nil || plan.Strategy != AggregationNative {
		t.Fatalf("native plan=%#v err=%v", plan, err)
	}
	if plan, err := PlanAggregation(grouped, QueryCapabilities{OrderBy: true, GroupKeyOrder: true}); err != nil || plan.Strategy != AggregationStreaming {
		t.Fatalf("stream plan=%#v err=%v", plan, err)
	}
	if plan, err := PlanAggregation(grouped, QueryCapabilities{}); err != nil || plan.Strategy != AggregationHash {
		t.Fatalf("hash plan=%#v err=%v", plan, err)
	}

	implicit := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}}
	effective := EffectiveAggregationColumns(implicit)
	if len(effective) != 1 || effective[0].Expression.String() != "category" {
		t.Fatalf("effective columns = %#v", effective)
	}
	if got := EffectiveAggregationColumns(grouped); len(got) != 2 {
		t.Fatalf("explicit columns = %#v", got)
	}
	if got := EffectiveAggregationColumns(From(sales).NewQuery().SelectColumns()); got != nil {
		t.Fatalf("empty columns = %#v", got)
	}

	invalid := []struct {
		name string
		q    StructuredQuery
		want string
	}{
		{"nil group", structuredQuery{from: From(sales), groupBy: []Expression{nil}}, "GROUP BY expression #0 is nil"},
		{"aggregate group", structuredQuery{from: From(sales), groupBy: []Expression{Count().Expression}}, "contains an aggregate"},
		{"bad group scalar", structuredQuery{from: From(sales), groupBy: []Expression{aggregationCoverageExpression{"bad"}}}, "unsupported scalar"},
		{"wildcard select", structuredQuery{from: From(sales), groupBy: []Expression{Field("x")}, columns: []Column{{Wildcard: &WildcardProjection{}}}}, "cannot be a wildcard"},
		{"nil select", structuredQuery{from: From(sales), groupBy: []Expression{Field("x")}, columns: []Column{{}}}, "has no expression"},
		{"duplicate alias", structuredQuery{from: From(sales), columns: []Column{SumAs(Field("x"), "v"), CountAs(Field("x"), "v")}}, "duplicate SELECT alias"},
		{"nil having operand", structuredQuery{from: From(sales), columns: []Column{Count()}, having: NewComparison(nil, Equal, NewConstant(1))}, "expression is nil"},
		{"ungrouped having field", structuredQuery{from: From(sales), columns: []Column{Count()}, having: NewComparison(Field("x"), Equal, NewConstant(1))}, "neither an aggregate"},
		{"unsupported having", structuredQuery{from: From(sales), columns: []Column{Count()}, having: aggregationCoverageCondition{}}, "unsupported condition"},
		{"bad order", structuredQuery{from: From(sales), columns: []Column{Count()}, orderBy: []OrderExpression{Ascending(Field("x"))}}, "ORDER BY expression"},
		{"count distinct star", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(COUNT, true, Star())}}}, "COUNT(DISTINCT *)"},
		{"sum star", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(SUM, false, Star())}}}, "SUM(*)"},
		{"min distinct", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(MIN, true, Field("x"))}}}, "DISTINCT is not supported"},
		{"max star", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(MAX, false, Star())}}}, "MAX(*)"},
		{"unsupported aggregate", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate("MEDIAN", false, Field("x"))}}}, "unsupported aggregate"},
		{"wrong arity", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(SUM, false)}}}, "exactly one argument"},
		{"nested aggregate", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(SUM, false, NewAggregate(MAX, false, Field("x")))}}}, "nested aggregates"},
		{"bad aggregate arg", structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(SUM, false, aggregationCoverageExpression{"bad"})}}}, "unsupported scalar"},
		{"bad binary operator", structuredQuery{from: From(sales), columns: []Column{SumAs(Binary(Field("x"), ArithmeticOperator("%"), NewConstant(2)), "v")}}, "unsupported arithmetic"},
		{"bad binary side", structuredQuery{from: From(sales), columns: []Column{SumAs(Binary(Field("x"), Add, aggregationCoverageExpression{"bad"}), "v")}}, "unsupported scalar"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAggregation(tc.q)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	groupedHaving := structuredQuery{
		from:    From(sales),
		groupBy: []Expression{Field("category")},
		columns: []Column{{Expression: Field("category")}, CountAs(Field("x"), "n")},
		having: NewGroupCondition(And,
			NewComparison(Field("category"), Equal, NewConstant("a")),
			NewGroupCondition(Or, NewComparison(Field("n"), GreaterThen, NewConstant(0))),
		),
	}
	if err := ValidateAggregation(groupedHaving); err != nil {
		t.Fatal(err)
	}

	stable := From(sales).NewQuery().SelectColumns(FirstAs(Field("x"), "first"), LastAs(Field("x"), "last"))
	if _, err := PlanAggregation(stable, QueryCapabilities{}); err == nil {
		t.Fatal("FIRST/LAST without stable order was accepted")
	}
	stableCaps := QueryCapabilities{StableRowOrder: true, Aggregate: AggregateCapabilities{First: true, Last: true}}
	if plan, err := PlanAggregation(stable, stableCaps); err != nil || plan.Strategy != AggregationNative {
		t.Fatalf("stable plan=%#v err=%v", plan, err)
	}
	if _, err := PlanAggregation(invalid[0].q, QueryCapabilities{}); err == nil {
		t.Fatal("invalid query was planned")
	}
	orderAggregate := structuredQuery{from: From(sales), orderBy: []OrderExpression{Ascending(Count().Expression)}}
	if !HasAggregation(orderAggregate) {
		t.Fatal("aggregate ORDER BY did not enter aggregate semantics")
	}

	allAggregates := structuredQuery{from: From(sales), columns: []Column{
		Count(), CountDistinctAs(Field("x"), "cd"), SumAs(Field("x"), "s"), SumDistinctAs(Field("x"), "sd"),
		AverageAs(Field("x"), "a"), AverageDistinctAs(Field("x"), "ad"), MinAs(Field("x"), "min"),
		MaxAs(Field("x"), "max"), FirstAs(Field("x"), "first"), LastAs(Field("x"), "last"),
	}}
	allCaps := QueryCapabilities{StableRowOrder: true, Aggregate: AggregateCapabilities{
		Count: true, CountDistinct: true, Sum: true, SumDistinct: true, Avg: true, AvgDistinct: true,
		Min: true, Max: true, First: true, Last: true,
	}}
	if !nativeAggregationSupported(allAggregates, allCaps) {
		t.Fatal("all aggregate capabilities were not accepted")
	}
	for _, q := range []StructuredQuery{
		structuredQuery{from: From(sales), groupBy: []Expression{Field("x")}, columns: []Column{Count()}},
		structuredQuery{from: From(sales), columns: []Column{Count()}, having: NewComparison(Count().Expression, Equal, NewConstant(1))},
		structuredQuery{from: From(sales), columns: []Column{Count()}, orderBy: []OrderExpression{Ascending(Count().Expression)}},
	} {
		if nativeAggregationSupported(q, QueryCapabilities{}) {
			t.Fatal("missing stage capability was accepted")
		}
	}
	unsupportedNative := structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate("OTHER", false, Field("x"))}}}
	if nativeAggregationSupported(unsupportedNative, allCaps) {
		t.Fatal("unsupported native aggregate was accepted")
	}

	if err := validateAggregateExpression(Binary(NewAggregate(SUM, false, Field("x")), Add, NewAggregate(MAX, false, Field("x")))); err != nil {
		t.Fatal(err)
	}
	if err := validateScalarExpression(NewParam("value")); err != nil {
		t.Fatal(err)
	}
	if err := validateScalarExpression(Binary(aggregationCoverageExpression{"bad"}, Add, Field("x"))); err == nil {
		t.Fatal("invalid left scalar was accepted")
	}
	badNestedHaving := NewGroupCondition(And, NewComparison(Field("x"), Equal, NewConstant(1)))
	if err := validateAggregateCondition(badNestedHaving, map[string]bool{}, map[string]Expression{}, "HAVING"); err == nil {
		t.Fatal("bad nested HAVING was accepted")
	}
	if err := validateAggregateCondition(NewComparison(NewConstant(1), Equal, aggregationCoverageExpression{"bad"}), map[string]bool{}, map[string]Expression{}, "HAVING"); err == nil {
		t.Fatal("bad right HAVING operand was accepted")
	}
	if err := validateGroupedExpression(Binary(aggregationCoverageExpression{"bad"}, Add, NewConstant(1)), map[string]bool{}, map[string]Expression{}); err == nil {
		t.Fatal("bad grouped binary left operand was accepted")
	}
	if err := validateAggregateExpression(Binary(NewAggregate("BAD", false, Field("x")), Add, Field("x"))); err == nil {
		t.Fatal("bad aggregate binary left operand was accepted")
	}
	if err := validateAggregateExpression(Field("x")); err != nil {
		t.Fatal(err)
	}
	walkQuery := structuredQuery{
		from:    From(sales),
		groupBy: []Expression{Binary(NewAggregate(SUM, false, Field("x")), Add, Field("y"))},
		having:  NewGroupCondition(And, NewComparison(NewAggregate(MAX, false, Field("z")), GreaterThen, NewConstant(0))),
	}
	visited := 0
	walkAggregates(walkQuery, func(AggregateFunc) { visited++ })
	if visited != 2 {
		t.Fatalf("walked aggregates = %d", visited)
	}
}

func TestValidatedDBAggregationRoutingCoverage(t *testing.T) {
	ctx := context.Background()
	sales := NewRootCollectionRef("sales", "")
	plain := NewTextQuery("select 1", nil)
	plainReader := &aggregationCoverageReader{}
	backend := &aggregationCoverageBackend{records: plainReader}
	db := validatedDB{Backend: backend}
	if got, err := db.ExecuteQueryToRecordsReader(ctx, plain); err != nil || got != plainReader {
		t.Fatalf("plain reader=%T err=%v", got, err)
	}

	countQuery := From(sales).NewQuery().SelectColumns(Count())
	nativeReader := &aggregationCoverageReader{}
	backend.records = nativeReader
	backend.capabilities = QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}
	if got, err := db.ExecuteQueryToRecordsReader(ctx, countQuery); err != nil || got != nativeReader {
		t.Fatalf("native reader=%T err=%v", got, err)
	}

	firstQuery := From(sales).NewQuery().SelectColumns(FirstAs(Field("x"), "first"))
	backend.capabilities = QueryCapabilities{}
	if _, err := db.ExecuteQueryToRecordsReader(ctx, firstQuery); err == nil || !strings.Contains(err.Error(), "stable input order") {
		t.Fatalf("plan error = %v", err)
	}

	boom := errors.New("backend failed")
	backend.recordsErr = boom
	if _, err := db.ExecuteQueryToRecordsReader(ctx, countQuery); !errors.Is(err, boom) {
		t.Fatalf("backend error = %v", err)
	}
	backend.recordsErr = nil
	backend.records = &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"x": 1})}}
	reader, err := db.ExecuteQueryToRecordsReader(ctx, countQuery)
	if err != nil {
		t.Fatal(err)
	}
	if rows := aggregationCoverageRows(t, reader); len(rows) != 1 || rows[0]["COUNT(*)"] != float64(1) {
		t.Fatalf("local rows = %#v", rows)
	}
	if source, ok := backend.seen.(StructuredQuery); !ok || HasAggregation(source) || source.Limit() != 0 {
		t.Fatalf("backend saw %#v", backend.seen)
	}
	if got := queryCapabilitiesOf(struct{}{}); got != (QueryCapabilities{}) {
		t.Fatalf("default capabilities = %#v", got)
	}
}

func TestValidatedDBAggregationRecordsetCoverage(t *testing.T) {
	ctx := context.Background()
	sales := NewRootCollectionRef("sales", "")
	backend := &aggregationCoverageBackend{}
	db := validatedDB{Backend: backend}

	direct := &aggregationRecordsetReader{recordset: recordset.NewColumnarRecordset("direct")}
	backend.recordset = direct
	if got, err := db.ExecuteQueryToRecordsetReader(ctx, NewTextQuery("select 1", nil)); err != nil || got != direct {
		t.Fatalf("plain recordset reader=%T err=%v", got, err)
	}

	q := From(sales).NewQuery().SelectColumns(Count())
	backend.capabilities = QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}
	if got, err := db.ExecuteQueryToRecordsetReader(ctx, q); err != nil || got != direct {
		t.Fatalf("native recordset reader=%T err=%v", got, err)
	}
	backend.capabilities = QueryCapabilities{}
	backend.records = &aggregationCoverageReader{}
	reader, err := db.ExecuteQueryToRecordsetReader(ctx, q, recordset.WithName("summary"))
	if err != nil {
		t.Fatal(err)
	}
	if reader.Recordset().Name() != "summary" {
		t.Fatalf("recordset name = %q", reader.Recordset().Name())
	}
	if cursor, err := reader.Cursor(); err != nil || cursor != "" {
		t.Fatalf("cursor=%q err=%v", cursor, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	row, rs, err := reader.Next()
	if err != nil || row == nil || rs != reader.Recordset() {
		t.Fatalf("row=%v rs=%v err=%v", row, rs, err)
	}
	if _, _, err := reader.Next(); err != ErrNoMoreRecords {
		t.Fatalf("end error = %v", err)
	}

	backend.recordsErr = errors.New("records failed")
	if _, err := db.ExecuteQueryToRecordsetReader(ctx, q); err == nil {
		t.Fatal("records error was swallowed")
	}
	backend.recordsErr = nil
	backend.records = &aggregationCoverageReader{err: errors.New("read failed")}
	if _, err := db.ExecuteQueryToRecordsetReader(ctx, q); err == nil {
		t.Fatal("read error was swallowed")
	}

	first := From(sales).NewQuery().SelectColumns(FirstAs(Field("x"), "first"))
	if _, err := db.ExecuteQueryToRecordsetReader(ctx, first); err == nil {
		t.Fatal("recordset plan error was swallowed")
	}
	backend.capabilities = QueryCapabilities{StableRowOrder: true, Aggregate: AggregateCapabilities{First: true}}
	backend.recordsetErr = errors.New("native recordset failed")
	if _, err := db.ExecuteQueryToRecordsetReader(ctx, first); err == nil {
		t.Fatal("native recordset error was swallowed")
	}

	txBackend := &aggregationCoverageBackend{recordset: direct, capabilities: QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}}
	tx := aggregationCoverageTx{backend: txBackend}
	readTx := &validatedReadTx{ReadTransaction: tx, capabilities: txBackend.capabilities}
	if got, err := readTx.ExecuteQueryToRecordsetReader(ctx, q); err != nil || got != direct {
		t.Fatalf("read tx recordset=%T err=%v", got, err)
	}
	writeTx := &validatedTx{ReadTransaction: tx, capabilities: txBackend.capabilities}
	if got, err := writeTx.ExecuteQueryToRecordsetReader(ctx, q); err != nil || got != direct {
		t.Fatalf("write tx recordset=%T err=%v", got, err)
	}
}

func TestValidatedTransactionAggregationCoverage(t *testing.T) {
	ctx := context.Background()
	sales := NewRootCollectionRef("sales", "")
	countQuery := From(sales).NewQuery().SelectColumns(Count())
	plainQuery := NewTextQuery("select 1", nil)
	nativeReader := &aggregationCoverageReader{}
	capabilities := QueryCapabilities{Aggregate: AggregateCapabilities{Count: true}}
	backend := &aggregationCoverageBackend{records: nativeReader, capabilities: capabilities}
	tx := aggregationCoverageTx{backend: backend}
	backend.readonlyTx = tx
	db := validatedDB{Backend: backend}

	called := false
	if err := db.RunReadonlyTransaction(ctx, func(_ context.Context, got ReadTransaction) error {
		called = true
		wrapped, ok := got.(*validatedReadTx)
		if !ok || wrapped.capabilities != capabilities {
			t.Fatalf("readonly tx = %#v", got)
		}
		return nil
	}); err != nil || !called {
		t.Fatalf("readonly transaction called=%v err=%v", called, err)
	}

	readTx := &validatedReadTx{ReadTransaction: tx, capabilities: capabilities}
	if got, err := readTx.ExecuteQueryToRecordsReader(ctx, countQuery); err != nil || got != nativeReader {
		t.Fatalf("read tx records=%T err=%v", got, err)
	}
	if got, err := readTx.Select(ctx, countQuery); err != nil || got != nativeReader {
		t.Fatalf("read tx aggregate select=%T err=%v", got, err)
	}
	backend.records = nativeReader
	if got, err := readTx.Select(ctx, plainQuery); err != nil || got != nativeReader {
		t.Fatalf("read tx fallback select=%T err=%v", got, err)
	}
	selected := &aggregationCoverageReader{}
	selecting := aggregationCoverageSelectingTx{aggregationCoverageTx: tx, selected: selected}
	readTx.ReadTransaction = selecting
	if got, err := readTx.Select(ctx, plainQuery); err != nil || got != selected {
		t.Fatalf("read tx optional select=%T err=%v", got, err)
	}

	writeTx := &validatedTx{ReadTransaction: tx, capabilities: capabilities}
	if got, err := writeTx.ExecuteQueryToRecordsReader(ctx, countQuery); err != nil || got != nativeReader {
		t.Fatalf("write tx records=%T err=%v", got, err)
	}
	if got, err := writeTx.Select(ctx, countQuery); err != nil || got != nativeReader {
		t.Fatalf("write tx aggregate select=%T err=%v", got, err)
	}
	if got, err := writeTx.Select(ctx, plainQuery); err != nil || got != nativeReader {
		t.Fatalf("write tx fallback select=%T err=%v", got, err)
	}
	writeTx.ReadTransaction = selecting
	if got, err := writeTx.Select(ctx, plainQuery); err != nil || got != selected {
		t.Fatalf("write tx optional select=%T err=%v", got, err)
	}
}

func TestLocalHashAggregationCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	count := Count()
	count.Alias = "rows"
	q := From(sales).NewQuery().
		GroupBy(Field("category")).
		Having(NewGroupCondition(And,
			NewComparison(Field("rows"), GreaterOrEqual, NewConstant(1)),
			NewComparison(Field("rows"), LessOrEqual, NewConstant(3)),
			NewComparison(Field("category"), Equal, NewConstant("b")),
		)).
		OrderBy(Descending(Field("total")), Ascending(Field("category"))).
		Offset(0).Limit(5).
		SelectColumns(
			Column{Expression: Field("category")},
			count,
			CountAs(Field("amount"), "non_null"),
			CountDistinctAs(Field("amount"), "distinct_count"),
			SumAs(Field("amount"), "total"),
			SumDistinctAs(Field("amount"), "distinct_total"),
			AverageAs(Field("amount"), "average"),
			AverageDistinctAs(Field("amount"), "distinct_average"),
			MinAs(Field("amount"), "minimum"),
			MaxAs(Field("amount"), "maximum"),
			SumAs(Binary(Field("quantity"), Multiply, Field("price")), "revenue"),
		)
	raw := &aggregationCoverageReader{records: []record.Record{
		aggregationCoverageRecord("1", map[string]any{"category": "a", "amount": 1, "quantity": 2, "price": 3}),
		aggregationCoverageRecord("2", map[string]any{"category": "b", "amount": 2, "quantity": 3, "price": 4}),
		aggregationCoverageRecord("3", map[string]any{"category": "b", "amount": 2, "quantity": 1, "price": 5}),
		aggregationCoverageRecord("4", map[string]any{"category": "b", "amount": nil, "quantity": 1, "price": 1}),
	}}
	reader := newLocalAggregationReader(context.Background(), q, raw, AggregationPlan{Strategy: AggregationHash})
	if cursor, err := reader.Cursor(); err != nil || cursor != "" {
		t.Fatalf("cursor=%q err=%v", cursor, err)
	}
	rows := aggregationCoverageRows(t, reader)
	if len(rows) != 1 || rows[0]["category"] != "b" || rows[0]["rows"] != float64(3) {
		t.Fatalf("rows = %#v", rows)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStreamingAggregationCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	count := Count()
	count.Alias = "rows"
	q := From(sales).NewQuery().
		GroupBy(Field("category")).
		Having(NewComparison(Field("rows"), GreaterThen, NewConstant(1))).
		Offset(1).Limit(1).
		SelectColumns(Column{Expression: Field("category")}, count)
	raw := &aggregationCoverageReader{records: []record.Record{
		aggregationCoverageRecord("1", map[string]any{"category": "a"}),
		aggregationCoverageRecord("2", map[string]any{"category": "b"}),
		aggregationCoverageRecord("3", map[string]any{"category": "b"}),
		aggregationCoverageRecord("4", map[string]any{"category": "c"}),
		aggregationCoverageRecord("5", map[string]any{"category": "c"}),
	}}
	reader := newLocalAggregationReader(context.Background(), q, raw, AggregationPlan{Strategy: AggregationStreaming})
	rows := aggregationCoverageRows(t, reader)
	if len(rows) != 1 || rows[0]["category"] != "c" {
		t.Fatalf("rows = %#v", rows)
	}
	if !raw.closed {
		t.Fatal("raw reader was not closed")
	}
}

func TestAggregationHelpersAndErrorsCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	q := From(sales).NewQuery().
		GroupBy(Field("nested.value"), Binary(Field("x"), Add, NewConstant(1))).
		OrderBy(Ascending(Field("sum"))).
		SelectColumns(SumAs(Field("x"), "sum"))
	source := newAggregationSourceQuery(q, true)
	if len(source.Columns()) != 3 || len(source.OrderBy()) != 2 || source.GroupBy() != nil || source.Having() != nil || source.Offset() != 0 || source.Limit() != 0 || source.StartFrom() != "" || source.StartAfter() != "" {
		t.Fatalf("source query = %#v", source)
	}
	conditionQuery := structuredQuery{
		from: From(sales),
		having: NewGroupCondition(And,
			NewComparison(Field("left"), Equal, Field("right")),
		),
	}
	if fields := collectSourceFields(conditionQuery); len(fields) != 2 {
		t.Fatalf("condition source fields = %#v", fields)
	}

	if value, ok := lookupAggregationField(map[string]any{"nested": map[string]any{"value": 7}}, "nested.value"); !ok || value != 7 {
		t.Fatalf("nested lookup = %v, %v", value, ok)
	}
	if _, ok := lookupAggregationField(map[string]any{"nested": 1}, "nested.value"); ok {
		t.Fatal("non-object nested lookup succeeded")
	}
	if _, ok := lookupAggregationField(map[string]any{}, "missing"); ok {
		t.Fatal("missing lookup succeeded")
	}

	values := []any{nil, false, true, float64(0), float64(1.5), "x", []byte{1, 2}}
	for _, value := range values {
		if _, err := encodeTypedValue(value); err != nil {
			t.Fatalf("encode %T: %v", value, err)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), struct{}{}} {
		if _, err := encodeTypedValue(value); err == nil {
			t.Fatalf("encode %T unexpectedly succeeded", value)
		}
	}

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	if got := normalizeAggregationValue(now); got != "2026-09-20T11:00:00Z" {
		t.Fatalf("normalized time = %v", got)
	}
	for _, value := range []any{int(1), int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1), float64(1)} {
		if number, ok := aggregationNumber(value); !ok || number != 1 {
			t.Fatalf("number %T = %v, %v", value, number, ok)
		}
	}
	if _, ok := aggregationNumber("1"); ok {
		t.Fatal("string recognized as number")
	}

	comparisons := []struct{ a, b any }{
		{nil, nil}, {nil, 1}, {1, nil}, {1, 2}, {2, 1}, {1, 1},
		{"a", "b"}, {false, true}, {true, false}, {true, true}, {[]int{1}, []int{2}},
	}
	for _, comparison := range comparisons {
		_ = compareAggregationValues(comparison.a, comparison.b)
	}
	if !valuesEqual(1, float64(1)) || valuesEqual(struct{}{}, struct{}{}) {
		t.Fatal("valuesEqual normalization/error behavior differs")
	}

	row := map[string]any{"x": 8.0}
	for _, tc := range []struct {
		expr Expression
		want any
	}{
		{Field("x"), 8.0},
		{NewConstant(int32(2)), 2.0},
		{Binary(Field("x"), Add, NewConstant(2)), 10.0},
		{Binary(Field("x"), Subtract, NewConstant(2)), 6.0},
		{Binary(Field("x"), Multiply, NewConstant(2)), 16.0},
		{Binary(Field("x"), Divide, NewConstant(2)), 4.0},
		{Binary(NewConstant("x"), Add, NewConstant(1)), nil},
		{Binary(NewConstant(1), Divide, NewConstant(0)), nil},
	} {
		got, err := evalScalar(tc.expr, row)
		if err != nil || got != tc.want {
			t.Fatalf("eval %s = %v, %v", tc.expr, got, err)
		}
	}
	for _, expr := range []Expression{
		Binary(Field("missing"), Add, NewConstant(1)),
		Binary(NewConstant(1), ArithmeticOperator("%"), NewConstant(1)),
		aggregationCoverageExpression{"bad"},
	} {
		if _, err := evalScalar(expr, row); expr.String() != "(missing + 1)" && err == nil {
			t.Fatalf("eval %s unexpectedly succeeded", expr)
		}
	}
	for _, expr := range []Expression{
		Binary(aggregationCoverageExpression{"bad"}, Add, NewConstant(1)),
		Binary(NewConstant(1), Add, aggregationCoverageExpression{"bad"}),
	} {
		if _, err := evalScalar(expr, row); err == nil {
			t.Fatalf("nested eval %s unexpectedly succeeded", expr)
		}
	}

	if got := aggregationColumnName(Column{Expression: Field("x"), Alias: "alias"}); got != "alias" {
		t.Fatal(got)
	}
	if got := aggregationColumnName(Column{Expression: Field("x")}); got != "x" {
		t.Fatal(got)
	}
	if got := aggregationColumnName(Column{Expression: Binary(Field("x"), Add, NewConstant(1))}); got == "" {
		t.Fatal("empty derived column name")
	}

	for _, data := range []any{nil, func() {}, []int{1}, map[string]any(nil)} {
		rec := aggregationCoverageRecord("bad", data)
		got, err := normalizedRecordMap(rec)
		switch data.(type) {
		case func(), []int:
			if err == nil {
				t.Fatalf("normalizedRecordMap(%T) unexpectedly succeeded", data)
			}
		default:
			if err != nil || got == nil {
				t.Fatalf("normalizedRecordMap(%T) = %#v, %v", data, got, err)
			}
		}
	}
}

func TestAggregationRetainedByteLimitPropagation(t *testing.T) {
	base := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")}, CountDistinctAs(Field("value"), "values"))
	recordA := aggregationCoverageRecord("1", map[string]any{"category": "a", "value": "x"})
	recordB := aggregationCoverageRecord("2", map[string]any{"category": "b", "value": "y"})

	initial := newLocalAggregationReader(context.Background(), base, &aggregationCoverageReader{records: []record.Record{recordA}}, AggregationPlan{Strategy: AggregationStreaming})
	initial.retainedBytes = defaultMaxAggregationBytes
	if _, err := initial.Next(); err == nil {
		t.Fatal("expected initial streaming group byte-limit error")
	}

	transition := newLocalAggregationReader(context.Background(), base, &aggregationCoverageReader{records: []record.Record{recordB}}, AggregationPlan{Strategy: AggregationStreaming})
	transition.current = &localGroup{key: "s:1:a"}
	transition.retainedBytes = defaultMaxAggregationBytes
	if _, err := transition.Next(); err == nil {
		t.Fatal("expected streaming transition byte-limit error")
	}

	hash := newLocalAggregationReader(context.Background(), base, &aggregationCoverageReader{records: []record.Record{recordA}}, AggregationPlan{Strategy: AggregationHash})
	hash.retainedBytes = defaultMaxAggregationBytes
	if _, err := hash.Next(); err == nil {
		t.Fatal("expected hash group byte-limit error")
	}

	emptyQuery := From(NewRootCollectionRef("sales", "")).NewQuery().SelectColumns(Count())
	empty := newLocalAggregationReader(context.Background(), emptyQuery, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	empty.retainedBytes = defaultMaxAggregationBytes
	if _, err := empty.Next(); err == nil {
		t.Fatal("expected implicit group byte-limit error")
	}

	distinct := newLocalAggregationReader(context.Background(), base, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	group, err := distinct.newGroup("s:1:a", map[string]any{"category": "a"})
	if err != nil {
		t.Fatal(err)
	}
	distinct.retainedBytes = defaultMaxAggregationBytes
	if err := distinct.updateGroup(group, map[string]any{"category": "a", "value": "x"}); err == nil {
		t.Fatal("expected distinct-key byte-limit error")
	}
}

func TestAggregationExecutionErrorBranchesCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	count := Count()
	count.Alias = "n"
	base := structuredQuery{from: From(sales), columns: []Column{count}}

	emptyStream := newLocalAggregationReader(context.Background(), base, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationStreaming})
	if _, err := emptyStream.nextStreaming(); err != ErrNoMoreRecords {
		t.Fatalf("empty stream error = %v", err)
	}
	if _, err := emptyStream.nextStreaming(); err != ErrNoMoreRecords {
		t.Fatalf("finished stream error = %v", err)
	}

	streamBoom := errors.New("stream boom")
	errRaw := &aggregationCoverageReader{err: streamBoom}
	errStream := newLocalAggregationReader(context.Background(), base, errRaw, AggregationPlan{Strategy: AggregationStreaming})
	if _, err := errStream.nextStreaming(); !errors.Is(err, streamBoom) || !errRaw.closed {
		t.Fatalf("stream error=%v closed=%v", err, errRaw.closed)
	}

	for name, rec := range map[string]record.Record{
		"normalize": aggregationCoverageRecord("bad", func() {}),
		"group key": aggregationCoverageRecord("bad", map[string]any{"key": map[string]any{"x": 1}}),
	} {
		t.Run(name, func(t *testing.T) {
			query := base
			if name == "group key" {
				query.groupBy = []Expression{Field("key")}
				query.columns = []Column{{Expression: Field("key")}, count}
			}
			raw := &aggregationCoverageReader{records: []record.Record{rec}}
			r := newLocalAggregationReader(context.Background(), query, raw, AggregationPlan{Strategy: AggregationStreaming})
			if _, err := r.nextStreaming(); err == nil {
				t.Fatal("expected streaming row error")
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelReader := newLocalAggregationReader(cancelled, base, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	if err := cancelReader.loadMaterialized(); !errors.Is(err, context.Canceled) {
		t.Fatalf("materialized cancellation = %v", err)
	}

	for name, rec := range map[string]record.Record{
		"normalize": aggregationCoverageRecord("bad", []int{1}),
		"group key": aggregationCoverageRecord("bad", map[string]any{"key": map[string]any{"x": 1}}),
	} {
		t.Run("materialized "+name, func(t *testing.T) {
			query := base
			if name == "group key" {
				query.groupBy = []Expression{Field("key")}
				query.columns = []Column{{Expression: Field("key")}, count}
			}
			r := newLocalAggregationReader(context.Background(), query, &aggregationCoverageReader{records: []record.Record{rec}}, AggregationPlan{Strategy: AggregationHash})
			if err := r.loadMaterialized(); err == nil {
				t.Fatal("expected materialized row error")
			}
		})
	}

	badArgQuery := structuredQuery{from: From(sales), columns: []Column{{Expression: NewAggregate(SUM, false, aggregationCoverageExpression{"bad"})}}}
	badArgReader := newLocalAggregationReader(context.Background(), badArgQuery, &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{})}}, AggregationPlan{Strategy: AggregationHash})
	if err := badArgReader.loadMaterialized(); err == nil {
		t.Fatal("aggregate argument error was swallowed")
	}

	badSelect := structuredQuery{from: From(sales), columns: []Column{{Expression: aggregationCoverageExpression{"bad"}}}}
	badFinalize := newLocalAggregationReader(context.Background(), badSelect, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	if err := badFinalize.loadMaterialized(); err == nil {
		t.Fatal("finalize error was swallowed")
	}

	badHaving := structuredQuery{from: From(sales), columns: []Column{count}, having: aggregationCoverageCondition{}}
	badHavingReader := newLocalAggregationReader(context.Background(), badHaving, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	if err := badHavingReader.loadMaterialized(); err == nil {
		t.Fatal("HAVING error was swallowed")
	}

	badOrder := structuredQuery{from: From(sales), columns: []Column{count}, orderBy: []OrderExpression{Ascending(Field("missing"))}}
	badOrderReader := newLocalAggregationReader(context.Background(), badOrder, &aggregationCoverageReader{}, AggregationPlan{Strategy: AggregationHash})
	if err := badOrderReader.loadMaterialized(); err == nil {
		t.Fatal("ORDER BY error was swallowed")
	}

	sortQuery := structuredQuery{
		from:    From(sales),
		groupBy: []Expression{Field("category")},
		columns: []Column{{Expression: Field("category")}, count},
		orderBy: []OrderExpression{Descending(Field("n")), Ascending(Field("category"))},
		offset:  10,
		limit:   1,
	}
	sortRaw := &aggregationCoverageReader{records: []record.Record{
		aggregationCoverageRecord("1", map[string]any{"category": "a"}),
		aggregationCoverageRecord("2", map[string]any{"category": "b"}),
		aggregationCoverageRecord("3", map[string]any{"category": "b"}),
		aggregationCoverageRecord("4", map[string]any{"category": "c"}),
		aggregationCoverageRecord("5", map[string]any{"category": "c"}),
	}}
	sortReader := newLocalAggregationReader(context.Background(), sortQuery, sortRaw, AggregationPlan{Strategy: AggregationHash})
	if err := sortReader.loadMaterialized(); err != nil || len(sortReader.rows) != 0 {
		t.Fatalf("sorted offset result=%d err=%v", len(sortReader.rows), err)
	}
	sortQuery.offset = 0
	sortReader = newLocalAggregationReader(context.Background(), sortQuery, &aggregationCoverageReader{records: sortRaw.records}, AggregationPlan{Strategy: AggregationHash})
	if err := sortReader.loadMaterialized(); err != nil || len(sortReader.rows) != 1 {
		t.Fatalf("sorted limited result=%d err=%v", len(sortReader.rows), err)
	}

	tieQuery := sortQuery
	tieQuery.orderBy = []OrderExpression{Descending(Field("n"))}
	tieQuery.limit = 0
	tieRaw := &aggregationCoverageReader{records: []record.Record{
		aggregationCoverageRecord("1", map[string]any{"category": "a"}),
		aggregationCoverageRecord("2", map[string]any{"category": "b"}),
	}}
	tieReader := newLocalAggregationReader(context.Background(), tieQuery, tieRaw, AggregationPlan{Strategy: AggregationHash})
	if err := tieReader.loadMaterialized(); err != nil || len(tieReader.rows) != 2 {
		t.Fatalf("tie sort result=%d err=%v", len(tieReader.rows), err)
	}

	groupLimitQuery := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}, columns: []Column{{Expression: Field("category")}}}
	groupLimitReader := newLocalAggregationReader(context.Background(), groupLimitQuery, &aggregationCoverageSequenceReader{total: defaultMaxAggregationGroups + 1}, AggregationPlan{Strategy: AggregationHash})
	if err := groupLimitReader.loadMaterialized(); err == nil || !strings.Contains(err.Error(), "group limit") {
		t.Fatalf("group limit error = %v", err)
	}

	stateLimitReader := newLocalAggregationReader(context.Background(), groupLimitQuery, &aggregationCoverageSequenceReader{total: defaultMaxAggregateStates/11 + 1}, AggregationPlan{Strategy: AggregationHash})
	stateLimitReader.aggregates = make([]AggregateFunc, 11)
	for i := range stateLimitReader.aggregates {
		stateLimitReader.aggregates[i] = NewAggregate(COUNT, false, Star())
	}
	if err := stateLimitReader.loadMaterialized(); err == nil || (!strings.Contains(err.Error(), "aggregate-state limit") && !strings.Contains(err.Error(), "byte limit")) {
		t.Fatalf("aggregate-state limit error = %v", err)
	}
}

func TestStreamingTransitionBranchesCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	count := Count()
	count.Alias = "n"
	base := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}, columns: []Column{{Expression: Field("category")}, count}}

	badFinalizeQuery := base
	badFinalizeQuery.having = aggregationCoverageCondition{}
	badFinalize := &localAggregationReader{
		ctx:        context.Background(),
		query:      badFinalizeQuery,
		raw:        &aggregationCoverageReader{},
		plan:       AggregationPlan{Strategy: AggregationStreaming},
		aggregates: []AggregateFunc{count.Expression.(AggregateFunc)},
		current:    &localGroup{key: "s:1:a", keyValues: map[string]any{"category": "a"}, states: map[string]*aggregateState{count.Expression.String(): {expression: count.Expression.(AggregateFunc), count: 1}}},
	}
	if _, err := badFinalize.nextStreaming(); err == nil {
		t.Fatal("EOF finalization error was swallowed")
	}

	filteredQuery := base
	filteredQuery.having = NewComparison(Field("n"), GreaterThen, NewConstant(2))
	filtered := &localAggregationReader{
		ctx:        context.Background(),
		query:      filteredQuery,
		raw:        &aggregationCoverageReader{},
		plan:       AggregationPlan{Strategy: AggregationStreaming},
		aggregates: []AggregateFunc{count.Expression.(AggregateFunc)},
		current:    &localGroup{key: "s:1:a", keyValues: map[string]any{"category": "a"}, states: map[string]*aggregateState{count.Expression.String(): {expression: count.Expression.(AggregateFunc), count: 1}}},
	}
	if _, err := filtered.nextStreaming(); err != ErrNoMoreRecords {
		t.Fatalf("filtered EOF error = %v", err)
	}

	badAggregate := NewAggregate(SUM, false, aggregationCoverageExpression{"bad"})
	for name, current := range map[string]*localGroup{
		"first": nil,
		"same":  {key: "s:1:a", keyValues: map[string]any{"category": "a"}, states: map[string]*aggregateState{badAggregate.String(): {expression: badAggregate}}},
		"next":  {key: "s:1:b", keyValues: map[string]any{"category": "b"}, states: map[string]*aggregateState{badAggregate.String(): {expression: badAggregate}}},
	} {
		t.Run(name+" update error", func(t *testing.T) {
			query := structuredQuery{from: From(sales), groupBy: []Expression{Field("category")}, columns: []Column{{Expression: badAggregate}}}
			r := &localAggregationReader{
				ctx: context.Background(), query: query,
				raw:        &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("1", map[string]any{"category": "a"})}},
				plan:       AggregationPlan{Strategy: AggregationStreaming},
				aggregates: []AggregateFunc{badAggregate}, current: current,
			}
			if _, err := r.nextStreaming(); err == nil {
				t.Fatal("update error was swallowed")
			}
		})
	}

	transitionBadQuery := base
	transitionBadQuery.having = aggregationCoverageCondition{}
	transitionBad := &localAggregationReader{
		ctx: context.Background(), query: transitionBadQuery,
		raw:        &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("2", map[string]any{"category": "b"})}},
		plan:       AggregationPlan{Strategy: AggregationStreaming},
		aggregates: []AggregateFunc{count.Expression.(AggregateFunc)},
		current:    &localGroup{key: "s:1:a", keyValues: map[string]any{"category": "a"}, states: map[string]*aggregateState{count.Expression.String(): {expression: count.Expression.(AggregateFunc), count: 1}}},
	}
	if _, err := transitionBad.nextStreaming(); err == nil {
		t.Fatal("transition finalization error was swallowed")
	}

	emit := &localAggregationReader{
		ctx: context.Background(), query: base,
		raw:        &aggregationCoverageReader{records: []record.Record{aggregationCoverageRecord("2", map[string]any{"category": "b"})}},
		plan:       AggregationPlan{Strategy: AggregationStreaming},
		aggregates: []AggregateFunc{count.Expression.(AggregateFunc)},
		current:    &localGroup{key: "s:1:a", keyValues: map[string]any{"category": "a"}, states: map[string]*aggregateState{count.Expression.String(): {expression: count.Expression.(AggregateFunc), count: 1}}},
	}
	if rec, err := emit.nextStreaming(); err != nil || rec == nil {
		t.Fatalf("transition emit rec=%v err=%v", rec, err)
	}
}

func TestAggregationStateAndResolutionCoverage(t *testing.T) {
	sales := NewRootCollectionRef("sales", "")
	query := structuredQuery{from: From(sales)}
	r := &localAggregationReader{query: query}

	if _, _, err := r.groupKey(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	r.query = structuredQuery{from: From(sales), groupBy: []Expression{aggregationCoverageExpression{"bad"}}}
	if _, _, err := r.groupKey(map[string]any{}); err == nil {
		t.Fatal("bad group expression was accepted")
	}
	r.query = structuredQuery{from: From(sales), groupBy: []Expression{Field("key")}}
	if _, _, err := r.groupKey(map[string]any{"key": struct{}{}}); err == nil {
		t.Fatal("bad group key was accepted")
	}

	makeState := func(aggregate AggregateFunc) (*localGroup, *aggregateState) {
		state := &aggregateState{expression: aggregate}
		group := &localGroup{states: map[string]*aggregateState{aggregate.String(): state}, keyValues: map[string]any{}}
		return group, state
	}

	group, state := makeState(NewAggregate(COUNT, false, Star()))
	state.count = math.MaxInt64
	r.aggregates = []AggregateFunc{state.expression}
	if err := r.updateGroup(group, map[string]any{}); err == nil {
		t.Fatal("COUNT overflow was accepted")
	}

	for name, aggregate := range map[string]AggregateFunc{
		"bad argument":     NewAggregate(SUM, false, aggregationCoverageExpression{"bad"}),
		"bad distinct key": NewAggregate(SUM, true, Constant{Value: struct{}{}}),
	} {
		t.Run(name, func(t *testing.T) {
			group, state := makeState(aggregate)
			if aggregateDistinct(aggregate) {
				state.distinct = map[string]struct{}{}
			}
			r.aggregates = []AggregateFunc{aggregate}
			if err := r.updateGroup(group, map[string]any{}); err == nil {
				t.Fatal("expected update error")
			}
		})
	}

	distinct := NewAggregate(SUM, true, Field("x"))
	group, state = makeState(distinct)
	state.distinct = make(map[string]struct{}, defaultMaxDistinctValues)
	for i := 0; i < defaultMaxDistinctValues; i++ {
		state.distinct["value-"+strconv.Itoa(i)] = struct{}{}
	}
	r.aggregates = []AggregateFunc{distinct}
	if err := r.updateGroup(group, map[string]any{"x": 1}); err == nil {
		t.Fatal("distinct limit was accepted")
	}
	group, state = makeState(distinct)
	state.distinct = map[string]struct{}{}
	r.aggregates = []AggregateFunc{distinct}
	r.totalDistinct = defaultMaxTotalDistinct
	if err := r.updateGroup(group, map[string]any{"x": 1}); err == nil {
		t.Fatal("total distinct limit was accepted")
	}
	r.totalDistinct = 0

	nonNumeric := NewAggregate(SUM, false, Field("x"))
	group, _ = makeState(nonNumeric)
	r.aggregates = []AggregateFunc{nonNumeric}
	if err := r.updateGroup(group, map[string]any{"x": "not numeric"}); err != nil {
		t.Fatal(err)
	}

	for name, setup := range map[string]func(*aggregateState) map[string]any{
		"non-finite": func(*aggregateState) map[string]any { return map[string]any{"x": math.NaN()} },
		"sum overflow": func(s *aggregateState) map[string]any {
			s.sum = math.MaxFloat64
			return map[string]any{"x": math.MaxFloat64}
		},
		"average count overflow": func(s *aggregateState) map[string]any { s.count = math.MaxInt64; return map[string]any{"x": 1} },
	} {
		t.Run(name, func(t *testing.T) {
			aggregate := NewAggregate(AVERAGE, false, Field("x"))
			group, state := makeState(aggregate)
			r.aggregates = []AggregateFunc{aggregate}
			if err := r.updateGroup(group, setup(state)); err == nil {
				t.Fatal("expected numeric update error")
			}
		})
	}

	for _, aggregate := range []AggregateFunc{
		NewAggregate(MIN, false, Field("x")), NewAggregate(MAX, false, Field("x")),
		NewAggregate(FIRST, false, Field("x")), NewAggregate(LAST, false, Field("x")),
	} {
		group, _ := makeState(aggregate)
		r.aggregates = []AggregateFunc{aggregate}
		if err := r.updateGroup(group, map[string]any{"x": nil}); err != nil {
			t.Fatal(err)
		}
		if err := r.updateGroup(group, map[string]any{"x": 2}); err != nil {
			t.Fatal(err)
		}
		if err := r.updateGroup(group, map[string]any{"x": 1}); err != nil {
			t.Fatal(err)
		}
	}

	aggregates := []AggregateFunc{
		NewAggregate(COUNT, false, Star()), NewAggregate(SUM, false, Field("x")),
		NewAggregate(AVERAGE, false, Field("x")), NewAggregate(MIN, false, Field("x")),
	}
	group = &localGroup{keyValues: map[string]any{"key": "value"}, states: map[string]*aggregateState{}}
	for _, aggregate := range aggregates {
		group.states[aggregate.String()] = &aggregateState{expression: aggregate}
		if _, err := r.resolveGroupExpression(aggregate, group, map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	group.states[aggregates[1].String()] = &aggregateState{expression: aggregates[1], count: 1, sum: 3}
	group.states[aggregates[2].String()] = &aggregateState{expression: aggregates[2], count: 2, sum: 3}
	group.states[aggregates[3].String()] = &aggregateState{expression: aggregates[3], hasValue: true, value: 1}
	for _, aggregate := range aggregates[1:] {
		if _, err := r.resolveGroupExpression(aggregate, group, map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	if value, err := r.resolveGroupExpression(Field("alias"), group, map[string]any{"alias": 7}); err != nil || value != 7 {
		t.Fatalf("alias=%v err=%v", value, err)
	}
	if value, err := r.resolveGroupExpression(aggregationCoverageExpression{"key"}, group, nil); err != nil || value != "value" {
		t.Fatalf("key=%v err=%v", value, err)
	}
	if value, err := r.resolveGroupExpression(NewConstant(2), group, nil); err != nil || value != float64(2) {
		t.Fatalf("constant=%v err=%v", value, err)
	}
	if _, err := r.resolveGroupExpression(NewAggregate(SUM, false, Field("missing")), group, nil); err == nil {
		t.Fatal("missing aggregate state was accepted")
	}
	if _, err := r.resolveGroupExpression(aggregationCoverageExpression{"missing"}, group, nil); err == nil {
		t.Fatal("missing group expression was accepted")
	}
	for _, expression := range []BinaryExpression{
		Binary(Field("alias"), Add, NewConstant(1)),
		Binary(aggregationCoverageExpression{"missing"}, Add, NewConstant(1)),
		Binary(NewConstant(1), Add, aggregationCoverageExpression{"missing"}),
	} {
		_, _ = r.resolveGroupExpression(expression, group, map[string]any{"alias": 2.0})
	}

	group.out = map[string]any{"left": 2.0, "nil": nil}
	conditions := []Condition{
		NewComparison(Field("left"), Equal, NewConstant(2)),
		NewComparison(Field("left"), GreaterThen, NewConstant(1)),
		NewComparison(Field("left"), GreaterOrEqual, NewConstant(2)),
		NewComparison(Field("left"), LessThen, NewConstant(3)),
		NewComparison(Field("left"), LessOrEqual, NewConstant(2)),
		NewGroupCondition(Or, NewComparison(Field("left"), Equal, NewConstant(3)), NewComparison(Field("left"), Equal, NewConstant(2))),
		NewGroupCondition(Or, NewComparison(Field("left"), Equal, NewConstant(3))),
		NewGroupCondition(And, NewComparison(Field("left"), Equal, NewConstant(2))),
		NewGroupCondition(And, NewComparison(Field("left"), Equal, NewConstant(3))),
	}
	for _, condition := range conditions {
		if _, err := r.evalHaving(condition, group); err != nil {
			t.Fatalf("HAVING %s: %v", condition, err)
		}
	}
	for _, condition := range []Condition{
		NewComparison(Field("missing"), Equal, NewConstant(1)),
		NewComparison(Field("left"), Equal, Field("missing")),
		NewComparison(Field("left"), Operator("bad"), NewConstant(1)),
		aggregationCoverageCondition{},
		NewGroupCondition(Or, aggregationCoverageCondition{}),
		NewGroupCondition(And, aggregationCoverageCondition{}),
	} {
		if _, err := r.evalHaving(condition, group); err == nil {
			t.Fatalf("HAVING %s unexpectedly succeeded", condition)
		}
	}
	uniqueQuery := structuredQuery{
		from:    From(sales),
		columns: []Column{{Expression: Binary(NewAggregate(SUM, false, Field("x")), Add, NewAggregate(MAX, false, Field("x")))}},
	}
	if got := uniqueAggregates(uniqueQuery); len(got) != 2 {
		t.Fatalf("unique binary aggregates = %#v", got)
	}
}
