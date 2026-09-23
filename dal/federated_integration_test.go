package dal_test

import (
	"context"
	"fmt"
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
