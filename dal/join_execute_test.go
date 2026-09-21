package dal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// This provider deliberately ignores joins. The framework must only send it
// single-relation scans, never the original joined query.
type ignoringJoinBackend struct {
	Backend
	data     map[string][]record.Record
	reads    map[string]int
	joined   bool
	selected bool
	fields   map[string][]string
}

type nativeJoinTestBackend struct {
	*ignoringJoinBackend
	accepted int
}

type joinFailingReader struct{ err error }

func (r joinFailingReader) Next() (record.Record, error) { return nil, r.err }
func (joinFailingReader) Cursor() (string, error)        { return "", nil }
func (joinFailingReader) Close() error                   { return nil }

type joinEOFReader struct{ closeErr error }

func (joinEOFReader) Next() (record.Record, error) {
	return nil, fmt.Errorf("wrapped: %w", ErrNoMoreRecords)
}
func (joinEOFReader) Cursor() (string, error) { return "", nil }
func (r joinEOFReader) Close() error          { return r.closeErr }

type joinEOFBackend struct {
	*ignoringJoinBackend
	closeErr error
}

func (b *joinEOFBackend) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	if q.(StructuredQuery).From().Base().Name() == "A" {
		return joinEOFReader{closeErr: b.closeErr}, nil
	}
	return b.ignoringJoinBackend.ExecuteQueryToRecordsReader(ctx, q)
}

type joinReadFailureBackend struct {
	*ignoringJoinBackend
	source string
}

type failingJoinFieldsBackend struct{ *ignoringJoinBackend }

func (*failingJoinFieldsBackend) JoinFields(context.Context, RecordsetSource) ([]string, error) {
	return nil, errors.New("schema offline")
}

type changingJoinFieldsBackend struct {
	*ignoringJoinBackend
	calls int
}

type joinBackendWithoutSelect struct {
	Backend
	base *ignoringJoinBackend
}

func (b joinBackendWithoutSelect) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	return b.base.ExecuteQueryToRecordsReader(ctx, q)
}
func (b *changingJoinFieldsBackend) JoinFields(_ context.Context, source RecordsetSource) ([]string, error) {
	b.calls++
	if b.calls > 2 {
		return nil, errors.New("schema changed")
	}
	return b.fields[source.Name()], nil
}

type joinTestReadTx struct {
	ReadTransaction
	backend *ignoringJoinBackend
	probes  int
}

func (tx *joinTestReadTx) Options() TransactionOptions { return NewTransactionOptions() }
func (tx *joinTestReadTx) CanExecuteJoin(context.Context, StructuredQuery) error {
	tx.probes++
	return errors.New("generic only")
}
func (tx *joinTestReadTx) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	return tx.backend.ExecuteQueryToRecordsReader(ctx, q)
}
func (tx *joinTestReadTx) ExecuteQueryToRecordsetReader(ctx context.Context, q Query, opts ...recordset.Option) (RecordsetReader, error) {
	return tx.backend.ExecuteQueryToRecordsetReader(ctx, q, opts...)
}
func (tx *joinTestReadTx) Select(context.Context, Query) (Reader, error) {
	return nil, errors.New("raw transaction Select bypassed planner")
}

type joinTestWriteTx struct {
	ReadwriteTransaction
	backend *ignoringJoinBackend
	probes  int
}

func (tx *joinTestWriteTx) ID() string                  { return "join-test" }
func (tx *joinTestWriteTx) Options() TransactionOptions { return NewTransactionOptions() }
func (tx *joinTestWriteTx) CanExecuteJoin(context.Context, StructuredQuery) error {
	tx.probes++
	return errors.New("generic only")
}
func (tx *joinTestWriteTx) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	return tx.backend.ExecuteQueryToRecordsReader(ctx, q)
}
func (tx *joinTestWriteTx) ExecuteQueryToRecordsetReader(ctx context.Context, q Query, opts ...recordset.Option) (RecordsetReader, error) {
	return tx.backend.ExecuteQueryToRecordsetReader(ctx, q, opts...)
}
func (tx *joinTestWriteTx) Select(context.Context, Query) (Reader, error) {
	return nil, errors.New("raw transaction Select bypassed planner")
}

type joinTransactionBackend struct {
	*ignoringJoinBackend
	read  *joinTestReadTx
	write *joinTestWriteTx
}

func (b *joinTransactionBackend) RunReadonlyTransaction(ctx context.Context, worker ROTxWorker, _ ...TransactionOption) error {
	return worker(ctx, b.read)
}
func (b *joinTransactionBackend) RunReadwriteTransaction(ctx context.Context, worker RWTxWorker, _ ...TransactionOption) error {
	return worker(ctx, b.write)
}

func (b *joinReadFailureBackend) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	if q.(StructuredQuery).From().Base().Name() == b.source {
		return joinFailingReader{err: errors.New("read failed")}, nil
	}
	return b.ignoringJoinBackend.ExecuteQueryToRecordsReader(ctx, q)
}

func (b *nativeJoinTestBackend) CanExecuteJoin(_ context.Context, _ StructuredQuery) error {
	b.accepted++
	return nil
}
func (b *nativeJoinTestBackend) ExecuteQueryToRecordsReader(ctx context.Context, q Query) (RecordsReader, error) {
	if hasJoin(q) {
		b.joined = true
		return NewRecordsReader([]record.Record{joinTestRecord("A", "native", map[string]any{"aid": 1, "bid": "b1", "cid": 9})}), nil
	}
	return b.ignoringJoinBackend.ExecuteQueryToRecordsReader(ctx, q)
}

func (b *nativeJoinTestBackend) ExecuteQueryToRecordsetReader(_ context.Context, q Query, _ ...recordset.Option) (RecordsetReader, error) {
	if !hasJoin(q) {
		return nil, errors.New("unexpected single-source native recordset query")
	}
	b.joined = true
	rs := recordset.NewColumnarRecordset("A", recordset.NewTypedColumn[any]("aid", nil))
	row := rs.NewRow()
	_ = row.SetValueByName("aid", 1, rs)
	return &aggregationRecordsetReader{recordset: rs}, nil
}

func (b *ignoringJoinBackend) Select(_ context.Context, _ Query) (Reader, error) {
	b.selected = true
	return nil, errors.New("provider Select bypassed planner")
}

