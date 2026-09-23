package dal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

func testFederatedAggregate() StructuredQuery {
	root := NewDatabaseCollectionRef("orders", "", "Invoice", "o")
	child := NewDatabaseCollectionRef("countries", "", "Country", "c")
	return From(root).Join(NewJoinedSource(child, JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().GroupBy(NewFieldRef("c", "id")).SelectIntoRecord(nil)
}

func TestFederatedStreamPlanSelection(t *testing.T) {
	if !canStreamFederatedAggregate(testFederatedAggregate()) {
		t.Fatal("expected streaming plan")
	}
	root := NewDatabaseCollectionRef("orders", "", "Invoice", "o")
	child := NewDatabaseCollectionRef("countries", "", "Country", "c")
	plain := From(root).Join(NewJoinedSource(child, JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().SelectIntoRecord(nil)
	if canStreamFederatedAggregate(plain) {
		t.Fatal("plain join should use generic plan")
	}
	nested := From(child).Join(NewJoinedSource(NewDatabaseCollectionRef("region", "", "Region", "r"), JoinInner, joinOn("c", "id", "r", "country_id")))
	variants := []StructuredQuery{
		From(root).Join(NewJoinedFrom(nested, JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().GroupBy(NewFieldRef("c", "id")).SelectIntoRecord(nil),
		From(root).Join(NewJoinedSource(child, JoinInner, joinOn("o", "country_id", "c", "id")).WithAlgorithms(JoinAlgorithmNestedLoop)).NewQuery().GroupBy(NewFieldRef("c", "id")).SelectIntoRecord(nil),
		From(root).Join(NewJoinedSource(child, JoinInner, NewComparison(NewFieldRef("o", "country_id"), GreaterThen, NewFieldRef("c", "id")))).NewQuery().GroupBy(NewFieldRef("c", "id")).SelectIntoRecord(nil),
		From(root).Join(NewJoinedSource(child, JoinInner)).NewQuery().GroupBy(NewFieldRef("c", "id")).SelectIntoRecord(nil),
	}
	for i, variant := range variants {
		if canStreamFederatedAggregate(variant) {
			t.Fatalf("variant %d should use generic plan", i)
		}
	}
}

func TestFederatedStreamLargeProgressAndFailures(t *testing.T) {
	ctx := context.Background()
	q := testFederatedAggregate()
	rows := make([]record.Record, 1024)
	for i := range rows {
		rows[i] = record.NewRecordWithData(record.NewKeyWithID("Invoice", i+1), map[string]any{"id": i + 1, "country_id": 1})
	}
	child := []record.Record{record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1})}
	var progress []FederatedProgress
	resolve := func(_ context.Context, database string) (QueryExecutor, error) {
		if database == "orders" {
			return federatedStub{rows: rows}, nil
		}
		return federatedStub{rows: child}, nil
	}
	reader, err := ExecuteFederatedQueryWithOptions(ctx, q, resolve, FederatedQueryOptions{OnProgress: func(item FederatedProgress) { progress = append(progress, item) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAllToRecords(ctx, reader); err != nil {
		t.Fatal(err)
	}
	if len(progress) < 4 {
		t.Fatalf("progress=%v", progress)
	}
	boom := errors.New("source offline")
	for _, failed := range []string{"orders", "countries"} {
		_, err := ExecuteFederatedQuery(ctx, q, func(_ context.Context, database string) (QueryExecutor, error) {
			if database == failed {
				return federatedStub{err: boom}, nil
			}
			if database == "orders" {
				return federatedStub{rows: rows}, nil
			}
			return federatedStub{rows: child}, nil
		})
		if err == nil || !strings.Contains(err.Error(), boom.Error()) {
			t.Fatalf("failed=%s err=%v", failed, err)
		}
	}
}

func TestFederatedJoinStreamReaderFailures(t *testing.T) {
	ctx := context.Background()
	q := testFederatedAggregate()
	boom := errors.New("read failure")
	for _, source := range []RecordsReader{
		joinFailingReader{err: boom},
		NewRecordsReader([]record.Record{record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"country_id": map[string]any{"bad": true}})}),
		NewRecordsReader([]record.Record{record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), "invalid")}),
	} {
		stream := &federatedJoinStream{source: source, execution: &joinExecution{ctx: ctx, q: q, keyRefs: map[string][]joinKeyReference{}}, root: q.From()}
		if cursor, err := stream.Cursor(); err != nil || cursor != "" {
			t.Fatalf("cursor=%q err=%v", cursor, err)
		}
		if _, err := stream.Next(); err == nil {
			t.Fatal("expected stream read/shape error")
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFederatedStreamAggregationPlanFailure(t *testing.T) {
	root := NewDatabaseCollectionRef("orders", "", "Invoice", "o")
	child := NewDatabaseCollectionRef("countries", "", "Country", "c")
	q := From(root).Join(NewJoinedSource(child, JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().GroupBy(nil).SelectIntoRecord(nil)
	routed := federatedQueryExecutor{resolve: func(context.Context, string) (QueryExecutor, error) {
		return federatedStub{rows: []record.Record{}}, nil
	}}
	if _, err := executeStreamingFederatedAggregate(context.Background(), q, routed, FederatedQueryOptions{}); err == nil {
		t.Fatal("expected invalid aggregation plan")
	}
}

func TestFederatedStreamJoinKeyBuildAndWhereErrors(t *testing.T) {
	ctx := context.Background()
	q := testFederatedAggregate()
	root := q.From()
	base := &joinExecution{ctx: ctx, q: q, scans: map[string][]scannedJoinRow{"c": {{key: record.NewKeyWithID("Country", 1), data: map[string]any{"id": 1}}}}, indexes: map[string]map[string][]scannedJoinRow{}, keyRefs: map[string][]joinKeyReference{"o": {{field: "country_id", path: "from.joins[0].on"}}}}
	invalidKey := record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"country_id": map[string]any{"bad": true}})
	stream := &federatedJoinStream{source: NewRecordsReader([]record.Record{invalidKey}), execution: base, root: root}
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected unsupported join key")
	}
	_ = stream.Close()
	large := make([]scannedJoinRow, maxJoinRows+1)
	for i := range large {
		large[i] = scannedJoinRow{key: record.NewKeyWithID("Country", i+1), data: map[string]any{"id": 1}}
	}
	base.scans["c"] = large
	valid := record.NewRecordWithData(record.NewKeyWithID("Invoice", 2), map[string]any{"country_id": 1})
	stream = &federatedJoinStream{source: NewRecordsReader([]record.Record{valid}), execution: base, root: root}
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected joined row bound")
	}
	_ = stream.Close()
	badWhere := From(NewDatabaseCollectionRef("orders", "", "Invoice", "o")).Join(NewJoinedSource(NewDatabaseCollectionRef("countries", "", "Country", "c"), JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().GroupBy(NewFieldRef("c", "id")).Where(NewComparison(NewFieldRef("missing", "id"), Equal, NewConstant(1))).SelectIntoRecord(nil)
	base.q = badWhere
	base.scans["c"] = large[:1]
	base.indexes = map[string]map[string][]scannedJoinRow{}
	stream = &federatedJoinStream{source: NewRecordsReader([]record.Record{valid}), execution: base, root: badWhere.From()}
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected where scope error")
	}
	_ = stream.Close()
}
