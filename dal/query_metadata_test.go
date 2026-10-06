package dal

import (
	"context"
	"encoding/json"
	"errors"
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
	closed    bool
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