func (b *ignoringJoinBackend) JoinFields(_ context.Context, source RecordsetSource) ([]string, error) {
	return b.fields[source.Name()], nil
}

func (b *ignoringJoinBackend) ExecuteQueryToRecordsReader(_ context.Context, query Query) (RecordsReader, error) {
	q := query.(StructuredQuery)
	if len(q.From().Joins()) != 0 {
		b.joined = true
	}
	name := q.From().Base().Name()
	b.reads[name]++
	if _, ok := b.data[name]; !ok {
		return nil, errors.New("unscannable relation")
	}
	return NewRecordsReader(b.data[name]), nil
}
func (b *ignoringJoinBackend) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, errors.New("unexpected direct recordset request")
}

func joinTestRecord(collection, id string, fields map[string]any) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID(collection, id), fields)
}

func joinTestQuery() StructuredQuery {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	c := NewRootCollectionRef("C", "c")
	bc := From(b).Join(NewJoinedSource(c, JoinInner, NewComparison(NewFieldRef("b", "cid"), Equal, NewFieldRef("c", "id"))))
	root := From(a).Join(NewJoinedFrom(bc, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	return root.NewQuery().SelectColumns(
		Column{Expression: NewFieldRef("a", "id"), Alias: "aid"},
		Column{Expression: NewFieldRef("b", "id"), Alias: "bid"},
		Column{Expression: NewFieldRef("c", "id"), Alias: "cid"},
	)
}

func TestGenericJoinNestedLeftInnerAndSingleScans(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1}), joinTestRecord("A", "two", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": "b1", "aid": 1, "cid": 9}), joinTestRecord("B", "b2", map[string]any{"id": "b2", "aid": 1, "cid": 10})},
		"C": {joinTestRecord("C", "c1", map[string]any{"id": 9})},
	}, reads: map[string]int{}}
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want matched and null-extended A, got %d", len(rows))
	}
	if got := rows[0].Data().(map[string]any); !reflect.DeepEqual(got, map[string]any{"aid": float64(1), "bid": "b1", "cid": float64(9)}) {
		t.Fatalf("matched row: %#v", got)
	}
	if got := rows[1].Data().(map[string]any); !reflect.DeepEqual(got, map[string]any{"aid": float64(2), "bid": nil, "cid": nil}) {
		t.Fatalf("null row: %#v", got)
	}
	if backend.joined || !reflect.DeepEqual(backend.reads, map[string]int{"A": 1, "B": 1, "C": 1}) {
		t.Fatalf("provider calls joined=%v reads=%v", backend.joined, backend.reads)
	}
}

func TestGenericJoinNestedLeftLeftPreservesBothUnmatchedLevels(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	c := NewRootCollectionRef("C", "c")
	child := From(b).Join(NewJoinedSource(c, JoinLeft, joinOn("b", "cid", "c", "id")))
	root := From(a).Join(NewNestedJoinedSource(child, JoinLeft, joinOn("a", "id", "b", "aid")))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1}), joinTestRecord("A", "a2", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": "B1", "aid": 1, "cid": 9})},
		"C": {},
	}, reads: map[string]int{}}
	q := root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("a", "id"), Alias: "aID"}, Column{Expression: NewFieldRef("b", "id"), Alias: "bID"}, Column{Expression: NewFieldRef("c", "id"), Alias: "cID"})
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("LEFT subtree returned %d rows", len(rows))
	}
	first, second := rows[0].Data().(map[string]any), rows[1].Data().(map[string]any)
	if first["bID"] != "B1" || first["cID"] != nil || second["bID"] != nil || second["cID"] != nil {
		t.Fatalf("nested LEFT null extension: %v / %v", first, second)
	}
	if !reflect.DeepEqual(backend.reads, map[string]int{"A": 1, "B": 1, "C": 1}) {
		t.Fatalf("relation scans: %v", backend.reads)
	}
}

func TestGenericJoinOptionalSelectUsesPlanner(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": "b1", "aid": 1, "cid": 9})},
		"C": {joinTestRecord("C", "c1", map[string]any{"id": 9})},
	}, reads: map[string]int{}}
	selector, ok := NewDB(backend).(interface {
		Select(context.Context, Query) (Reader, error)
	})
	if !ok {
		t.Fatal("wrapped DB does not expose Select")
	}
	reader, err := selector.Select(context.Background(), joinTestQuery())
	if err != nil {
		t.Fatal(err)
	}
	if backend.selected || backend.joined {
		t.Fatalf("backend bypass: selected=%v joined=%v", backend.selected, backend.joined)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	plain := From(NewRootCollectionRef("A", "")).NewQuery().SelectIntoRecord(nil)
	_, err = selector.Select(context.Background(), plain)
	if err == nil || !backend.selected {
		t.Fatalf("plain Select should retain adapter behavior: selected=%v err=%v", backend.selected, err)
	}
}

func TestNativeJoinProviderReceivesCompleteTree(t *testing.T) {
	backend := &nativeJoinTestBackend{ignoringJoinBackend: &ignoringJoinBackend{data: map[string][]record.Record{}, reads: map[string]int{}}}
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !backend.joined || backend.accepted != 1 || len(backend.reads) != 0 {
		t.Fatalf("native route: rows=%d joined=%v accepted=%d scans=%v", len(rows), backend.joined, backend.accepted, backend.reads)
	}
	backend.joined = false
	rsReader, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), joinTestQuery())
	if err != nil {
		t.Fatal(err)
	}
	if !backend.joined || rsReader.Recordset().RowsCount() != 1 || backend.accepted != 2 {
		t.Fatalf("native recordset route: joined=%v rows=%d accepted=%d", backend.joined, rsReader.Recordset().RowsCount(), backend.accepted)
	}
}

