package dal

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

func TestJoinAlgorithmModelCopiesAndIndependentEdges(t *testing.T) {
	outerPreferences := []JoinAlgorithm{JoinAlgorithmMerge, JoinAlgorithmHash}
	outer := NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")).WithAlgorithms(outerPreferences...)
	outerPreferences[0] = JoinAlgorithmLookup
	if got := outer.Algorithms(); !reflect.DeepEqual(got, []JoinAlgorithm{JoinAlgorithmMerge, JoinAlgorithmHash}) {
		t.Fatalf("input slice leaked: %v", got)
	}
	returned := outer.Algorithms()
	returned[0] = JoinAlgorithmLookup
	if outer.Algorithms()[0] != JoinAlgorithmMerge {
		t.Fatal("accessor returned mutable model storage")
	}
	child := From(NewRootCollectionRef("B", "b")).Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinLeft, joinOn("b", "cid", "c", "id")).WithAlgorithms(JoinAlgorithmNestedLoop))
	root := From(NewRootCollectionRef("A", "a")).Join(NewNestedJoinedSource(child, JoinInner, joinOn("a", "id", "b", "aid")).WithAlgorithms(JoinAlgorithmMerge, JoinAlgorithmHash)).Join(NewJoinedSource(NewRootCollectionRef("D", "d"), JoinLeft, joinOn("a", "id", "d", "aid")).WithAlgorithms(JoinAlgorithmLookup, JoinAlgorithmHash))
	cloned := root.NewQuery().SelectIntoRecord(nil).From().Joins()
	if !reflect.DeepEqual(cloned[0].Algorithms(), []JoinAlgorithm{JoinAlgorithmMerge, JoinAlgorithmHash}) || !reflect.DeepEqual(cloned[0].From().Joins()[0].Algorithms(), []JoinAlgorithm{JoinAlgorithmNestedLoop}) || !reflect.DeepEqual(cloned[1].Algorithms(), []JoinAlgorithm{JoinAlgorithmLookup, JoinAlgorithmHash}) {
		t.Fatalf("nested/sibling hints were not cloned: %#v", cloned)
	}
	if NewJoinedSource(NewRootCollectionRef("X", "x"), JoinInner).Algorithms() != nil || NewJoinedSource(NewRootCollectionRef("X", "x"), JoinInner).WithAlgorithms().Algorithms() == nil {
		t.Fatal("omission and explicit empty list collapsed")
	}
}

func TestJoinAlgorithmDirectModelRejectsBeforeReads(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": {}, "B": {}}, reads: map[string]int{}}
	for _, tt := range []struct {
		name       string
		algorithms []JoinAlgorithm
		path       string
	}{
		{"empty", []JoinAlgorithm{}, "from.joins[0].hints.algorithms"},
		{"unknown", []JoinAlgorithm{"bogus"}, "from.joins[0].hints.algorithms[0]"},
		{"case", []JoinAlgorithm{"Hash"}, "from.joins[0].hints.algorithms[0]"},
		{"duplicate", []JoinAlgorithm{JoinAlgorithmHash, JoinAlgorithmHash}, "from.joins[0].hints.algorithms[1]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			join := NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")).WithAlgorithms(tt.algorithms...)
			query := From(NewRootCollectionRef("A", "a")).Join(join).NewQuery().SelectIntoRecord(nil)
			_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
			var diagnostic *JoinValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != "join_algorithm" || diagnostic.Path != tt.path {
				t.Fatalf("want join_algorithm at %s, got %v", tt.path, err)
			}
			if len(backend.reads) != 0 {
				t.Fatalf("provider read before validation: %v", backend.reads)
			}
		})
	}
	child := From(NewRootCollectionRef("B", "b")).Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinLeft, joinOn("b", "cid", "c", "id")).WithAlgorithms("bad"))
	query := From(NewRootCollectionRef("A", "a")).Join(NewNestedJoinedSource(child, JoinInner, joinOn("a", "id", "b", "aid"))).NewQuery().SelectIntoRecord(nil)
	if err := ValidateJoinTree(query.From()); err == nil || !strings.Contains(err.Error(), "from.joins[0].from.joins[0].hints.algorithms[0]") {
		t.Fatalf("nested hint path: %v", err)
	}
}

