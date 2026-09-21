package dal

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/record"
)

func TestGenericRecursiveExistsAndScalar(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1}), joinTestRecord("Customer", "2", map[string]any{"id": 2})},
		"Invoice":  {joinTestRecord("Invoice", "a", map[string]any{"customer": 1, "total": 9})},
	}, reads: map[string]int{}}
	invoice := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "customer"), Equal, NewFieldRef("c", "id"))).SelectColumns(Column{Expression: NewFieldRef("i", "total")})
	query := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(invoice)).SelectColumns(
		Column{Expression: NewFieldRef("c", "id")},
		Column{Expression: NewQueryExpression(invoice, "total")},
	)
	reader, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	records, err := ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	data := records[0].Data().(map[string]any)
	if data["id"] != float64(1) || data["total"] != float64(9) {
		t.Fatalf("row = %#v", data)
	}
	if backend.reads["Invoice"] < 2 {
		t.Fatalf("nested leaf scans = %d", backend.reads["Invoice"])
	}
}

func TestGenericRecursiveScalarDiagnosticsAndEmptyRecordsetShape(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
		"Invoice": {
			joinTestRecord("Invoice", "a", map[string]any{"id": 1, "total": 9}),
			joinTestRecord("Invoice", "b", map[string]any{"id": 2, "total": 8}),
		},
	}, reads: map[string]int{}}
	outer := From(NewRootCollectionRef("Customer", "c")).NewQuery()
	rows := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	manyRows := outer.SelectColumns(Column{Expression: NewQueryExpression(rows, "invoice")})
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), manyRows)
	var validation *QueryValidationError
	if !errors.As(err, &validation) || validation.Category != "cardinality" || validation.Path != "columns[0].query" {
		t.Fatalf("many-row scalar error = %#v", err)
	}
	manyColumns := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("i", "id"), Equal, Constant{Value: 1})).SelectColumns(
		Column{Expression: NewFieldRef("i", "id")},
		Column{Expression: NewFieldRef("i", "total")},
	)
	_, err = NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), outer.SelectColumns(Column{Expression: NewQueryExpression(manyColumns, "invoice")}))
	if !errors.As(err, &validation) || validation.Category != "shape" || validation.Path != "columns[0].query.columns" {
		t.Fatalf("many-column scalar error = %#v", err)
	}
	empty := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewNotExistsCondition(rows)).SelectColumns(
		Column{Expression: NewFieldRef("c", "id")},
		Column{Expression: NewQueryExpression(rows, "invoice")},
	)
	recordsetReader, err := NewDB(backend).ExecuteQueryToRecordsetReader(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	defer recordsetReader.Close()
	var names []string
	for _, column := range recordsetReader.Recordset().Columns() {
		names = append(names, column.Name())
	}
	if !reflect.DeepEqual(names, []string{"id", "invoice"}) {
		t.Fatalf("empty recordset columns = %v", names)
	}
}

func TestGenericRecursiveValidatesScopeBeforeLeafScan(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"id": 1})},
	}, reads: map[string]int{}}
	invalid := From(NewRootCollectionRef("Customer", "c")).NewQuery().Where(NewExistsCondition(
		From(NewRootCollectionRef("Customer", "inner")).NewQuery().Where(NewComparison(NewFieldRef("inner", "id"), Equal, NewFieldRef("missing", "id"))).SelectIntoRecord(nil),
	)).SelectIntoRecord(nil)
	if _, err := ExecuteRecursiveQuery(context.Background(), backend, invalid); err == nil {
		t.Fatal("invalid recursive scope succeeded")
	}
	if len(backend.reads) != 0 {
		t.Fatalf("invalid scope performed leaf scans: %#v", backend.reads)
	}
}

func TestGenericRecursiveBindsAmbiguousUnqualifiedFieldWithSchema(t *testing.T) {
	backend := &ignoringJoinBackend{data: map[string][]record.Record{
		"Customer": {joinTestRecord("Customer", "1", map[string]any{"CustomerId": 1})},
		"Ledger":   {joinTestRecord("Ledger", "1", map[string]any{"CustomerId": 1})},
	}, fields: map[string][]string{
		"Customer": {"CustomerId"},
		"Ledger":   {"CustomerId"},
	}, reads: map[string]int{}}
	ledger := NewRootCollectionRef("Ledger", "l")
	from := From(NewRootCollectionRef("Customer", "c")).Join(NewJoinedSource(ledger, JoinInner,
		NewComparison(NewFieldRef("c", "CustomerId"), Equal, NewFieldRef("l", "CustomerId"))))
	// The EXISTS node selects the recursive evaluator; the unqualified output
	// must then be resolved from supplied schema rather than guessed as c.
	query := from.NewQuery().Where(NewExistsCondition(From(NewRootCollectionRef("Customer", "e")).NewQuery().SelectIntoRecord(nil))).SelectColumns(
		Column{Expression: NewFieldRef("", "CustomerId")},
	)
	_, err := NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
	var diagnostic *QueryValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Category != "scope" || diagnostic.Path != "columns[0]" {
		t.Fatalf("ambiguous field diagnostic = %v", err)
	}
}