func TestGenericJoinReadAndWriteTransactions(t *testing.T) {
	base := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": "b1", "aid": 1, "cid": 9})},
		"C": {joinTestRecord("C", "c1", map[string]any{"id": 9})},
	}, reads: map[string]int{}}
	backend := &joinTransactionBackend{ignoringJoinBackend: base}
	backend.read = &joinTestReadTx{backend: base}
	backend.write = &joinTestWriteTx{backend: base}
	db := NewDB(backend)
	if err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx ReadTransaction) error {
		reader, err := tx.ExecuteQueryToRecordsReader(ctx, joinTestQuery())
		if err != nil {
			return err
		}
		rows, err := ReadAllToRecords(ctx, reader)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return errors.New("readonly transaction lost joined row")
		}
		recordsetReader, err := tx.ExecuteQueryToRecordsetReader(ctx, joinTestQuery())
		if err != nil {
			return err
		}
		if recordsetReader.Recordset().RowsCount() != 1 {
			return errors.New("readonly transaction recordset lost joined row")
		}
		selector := tx.(interface {
			Select(context.Context, Query) (Reader, error)
		})
		selected, err := selector.Select(ctx, joinTestQuery())
		if err != nil {
			return err
		}
		return selected.Close()
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx ReadwriteTransaction) error {
		reader, err := tx.ExecuteQueryToRecordsetReader(ctx, joinTestQuery())
		if err != nil {
			return err
		}
		if reader.Recordset().RowsCount() != 1 {
			return errors.New("readwrite transaction lost joined row")
		}
		selector := tx.(interface {
			Select(context.Context, Query) (Reader, error)
		})
		selected, err := selector.Select(ctx, joinTestQuery())
		if err != nil {
			return err
		}
		return selected.Close()
	}); err != nil {
		t.Fatal(err)
	}
	if backend.read.probes != 3 || backend.write.probes != 2 || base.joined {
		t.Fatalf("tx planner bypass: read=%d write=%d joined=%v", backend.read.probes, backend.write.probes, base.joined)
	}
}

func TestGenericJoinRejectsInvalidKeyBeforeWhere(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "bad", map[string]any{"aid": map[string]any{"x": 1}})},
		"C": {},
	}, reads: map[string]int{}}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_key_type" {
		t.Fatalf("want join_key_type, got %v", err)
	}
}

func TestGenericJoinUnscannableFailsBeforeRows(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": {}}, reads: map[string]int{}}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" {
		t.Fatalf("want join_plan, got %v", err)
	}
}

func TestGenericJoinScanFailureAndCancellation(t *testing.T) {
	base := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1})},
		"B": {}, "C": {},
	}, reads: map[string]int{}}
	backend := &joinReadFailureBackend{ignoringJoinBackend: base, source: "B"}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || diagnostic.Path != "from.joins[0].from" {
		t.Fatalf("scan failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewDB(base).ExecuteQueryToRecordsReader(ctx, joinTestQuery())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan: %v", err)
	}
	base.data["A"] = []record.Record{record.NewRecordWithData(record.NewKeyWithID("A", "bad"), []string{"not", "an", "object"})}
	_, err = NewDB(base).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || diagnostic.Path != "from" {
		t.Fatalf("nonobject row: %v", err)
	}
}

func TestGenericJoinWrappedEOFAndCloseFailure(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	base := &ignoringJoinBackend{data: map[string][]record.Record{"B": {}}, reads: map[string]int{}}
	backend := &joinEOFBackend{ignoringJoinBackend: base}
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil)); err != nil {
		t.Fatalf("wrapped EOF rejected: %v", err)
	}
	backend.closeErr = errors.New("close failed")
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil)); err == nil || !strings.Contains(err.Error(), "close scan a") {
		t.Fatalf("close failure swallowed: %v", err)
	}
}

func TestGenericJoinCorrelatedNestedAndSiblings(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	c := NewRootCollectionRef("C", "c")
	d := NewRootCollectionRef("D", "d")
	child := From(b).Join(NewJoinedSource(c, JoinInner, NewComparison(NewFieldRef("c", "aid"), Equal, NewFieldRef("a", "id"))))
	root := From(a).Join(NewJoinedFrom(child, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	root.Join(NewJoinedSource(d, JoinLeft, NewComparison(NewFieldRef("b", "id"), Equal, NewFieldRef("d", "bid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1}), joinTestRecord("A", "a2", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": 10, "aid": 1}), joinTestRecord("B", "b2", map[string]any{"id": 20, "aid": 2})},
		"C": {joinTestRecord("C", "c1", map[string]any{"aid": 1})},
		"D": {joinTestRecord("D", "d1", map[string]any{"bid": 10}), joinTestRecord("D", "d2", map[string]any{"bid": 10})},
	}, reads: map[string]int{}}
	q := root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("a", "id"), Alias: "aid"}, Column{Expression: NewFieldRef("b", "id"), Alias: "bid"}, Column{Expression: NewFieldRef("d", "bid"), Alias: "dbid"})
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want two matches and one null extension, got %d", len(rows))
	}
	for _, rec := range rows[:2] {
		if got := rec.Data().(map[string]any)["dbid"]; got != float64(10) {
			t.Fatalf("sibling result: %v", rec.Data())
		}
	}
	if got := rows[2].Data().(map[string]any); got["aid"] != float64(2) || got["bid"] != nil || got["dbid"] != nil {
		t.Fatalf("unmatched parent: %v", got)
	}
	if backend.joined || !reflect.DeepEqual(backend.reads, map[string]int{"A": 1, "B": 1, "C": 1, "D": 1}) {
		t.Fatalf("provider calls joined=%v reads=%v", backend.joined, backend.reads)
	}
	hintedChild := From(b).Join(NewJoinedSource(c, JoinInner, NewComparison(NewFieldRef("c", "aid"), Equal, NewFieldRef("a", "id"))).WithAlgorithms(JoinAlgorithmNestedLoop, JoinAlgorithmHash))
	hintedRoot := From(a).Join(NewJoinedFrom(hintedChild, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))).WithAlgorithms(JoinAlgorithmMerge, JoinAlgorithmHash))
	hintedRoot.Join(NewJoinedSource(d, JoinLeft, NewComparison(NewFieldRef("b", "id"), Equal, NewFieldRef("d", "bid"))).WithAlgorithms(JoinAlgorithmLookup, JoinAlgorithmHash))
	hinted := hintedRoot.NewQuery().SelectColumns(q.Columns()...)
	hintedReader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), hinted)
	if err != nil {
		t.Fatal(err)
	}
	hintedRows, err := ReadAllToRecords(context.Background(), hintedReader)
	if err != nil || len(hintedRows) != len(rows) {
		t.Fatalf("hinted recursive rows: %v, %v", hintedRows, err)
	}
	for i := range rows {
		if !reflect.DeepEqual(rows[i].Data(), hintedRows[i].Data()) {
			t.Fatalf("hinted row %d changed: %v vs %v", i, rows[i].Data(), hintedRows[i].Data())
		}
	}
}

