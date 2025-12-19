package dal

import (
	"errors"
	"testing"
)

// helper to build a simple Condition
func cmp(left, right any) Comparison {
	return NewComparison(Constant{Value: left}, Equal, Constant{Value: right})
}

func TestFrom_Join_Joins_NewQuery_DeepCopy(t *testing.T) {
	// Prepare base source
	base := NewRootCollectionRef("users", "")
	f := From(base).(*from)

	// Prepare a join with ON conditions
	j := JoinedSource{
		RecordsetSource: NewCollectionGroupRef("orders", ""),
		on:              []Condition{cmp(1, 1), cmp(2, 2)},
	}

	// Append join and verify chainability
	returned := f.Join(j)
	if returned == nil {
		t.Fatalf("Join should return FromSource, got nil")
	}

	// Joins must return a copy slice (shallow copy)
	got := f.Joins()
	if len(got) != 1 {
		t.Fatalf("expected 1 join, got %d", len(got))
	}
	// Reassign element in returned slice; original should not change
	origFirstName := f.joins[0].Name()
	got[0] = JoinedSource{RecordsetSource: NewRootCollectionRef("changed", ""), on: []Condition{cmp("x", "y")}}
	if f.joins[0].Name() != origFirstName {
		t.Fatalf("Joins must return a copy of slice; reassigning element in returned slice should not affect original")
	}

	// Access On accessor
	ons := f.joins[0].On()
	if len(ons) != 2 {
		t.Fatalf("expected 2 ON conditions, got %d", len(ons))
	}

	// NewQuery must deep‑copy joins and ON slice
	qb := f.NewQuery()
	if qb == nil {
		t.Fatalf("NewQuery returned nil builder")
	}

	// Pull back the embedded from from the query builder
	sq := qb.SelectKeysOnly(0) // produce structured query without altering from
	gotFrom := sq.From().(*from)

	if len(gotFrom.joins) != len(f.joins) {
		t.Fatalf("expected %d joins in cloned from, got %d", len(f.joins), len(gotFrom.joins))
	}

	// Mutate original and ensure clone is unaffected
	f.joins[0].on[0] = cmp("a", "b")
	// Mutate clone and ensure original is unaffected
	gotFrom.joins[0].on[1] = cmp("c", "d")

	// Validate independence
	if equal := gotFrom.joins[0].on[0].(Comparison).Left.(Constant).Value == "a"; equal {
		t.Fatalf("clone should be independent from original joins (first on condition)")
	}
	if equal := f.joins[0].on[1].(Comparison).Left.(Constant).Value == "c"; equal {
		t.Fatalf("original should be independent from cloned joins (second on condition)")
	}
}

// Additional targeted reader to stimulate error paths for SelectAll
type errReader struct {
	nextErr  error
	closed   bool
	closeErr error
}

func (e *errReader) ReadAll(_ func(dest any) error) (int, error) { return 0, errors.New("not used") }
func (e *errReader) Next() (Record, error)                       { return nil, e.nextErr }
func (e *errReader) Cursor() (string, error)                     { return "", nil }
func (e *errReader) Close() error                                { e.closed = true; return e.closeErr }

func TestSelectAll_ErrorPaths(t *testing.T) {
	// Case 1: Next returns some non-ErrNoMoreRecords error; that error should be returned
	nextErr := errors.New("boom")
	_, err := SelectAllIDs[int](&errReader{nextErr: nextErr}, WithLimit(5))
	if !errors.Is(err, nextErr) {
		t.Fatalf("expected next error to propagate, got: %v", err)
	}

	// Case 2: No prior error, Close returns error which should be wrapped and returned
	e := &errReader{nextErr: ErrNoMoreRecords, closeErr: errors.New("close failed")}
	_, err = SelectAllIDs[int](e)
	if err == nil || !errors.Is(err, e.closeErr) {
		t.Fatalf("expected close error to be returned when no prior error, got: %v", err)
	}
}
