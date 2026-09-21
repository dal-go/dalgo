package dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

func TestPlanAggregationStrategies(t *testing.T) {
	count := Count()
	count.Alias = "n"
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")}, count)
	nativeCaps := QueryCapabilities{GroupBy: true, Aggregate: AggregateCapabilities{Count: true}}
	plan, err := PlanAggregation(q, nativeCaps)
	if err != nil || plan.Strategy != AggregationNative {
		t.Fatalf("native plan=%#v err=%v", plan, err)
	}
	plan, err = PlanAggregation(q, QueryCapabilities{OrderBy: true, GroupKeyOrder: true})
	if err != nil || plan.Strategy != AggregationStreaming {
		t.Fatalf("stream plan=%#v err=%v", plan, err)
	}
	plan, err = PlanAggregation(q, QueryCapabilities{})
	if err != nil || plan.Strategy != AggregationHash {
		t.Fatalf("hash plan=%#v err=%v", plan, err)
	}
}

func TestPlanAggregationUsesHashForGenericOrderOnly(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")}, Count())
	plan, err := PlanAggregation(q, QueryCapabilities{OrderBy: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != AggregationHash {
		t.Fatalf("strategy = %s, want %s", plan.Strategy, AggregationHash)
	}
}

func TestPlanAggregationUsesHashForGroupKeyOrderWithoutOrderBy(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")}, Count())
	plan, err := PlanAggregation(q, QueryCapabilities{GroupKeyOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != AggregationHash {
		t.Fatalf("strategy = %s, want %s", plan.Strategy, AggregationHash)
	}
}

func TestPlanAggregationUsesHashForExpressionGroupKeys(t *testing.T) {
	group := Binary(Field("amount"), Divide, NewConstant(10))
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(group).
		SelectColumns(Column{Expression: group}, Count())
	plan, err := PlanAggregation(q, QueryCapabilities{OrderBy: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != AggregationHash {
		t.Fatalf("strategy = %s, want %s", plan.Strategy, AggregationHash)
	}
}

func TestValidateAggregationRejectsNonGroupedSelection(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("amount")})
	if err := ValidateAggregation(q); err == nil {
		t.Fatal("expected grouping validation error")
	}
}

func TestValidateAggregationRejectsNonGroupedFieldInsideAggregateArithmetic(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Binary(NewAggregate(SUM, false, Field("amount")), Add, Field("city"))})
	if err := ValidateAggregation(q); err == nil {
		t.Fatal("expected mixed aggregate/scalar grouping validation error")
	}
}

func TestPlanAggregationRejectsUnorderedFirst(t *testing.T) {
	q := From(NewRootCollectionRef("events", "")).NewQuery().
		SelectColumns(FirstAs(Field("status"), "first"))
	if _, err := PlanAggregation(q, QueryCapabilities{}); err == nil {
		t.Fatal("expected deterministic-order error")
	}
}

type aggregationErrorReader struct {
	records []record.Record
	index   int
	err     error
	closed  bool
}

func (r *aggregationErrorReader) Next() (record.Record, error) {
	if r.index < len(r.records) {
		value := r.records[r.index]
		r.index++
		return value, nil
	}
	return nil, r.err
}
func (r *aggregationErrorReader) Cursor() (string, error) { return "", nil }
func (r *aggregationErrorReader) Close() error            { r.closed = true; return nil }

type interleavingOrderExecutor struct {
	records []record.Record
	orders  []OrderExpression
}

func (e *interleavingOrderExecutor) ExecuteQueryToRecordsReader(_ context.Context, query Query) (RecordsReader, error) {
	e.orders = query.(StructuredQuery).OrderBy()
	return NewRecordsReader(e.records), nil
}

func (*interleavingOrderExecutor) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, ErrNotSupported
}

func TestGenericOrderOnlyFallsBackToHashForInterleavedTypedGroups(t *testing.T) {
	// An ORDER BY comparator that considers false and "false" equal can return
	// bool(false), string("false"), bool(false). The boolean group's DALgo key
	// is therefore not contiguous even though the provider claims generic order.
	executor := &interleavingOrderExecutor{records: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("sales", "1"), map[string]any{"category": false}),
		record.NewRecordWithData(record.NewKeyWithID("sales", "2"), map[string]any{"category": "false"}),
		record.NewRecordWithData(record.NewKeyWithID("sales", "3"), map[string]any{"category": false}),
	}}
	count := Count()
	count.Alias = "rows"
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")}, count)
	reader, err := executeAggregationRecords(context.Background(), executor, q, QueryCapabilities{OrderBy: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if len(executor.orders) != 0 {
		t.Fatalf("hash fallback unexpectedly requested raw ORDER BY: %#v", executor.orders)
	}
	rows := map[string]int64{}
	for {
		rec, err := reader.Next()
		if err == ErrNoMoreRecords {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data := rec.Data().(map[string]any)
		rows[encodeTestGroupValue(t, data["category"])] = data["rows"].(int64)
	}
	if rows["b:false"] != 2 || rows["s:5:false"] != 1 {
		t.Fatalf("groups = %#v, want bool false=2 and string false=1", rows)
	}
}

func encodeTestGroupValue(t *testing.T, value any) string {
	t.Helper()
	key, err := encodeTypedValue(value)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestLocalAggregationPropagatesMidstreamError(t *testing.T) {
	boom := errors.New("provider failed")
	raw := &aggregationErrorReader{
		records: []record.Record{record.NewRecordWithData(record.NewKeyWithID("sales", "1"), map[string]any{"amount": 1})},
		err:     boom,
	}
	q := From(NewRootCollectionRef("sales", "")).NewQuery().SelectColumns(SumAs(Field("amount"), "total"))
	reader := newLocalAggregationReader(context.Background(), q, raw, AggregationPlan{Strategy: AggregationHash})
	if _, err := reader.Next(); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestLocalAggregationHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw := &aggregationErrorReader{err: ErrNoMoreRecords}
	q := From(NewRootCollectionRef("sales", "")).NewQuery().SelectColumns(Count())
	reader := newLocalAggregationReader(ctx, q, raw, AggregationPlan{Strategy: AggregationHash})
	if _, err := reader.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if !raw.closed {
		t.Fatal("raw reader was not closed")
	}
}

func TestLocalAggregationRetainedKeyByteBudget(t *testing.T) {
	reader := &localAggregationReader{}
	if err := reader.reserveAggregationBytes(defaultMaxAggregationBytes); err != nil {
		t.Fatal(err)
	}
	if err := reader.reserveAggregationBytes(1); err == nil {
		t.Fatal("expected retained-key byte limit error")
	}
	if err := reader.reserveAggregationBytes(-1); err == nil {
		t.Fatal("expected negative reservation error")
	}
	group := &localGroup{bytes: defaultMaxAggregationBytes}
	reader.releaseGroup(group)
	if reader.retainedBytes != 0 || group.bytes != 0 {
		t.Fatalf("release left reader=%d group=%d", reader.retainedBytes, group.bytes)
	}
	reader.releaseGroup(nil)

	reader.retainedBytes = defaultMaxAggregationBytes
	if err := reader.setAggregateStateValue(&localGroup{}, &aggregateState{}, "x"); err == nil {
		t.Fatal("expected retained aggregate-value byte limit error")
	}
	oldBytes := aggregationValueBytes("long")
	reader.retainedBytes = oldBytes
	valueGroup := &localGroup{bytes: oldBytes}
	state := &aggregateState{value: "long", hasValue: true, valueBytes: oldBytes}
	if err := reader.setAggregateStateValue(valueGroup, state, "x"); err != nil {
		t.Fatal(err)
	}
	newBytes := aggregationValueBytes("x")
	if reader.retainedBytes != newBytes || valueGroup.bytes != newBytes || state.valueBytes != newBytes {
		t.Fatalf("replacement accounting reader=%d group=%d state=%d", reader.retainedBytes, valueGroup.bytes, state.valueBytes)
	}
	if aggregationValueBytes([]any{"long", map[string]any{"nested": "value"}}) <= aggregationValueBytes("x") {
		t.Fatal("aggregate value byte accounting mismatch")
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if aggregationValueBytes(cyclic) <= defaultMaxAggregationBytes {
		t.Fatal("unmeasurable aggregate value did not exceed the byte budget")
	}
}

func TestLocalAggregationAccountsForGroupStateAndMaterializedOutput(t *testing.T) {
	count := Count()
	reader := newLocalAggregationReader(context.Background(), From(NewRootCollectionRef("sales", "")).NewQuery().SelectColumns(count), &aggregationErrorReader{}, AggregationPlan{Strategy: AggregationHash})
	groupBytes := len("implicit")*2 + aggregationGroupOverheadBytes + aggregationAggregateStateOverheadBytes + 2*aggregationMapEntryOverheadBytes
	reader.retainedBytes = defaultMaxAggregationBytes - groupBytes + 1
	if _, err := reader.newGroup("implicit", map[string]any{}); err == nil {
		t.Fatal("expected group/state overhead to exhaust byte budget")
	}

	output := map[string]any{"category": "A", "rows": int64(1)}
	reader.retainedBytes = defaultMaxAggregationBytes - aggregationOutputMapOverheadBytes - aggregationRecordOverheadBytes
	if err := reader.reserveMaterializedOutput(output); err == nil {
		t.Fatal("expected materialized output to exhaust byte budget")
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if err := (&localAggregationReader{}).reserveMaterializedOutput(map[string]any{"value": cyclic}); err == nil {
		t.Fatal("expected unmeasurable materialized output to exhaust byte budget")
	}
	distinct := CountDistinctAs(Field("category"), "categories")
	distinctReader := newLocalAggregationReader(context.Background(), From(NewRootCollectionRef("sales", "")).NewQuery().SelectColumns(distinct), &aggregationErrorReader{}, AggregationPlan{Strategy: AggregationHash})
	distinctGroup, err := distinctReader.newGroup("implicit", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	distinctReader.retainedBytes = defaultMaxAggregationBytes - aggregationMapEntryOverheadBytes - len("s:1:A") + 1
	if err := distinctReader.updateGroup(distinctGroup, map[string]any{"category": "A"}); err == nil {
		t.Fatal("expected distinct-map entry overhead to exhaust byte budget")
	}
}

func TestLocalAggregationValueBudgetErrorsPropagate(t *testing.T) {
	for _, column := range []Column{
		MinAs(Field("value"), "value"),
		FirstAs(Field("value"), "value"),
		LastAs(Field("value"), "value"),
	} {
		q := From(NewRootCollectionRef("values", "")).NewQuery().SelectColumns(column)
		reader := newLocalAggregationReader(context.Background(), q, &aggregationErrorReader{err: ErrNoMoreRecords}, AggregationPlan{Strategy: AggregationHash})
		group, err := reader.newGroup("implicit", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		reader.retainedBytes = defaultMaxAggregationBytes
		if err := reader.updateGroup(group, map[string]any{"value": "x"}); err == nil {
			t.Fatalf("expected %s byte-budget error", column.Expression)
		}
	}
}