func TestGenericJoinQualifiedWildcardAndCollision(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1, "name": "A"})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "name": "B"})},
	}, reads: map[string]int{}, fields: map[string][]string{"A": {"id", "name"}, "B": {"aid", "name"}}}
	q := root.NewQuery().SelectColumns(AllColumnsExceptFrom("a", "name"), AllColumnsExceptFrom("b", "name"))
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Data().(map[string]any); !reflect.DeepEqual(got, map[string]any{"id": float64(1), "aid": float64(1)}) {
		t.Fatalf("wildcard: %v", got)
	}
	q = root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("b", "aid"), Alias: "joinedID"}, AllColumnsExceptFrom("a"))
	columnar, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), q, recordset.WithName("joined"))
	if err != nil {
		t.Fatal(err)
	}
	if got := columnar.Recordset().Name(); got != "joined" {
		t.Fatalf("recordset name = %q", got)
	}
	var names []string
	for _, col := range columnar.Recordset().Columns() {
		names = append(names, col.Name())
	}
	if !reflect.DeepEqual(names, []string{"joinedID", "id", "name"}) {
		t.Fatalf("mixed projection schema order = %v", names)
	}
	changing := &changingJoinFieldsBackend{ignoringJoinBackend: backend}
	if _, err := NewDB(changing).ExecuteQueryToRecordsetReader(context.Background(), q); err == nil || !strings.Contains(err.Error(), "cannot load wildcard fields") {
		t.Fatalf("schema changed after scan: %v", err)
	}
	q = root.NewQuery().SelectColumns(AllColumnsExceptFrom("a"), AllColumnsExceptFrom("b"))
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_field" {
		t.Fatalf("want collision diagnostic, got %v", err)
	}
}

func TestGenericJoinWhereOrderPageAndAggregate(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1}), joinTestRecord("A", "a2", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "value": 3}), joinTestRecord("B", "b2", map[string]any{"aid": 1, "value": 7})},
	}, reads: map[string]int{}}
	q := root.NewQuery().Where(NewComparison(NewFieldRef("b", "value"), GreaterThen, Constant{Value: 0})).OrderBy(Descending(NewFieldRef("b", "value"))).Offset(1).Limit(1).SelectColumns(Column{Expression: NewFieldRef("b", "value"), Alias: "value"})
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Data().(map[string]any)["value"] != float64(3) {
		t.Fatalf("composed rows: %v", rows)
	}
	q = root.NewQuery().SelectColumns(CountAs(Star(), "count"), CountAs(NewFieldRef("b", "value"), "matched"))
	reader, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Data().(map[string]any)["count"] != int64(3) || rows[0].Data().(map[string]any)["matched"] != int64(2) {
		t.Fatalf("aggregate rows: %v", rows)
	}
	q = root.NewQuery().Where(NewComparison(NewFieldRef("a", "id"), In, NewArray([]int{2}))).SelectColumns(Column{Expression: NewFieldRef("a", "id"), Alias: "id"})
	reader, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Data().(map[string]any)["id"] != float64(2) {
		t.Fatalf("IN filter rows: %v", rows)
	}
}

func TestGenericJoinRecordsetRetainsMultiplicity(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "one", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"id": "b1", "aid": 1, "cid": 9}), joinTestRecord("B", "b2", map[string]any{"id": "b2", "aid": 1, "cid": 9})},
		"C": {joinTestRecord("C", "c1", map[string]any{"id": 9})},
	}, reads: map[string]int{}}
	reader, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), joinTestQuery(), recordset.WithName("joined"))
	if err != nil {
		t.Fatal(err)
	}
	if reader.Recordset().RowsCount() != 2 {
		t.Fatalf("want two rows, got %d", reader.Recordset().RowsCount())
	}
	if reader.Recordset().Name() != "joined" {
		t.Fatalf("recordset name = %q", reader.Recordset().Name())
	}
	if backend.joined {
		t.Fatal("provider received joined query")
	}
}

func TestGenericJoinFetchedRowAndByteBounds(t *testing.T) {
	tooMany := make([]record.Record, maxJoinRows+1)
	for i := range tooMany {
		tooMany[i] = joinTestRecord("A", "same", map[string]any{"id": i})
	}
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": tooMany, "B": {}, "C": {}}, reads: map[string]int{}}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" {
		t.Fatalf("row bound: %v", err)
	}
	backend.data["A"] = []record.Record{joinTestRecord("A", "huge", map[string]any{"id": 1, "payload": strings.Repeat("x", maxJoinBytes)})}
	backend.reads = map[string]int{}
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery())
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" {
		t.Fatalf("byte bound: %v", err)
	}
	backend.data["A"] = []record.Record{joinTestRecord("A", "large", map[string]any{"id": 1, "payload": strings.Repeat("x", maxJoinBytes/2+1)})}
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), joinTestQuery().From().NewQuery().SelectIntoRecord(nil))
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" {
		t.Fatalf("output byte bound: %v", err)
	}
}

func TestGenericJoinSameScopeCandidateBound(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("a", "other"))))
	left := make([]record.Record, 400)
	right := make([]record.Record, 400)
	for i := range left {
		left[i] = joinTestRecord("A", "same", map[string]any{"id": 1, "other": 2})
	}
	for i := range right {
		right[i] = joinTestRecord("B", "same", map[string]any{"id": i})
	}
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": left, "B": right}, reads: map[string]int{}}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil))
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || !strings.Contains(diagnostic.Message, "candidate") {
		t.Fatalf("candidate bound: %v", err)
	}
}

func TestGenericJoinResultRowBound(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("a", "id"))))
	left := make([]record.Record, 101)
	right := make([]record.Record, 100)
	for i := range left {
		left[i] = joinTestRecord("A", "same", map[string]any{"id": 1})
	}
	for i := range right {
		right[i] = joinTestRecord("B", "same", map[string]any{"id": i})
	}
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": left, "B": right}, reads: map[string]int{}}
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil))
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || !strings.Contains(diagnostic.Message, "joined row") {
		t.Fatalf("result row bound: %v", err)
	}
}