func TestSelectGenericJoinAlgorithmOrderAndFallback(t *testing.T) {
	for _, tt := range []struct {
		name           string
		preferences    []JoinAlgorithm
		hashApplicable bool
		want           genericJoinAlgorithm
	}{
		{"unhinted hash", nil, true, genericJoinHash},
		{"unhinted loop", nil, false, genericJoinNestedLoop},
		{"loop before hash", []JoinAlgorithm{JoinAlgorithmNestedLoop, JoinAlgorithmHash}, true, genericJoinNestedLoop},
		{"hash before loop", []JoinAlgorithm{JoinAlgorithmHash, JoinAlgorithmNestedLoop}, true, genericJoinHash},
		{"unavailable before hash", []JoinAlgorithm{JoinAlgorithmMerge, JoinAlgorithmLookup, JoinAlgorithmBatchedLookup, JoinAlgorithmHash}, true, genericJoinHash},
		{"unavailable fallback hash", []JoinAlgorithm{JoinAlgorithmMerge, JoinAlgorithmLookup, JoinAlgorithmBatchedLookup}, true, genericJoinHash},
		{"hash inapplicable then loop", []JoinAlgorithm{JoinAlgorithmHash, JoinAlgorithmNestedLoop}, false, genericJoinNestedLoop},
		{"unavailable fallback loop", []JoinAlgorithm{JoinAlgorithmMerge}, false, genericJoinNestedLoop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectGenericJoinAlgorithm(tt.preferences, tt.hashApplicable); got != tt.want {
				t.Fatalf("selected %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGenericJoinHintedResultsMatchUnhintedAndLoopCapDoesNotRetryHash(t *testing.T) {
	base := make([]record.Record, 400)
	joined := make([]record.Record, 400)
	for i := range base {
		base[i] = joinTestRecord("A", fmt.Sprintf("a%d", i), map[string]any{"id": i})
		joined[i] = joinTestRecord("B", fmt.Sprintf("b%d", i), map[string]any{"aid": i})
	}
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": base, "B": joined}, reads: map[string]int{}}
	query := func(algorithms ...JoinAlgorithm) StructuredQuery {
		join := NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid"))
		if algorithms != nil {
			join = join.WithAlgorithms(algorithms...)
		}
		return From(NewRootCollectionRef("A", "a")).Join(join).NewQuery().OrderBy(Ascending(NewFieldRef("a", "id"))).SelectColumns(Column{Expression: NewFieldRef("a", "id"), Alias: "id"})
	}
	baselineReader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query())
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := ReadAllToRecords(context.Background(), baselineReader)
	if err != nil {
		t.Fatal(err)
	}
	for _, algorithms := range [][]JoinAlgorithm{{JoinAlgorithmHash}, {JoinAlgorithmMerge, JoinAlgorithmHash}, {JoinAlgorithmLookup, JoinAlgorithmBatchedLookup}} {
		reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query(algorithms...))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := ReadAllToRecords(context.Background(), reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(baseline) {
			t.Fatalf("hinted rows = %d, baseline = %d", len(rows), len(baseline))
		}
		for i := range rows {
			if !reflect.DeepEqual(rows[i].Data(), baseline[i].Data()) {
				t.Fatalf("hinted row %d differs", i)
			}
		}
	}
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query(JoinAlgorithmNestedLoop, JoinAlgorithmHash))
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || diagnostic.Path != "from.joins[0]" || !strings.Contains(err.Error(), "candidate evaluation bound") {
		t.Fatalf("selected loop must stop at candidate cap without hash retry: %v", err)
	}
}
