package dal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type federatedStub struct {
	rows []record.Record
	err  error
}

type federatedFieldsStub struct{ federatedStub }

func (federatedFieldsStub) JoinFields(context.Context, RecordsetSource) ([]string, error) {
	return []string{"id"}, nil
}

func (s federatedStub) ExecuteQueryToRecordsReader(context.Context, Query) (RecordsReader, error) {
	if s.err != nil {
		return nil, s.err
	}
	return NewRecordsReader(s.rows), nil
}
func (s federatedStub) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, s.err
}

func TestFederatedSimpleRoutingAndProgress(t *testing.T) {
	ctx := context.Background()
	ref := NewDatabaseCollectionRef("orders", "", "Invoice", "")
	rows := make([]record.Record, 1024)
	for i := range rows {
		rows[i] = record.NewRecordWithData(record.NewKeyWithID("Invoice", i), map[string]any{"id": i})
	}
	var updates []FederatedProgress
	resolve := func(_ context.Context, database string) (QueryExecutor, error) {
		if database != "orders" {
			t.Fatalf("database=%q", database)
		}
		return federatedStub{rows: rows}, nil
	}
	query := From(ref).NewQuery().SelectIntoRecord(nil)
	reader, err := ExecuteFederatedQueryWithOptions(ctx, query, resolve, FederatedQueryOptions{OnProgress: func(item FederatedProgress) { updates = append(updates, item) }})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadAllToRecords(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1024 || len(updates) < 2 || updates[len(updates)-1].Rows != 1024 {
		t.Fatalf("rows=%d progress=%v", len(got), updates)
	}
	reader, err = ExecuteFederatedQuery(ctx, query, resolve)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
}

func TestFederatedValidationAndExecutorErrors(t *testing.T) {
	ctx := context.Background()
	resolve := func(context.Context, string) (QueryExecutor, error) {
		return federatedStub{err: errors.New("offline")}, nil
	}
	query := From(NewDatabaseCollectionRef("orders", "", "Invoice", "")).NewQuery().SelectIntoRecord(nil)
	for _, input := range []struct {
		query   StructuredQuery
		resolve DatabaseResolver
	}{{nil, resolve}, {query, nil}} {
		if _, err := ExecuteFederatedQuery(ctx, input.query, input.resolve); err == nil {
			t.Fatal("expected validation error")
		}
	}
	if _, err := ExecuteFederatedQuery(ctx, query, resolve); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("error=%v", err)
	}
	if _, err := ExecuteFederatedQuery(ctx, From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectIntoRecord(nil), resolve); err == nil {
		t.Fatal("expected missing database")
	}
	routed := federatedQueryExecutor{resolve: resolve}
	if _, err := routed.executor(ctx, nil); err == nil {
		t.Fatal("expected non-query error")
	}
	if _, err := routed.sourceExecutor(ctx, NewRootCollectionRef("Invoice", "")); err == nil {
		t.Fatal("expected missing database")
	}
	ref := NewDatabaseCollectionRef("orders", "", "Invoice", "")
	if _, err := routed.sourceExecutor(ctx, &ref); err != nil {
		t.Fatal(err)
	}
	if _, err := routed.ExecuteQueryToRecordsetReader(ctx, query); err == nil {
		t.Fatal("expected recordset error")
	}
	if _, err := routed.ExecuteQueryToRecordsetReader(ctx, From(NewRootCollectionRef("Invoice", "")).NewQuery().SelectIntoRecord(nil)); err == nil {
		t.Fatal("expected recordset source error")
	}
	if _, err := routed.JoinFields(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := routed.JoinFields(ctx, NewRootCollectionRef("Invoice", "")); err == nil {
		t.Fatal("expected join-fields source error")
	}
	withFields := federatedQueryExecutor{resolve: func(context.Context, string) (QueryExecutor, error) { return federatedFieldsStub{}, nil }}
	if fields, err := withFields.JoinFields(ctx, ref); err != nil || len(fields) != 1 {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
}

func TestFederatedGenericJoinProgressAndError(t *testing.T) {
	ctx := context.Background()
	root := NewDatabaseCollectionRef("orders", "", "Invoice", "o")
	child := NewDatabaseCollectionRef("countries", "", "Country", "c")
	query := From(root).Join(NewJoinedSource(child, JoinInner, joinOn("o", "country_id", "c", "id"))).NewQuery().OrderBy(Ascending(NewFieldRef("o", "id"))).SelectIntoRecord(nil)
	orders := []record.Record{record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"id": 1, "country_id": 1})}
	countries := []record.Record{record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1})}
	resolve := func(_ context.Context, database string) (QueryExecutor, error) {
		if database == "orders" {
			return federatedStub{rows: orders}, nil
		}
		return federatedStub{rows: countries}, nil
	}
	var progress []FederatedProgress
	reader, err := ExecuteFederatedQueryWithOptions(ctx, query, resolve, FederatedQueryOptions{OnProgress: func(item FederatedProgress) { progress = append(progress, item) }})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAllToRecords(ctx, reader)
	if err != nil || len(rows) != 1 || len(progress) == 0 {
		t.Fatalf("rows=%d progress=%v err=%v", len(rows), progress, err)
	}
	_, err = ExecuteFederatedQuery(ctx, query, func(_ context.Context, database string) (QueryExecutor, error) {
		if database == "countries" {
			return federatedStub{err: errors.New("offline")}, nil
		}
		return federatedStub{rows: orders}, nil
	})
	if err == nil {
		t.Fatal("expected generic source error")
	}
}

func TestFederatedSourceScanAndProgressReaderErrors(t *testing.T) {
	ctx := context.Background()
	ref := NewDatabaseCollectionRef("orders", "", "Invoice", "").WithScan(2, DescendingField("id"))
	query := From(ref).NewQuery().Limit(1).SelectIntoRecord(nil)
	resolve := func(context.Context, string) (QueryExecutor, error) {
		return federatedStub{rows: []record.Record{record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"id": 1})}}, nil
	}
	reader, err := ExecuteFederatedQuery(ctx, query, resolve)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	query = From(ref).NewQuery().WhereField("id", Equal, 1).SelectIntoRecord(nil)
	reader, err = ExecuteFederatedQuery(ctx, query, resolve)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	wrapped := &federatedProgressReader{RecordsReader: joinFailingReader{err: errors.New("read failed")}, report: func(FederatedProgress) {}}
	if _, err := wrapped.Next(); err == nil {
		t.Fatal("expected read failure")
	}
	if _, err := wrapped.Cursor(); err != nil {
		t.Fatal(err)
	}
}

func TestNewDatabaseCollectionRefRequiresDatabase(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	NewDatabaseCollectionRef("", "", "Invoice", "")
}
