package tugql

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"gopkg.in/yaml.v3"
)

const crossDatabaseSample = `databases (
  sales = 'datatug://chinook/sales?env=prod'
  geo
)
parameters (
  @MinSales integer default 1000
)
with BigInvoices as (
  from sales::Invoice as i
  where i.Total > 10
  select (
    i.InvoiceId
    i.BillingCountry
    i.Total
    CountryInvoices as (
      from sales::Invoice as x
      where x.BillingCountry = i.BillingCountry
      select COUNT(*) as N
    )
  )
)
from BigInvoices as b
join geo::Country as c
  on b.BillingCountry = c.Name
group by c.Name, b.CountryInvoices
having SUM(b.Total) >= @MinSales
select (
  c.Name as Country
  SUM(b.Total) as TotalSales
  b.CountryInvoices as InvoiceCount
)`

func crossDatabaseSchemas() []TugQLAuthorizedSchema {
	return []TugQLAuthorizedSchema{
		{Database: "sales", Version: "sales-v1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{
			{Name: "InvoiceId", Type: "integer", Authorized: true},
			{Name: "BillingCountry", Type: "string", Authorized: true},
			{Name: "Total", Type: "integer", Authorized: true},
		}}}},
		{Database: "geo", Version: "geo-v1", Tables: []TugQLTable{{Name: "Country", Fields: []TugQLField{
			{Name: "Name", Type: "string", Authorized: true},
		}}}},
	}
}

// crossDatabaseExecutor is one in-memory database. It serves plain scans of
// its own tables only, so a source routed to the wrong database fails.
type crossDatabaseExecutor struct {
	name   string
	tables map[string][]record.Record
	reads  map[string]int
}

func (e *crossDatabaseExecutor) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q, ok := query.(dal.StructuredQuery)
	if !ok || len(q.From().Joins()) != 0 {
		return nil, errors.New("cross-database executor received a non-leaf query")
	}
	name := q.From().Base().Name()
	rows, found := e.tables[name]
	if !found {
		return nil, fmt.Errorf("database %s has no table %s", e.name, name)
	}
	e.reads[name]++
	return dal.NewRecordsReader(rows), nil
}

func (*crossDatabaseExecutor) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("recordsets unsupported")
}

func newCrossDatabaseExecutor(name, table string, rows ...map[string]any) *crossDatabaseExecutor {
	e := &crossDatabaseExecutor{name: name, tables: map[string][]record.Record{}, reads: map[string]int{}}
	for i, data := range rows {
		e.tables[table] = append(e.tables[table], record.NewRecordWithData(record.NewKeyWithID(table, i+1), data))
	}
	return e
}

