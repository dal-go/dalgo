package dal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dal-go/dalgo/datarights"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"testing"
)

func rightsMetadata() datarights.QueryMetadata {
	return datarights.QueryMetadata{SourceRights: []datarights.SourceRight{{SourceID: "a", Declaration: datarights.Declaration{Text: "original"}}}, UsedSourceIDs: []string{"a"}}
}

func TestOptionalQueryReaderMetadata(t *testing.T) {
	raw := &aggregationCoverageReader{records: []record.Record{joinTestRecord("A", "1", map[string]any{"id": 1})}}
	if _, ok := ReadQueryMetadata(raw); ok {
		t.Fatal("ordinary reader unexpectedly has metadata")
	}
	if err := requireUnannotatedQueryInput(raw); err != nil {
		t.Fatal(err)
	}
	metadata := rightsMetadata()
	reader := WithRecordsQueryMetadata(raw, metadata)
	metadata.SourceRights[0].Declaration.Text = "changed"
	got, ok := ReadQueryMetadata(reader)
	if !ok || got.SourceRights[0].Declaration.Text != "original" {
		t.Fatal("metadata missing before first row")
	}
	got.SourceRights[0].Declaration.Text = "changed again"
	got, _ = ReadQueryMetadata(reader)
	if got.SourceRights[0].Declaration.Text != "original" {
		t.Fatal("reader snapshot is mutable")
	}
	if _, err := reader.Next(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil || !raw.closed {
		t.Fatal("Close was not forwarded")
	}
	if err := requireUnannotatedQueryInput(reader); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("annotated input must refuse: %v", err)
	}
	if err := requireUnannotatedQueryInput(WithRecordsQueryMetadata(EmptyReader{}, datarights.QueryMetadata{})); err != nil {
		t.Fatal(err)
	}
	if err := requireUnannotatedQueryInput(WithRecordsQueryMetadata(EmptyReader{}, datarights.QueryMetadata{UsedSourceIDs: []string{}})); !errors.Is(err, ErrNotSupported) {
		t.Fatal("known empty inventory was dropped")
	}
	rs := recordset.NewColumnarRecordset("A", recordset.NewTypedColumn[any]("id", nil))
	columnar := WithRecordsetQueryMetadata(&aggregationRecordsetReader{recordset: rs}, rightsMetadata())
	if columnar.Recordset() != rs {
		t.Fatal("Recordset was not forwarded")
	}
	if _, _, err := columnar.Next(); !errors.Is(err, ErrNoMoreRecords) {
		t.Fatal(err)
	}
	if cursor, err := columnar.Cursor(); err != nil || cursor != "" {
		t.Fatal(err)
	}
	if err := columnar.Close(); err != nil {
		t.Fatal(err)
	}
	if got, ok := ReadQueryMetadata(columnar); !ok || got.UsedSourceIDs[0] != "a" {
		t.Fatal("empty output lost source terms")
	}
}

type annotatedInputBackend struct {
	*ignoringJoinBackend
	annotated string
}

func (b *annotatedInputBackend) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	raw, err := b.ignoringJoinBackend.ExecuteQueryToRecordsReader(ctx, q)
	if err != nil || q.(StructuredQuery).From().Base().Name() != b.annotated {
		return raw, err
	}
	return WithRecordsQueryMetadata(raw, rightsMetadata()), nil
}

func TestGenericMixedJoinRefusesSourceRights(t *testing.T) {
	backend := &annotatedInputBackend{ignoringJoinBackend: &ignoringJoinBackend{data: map[string][]record.Record{"A": {joinTestRecord("A", "1", map[string]any{"aid": 1})}, "B": {}}, reads: map[string]int{}}, annotated: "B"}
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	if reader != nil || !errors.Is(err, ErrNotSupported) {
		t.Fatalf("rights-bearing empty join input lost metadata: reader=%v err=%v", reader, err)
	}
}

func TestLocalAggregationRefusesSourceRights(t *testing.T) {
	raw := &aggregationCoverageReader{records: []record.Record{}}
	backend := &aggregationCoverageBackend{records: WithRecordsQueryMetadata(raw, rightsMetadata())}
	q := From(NewRootCollectionRef("A", "")).NewQuery().SelectColumns(Count())
	reader, err := executeAggregationLocal(context.Background(), backend, q, hashFallbackPlan)
	if reader != nil || !errors.Is(err, ErrNotSupported) || !raw.closed {
		t.Fatalf("aggregation must refuse and close: reader=%v err=%v closed=%v", reader, err, raw.closed)
	}
}

func TestMetadataJSONDoesNotAssignOutputLicence(t *testing.T) {
	data, err := json.Marshal(rightsMetadata())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["sourceRights"] == nil || fields["usedSourceIds"] == nil || fields["license"] != nil {
		t.Fatal(string(data))
	}
}

func TestRecursiveCappedScanRefusesSourceRights(t *testing.T) {
	raw := &aggregationCoverageReader{records: []record.Record{}}
	backend := &aggregationCoverageBackend{records: WithRecordsQueryMetadata(raw, rightsMetadata())}
	q := From(NewRootCollectionRef("A", "a")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("a", "id")})
	execution := &joinExecution{ctx: context.Background(), executor: backend}
	rows, err := execution.executeSimpleCapped(q, nil, 1, true)
	if rows != nil || !errors.Is(err, ErrNotSupported) || !raw.closed {
		t.Fatalf("capped recursive scan must refuse and close before output: rows=%v err=%v closed=%v", rows, err, raw.closed)
	}
}

