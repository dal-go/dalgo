package dal_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dtql"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type federatedFixtureSource struct {
	name string
	rows []record.Record
}

func (s federatedFixtureSource) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q := query.(dal.StructuredQuery)
	if q.From().Base().Name() != s.name {
		return nil, fmt.Errorf("wrong source")
	}
	rows := s.rows
	if q.Limit() > 0 && len(rows) > q.Limit() {
		rows = rows[:q.Limit()]
	}
	return dal.NewRecordsReader(rows), nil
}
func (federatedFixtureSource) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, fmt.Errorf("unsupported")
}

func TestFederatedQueryAcrossTwoSources(t *testing.T) {
	const document = `from:
  database: orders
  name: Invoice
  alias: o
  scan: {orderBy: [{field: id, desc: true}], limit: 2}
  joins:
    - from: {database: countries, name: Country, alias: c}
      on: [{left: {field: country_id, source: o}, op: '==', right: {field: id, source: c}}]
groupBy: [{field: id, source: c}, {field: population, source: c}]
columns:
  - {field: id, source: c, as: countryId}
  - {aggregate: {function: sum, args: [{field: amount, source: o}]}, as: totalSales}
  - {binary: {op: '/', left: {aggregate: {function: sum, args: [{field: amount, source: o}]}}, right: {field: population, source: c}}, as: salesPerCapita}
`
	query, err := dtql.Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]federatedFixtureSource{
		"orders": {name: "Invoice", rows: []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 3), map[string]any{"id": 3, "country_id": 1, "amount": 10}),
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 2), map[string]any{"id": 2, "country_id": 2, "amount": 20}),
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"id": 1, "country_id": 1, "amount": 999}),
		}},
		"countries": {name: "Country", rows: []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1, "population": 100}),
			record.NewRecordWithData(record.NewKeyWithID("Country", 2), map[string]any{"id": 2, "population": 200}),
		}},
	}
	var progress []dal.FederatedProgress
	reader, err := dal.ExecuteFederatedQueryWithOptions(context.Background(), query, func(_ context.Context, database string) (dal.QueryExecutor, error) {
		source, ok := sources[database]
		if !ok {
			return nil, fmt.Errorf("unknown database")
		}
		return source, nil
	}, dal.FederatedQueryOptions{OnProgress: func(item dal.FederatedProgress) { progress = append(progress, item) }})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(progress) == 0 {
		t.Fatalf("rows=%d progress=%v", len(rows), progress)
	}
	for _, row := range rows {
		data := row.Data().(map[string]any)
		if data["salesPerCapita"] != 0.1 {
			t.Fatalf("unexpected row: %v", data)
		}
	}
}

func TestFederatedMoneyFromDTQL(t *testing.T) {
	const document = `from:
  database: orders
  name: Invoice
  alias: o
  joins:
    - from: {database: countries, name: Country, alias: c}
      on: [{left: {field: country_id, source: o}, op: '==', right: {field: id, source: c}}]
groupBy: [{field: id, source: c}, {field: population, source: c}]
money: {minorUnitScale: 2, divisionScale: 4, rounding: halfEven}
columns:
  - {aggregate: {function: sum, args: [{field: amount, source: o}]}, as: totalSales}
  - {aggregate: {function: avg, args: [{field: amount, source: o}]}, as: avgSale}
  - {binary: {op: '/', left: {aggregate: {function: sum, args: [{field: amount, source: o}]}}, right: {field: population, source: c}}, as: salesPerCapita}
`
	query, err := dtql.Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := dtql.Serialize(query)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "minorUnitScale: 2") {
		t.Fatal("lost money policy")
	}
	sources := map[string]federatedFixtureSource{
		"orders": {name: "Invoice", rows: []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"id": 1, "country_id": 1, "amount": "0.10"}),
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 2), map[string]any{"id": 2, "country_id": 1, "amount": "0.20"}),
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 3), map[string]any{"id": 3, "country_id": 1, "amount": "9007199254740993"}),
		}},
		"countries": {name: "Country", rows: []record.Record{record.NewRecordWithData(record.NewKeyWithID("Country", 1), map[string]any{"id": 1, "population": 3})}},
	}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(_ context.Context, database string) (dal.QueryExecutor, error) {
		return sources[database], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	data := rows[0].Data().(map[string]any)
	for key, want := range map[string]string{"totalSales": "9007199254740993.3", "avgSale": "3002399751580331.1", "salesPerCapita": "3002399751580331.1"} {
		if data[key] != want {
			t.Fatalf("%s=%v want %s", key, data[key], want)
		}
	}
}

