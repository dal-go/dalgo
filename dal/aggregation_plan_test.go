package dal

import (
	"context"
	"errors"
	"testing"

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
	plan, err = PlanAggregation(q, QueryCapabilities{OrderBy: true})
	if err != nil || plan.Strategy != AggregationStreaming {
		t.Fatalf("stream plan=%#v err=%v", plan, err)
	}
	plan, err = PlanAggregation(q, QueryCapabilities{})
	if err != nil || plan.Strategy != AggregationHash {
		t.Fatalf("hash plan=%#v err=%v", plan, err)
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
