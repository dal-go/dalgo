package dal

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/record"
)

type hintCapturingNativeBackend struct {
	*nativeJoinTestBackend
	planned  map[string][]JoinAlgorithm
	executed map[string][]JoinAlgorithm
}

func (b *hintCapturingNativeBackend) CanExecuteJoin(ctx context.Context, q StructuredQuery) error {
	b.planned = joinHintPaths(q.From(), "from", map[string][]JoinAlgorithm{})
	return b.nativeJoinTestBackend.CanExecuteJoin(ctx, q)
}

func (b *hintCapturingNativeBackend) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	if structured, ok := q.(StructuredQuery); ok && hasJoin(q) {
		b.executed = joinHintPaths(structured.From(), "from", map[string][]JoinAlgorithm{})
	}
	return b.nativeJoinTestBackend.ExecuteQueryToRecordsReader(ctx, q)
}

func joinHintPaths(from FromSource, path string, got map[string][]JoinAlgorithm) map[string][]JoinAlgorithm {
	for i, join := range from.Joins() {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		got[joinPath] = join.Algorithms()
		if child := join.From(); child != nil {
			joinHintPaths(child, joinPath+".from", got)
		}
	}
	return got
}

func TestNativeJoinProviderReceivesOrderedHintsAtPlanningAndExecution(t *testing.T) {
	child := From(NewRootCollectionRef("B", "b")).Join(
		NewJoinedSource(NewRootCollectionRef("C", "c"), JoinInner, joinOn("b", "cid", "c", "id")).WithAlgorithms(JoinAlgorithmNestedLoop, JoinAlgorithmHash),
	)
	root := From(NewRootCollectionRef("A", "a")).Join(
		NewNestedJoinedSource(child, JoinLeft, joinOn("a", "id", "b", "aid")).WithAlgorithms(JoinAlgorithmMerge, JoinAlgorithmHash),
	).Join(
		NewJoinedSource(NewRootCollectionRef("D", "d"), JoinLeft, joinOn("a", "id", "d", "aid")).WithAlgorithms(JoinAlgorithmLookup, JoinAlgorithmHash),
	)
	want := map[string][]JoinAlgorithm{
		"from.joins[0]":               {JoinAlgorithmMerge, JoinAlgorithmHash},
		"from.joins[0].from.joins[0]": {JoinAlgorithmNestedLoop, JoinAlgorithmHash},
		"from.joins[1]":               {JoinAlgorithmLookup, JoinAlgorithmHash},
	}
	backend := &hintCapturingNativeBackend{nativeJoinTestBackend: &nativeJoinTestBackend{
		ignoringJoinBackend: &ignoringJoinBackend{data: map[string][]record.Record{}, reads: map[string]int{}},
	}}
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.planned, want) || !reflect.DeepEqual(backend.executed, want) {
		t.Fatalf("native hints changed: planned=%v executed=%v want=%v", backend.planned, backend.executed, want)
	}
	if len(rows) != 1 || !backend.joined || backend.accepted != 1 || len(backend.reads) != 0 {
		t.Fatalf("native path was not used: rows=%d joined=%v accepted=%d scans=%v", len(rows), backend.joined, backend.accepted, backend.reads)
	}
}