func TestTugQLCrossDatabaseExecutesThroughFederatedQuery(t *testing.T) {
	doc, diagnostics := ParseTugQL(crossDatabaseSample)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %+v", diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{
		AuthorizedSchemas: crossDatabaseSchemas(),
		Bindings:          []TugQLBinding{{Name: "MinSales", Set: true, Value: 100}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %+v", diagnostics)
	}
	if resolved.SchemaVersion != "geo-v1,sales-v1" {
		t.Fatalf("schema version = %q", resolved.SchemaVersion)
	}
	sales := newCrossDatabaseExecutor("sales", "Invoice",
		map[string]any{"InvoiceId": 1, "BillingCountry": "USA", "Total": 600},
		map[string]any{"InvoiceId": 2, "BillingCountry": "USA", "Total": 500},
		map[string]any{"InvoiceId": 3, "BillingCountry": "Canada", "Total": 5},
		map[string]any{"InvoiceId": 4, "BillingCountry": "Canada", "Total": 900},
		map[string]any{"InvoiceId": 5, "BillingCountry": "Norway", "Total": 50},
	)
	geo := newCrossDatabaseExecutor("geo", "Country",
		map[string]any{"Name": "USA"}, map[string]any{"Name": "Canada"}, map[string]any{"Name": "Norway"}, map[string]any{"Name": "Brazil"},
	)
	var routed []string
	reader, err := dal.ExecuteFederatedQuery(context.Background(), resolved.Query, func(_ context.Context, database string) (dal.QueryExecutor, error) {
		routed = append(routed, database)
		switch database {
		case "sales":
			return sales, nil
		case "geo":
			return geo, nil
		}
		return nil, fmt.Errorf("unknown database %q", database)
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range rows {
		data := row.Data().(map[string]any)
		got = append(got, fmt.Sprintf("%v:%v:%v", data["Country"], data["TotalSales"], data["InvoiceCount"]))
	}
	sort.Strings(got)
	if fmt.Sprint(got) != "[Canada:900:2 USA:1100:2]" {
		t.Fatalf("rows = %v", got)
	}
	if sales.reads["Invoice"] == 0 || geo.reads["Country"] == 0 {
		t.Fatalf("routing: sales=%v geo=%v routed=%v", sales.reads, geo.reads, routed)
	}
}

func TestTugQLCrossDatabaseTreeEdges(t *testing.T) {
	invalid := TugQLTree{Format: "tugqtree", Version: 1, Databases: []TugQLDatabase{{Name: "select"}}, Query: map[string]any{"from": map[string]any{"name": "Invoice"}}}
	if _, err := yaml.Marshal(invalid); err == nil || !strings.Contains(err.Error(), "reserved keyword") {
		t.Fatalf("yaml.Marshal accepted an invalid declaration: %v", err)
	}
	cte := TugQLTree{Format: "tugqtree", Version: 1, Databases: []TugQLDatabase{{Name: "sales"}},
		Definitions: []TugQLDefinition{{Kind: "cte", Name: "C", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"database": "geo", "name": "Country"}}}}},
		Query:       map[string]any{"from": map[string]any{"database": "sales", "name": "C"}}}
	if _, diagnostics := PrintTugQL(cte); len(diagnostics) != 1 || diagnostics[0].Code != "undeclared_database" || !strings.Contains(diagnostics[0].Message, `"geo"`) {
		t.Fatalf("undeclared CTE source: %+v", diagnostics)
	}
	many := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "Invoice"}}}
	for i := 0; i <= maxTugQLSemanticNodes; i++ {
		many.Databases = append(many.Databases, TugQLDatabase{Name: fmt.Sprintf("d%d", i)})
	}
	if _, diagnostics := ResolveTugQL(tugqlTestDocument(&many), TugQLResolveContext{}); len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" {
		t.Fatalf("declaration budget: %+v", diagnostics)
	}
	// An ordered aggregate argument is walked like any other expression.
	geo := "geo"
	var names []string
	collectTugQLExpressionDatabases(&exprYAML{Aggregate: &aggregateYAML{Function: "string_agg", OrderBy: []orderYAML{{exprYAML: exprYAML{Query: &document{From: fromYAML{Database: &geo, Name: "Country"}}}}}}}, func(name string) {
		names = append(names, name)
	})
	if fmt.Sprint(names) != "[geo]" {
		t.Fatalf("ordered aggregate databases = %v", names)
	}
}