func TestGenericJoinTypedEqualityAndInvalidKeys(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	q := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid")))).NewQuery().SelectColumns(Column{Expression: NewFieldRef("b", "aid"), Alias: "matched"})
	tests := []struct {
		name        string
		left, right any
		matches     int
		category    string
	}{
		{"integer equals float", 1, 1.0, 1, ""},
		{"negative zero equals zero", math.Copysign(0, -1), 0, 1, ""},
		{"number differs string", 1, "1", 0, ""},
		{"number differs boolean", 1, true, 0, ""},
		{"null never matches", nil, nil, 0, ""},
		{"object rejected", 1, map[string]any{"x": 1}, 0, "join_key_type"},
		{"bytes rejected", 1, []byte{1}, 0, "join_key_type"},
		{"date rejected", 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 0, "join_key_type"},
		{"unsafe integer rejected", 1, int64(9007199254740993), 0, "join_key_type"},
		{"unsafe integral float rejected", 1, float64(9007199254740992), 0, "join_key_type"},
		{"unsafe unsigned integer rejected", 1, uint64(9007199254740992), 0, "join_key_type"},
		{"nonfinite rejected", 1, math.NaN(), 0, "join_key_type"},
		{"infinity rejected", 1, math.Inf(1), 0, "join_key_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &ignoringJoinBackend{data: map[string][]record.Record{
				"A": {joinTestRecord("A", "a", map[string]any{"id": tt.left})},
				"B": {joinTestRecord("B", "b", map[string]any{"aid": tt.right})},
			}, reads: map[string]int{}}
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
			if tt.category != "" {
				var diagnostic *JoinValidationError
				if !errors.As(err, &diagnostic) || diagnostic.Category != tt.category {
					t.Fatalf("want %s, got %v", tt.category, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			rows, err := ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tt.matches {
				t.Fatalf("want %d matches, got %d", tt.matches, len(rows))
			}
		})
	}
}

func TestGenericJoinUnaliasedSourcesAndBaseFieldCompatibility(t *testing.T) {
	a := NewRootCollectionRef("A", "")
	b := NewRootCollectionRef("B", "")
	root := From(a).Join(NewJoinedSource(b, JoinLeft, NewComparison(NewFieldRef("A", "id"), Equal, NewFieldRef("B", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1, "name": "base"})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "name": "joined"})},
	}, reads: map[string]int{}}
	q := root.NewQuery().SelectColumns(Column{Expression: Field("name"), Alias: "baseName"}, Column{Expression: NewFieldRef("B", "name"), Alias: "joinedName"})
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Data().(map[string]any); got["baseName"] != "base" || got["joinedName"] != "joined" {
		t.Fatalf("base field: %v", got)
	}
	reader, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil))
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Data().(map[string]any)["name"]; got != "joined" {
		t.Fatalf("flat later source should overwrite name, got %v", got)
	}
}

func TestGenericJoinGroupHavingAggregatePipeline(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinLeft, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1}), joinTestRecord("A", "a2", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "v": 3}), joinTestRecord("B", "b2", map[string]any{"aid": 1, "v": 3}), joinTestRecord("B", "b3", map[string]any{"aid": 1, "v": 7})},
	}, reads: map[string]int{}}
	value := NewFieldRef("b", "v")
	count := NewAggregate(COUNT, false, Star())
	q := root.NewQuery().GroupBy(NewFieldRef("a", "id")).Having(NewComparison(count, GreaterThen, Constant{Value: 1})).OrderBy(Descending(NewAggregate(SUM, false, value))).SelectColumns(
		Column{Expression: NewFieldRef("a", "id"), Alias: "id"},
		CountAs(Star(), "rows"), CountAs(value, "matches"), CountDistinctAs(value, "distinct"),
		SumAs(value, "sum"), AverageAs(value, "avg"), MinAs(value, "min"), MaxAs(value, "max"), FirstAs(value, "first"), LastAs(value, "last"),
	)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want one group, got %d", len(rows))
	}
	got := rows[0].Data().(map[string]any)
	if got["id"] != float64(1) || got["rows"] != int64(3) || got["matches"] != int64(3) || got["distinct"] != int64(2) || got["sum"] != float64(13) || got["avg"] != float64(13)/3 || got["min"] != float64(3) || got["max"] != float64(7) || got["first"] != float64(3) || got["last"] != float64(7) {
		t.Fatalf("aggregate: %#v", got)
	}
}

func TestGenericJoinQualifiedFieldDiagnostics(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1})},
	}, reads: map[string]int{}, fields: map[string][]string{"A": {"id"}, "B": {"aid"}}}
	tests := []struct {
		name           string
		q              StructuredQuery
		category, path string
	}{
		{"unknown source", root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("missing", "id")}), "join_scope", "columns[0].source"},
		{"known source missing field", root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("b", "absent")}), "join_field", "columns[0].field"},
		{"unknown wildcard", root.NewQuery().SelectColumns(AllColumnsExceptFrom("missing")), "join_scope", "columns[0].source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), tt.q)
			var diagnostic *JoinValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != tt.category || diagnostic.Path != tt.path {
				t.Fatalf("want %s at %s, got %v", tt.category, tt.path, err)
			}
		})
	}
	badOn := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "missing"), Equal, NewFieldRef("b", "aid")))).NewQuery().SelectIntoRecord(nil)
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), badOn)
	var diagnostic *JoinValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_field" || diagnostic.Path != "from.joins[0].on[0].left.field" {
		t.Fatalf("ON missing field: %v", err)
	}
	_, err = NewDB(&failingJoinFieldsBackend{ignoringJoinBackend: backend}).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().SelectIntoRecord(nil))
	if !errors.As(err, &diagnostic) || diagnostic.Category != "join_plan" || diagnostic.Path != "from" {
		t.Fatalf("schema provider failure: %v", err)
	}
}