type federatedMetadataExecutor struct {
	QueryExecutor
	raw      *aggregationCoverageReader
	metadata *datarights.QueryMetadata
}

func (e *federatedMetadataExecutor) ExecuteQueryToRecordsReader(context.Context, Query) (RecordsReader, error) {
	if e.metadata == nil {
		return e.raw, nil
	}
	return WithRecordsQueryMetadata(e.raw, *e.metadata), nil
}
func (*federatedMetadataExecutor) JoinFields(context.Context, RecordsetSource) ([]string, error) {
	return []string{"id"}, nil
}

func federatedRightsQuery(kind string) StructuredQuery {
	a := NewDatabaseCollectionRef("dbA", "", "A", "a")
	if kind == "single" {
		return From(a).NewQuery().SelectIntoRecord(nil)
	}
	if kind == "money-single" {
		return From(a).NewQuery().SelectColumns(Count())
	}
	b := NewDatabaseCollectionRef("dbB", "", "B", "b")
	joined := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "id")))).NewQuery()
	if kind == "aggregate" || kind == "money-joined" {
		return joined.SelectColumns(Count())
	}
	if kind == "ordered" {
		joined.OrderBy(Ascending(NewFieldRef("a", "id")))
	}
	return joined.SelectColumns(Column{Expression: NewFieldRef("a", "id")})
}

func TestFederatedTransformsRefuseAnnotatedInputsBeforeRows(t *testing.T) {
	for _, tc := range []struct {
		name, kind, annotated string
		empty, progress       bool
	}{
		{name: "stream root", kind: "joined", annotated: "dbA"},
		{name: "stream dimension", kind: "joined", annotated: "dbB"},
		{name: "empty stream root", kind: "joined", annotated: "dbA", empty: true},
		{name: "empty stream dimension", kind: "joined", annotated: "dbB", empty: true},
		{name: "joined aggregate root", kind: "aggregate", annotated: "dbA"},
		{name: "joined aggregate dimension", kind: "aggregate", annotated: "dbB"},
		{name: "money single", kind: "money-single", annotated: "dbA"},
		{name: "money joined", kind: "money-joined", annotated: "dbA"},
		{name: "ordered join root with progress", kind: "ordered", annotated: "dbA", progress: true},
		{name: "ordered join dimension with progress", kind: "ordered", annotated: "dbB", progress: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executors := map[string]*federatedMetadataExecutor{}
			for _, db := range []string{"dbA", "dbB"} {
				rows := []record.Record{joinTestRecord(db, "1", map[string]any{"id": 1})}
				if tc.empty && db == tc.annotated {
					rows = []record.Record{}
				}
				executors[db] = &federatedMetadataExecutor{raw: &aggregationCoverageReader{records: rows}}
			}
			metadata := rightsMetadata()
			executors[tc.annotated].metadata = &metadata
			options := FederatedQueryOptions{}
			if tc.progress {
				options.OnProgress = func(FederatedProgress) {}
			}
			if tc.kind == "money-single" || tc.kind == "money-joined" {
				options.Money = &MoneyConfig{Rounding: "halfEven"}
			}
			reader, err := ExecuteFederatedQueryWithOptions(context.Background(), federatedRightsQuery(tc.kind), func(_ context.Context, db string) (QueryExecutor, error) { return executors[db], nil }, options)
			raw := executors[tc.annotated].raw
			if reader != nil || !errors.Is(err, ErrNotSupported) || !raw.closed || raw.index != 0 {
				t.Fatalf("must refuse/close annotated source before rows: reader=%v err=%v closed=%v reads=%d", reader, err, raw.closed, raw.index)
			}
		})
	}
}

func TestFederatedProgressPreservesOptionalMetadata(t *testing.T) {
	for _, annotated := range []bool{false, true} {
		t.Run(fmt.Sprint(annotated), func(t *testing.T) {
			raw := &aggregationCoverageReader{records: []record.Record{joinTestRecord("A", "1", map[string]any{"id": 1})}}
			executor := &federatedMetadataExecutor{raw: raw}
			if annotated {
				metadata := rightsMetadata()
				executor.metadata = &metadata
			}
			var progress []FederatedProgress
			reader, err := ExecuteFederatedQueryWithOptions(context.Background(), federatedRightsQuery("single"), func(context.Context, string) (QueryExecutor, error) { return executor, nil }, FederatedQueryOptions{OnProgress: func(p FederatedProgress) { progress = append(progress, p) }})
			if err != nil {
				t.Fatal(err)
			}
			got, exists := ReadQueryMetadata(reader)
			if exists != annotated {
				t.Fatalf("optional capability changed: %v", exists)
			}
			if annotated {
				if got.SourceRights[0].SourceID != "a" || got.UsedSourceIDs[0] != "a" {
					t.Fatal("metadata was lost before first row")
				}
				got.SourceRights[0].Declaration.Text = "mutated"
				again, _ := ReadQueryMetadata(reader)
				if again.SourceRights[0].Declaration.Text != "original" {
					t.Fatal("progress reader exposes mutable metadata")
				}
			}
			records, err := ReadAllToRecords(context.Background(), reader)
			if err != nil || len(records) != 1 || !raw.closed || len(progress) != 1 || progress[0].Rows != 1 {
				t.Fatalf("native leaf/progress behavior changed: records=%v err=%v progress=%v", records, err, progress)
			}
		})
	}
}