func TestTugQLCrossDatabaseSourceEdges(t *testing.T) {
	source := "databases (\n  sales\n\n  geo = 'datatug://chinook/geo'\n)\nfrom sales::Invoice as i\njoin geo::Country as c\n  on i.BillingCountry = c.Name\nselect i.InvoiceId"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || len(doc.Tree.Databases) != 2 {
		t.Fatalf("blank line in DATABASES: %+v %+v", doc.Tree, diagnostics)
	}
	upper, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "tab"})
	if len(diagnostics) != 0 || !strings.HasPrefix(upper, "DATABASES (\n\tsales\n\n\tgeo = 'datatug://chinook/geo'\n)\nFROM sales::Invoice AS i\nJOIN geo::Country AS c\n\tON ") {
		t.Fatalf("format=%q diagnostics=%+v", upper, diagnostics)
	}
	if _, diagnostics := ParseTugQL("Databases (\n  sales\n)\nfrom sales::Invoice\nselect Id"); len(diagnostics) == 0 || diagnostics[0].Code != "keyword_case" {
		t.Fatalf("mixed-case header: %+v", diagnostics)
	}
	if _, diagnostics := ParseTugQL("DATABASES (\n  sales\n)\nfrom sales::Invoice\nselect Id"); len(diagnostics) == 0 || diagnostics[0].Code != "keyword_case" {
		t.Fatalf("inconsistent header case: %+v", diagnostics)
	}
	// `databases` is a keyword only as the word of a `databases (` header line.
	for text, want := range map[string]bool{
		"databases (\n":                        true,
		"from T\nDATABASES (\n  a\n)":          true,
		"from databases\nselect a":             false,
		"from T\nselect databases(x)":          false,
		"from T\nselect (\n  databases (x)\n)": false,
		"from T as databases\nselect a":        false,
		"from T\nselect (\n  a\n  databases (\n    from U\n    select b\n  )\n)": true,
	} {
		tokens := mustLex(t, text)
		found := false
		for i, token := range tokens {
			if strings.EqualFold(token.Text, "databases") {
				found = true
				if got := isTugQLStyleToken(tokens, i); got != want {
					t.Errorf("%q: header=%v, want %v", text, got, want)
				}
			}
		}
		if !found {
			t.Fatalf("%q has no databases token", text)
		}
	}
}

func mustLex(t *testing.T, text string) []tugqlToken {
	t.Helper()
	tokens, diagnostics := lexTugQL(text)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	return tokens
}

// Federated execution routes every source by alias, so it requires every
// source to be qualified: a caller wanting a default database binds it as an
// alias. A mixed document resolves but fails closed when executed.
func TestTugQLCrossDatabaseFederatedExecutionRequiresQualifiedSources(t *testing.T) {
	schemas := []TugQLAuthorizedSchema{
		{Version: "default-v1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "BillingCountry", Type: "string", Authorized: true}}}}},
		{Database: "main", Version: "main-v1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "BillingCountry", Type: "string", Authorized: true}}}}},
		{Database: "geo", Version: "geo-v1", Tables: []TugQLTable{{Name: "Country", Fields: []TugQLField{{Name: "Name", Type: "string", Authorized: true}}}}},
	}
	invoices := newCrossDatabaseExecutor("main", "Invoice", map[string]any{"InvoiceId": 1, "BillingCountry": "USA"})
	countries := newCrossDatabaseExecutor("geo", "Country", map[string]any{"Name": "USA"})
	resolve := func(_ context.Context, database string) (dal.QueryExecutor, error) {
		switch database {
		case "main":
			return invoices, nil
		case "geo":
			return countries, nil
		}
		return nil, fmt.Errorf("unknown database %q", database)
	}
	run := func(text string) ([]record.Record, error) {
		t.Helper()
		doc, diagnostics := ParseTugQL(text)
		if len(diagnostics) != 0 {
			t.Fatalf("parse: %+v", diagnostics)
		}
		resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{AuthorizedSchemas: schemas})
		if len(diagnostics) != 0 {
			t.Fatalf("resolve: %+v", diagnostics)
		}
		reader, err := dal.ExecuteFederatedQuery(context.Background(), resolved.Query, resolve)
		if err != nil {
			return nil, err
		}
		return dal.ReadAllToRecords(context.Background(), reader)
	}
	if _, err := run("databases (\n  geo\n)\nfrom Invoice as i\njoin geo::Country as c\n  on i.BillingCountry = c.Name\nselect i.InvoiceId, c.Name"); err == nil || !strings.Contains(err.Error(), `federated source "Invoice" has no database`) {
		t.Fatalf("mixed sources must fail closed: %v", err)
	}
	rows, err := run("databases (\n  main\n  geo\n)\nfrom main::Invoice as i\njoin geo::Country as c\n  on i.BillingCountry = c.Name\nselect i.InvoiceId, c.Name")
	if err != nil || len(rows) != 1 || fmt.Sprint(rows[0].Data()) != "map[InvoiceId:1 Name:USA]" {
		t.Fatalf("qualified sources: rows=%+v err=%v", rows, err)
	}
}