func TestGenericJoinFilterOperatorsAndExpressions(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "v": 3})},
	}, reads: map[string]int{}}
	v := NewFieldRef("b", "v")
	comparison := func(op Operator, n any) Condition { return NewComparison(v, op, Constant{Value: n}) }
	tests := []struct {
		name    string
		where   Condition
		matches bool
	}{
		{"equal", comparison(Equal, 3), true},
		{"greater", comparison(GreaterThen, 2), true},
		{"greater equal", comparison(GreaterOrEqual, 3), true},
		{"less", comparison(LessThen, 4), true},
		{"less equal", comparison(LessOrEqual, 3), true},
		{"false compare", comparison(GreaterThen, 4), false},
		{"null compare", comparison(Equal, nil), false},
		{"or", NewGroupCondition(Or, comparison(Equal, 4), comparison(Equal, 3)), true},
		{"and", NewGroupCondition(And, comparison(Equal, 3), comparison(GreaterThen, 2)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := root.NewQuery().Where(tt.where).SelectColumns(Column{Expression: v})
			reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			if (len(rows) == 1) != tt.matches {
				t.Fatalf("matches=%v rows=%d", tt.matches, len(rows))
			}
		})
	}
	q := root.NewQuery().SelectColumns(Column{Expression: Binary(v, Multiply, Constant{Value: 2}), Alias: "double"}, Column{Expression: Constant{Value: "ok"}, Alias: "label"})
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Data().(map[string]any); got["double"] != float64(6) || got["label"] != "ok" {
		t.Fatalf("computed projection: %v", got)
	}
	q = root.NewQuery().SelectIntoRecord(nil)
	reader, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Data().(map[string]any); got["id"] != float64(1) || got["aid"] != float64(1) || got["v"] != float64(3) {
		t.Fatalf("flat merge: %v", got)
	}
}

func TestJoinRawFieldPreservesKeyTypesAcrossRecordShapes(t *testing.T) {
	type nested struct {
		ID     int `json:"id"`
		Hidden int
	}
	type invoice struct {
		Customer nested `json:"customer"`
	}
	tests := []struct {
		data any
		path string
		want any
	}{
		{map[string]any{"customer": map[string]any{"id": int64(5)}}, "customer.id", int64(5)},
		{&invoice{Customer: nested{ID: 7}}, "customer.id", 7},
		{&invoice{Customer: nested{ID: 7}}, "customer.missing", nil},
		{map[string]any{"customer": nil}, "customer.id", nil},
		{map[string]any{"customer": []int{1}}, "customer.id", nil},
		{map[int]any{1: "wrong"}, "id", nil},
		{nil, "id", nil},
	}
	for i, tt := range tests {
		if got := rawJoinField(tt.data, tt.path); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("case %d got=%#v want=%#v", i, got, tt.want)
		}
	}
}

func TestJoinRecordNormalizationPreservesPortableNumbers(t *testing.T) {
	records := []struct {
		data      any
		wantError bool
		want      map[string]any
	}{
		{nil, false, map[string]any{}},
		{map[string]any(nil), false, map[string]any{}},
		{map[string]any{"integer": 3, "decimal": 1.25, "nested": []any{1, map[string]any{"n": 2}}}, false, map[string]any{"integer": float64(3), "decimal": 1.25, "nested": []any{float64(1), map[string]any{"n": float64(2)}}}},
		{map[string]any{"large": int64(9007199254740993)}, false, map[string]any{"large": json.Number("9007199254740993")}},
		{map[string]any{"huge": json.Number("1e999")}, false, map[string]any{"huge": json.Number("1e999")}},
		{[]string{"not", "an", "object"}, true, nil},
		{map[string]any{"bad": make(chan int)}, true, nil},
	}
	for i, tt := range records {
		rec := record.NewRecordWithData(record.NewKeyWithID("A", "same"), tt.data)
		got, err := normalizedJoinRecordMap(rec)
		if tt.wantError {
			if err == nil {
				t.Fatalf("case %d: expected error", i)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("case %d got=%#v err=%v want=%#v", i, got, err, tt.want)
		}
	}
}

func TestGenericJoinRejectsUnsupportedQueryExpressions(t *testing.T) {
	a := NewRootCollectionRef("A", "a")
	b := NewRootCollectionRef("B", "b")
	root := From(a).Join(NewJoinedSource(b, JoinInner, NewComparison(NewFieldRef("a", "id"), Equal, NewFieldRef("b", "aid"))))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1}), joinTestRecord("B", "b2", map[string]any{"aid": 1})},
	}, reads: map[string]int{}}
	bad := aggregationCoverageExpression{text: "unknown()"}
	tests := []struct {
		name     string
		query    StructuredQuery
		category string
	}{
		{"projection", root.NewQuery().SelectColumns(Column{Expression: bad, Alias: "bad"}), "join_plan"},
		{"binary left", root.NewQuery().SelectColumns(Column{Expression: Binary(bad, Add, Constant{Value: 1}), Alias: "bad"}), "join_plan"},
		{"binary right", root.NewQuery().SelectColumns(Column{Expression: Binary(Constant{Value: 1}, Add, bad), Alias: "bad"}), "join_plan"},
		{"WHERE left", root.NewQuery().Where(NewComparison(bad, Equal, Constant{Value: 1})).SelectIntoRecord(nil), "join_plan"},
		{"WHERE right", root.NewQuery().Where(NewComparison(Constant{Value: 1}, Equal, bad)).SelectIntoRecord(nil), "join_plan"},
		{"WHERE condition", root.NewQuery().Where(aggregationCoverageCondition{}).SelectIntoRecord(nil), "join_plan"},
		{"WHERE operator", root.NewQuery().Where(NewComparison(NewFieldRef("a", "id"), Operator("??"), Constant{Value: 1})).SelectIntoRecord(nil), "join_plan"},
		{"IN nonarray", root.NewQuery().Where(NewComparison(NewFieldRef("a", "id"), In, Constant{Value: 1})).SelectIntoRecord(nil), "join_plan"},
		{"ORDER", root.NewQuery().OrderBy(Ascending(bad)).SelectIntoRecord(nil), "join_plan"},
		{"nonfinite output", root.NewQuery().SelectColumns(Column{Expression: Constant{Value: math.NaN()}, Alias: "bad"}), "join_plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), tt.query)
			var diagnostic *JoinValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != tt.category {
				t.Fatalf("want %s, got %v", tt.category, err)
			}
		})
	}
}