func TestFederatedMoneySingleSourceExactProductsWhereAndOrder(t *testing.T) {
	const document = `from: {database: orders, name: Invoice, alias: o}
money: {minorUnitScale: 2, divisionScale: 2, rounding: halfEven}
columns:
  - {aggregate: {function: sum, args: [{binary: {op: '*', left: {field: price, source: o}, right: {field: quantity, source: o}}}]}, as: total}
  - {aggregate: {function: min, args: [{field: price, source: o}]}, as: cheapest}
  - {aggregate: {function: max, args: [{field: price, source: o}]}, as: priciest}
where: {left: {field: price, source: o}, op: '>', right: {value: '9'}}
`
	query, err := dtql.Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	source := federatedFixtureSource{name: "Invoice", rows: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("Invoice", "1"), map[string]any{"price": "9", "quantity": "100"}),
		record.NewRecordWithData(record.NewKeyWithID("Invoice", "2"), map[string]any{"price": "10.00", "quantity": "0.2"}),
		record.NewRecordWithData(record.NewKeyWithID("Invoice", "3"), map[string]any{"price": "9007199254740993", "quantity": "1"}),
	}}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(context.Context, string) (dal.QueryExecutor, error) { return source, nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	got := rows[0].Data().(map[string]any)
	for key, want := range map[string]string{"total": "9007199254740995", "cheapest": "10.00", "priciest": "9007199254740993"} {
		if got[key] != want {
			t.Errorf("%s=%v want %s", key, got[key], want)
		}
	}
}

func TestFederatedMoneyKeepsTextDimensionsLexical(t *testing.T) {
	const document = `from: {database: customers, name: Customer, alias: c}
groupBy: [{field: customerName, source: c}]
money: {minorUnitScale: 2, divisionScale: 2, rounding: halfEven}
columns:
  - {field: customerName, source: c}
  - {aggregate: {function: count, distinct: true, args: [{field: label, source: c}]}, as: labels}
  - {aggregate: {function: min, args: [{field: label, source: c}]}, as: firstLabel}
  - {aggregate: {function: sum, args: [{field: amount, source: c}]}, as: total}
where: {left: {field: customerName, source: c}, op: '==', right: {value: Bob}}
`
	query, err := dtql.Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	source := federatedFixtureSource{name: "Customer", rows: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("Customer", "1"), map[string]any{"customerName": "Bob", "label": "zeta", "amount": "1.00"}),
		record.NewRecordWithData(record.NewKeyWithID("Customer", "2"), map[string]any{"customerName": "Bob", "label": "alpha", "amount": "2.00"}),
		record.NewRecordWithData(record.NewKeyWithID("Customer", "3"), map[string]any{"customerName": "007", "label": "numeric-looking", "amount": "9.00"}),
	}}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(context.Context, string) (dal.QueryExecutor, error) { return source, nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	got := rows[0].Data().(map[string]any)
	for key, want := range map[string]any{"customerName": "Bob", "labels": int64(2), "firstLabel": "alpha", "total": "3"} {
		if got[key] != want {
			t.Errorf("%s=%v want %v", key, got[key], want)
		}
	}
}

func TestFederatedMoneyOrderByUsesNumericDecimalOrder(t *testing.T) {
	const document = `from: {database: orders, name: Invoice, alias: o}
groupBy: [{field: price, source: o}]
orderBy: [{field: price, source: o}]
money: {minorUnitScale: 2, divisionScale: 2, rounding: halfEven}
columns:
  - {field: price, source: o}
  - {aggregate: {function: sum, args: [{field: amount, source: o}]}, as: total}
`
	query, err := dtql.Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	source := federatedFixtureSource{name: "Invoice", rows: []record.Record{
		record.NewRecordWithData(record.NewKeyWithID("Invoice", "1"), map[string]any{"price": "10", "amount": "1"}),
		record.NewRecordWithData(record.NewKeyWithID("Invoice", "2"), map[string]any{"price": "9", "amount": "2"}),
	}}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(context.Context, string) (dal.QueryExecutor, error) { return source, nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0].Data().(map[string]any)["price"] != "9" || rows[1].Data().(map[string]any)["price"] != "10" {
		t.Fatalf("numeric order = %#v, %#v", rows[0].Data(), rows[1].Data())
	}
}
