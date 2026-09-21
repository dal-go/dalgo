package dtql

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// This provider intentionally ignores JOINs. The DALgo executor must pass it
// one source scan at a time, even when the DTQL tree carries physical hints.
type chinookHintScanBackend struct {
	dal.Backend
	data  map[string][]record.Record
	reads map[string]int
}

func (b *chinookHintScanBackend) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	structured := query.(dal.StructuredQuery)
	if len(structured.From().Joins()) != 0 {
		return nil, errors.New("joined query leaked to source provider")
	}
	source := structured.From().Base()
	b.reads[source.Alias()]++
	return dal.NewRecordsReader(b.data[source.Name()]), nil
}

func chinookHintRecord(table, id string, fields map[string]any) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID(table, id), fields)
}

func TestCanonicalHintedChinookExecutesLikeOriginal(t *testing.T) {
	fixtures := map[string][]byte{}
	for _, name := range []string{"chinook-nested.dtql.yaml", "chinook-hinted.dtql.yaml"} {
		data, err := os.ReadFile("testdata/joins/" + name)
		if err != nil {
			t.Fatal(err)
		}
		fixtures[name] = data
	}
	expectedJSON, err := os.ReadFile("testdata/joins/chinook-nested.rows.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []map[string]any
	if err := json.Unmarshal(expectedJSON, &expected); err != nil {
		t.Fatal(err)
	}
	data := map[string][]record.Record{
		"Invoice": {
			chinookHintRecord("Invoice", "i1", map[string]any{"InvoiceId": 1, "CustomerId": 10}),
			chinookHintRecord("Invoice", "i2", map[string]any{"InvoiceId": 2, "CustomerId": 10}),
			chinookHintRecord("Invoice", "i3", map[string]any{"InvoiceId": 3, "CustomerId": 20}),
			chinookHintRecord("Invoice", "i4", map[string]any{"InvoiceId": 4, "CustomerId": 1}),
		},
		"Customer": {
			chinookHintRecord("Customer", "c10", map[string]any{"CustomerId": 10, "FirstName": "Ada", "SupportRepId": 7}),
			chinookHintRecord("Customer", "c20", map[string]any{"CustomerId": 20, "FirstName": "Bea", "SupportRepId": nil}),
			chinookHintRecord("Customer", "c1", map[string]any{"CustomerId": 1, "FirstName": "Ignored", "SupportRepId": 7}),
		},
		"Employee": {chinookHintRecord("Employee", "e7", map[string]any{"EmployeeId": 7, "FirstName": "Evan"})},
	}
	for _, name := range []string{"chinook-nested.dtql.yaml", "chinook-hinted.dtql.yaml"} {
		t.Run(name, func(t *testing.T) {
			query, err := Deserialize(fixtures[name])
			if err != nil {
				t.Fatal(err)
			}
			backend := &chinookHintScanBackend{data: data, reads: map[string]int{}}
			reader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			records, err := dal.ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			rows := make([]map[string]any, len(records))
			for i, item := range records {
				rows[i] = item.Data().(map[string]any)
			}
			if !reflect.DeepEqual(rows, expected) {
				t.Fatalf("canonical ordered rows = %#v, want %#v", rows, expected)
			}
			wantReads := map[string]int{"i": 1, "c": 1, "e": 1}
			if name == "chinook-hinted.dtql.yaml" {
				wantReads["c2"] = 1
			}
			if !reflect.DeepEqual(backend.reads, wantReads) {
				t.Fatalf("source reads = %v, want %v", backend.reads, wantReads)
			}
		})
	}
}