func TestJoinClauseValidationWithSchemaMetadata(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	base := func(q StructuredQuery, fields map[string][]string) error {
		e := &joinExecution{q: q, aliases: []string{"a", "b"}, fields: fields}
		return e.validateQueryFields()
	}
	good := map[string][]string{"a": {"id", "name"}, "b": {"aid", "label"}}
	bad := NewFieldRef("missing", "id")
	tests := []struct {
		name   string
		q      StructuredQuery
		fields map[string][]string
		want   string
	}{
		{"unknown WHERE", root.NewQuery().Where(NewComparison(bad, Equal, Constant{Value: 1})).SelectIntoRecord(nil), good, "join_scope at where.left.source"},
		{"unknown GROUP", root.NewQuery().GroupBy(bad).SelectIntoRecord(nil), good, "join_scope at groupBy[0].source"},
		{"unknown HAVING", root.NewQuery().GroupBy(NewFieldRef("a", "id")).Having(NewComparison(bad, Equal, Constant{Value: 1})).SelectIntoRecord(nil), good, "join_scope at having.left.source"},
		{"unknown ORDER", root.NewQuery().OrderBy(Ascending(bad)).SelectIntoRecord(nil), good, "join_scope at orderBy[0].source"},
		{"missing field", root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("b", "missing")}), good, "join_field at columns[0].field"},
		{"binary left", root.NewQuery().SelectColumns(Column{Expression: Binary(bad, Add, Constant{Value: 1}), Alias: "value"}), good, "join_scope at columns[0].left.source"},
		{"aggregate argument", root.NewQuery().SelectColumns(Column{Expression: NewAggregate("sum", false, bad), Alias: "value"}), good, "join_scope at columns[0].args[0].source"},
		{"group condition", root.NewQuery().Where(NewGroupCondition(And, NewComparison(bad, Equal, Constant{Value: 1}))).SelectIntoRecord(nil), good, "join_scope at where.conditions[0].left.source"},
		{"empty wildcard source", root.NewQuery().SelectColumns(AllColumnsExceptFrom("", "id")), good, "join_field at columns[0]"},
		{"no wildcard metadata", root.NewQuery().SelectColumns(AllColumnsExceptFrom("a", "id")), map[string][]string{}, "join_plan at columns[0]"},
		{"duplicate wildcard field", root.NewQuery().SelectColumns(AllColumnsExceptFrom("a", "id"), Column{Expression: NewFieldRef("a", "name")}), good, "join_field at columns[1]"},
		{"non-field without alias", root.NewQuery().SelectColumns(Column{Expression: Constant{Value: 1}}), good, "join_shape at columns[0].as"},
		{"duplicate explicit output", root.NewQuery().SelectColumns(Column{Expression: NewFieldRef("a", "id")}, Column{Expression: NewFieldRef("b", "aid"), Alias: "id"}), good, "join_field at columns[1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := base(tt.q, tt.fields); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
		})
	}
}

func TestGenericJoinPaginationCursorAndPlanErrors(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1}), joinTestRecord("A", "a2", map[string]any{"id": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1}), joinTestRecord("B", "b2", map[string]any{"aid": 2})},
	}, reads: map[string]int{}}
	for _, cursor := range []StructuredQuery{
		root.NewQuery().StartFrom("cursor").SelectIntoRecord(nil),
		root.NewQuery().StartAfter("cursor").SelectIntoRecord(nil),
	} {
		if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), cursor); err == nil || !strings.Contains(err.Error(), "join_plan at from") {
			t.Fatalf("cursor was accepted: %v", err)
		}
	}
	invalid := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinRight, joinOn("a", "id", "c", "aid"))).NewQuery().SelectIntoRecord(nil)
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), invalid); err == nil || !strings.Contains(err.Error(), "join_type") {
		t.Fatalf("invalid plan: %v", err)
	}
	for _, offset := range []int{-1, 10} {
		q := root.NewQuery().OrderBy(Descending(NewFieldRef("a", "id"))).Offset(offset).Limit(1).SelectIntoRecord(nil)
		reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := ReadAllToRecords(context.Background(), reader)
		if err != nil {
			t.Fatal(err)
		}
		if offset < 0 && len(rows) != 1 || offset > 0 && len(rows) != 0 {
			t.Fatalf("offset %d returned %d rows", offset, len(rows))
		}
	}
	q := root.NewQuery().OrderBy(Ascending(NewFieldRef("a", "id")), Descending(NewFieldRef("b", "aid"))).SelectIntoRecord(nil)
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q); err != nil {
		t.Fatal(err)
	}
}

func TestGenericJoinRecordsetErrorsAndFlatColumns(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1, "z": "last"})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "a": "first"})},
	}, reads: map[string]int{}}
	flat, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), root.NewQuery().SelectIntoRecordset())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, column := range flat.Recordset().Columns() {
		names = append(names, column.Name())
	}
	if !reflect.DeepEqual(names, []string{"a", "aid", "id", "z"}) {
		t.Fatalf("flat columns = %v", names)
	}
	invalid := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinRight, joinOn("a", "id", "b", "aid"))).NewQuery().SelectIntoRecordset()
	if _, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), invalid); err == nil || !strings.Contains(err.Error(), "join_type") {
		t.Fatalf("invalid recordset plan: %v", err)
	}
	missing := &ignoringJoinBackend{data: map[string][]record.Record{}, reads: map[string]int{}}
	if _, err := NewDB(missing).ExecuteQueryToRecordsetReader(context.Background(), root.NewQuery().SelectIntoRecordset()); err == nil || !strings.Contains(err.Error(), "cannot scan") {
		t.Fatalf("recordset scan failure: %v", err)
	}
}

