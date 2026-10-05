package dal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// scriptedExecutor is a query executor whose answers a test chooses.
type scriptedExecutor struct {
	open func(Query) (RecordsReader, error)
}

func (e scriptedExecutor) ExecuteQueryToRecordsReader(_ context.Context, query Query) (RecordsReader, error) {
	return e.open(query)
}

func (scriptedExecutor) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, errors.New("recordsets are not scripted")
}

// scriptedFieldsExecutor is a scriptedExecutor that also names its fields.
type scriptedFieldsExecutor struct {
	scriptedExecutor
	fields func(RecordsetSource) ([]string, error)
}

func (e scriptedFieldsExecutor) JoinFields(_ context.Context, source RecordsetSource) ([]string, error) {
	return e.fields(source)
}

var errLeaf = errors.New("leaf failure")

func TestJoinValidationErrorUnwrapsItsCause(t *testing.T) {
	var none *JoinValidationError
	if none = joinError("join_plan", "from", "no cause").(*JoinValidationError); none.Unwrap() != nil {
		t.Fatalf("an error with no cause unwraps to %v", none.Unwrap())
	}
	caused := joinErrorFrom(errLeaf, "join_plan", "from", "with a cause")
	if !errors.Is(caused, errLeaf) || caused.(*JoinValidationError).Unwrap() != errLeaf {
		t.Fatal("the cause is not reachable")
	}
	if got, want := caused.Error(), "join_plan at from: with a cause"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	var diagnostic *JoinValidationError
	if !errors.As(caused, &diagnostic) || diagnostic.Category != "join_plan" {
		t.Fatal("the diagnostic is not reachable")
	}
}

// What a leaf scan answers with, an error that matches a sentinel, still matches
// it when it comes back through the engine, and the message is the same text as
// before.
func TestErrorsFromALeafScanKeepTheirIdentityThroughTheEngine(t *testing.T) {
	leafQuery := func() StructuredQuery {
		return From(NewRootCollectionRef("A", "a")).NewQuery().SelectIntoRecord(nil)
	}
	openFails := scriptedExecutor{open: func(Query) (RecordsReader, error) {
		return nil, fmt.Errorf("refused: %w", ErrNotSupported)
	}}
	readFails := scriptedExecutor{open: func(Query) (RecordsReader, error) {
		return joinFailingReader{err: fmt.Errorf("broken: %w", errLeaf)}, nil
	}}
	closeFails := scriptedExecutor{open: func(Query) (RecordsReader, error) {
		return joinEOFReader{closeErr: fmt.Errorf("stuck: %w", errLeaf)}, nil
	}}
	fieldsFail := scriptedFieldsExecutor{
		scriptedExecutor: scriptedExecutor{open: func(Query) (RecordsReader, error) { return EmptyReader{}, nil }},
		fields:           func(RecordsetSource) ([]string, error) { return nil, fmt.Errorf("no schema: %w", errLeaf) },
	}
	cases := []struct {
		name     string
		executor QueryExecutor
		sentinel error
		text     string
	}{
		{"opening the scan", openFails, ErrNotSupported, "cannot scan a: refused: "},
		{"reading the scan", readFails, errLeaf, "scan a: broken: "},
		{"closing the scan", closeFails, errLeaf, "close scan a: stuck: "},
		{"loading the fields", fieldsFail, errLeaf, "cannot load fields for a: no schema: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ExecuteRecursiveQuery(context.Background(), c.executor, leafQuery())
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("error = %v, want it to match %v", err, c.sentinel)
			}
			var diagnostic *JoinValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || !strings.Contains(err.Error(), c.text) {
				t.Fatalf("error = %v, want a join_plan diagnostic containing %q", err, c.text)
			}
		})
	}
}

// An error that crossed the engine twice, one nested query inside another, is
// still reachable.
func TestErrorsKeepTheirIdentityThroughNestedQueries(t *testing.T) {
	inner := From(NewRootCollectionRef("A", "a")).NewQuery().SelectKeysOnly(0)
	outer := From(NewRootCollectionRef("B", "b")).NewQuery().Where(NewExistsCondition(inner)).SelectKeysOnly(0)
	executor := scriptedExecutor{open: func(query Query) (RecordsReader, error) {
		if name := query.(StructuredQuery).From().Base().Name(); name == "A" {
			return nil, fmt.Errorf("refused: %w", ErrNotSupported)
		}
		return NewRecordsReader([]record.Record{joinTestRecord("B", "1", map[string]any{"id": 1})}), nil
	}}
	_, err := ExecuteRecursiveQuery(context.Background(), executor, outer)
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("error = %v, want it to match ErrNotSupported", err)
	}
}

func TestWildcardFieldFailureKeepsItsIdentity(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1, "name": "A"})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "name": "B"})},
	}, reads: map[string]int{}, fields: map[string][]string{"A": {"id", "name"}, "B": {"aid", "name"}}}
	changing := &lateFailingJoinFieldsBackend{ignoringJoinBackend: backend, err: fmt.Errorf("schema changed: %w", errLeaf)}
	q := root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("b", "aid"), Alias: "joinedID"}, AllColumnsExceptFrom("a"))
	_, err := NewDB(changing).ExecuteQueryToRecordsetReader(context.Background(), q)
	if !errors.Is(err, errLeaf) || !strings.Contains(err.Error(), "cannot load wildcard fields") {
		t.Fatalf("error = %v", err)
	}
}

// lateFailingJoinFieldsBackend names its fields for the scan and fails to name them
// again once the scan is done.
type lateFailingJoinFieldsBackend struct {
	*ignoringJoinBackend
	calls int
	err   error
}

func (b *lateFailingJoinFieldsBackend) JoinFields(_ context.Context, source RecordsetSource) ([]string, error) {
	b.calls++
	if b.calls > 2 {
		return nil, b.err
	}
	return b.fields[source.Name()], nil
}

func TestFederatedStreamFieldFailureKeepsItsIdentity(t *testing.T) {
	routed := federatedQueryExecutor{resolve: func(context.Context, string) (QueryExecutor, error) {
		return scriptedFieldsExecutor{
			scriptedExecutor: scriptedExecutor{open: func(Query) (RecordsReader, error) { return EmptyReader{}, nil }},
			fields:           func(RecordsetSource) ([]string, error) { return nil, fmt.Errorf("no schema: %w", errLeaf) },
		}, nil
	}}
	_, err := newFederatedJoinStream(context.Background(), testFederatedAggregate(), routed, FederatedQueryOptions{})
	if !errors.Is(err, errLeaf) || !strings.Contains(err.Error(), "cannot load fields for o") {
		t.Fatalf("error = %v", err)
	}
}
