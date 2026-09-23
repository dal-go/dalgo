package dtql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type salesDatabase struct {
	name  string
	rows  []record.Record
	reads int
	limit int
}

func TestFederatedSalesStreams120000Rows(t *testing.T) {
	document := strings.Replace(`from:
  database: orders
  name: Invoice
  alias: o
  scan: {orderBy: [{field: id, desc: true}], limit: 100}
  joins:
    - from: {database: countries, name: Country, alias: c}
      on: [{left: {field: country_id, source: o}, op: '==', right: {field: id, source: c}}]
groupBy: [{field: id, source: c}, {field: name, source: c}, {field: population, source: c}]
columns:
  - {field: name, source: c, as: country}
  - {aggregate: {function: sum, args: [{field: amount, source: o}]}, as: totalSales}
  - {binary: {op: '/', left: {aggregate: {function: sum, args: [{field: amount, source: o}]}}, right: {field: population, source: c}}, as: salesPerCapita}
`, "  scan: {orderBy: [{field: id, desc: true}], limit: 100}\n", "", 1)
	query, err := Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	orders := &salesDatabase{name: "Invoice"}
	for id := 1; id <= 120000; id++ {
		orders.rows = append(orders.rows, record.NewRecordWithData(record.NewKeyWithID("Invoice", id), map[string]any{"id": id, "country_id": id%2 + 1, "amount": 10}))
	}
	countries := &salesDatabase{name: "Country", rows: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1, "name": "Alpha", "population": 100}),
		record.NewRecordWithData(record.NewKeyWithID("Country", 2), map[string]any{"id": 2, "name": "Beta", "population": 200}),
	}}
	var downloaded, processed int64
	reader, err := dal.ExecuteFederatedQueryWithOptions(context.Background(), query, func(_ context.Context, database string) (dal.QueryExecutor, error) {
		if database == "orders" {
			return orders, nil
		}
		return countries, nil
	}, dal.FederatedQueryOptions{OnProgress: func(item dal.FederatedProgress) {
		if item.Phase == "download" && item.Database == "orders" {
			downloaded = item.Rows
		}
		if item.Phase == "process" {
			processed = item.Rows
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || downloaded != 120000 || processed != 120000 {
		t.Fatalf("rows=%d downloaded=%d processed=%d", len(got), downloaded, processed)
	}
}

func (db *salesDatabase) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q := query.(dal.StructuredQuery)
	if q.From().Base().Name() != db.name || len(q.From().Joins()) != 0 {
		return nil, fmt.Errorf("wrong database for %s", q.From().Base().Name())
	}
	db.reads++
	db.limit = q.Limit()
	rows := append([]record.Record(nil), db.rows...)
	if db.name == "Invoice" {
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Data().(map[string]any)["id"].(int) > rows[j].Data().(map[string]any)["id"].(int)
		})
	}
	if db.limit > 0 && len(rows) > db.limit {
		rows = rows[:db.limit]
	}
	return dal.NewRecordsReader(rows), nil
}

func (*salesDatabase) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, fmt.Errorf("recordsets unsupported")
}

func TestFederatedSalesPerCapitaLatest100(t *testing.T) {
	const document = `from:
  database: orders
  name: Invoice
  alias: o
  scan: {orderBy: [{field: id, desc: true}], limit: 100}
  joins:
    - from: {database: countries, name: Country, alias: c}
      on: [{left: {field: country_id, source: o}, op: '==', right: {field: id, source: c}}]
groupBy: [{field: id, source: c}, {field: name, source: c}, {field: population, source: c}]
columns:
  - {field: name, source: c, as: country}
  - {aggregate: {function: sum, args: [{field: amount, source: o}]}, as: totalSales}
  - {binary: {op: '/', left: {aggregate: {function: sum, args: [{field: amount, source: o}]}}, right: {field: population, source: c}}, as: salesPerCapita}
`
	query, err := Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	orders := &salesDatabase{name: "Invoice"}
	for id := 1; id <= 102; id++ {
		amount := 20
		if id%2 == 0 {
			amount = 10
		}
		if id <= 2 {
			amount = 999
		}
		orders.rows = append(orders.rows, record.NewRecordWithData(record.NewKeyWithID("Invoice", id), map[string]any{"id": id, "country_id": id%2 + 1, "amount": amount}))
	}
	countries := &salesDatabase{name: "Country", rows: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1, "name": "Alpha", "population": 100}),
		record.NewRecordWithData(record.NewKeyWithID("Country", 2), map[string]any{"id": 2, "name": "Beta", "population": 200}),
	}}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(_ context.Context, database string) (dal.QueryExecutor, error) {
		switch database {
		case "orders":
			return orders, nil
		case "countries":
			return countries, nil
		default:
			return nil, fmt.Errorf("unknown database %q", database)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d countries", len(got))
	}
	byName := map[string]map[string]any{}
	for _, row := range got {
		data := row.Data().(map[string]any)
		byName[data["country"].(string)] = data
	}
	for name, want := range map[string]float64{"Alpha": 500, "Beta": 1000} {
		data := byName[name]
		if data == nil || data["totalSales"] != want || data["salesPerCapita"] != float64(5) {
			t.Errorf("%s: %v", name, data)
		}
	}
	if orders.reads != 1 || orders.limit != 100 || countries.reads != 1 {
		t.Fatalf("routing: orders=%d/%d countries=%d", orders.reads, orders.limit, countries.reads)
	}
	serialized, err := Serialize(query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Deserialize(serialized); err != nil {
		t.Fatal(err)
	}
}