func TestMaterializedJoinAggregationCapacityAtFinalization(t *testing.T) {
	q := From(NewRootCollectionRef("A", "a")).NewQuery().SelectColumns(CountAs(Star(), "count"))
	plan, err := PlanAggregation(q, QueryCapabilities{StableRowOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	row := joinTestRecord("A", "a1", map[string]any{"id": 1})
	reader := newLocalAggregationReader(context.Background(), q, NewRecordsReader([]record.Record{row}), plan)
	reader.aggregates = make([]AggregateFunc, defaultMaxAggregateStates+1)
	if err := reader.loadMaterialized(); err == nil || !strings.Contains(err.Error(), "aggregate-state limit") {
		t.Fatalf("state bound: %v", err)
	}
	reader = newLocalAggregationReader(context.Background(), q, NewRecordsReader([]record.Record{row}), plan)
	group, err := reader.newGroup("implicit", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	reader.retainedBytes = defaultMaxAggregationBytes - group.bytes - 1
	if err := reader.loadMaterialized(); err == nil || !strings.Contains(err.Error(), "retained aggregation byte limit") {
		t.Fatalf("finalization byte bound: %v", err)
	}
}

func TestJoinedAggregatePlanAndRecordsetReaderFailures(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1, "amount": 1e308}), joinTestRecord("B", "b2", map[string]any{"aid": 1, "amount": 1e308})},
	}, reads: map[string]int{}}
	count := root.NewQuery().SelectColumns(CountAs(Field("id"), "count"))
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), count)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAllToRecords(context.Background(), reader); err != nil {
		t.Fatal(err)
	}
	invalid := root.NewQuery().SelectColumns(CountAs(Star(), "count"), Column{Expression: NewFieldRef("a", "id"), Alias: "id"})
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), invalid); err == nil {
		t.Fatal("invalid aggregate plan accepted")
	}
	sum := root.NewQuery().SelectColumns(SumAs(NewFieldRef("b", "amount"), "sum"))
	if _, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), sum); err == nil {
		t.Fatal("invalid aggregate input reached recordset")
	}
}

func TestJoinSelectFallbackAndRawPrivateField(t *testing.T) {
	base := &ignoringJoinBackend{data: map[string][]record.Record{"A": {joinTestRecord("A", "a1", map[string]any{"id": 1})}}, reads: map[string]int{}}
	db := NewDB(joinBackendWithoutSelect{base: base})
	selector := db.(interface {
		Select(context.Context, Query) (Reader, error)
	})
	reader, err := selector.Select(context.Background(), From(NewRootCollectionRef("A", "a")).NewQuery().SelectIntoRecord(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAllToRecords(context.Background(), reader.(RecordsReader)); err != nil {
		t.Fatal(err)
	}
	type recordData struct {
		hidden int
		ID     int `json:"id"`
	}
	if got := rawJoinField(recordData{hidden: 1, ID: 2}, "hidden"); got != nil {
		t.Fatalf("private field leaked: %v", got)
	}
}

func TestJoinBuildBoundsAndGroupOutcomes(t *testing.T) {
	outer := make([]joinRow, 101)
	candidates := make([]scannedJoinRow, 101)
	for i := range outer {
		outer[i] = joinRow{sources: map[string]map[string]any{"a": {"id": float64(1)}}}
	}
	for i := range candidates {
		candidates[i] = scannedJoinRow{data: map[string]any{"id": float64(1)}}
	}
	e := &joinExecution{scans: map[string][]scannedJoinRow{}}
	if _, err := e.build(From(NewRootCollectionRef("B", "b")), "from", outer, candidates); err == nil || !strings.Contains(err.Error(), "joined row bound") {
		t.Fatalf("base build bound: %v", err)
	}
	child := From(NewRootCollectionRef("B", "b")).Join(NewJoinedSource(NewRootCollectionRef("C", "c"), JoinInner, joinOn("b", "id", "c", "bid")))
	e = &joinExecution{scans: map[string][]scannedJoinRow{"b": candidates, "c": make([]scannedJoinRow, 101)}, indexes: map[string]map[string][]scannedJoinRow{}}
	for i := range e.scans["c"] {
		e.scans["c"][i] = scannedJoinRow{data: map[string]any{"bid": float64(1)}}
	}
	parent := joinRow{sources: map[string]map[string]any{"a": {"id": float64(1)}}}
	if _, err := e.applyJoin([]joinRow{parent}, NewNestedJoinedSource(child, JoinInner, joinOn("a", "id", "b", "id")), "from.joins[0]"); err == nil || !strings.Contains(err.Error(), "joined row bound") {
		t.Fatalf("nested build bound: %v", err)
	}
	row := joinRow{base: "a", sources: map[string]map[string]any{"a": {"id": float64(1)}}}
	for _, condition := range []Condition{
		NewGroupCondition(Or, NewComparison(NewFieldRef("a", "id"), Equal, Constant{Value: 2})),
		NewGroupCondition(And, NewComparison(NewFieldRef("a", "id"), Equal, Constant{Value: 1})),
	} {
		if _, err := evalJoinCondition(condition, row); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJoinOrderEvaluationErrorsAndEqualKeys(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"A": {joinTestRecord("A", "a1", map[string]any{"id": 1, "rank": 1}), joinTestRecord("A", "a2", map[string]any{"id": 1}), joinTestRecord("A", "a3", map[string]any{"id": 1, "rank": 2})},
		"B": {joinTestRecord("B", "b1", map[string]any{"aid": 1})},
	}, reads: map[string]int{}}
	bad := Binary(NewFieldRef("a", "rank"), ArithmeticOperator("??"), Constant{Value: 1})
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().OrderBy(Ascending(bad)).SelectIntoRecord(nil)); err == nil || !strings.Contains(err.Error(), "unsupported arithmetic operator") {
		t.Fatalf("ORDER error: %v", err)
	}
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), root.NewQuery().OrderBy(Ascending(NewFieldRef("a", "id"))).SelectIntoRecord(nil)); err != nil {
		t.Fatal(err)
	}
}

func TestJoinedAggregationOutputByteBound(t *testing.T) {
	root := From(NewRootCollectionRef("A", "a")).Join(NewJoinedSource(NewRootCollectionRef("B", "b"), JoinInner, joinOn("a", "id", "b", "aid")))
	large := strings.Repeat("x", 1900)
	base := make([]record.Record, 5000)
	for i := range base {
		base[i] = joinTestRecord("A", fmt.Sprintf("a%d", i), map[string]any{"id": 1, "payload": large})
	}
	backend := &ignoringJoinBackend{data: map[string][]record.Record{"A": base, "B": {joinTestRecord("B", "b1", map[string]any{"aid": 1})}}, reads: map[string]int{}}
	q := root.NewQuery().SelectColumns(CountAs(Star(), "count"))
	if _, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q); err == nil || !strings.Contains(err.Error(), "joined byte bound") {
		t.Fatalf("aggregate scan/output byte bound: %v", err)
	}
}
