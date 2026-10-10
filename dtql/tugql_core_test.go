package dtql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"gopkg.in/yaml.v3"
)

func tugqlTestDocument(tree *TugQLTree) TugQLDocument {
	return TugQLDocument{Tree: tree, SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}}
}

func TestTugQLSharedGoldenCorpus(t *testing.T) {
	path := filepath.Join("testdata", "tugql", "v1", "suite.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Cases   []struct {
			Name     string `json:"name"`
			Source   string `json:"source"`
			Expected struct {
				Tree        *TugQLTree        `json:"tree"`
				Diagnostics []TugQLDiagnostic `json:"diagnostics"`
				YAML        string            `json:"yaml"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Format != "tugql-golden-suite" || suite.Version != 1 || len(suite.Cases) == 0 {
		t.Fatalf("invalid shared corpus header: %+v", suite)
	}
	for _, testCase := range suite.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			document, diagnostics := ParseTugQL(testCase.Source)
			if diagnostics == nil {
				diagnostics = []TugQLDiagnostic{}
			}
			got, err := json.Marshal(struct {
				Tree        *TugQLTree        `json:"tree"`
				Diagnostics []TugQLDiagnostic `json:"diagnostics"`
			}{document.Tree, diagnostics})
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(struct {
				Tree        *TugQLTree        `json:"tree"`
				Diagnostics []TugQLDiagnostic `json:"diagnostics"`
			}{testCase.Expected.Tree, testCase.Expected.Diagnostics})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("parse differs from shared fixture\n got: %s\nwant: %s", got, want)
			}
			if testCase.Expected.YAML != "" {
				actual, err := yaml.Marshal(*document.Tree)
				if err != nil {
					t.Fatal(err)
				}
				if string(actual) != testCase.Expected.YAML {
					t.Fatalf("canonical YAML differs\n got: %s\nwant: %s", actual, testCase.Expected.YAML)
				}
			}
		})
	}
}

func TestTugQLSharedResolveCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "resolve.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Cases   []struct {
			Name     string              `json:"name"`
			Source   string              `json:"source"`
			Tree     *TugQLTree          `json:"tree"`
			Document *TugQLDocument      `json:"document,omitempty"`
			Context  TugQLResolveContext `json:"context"`
			Expected struct {
				Query         string                       `json:"query"`
				Columns       []TugQLOutputColumn          `json:"columns"`
				SchemaVersion string                       `json:"schemaVersion"`
				Dependencies  []TugQLDependencyReceipt     `json:"dependencies"`
				Relationships []TugQLRelationshipExpansion `json:"relationships"`
				Diagnostics   []TugQLDiagnostic            `json:"diagnostics"`
			} `json:"expected"`
			ParseDiagnostics []TugQLDiagnostic `json:"parseDiagnostics"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Format != "tugql-resolve-suite" || suite.Version != 1 || len(suite.Cases) == 0 {
		t.Fatalf("invalid resolver corpus header: %+v", suite)
	}
	for _, testCase := range suite.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			doc := TugQLDocument{Tree: testCase.Tree, SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}}
			var parseDiagnostics []TugQLDiagnostic
			if testCase.Document != nil {
				doc = *testCase.Document
				if doc.Source != "" {
					_, parseDiagnostics = ParseTugQL(doc.Source)
				}
			} else if testCase.Source != "" {
				doc, parseDiagnostics = ParseTugQL(testCase.Source)
			}
			if !reflect.DeepEqual(parseDiagnostics, testCase.ParseDiagnostics) {
				t.Fatalf("parse diagnostics=%+v want=%+v", parseDiagnostics, testCase.ParseDiagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, testCase.Context)
			var query string
			if resolved.Query != nil {
				encoded, err := Serialize(resolved.Query)
				if err != nil {
					t.Fatal(err)
				}
				query = string(encoded)
			}
			if query != testCase.Expected.Query || !reflect.DeepEqual(resolved.Columns, testCase.Expected.Columns) || resolved.SchemaVersion != testCase.Expected.SchemaVersion || !reflect.DeepEqual(resolved.Dependencies, testCase.Expected.Dependencies) || !reflect.DeepEqual(resolved.Relationships, testCase.Expected.Relationships) || !reflect.DeepEqual(diagnostics, testCase.Expected.Diagnostics) {
				t.Fatalf("resolved query=%s columns=%+v schema=%s deps=%+v rels=%+v diagnostics=%+v", query, resolved.Columns, resolved.SchemaVersion, resolved.Dependencies, resolved.Relationships, diagnostics)
			}
		})
	}
}

func TestTugQLSharedBudgetCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "budget.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Cases   []struct {
			Name         string `json:"name"`
			Kind         string `json:"kind"`
			Count        int    `json:"count"`
			ExpectedCode string `json:"expectedCode"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Format != "tugql-budget-suite" || suite.Version != 1 || len(suite.Cases) == 0 {
		t.Fatalf("invalid budget corpus header: %+v", suite)
	}
	for _, testCase := range suite.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			tree := &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
				"from": map[string]any{"name": "T"},
			}}
			switch testCase.Kind {
			case "columns":
				columns := make([]any, testCase.Count)
				for i := range columns {
					columns[i] = map[string]any{"field": "Id", "as": fmt.Sprintf("c%d", i)}
				}
				tree.Query["columns"] = columns
			case "parameters":
				parameters := make([]TugQLParameter, testCase.Count)
				for i := range parameters {
					value := any(1)
					parameters[i] = TugQLParameter{Name: fmt.Sprintf("P%d", i), Type: "integer", Default: &value}
				}
				tree.Parameters = parameters
			default:
				t.Fatalf("unknown budget case kind %q", testCase.Kind)
			}
			context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
			resolved, diagnostics := ResolveTugQL(tugqlTestDocument(tree), context)
			if testCase.ExpectedCode == "" {
				if resolved.Query == nil || len(diagnostics) != 0 {
					t.Fatalf("expected resolution, query=%v diagnostics=%+v", resolved.Query, diagnostics)
				}
				return
			}
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != testCase.ExpectedCode {
				t.Fatalf("expected %s, query=%v diagnostics=%+v", testCase.ExpectedCode, resolved.Query, diagnostics)
			}
		})
	}
}

func TestTugQLSharedExecutionCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "execute.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Cases   []struct {
			Name    string              `json:"name"`
			Source  string              `json:"source"`
			Context TugQLResolveContext `json:"context"`
			Input   struct {
				Tables map[string][]map[string]any `json:"tables"`
			} `json:"input"`
			Expected struct {
				Records []map[string]any `json:"records"`
				Error   *struct {
					Code string `json:"code"`
					Path string `json:"path"`
				} `json:"error"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Format != "tugql-execute-suite" || suite.Version != 1 || len(suite.Cases) == 0 {
		t.Fatalf("invalid execution corpus header: %+v", suite)
	}
	for _, testCase := range suite.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			doc, parseDiagnostics := ParseTugQL(testCase.Source)
			if len(parseDiagnostics) != 0 {
				t.Fatalf("parse diagnostics: %+v", parseDiagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, testCase.Context)
			if len(diagnostics) != 0 || resolved.Query == nil {
				t.Fatalf("resolve query=%v diagnostics=%+v", resolved.Query, diagnostics)
			}
			tables := make(map[string][]record.Record, len(testCase.Input.Tables))
			for table, rows := range testCase.Input.Tables {
				for i, row := range rows {
					tables[table] = append(tables[table], record.NewRecordWithData(record.NewKeyWithID(table, i), row))
				}
			}
			reader, executeErr := dal.ExecuteRecursiveQuery(context.Background(), fixtureLeafExecutor{tables: tables}, resolved.Query)
			if testCase.Expected.Error != nil {
				var validation *dal.QueryValidationError
				if !errors.As(executeErr, &validation) || validation.Category != testCase.Expected.Error.Code || validation.Path != testCase.Expected.Error.Path {
					t.Fatalf("execution error=%v, want %s at %s", executeErr, testCase.Expected.Error.Code, testCase.Expected.Error.Path)
				}
				return
			}
			if executeErr != nil {
				t.Fatal(executeErr)
			}
			rows, err := dal.ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]map[string]any, len(rows))
			for i, row := range rows {
				got[i] = row.Data().(map[string]any)
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(testCase.Expected.Records)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("records=%s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestTugQLSharedFormatCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "format.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Cases   []struct {
			Name     string             `json:"name"`
			Source   string             `json:"source"`
			Options  TugQLFormatOptions `json:"options"`
			Expected struct {
				Source      string            `json:"source"`
				Diagnostics []TugQLDiagnostic `json:"diagnostics"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Format != "tugql-format-suite" || suite.Version != 1 || len(suite.Cases) == 0 {
		t.Fatalf("invalid format corpus header: %+v", suite)
	}
	for _, testCase := range suite.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			doc, _ := ParseTugQL(testCase.Source)
			source, diagnostics := FormatTugQL(doc, testCase.Options)
			if source != testCase.Expected.Source || !reflect.DeepEqual(diagnostics, testCase.Expected.Diagnostics) {
				t.Fatalf("formatted=%q diagnostics=%+v; want %q %+v", source, diagnostics, testCase.Expected.Source, testCase.Expected.Diagnostics)
			}
			if len(diagnostics) == 0 {
				formatted, _ := ParseTugQL(source)
				reformatted, repeatedDiagnostics := FormatTugQL(formatted, testCase.Options)
				if len(repeatedDiagnostics) != 0 || reformatted != source {
					t.Fatalf("format is not idempotent: first=%q second=%q diagnostics=%+v", source, reformatted, repeatedDiagnostics)
				}
			}
		})
	}
}

func TestLexTugQLPreservesUnicodeScalarSpansAndIgnoresComments(t *testing.T) {
	tokens, diags := lexTugQL("-- FROM ignored\r\nFROM \"Order\" WHERE name = 'x''y' -- SELECT ignored\nWHERE value = @值")
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(tokens) != 10 {
		t.Fatalf("got %d tokens: %#v", len(tokens), tokens)
	}
	if got := tokens[0].Span.Start; got != (tugqlPosition{Line: 2, Column: 1}) {
		t.Fatalf("FROM start = %+v", got)
	}
	if got := tokens[1].Text; got != `"Order"` {
		t.Fatalf("quoted identifier = %q", got)
	}
	last := tokens[len(tokens)-1]
	if last.Text != "@值" || last.Span.End.Column-last.Span.Start.Column != 2 {
		t.Fatalf("Unicode parameter token = %+v", last)
	}
	if isKeyword(tokens[1], "order") {
		t.Fatal("quoted identifier was treated as a keyword")
	}
}

func TestLexTugQLReportsMalformedAndBoundedInput(t *testing.T) {
	_, diags := lexTugQL("FROM t WHERE x = 'unfinished")
	if len(diags) != 1 || diags[0].Code != "unterminated_quote" {
		t.Fatalf("diagnostics = %+v", diags)
	}
	_, diags = lexTugQL(strings.Repeat("x", maxTugQLRunes+1))
	if len(diags) != 1 || diags[0].Code != "input_too_large" {
		t.Fatalf("diagnostics = %+v", diags)
	}
}

func TestTugQLParameterLiteralBoundariesAndInvalidUTF8(t *testing.T) {
	valid := []string{
		"parameters (\n  @Min integer default -9007199254740991\n  @Max integer default 9007199254740991\n  @Ratio decimal default -12.50\n  @When date default '2024-02-29'\n)\nfrom T\nselect Id",
		"parameters (\n  @When timestamp default '2024-01-01T00:00:00.1234567890Z'\n)\nfrom T\nselect Id",
	}
	for _, source := range valid {
		if _, diagnostics := ParseTugQL(source); len(diagnostics) != 0 {
			t.Fatalf("valid typed defaults rejected: %+v", diagnostics)
		}
	}
	for _, source := range []string{
		"parameters (\n  @TooLarge integer default 9007199254740992\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad decimal default NaN\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad date default '2024-02-30'\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad timestamp default '2024-01-01T0:00:00Z'\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad timestamp default '2024-01-01T00:00:00+24:00'\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad timestamp default '2024-01-01T00:00:00+00:60'\n)\nfrom T\nselect Id",
		"parameters (\n  @Bad timestamp default '2024-01-01T00:00:00,123Z'\n)\nfrom T\nselect Id",
	} {
		if _, diagnostics := ParseTugQL(source); len(diagnostics) == 0 {
			t.Fatalf("invalid typed default accepted: %q", source)
		}
	}
	if tokens, diagnostics := lexTugQL("from T\nselect \xff"); len(tokens) != 0 || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_utf8" || diagnostics[0].Span.Start != (tugqlPosition{Line: 2, Column: 8}) {
		t.Fatalf("invalid UTF-8 tokens=%+v diagnostics=%+v", tokens, diagnostics)
	}
}

func TestTugQLExpressionPrecedenceAndStructuredShapes(t *testing.T) {
	expr, err := parseTugQLExpression([]tugqlToken{
		{Kind: tugqlIdentifier, Text: "i"}, {Kind: tugqlSymbol, Text: "."}, {Kind: tugqlIdentifier, Text: "Total"},
		{Kind: tugqlSymbol, Text: "+"}, {Kind: tugqlNumber, Text: "2"}, {Kind: tugqlSymbol, Text: "*"}, {Kind: tugqlParameterToken, Text: "@rate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if expr.Binary == nil || expr.Binary.Op != "+" || expr.Binary.Left.Field != "Total" || expr.Binary.Left.Source != "i" || expr.Binary.Right.Binary == nil || expr.Binary.Right.Binary.Op != "*" {
		t.Fatalf("unexpected expression tree: op=%q left=%#v right=%#v", expr.Binary.Op, expr.Binary.Left, expr.Binary.Right)
	}
	for _, tc := range []struct {
		text string
		kind tugqlTokenKind
		want func(exprYAML) bool
	}{
		{"'hello'", tugqlString, func(e exprYAML) bool { return e.Value != nil && *e.Value == "hello" }},
		{"true", tugqlIdentifier, func(e exprYAML) bool { return e.Value != nil && *e.Value == true }},
		{"NULL", tugqlIdentifier, func(e exprYAML) bool { return e.Value != nil && *e.Value == nil }},
		{"*", tugqlSymbol, func(e exprYAML) bool { return e.Star }},
		{"SUM", tugqlIdentifier, func(e exprYAML) bool { return e.Aggregate != nil && e.Aggregate.Function == "SUM" }},
	} {
		tokens := []tugqlToken{{Kind: tc.kind, Text: tc.text}}
		if tc.text == "SUM" {
			tokens = append(tokens, tugqlToken{Kind: tugqlSymbol, Text: "("}, tugqlToken{Kind: tugqlSymbol, Text: "*"}, tugqlToken{Kind: tugqlSymbol, Text: ")"})
		}
		got, err := parseTugQLExpression(tokens)
		if err != nil || !tc.want(got) {
			t.Fatalf("%s: got %+v, err %v", tc.text, got, err)
		}
	}
}

func TestTugQLExpressionRejectsLossyAndMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		tokens []tugqlToken
		want   string
	}{
		{[]tugqlToken{{Kind: tugqlNumber, Text: "1.234567890123456789"}}, "precision loss"},
		{[]tugqlToken{{Kind: tugqlNumber, Text: "9007199254740993"}}, "exact portable range"},
		{[]tugqlToken{{Kind: tugqlNumber, Text: "999999999999999999999999"}}, "invalid number"},
		{[]tugqlToken{{Kind: tugqlSymbol, Text: "("}, {Kind: tugqlNumber, Text: "1"}}, "closing parenthesis"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "COUNT"}, {Kind: tugqlSymbol, Text: "("}, {Kind: tugqlNumber, Text: "1"}, {Kind: tugqlSymbol, Text: ","}, {Kind: tugqlSymbol, Text: ")"}}, "unexpected token"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "t"}, {Kind: tugqlSymbol, Text: "."}, {Kind: tugqlSymbol, Text: "*"}}, "qualified wildcard"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: "%"}, {Kind: tugqlNumber, Text: "2"}}, "unexpected token"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: "="}, {Kind: tugqlNumber, Text: "1"}, {Kind: tugqlIdentifier, Text: "unexpected"}}, "unexpected token"},
	} {
		_, err := parseTugQLExpression(tc.tokens)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
	call, err := parseTugQLExpression([]tugqlToken{{Kind: tugqlIdentifier, Text: "COALESCE"}, {Kind: tugqlSymbol, Text: "("}, {Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: ")"}})
	if err != nil || call.tugqlCall == nil || call.tugqlCall.Function != "COALESCE" {
		t.Fatalf("scalar call: %+v, %v", call, err)
	}
	deep := []tugqlToken{}
	for i := 0; i < maxTugQLDepth+1; i++ {
		deep = append(deep, tugqlToken{Kind: tugqlSymbol, Text: "("})
	}
	deep = append(deep, tugqlToken{Kind: tugqlNumber, Text: "1"})
	for i := 0; i < maxTugQLDepth+1; i++ {
		deep = append(deep, tugqlToken{Kind: tugqlSymbol, Text: ")"})
	}
	if _, err := parseTugQLExpression(deep); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deep expression error = %v", err)
	}
}

func TestTugQLConditionsAndLists(t *testing.T) {
	condition, err := parseTugQLCondition([]tugqlToken{
		{Kind: tugqlIdentifier, Text: "a"}, {Kind: tugqlSymbol, Text: "="}, {Kind: tugqlNumber, Text: "1"},
		{Kind: tugqlIdentifier, Text: "OR"}, {Kind: tugqlSymbol, Text: "("}, {Kind: tugqlIdentifier, Text: "b"},
		{Kind: tugqlIdentifier, Text: "IS"}, {Kind: tugqlIdentifier, Text: "NULL"}, {Kind: tugqlSymbol, Text: ")"},
	})
	if err != nil || len(condition.Or) != 2 || condition.Or[1].IsNull == nil {
		t.Fatalf("condition=%+v err=%v", condition, err)
	}
	for _, tc := range []struct {
		tokens []tugqlToken
		want   string
	}{
		{nil, "condition is required"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlIdentifier, Text: "AND"}}, "missing expression"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlIdentifier, Text: "AND"}, {Kind: tugqlIdentifier, Text: "y"}, {Kind: tugqlIdentifier, Text: "AND"}}, "missing expression"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}}, "unsupported condition"},
	} {
		_, err := parseTugQLCondition(tc.tokens)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
	for _, tc := range []struct {
		tokens []tugqlToken
		want   string
	}{
		{[]tugqlToken{{Kind: tugqlSymbol, Text: ","}}, "empty list item"},
		{[]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: ","}}, "empty list item"},
		{[]tugqlToken{{Kind: tugqlSymbol, Text: "("}}, "unclosed parenthesis"},
		{[]tugqlToken{{Kind: tugqlSymbol, Text: ")"}}, "unexpected closing parenthesis"},
	} {
		_, err := splitTugQLComma(tc.tokens)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
}

func TestTugQLFromColumnsAndOrder(t *testing.T) {
	from, err := parseTugQLFrom([]tugqlToken{{Kind: tugqlIdentifier, Text: `"sales"`}, {Kind: tugqlSymbol, Text: "."}, {Kind: tugqlIdentifier, Text: `"Order"`}, {Kind: tugqlIdentifier, Text: "AS"}, {Kind: tugqlIdentifier, Text: "o"}})
	if err != nil || from.Name != "Order" || from.Schema == nil || *from.Schema != "sales" || from.Alias != "o" {
		t.Fatalf("from=%+v err=%v", from, err)
	}
	if _, err := parseTugQLFrom([]tugqlToken{{Kind: tugqlIdentifier, Text: "t"}, {Kind: tugqlIdentifier, Text: "badAlias"}}); err == nil {
		t.Fatal("alias without AS was accepted")
	}
	cols, err := parseTugQLColumns([]tugqlToken{{Kind: tugqlIdentifier, Text: "o"}, {Kind: tugqlSymbol, Text: "."}, {Kind: tugqlIdentifier, Text: "id"}, {Kind: tugqlIdentifier, Text: "AS"}, {Kind: tugqlIdentifier, Text: "ID"}, {Kind: tugqlSymbol, Text: ","}, {Kind: tugqlIdentifier, Text: "COUNT"}, {Kind: tugqlSymbol, Text: "("}, {Kind: tugqlSymbol, Text: "*"}, {Kind: tugqlSymbol, Text: ")"}})
	if err != nil || len(cols) != 2 || cols[0].As != "ID" || cols[1].Aggregate == nil {
		t.Fatalf("columns=%+v err=%v", cols, err)
	}
	if _, err := parseTugQLColumns([]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlIdentifier, Text: "alias"}}); err == nil {
		t.Fatal("alias without AS was accepted")
	}
	order, err := parseTugQLOrder([]tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlIdentifier, Text: "DESC"}, {Kind: tugqlSymbol, Text: ","}, {Kind: tugqlIdentifier, Text: "y"}, {Kind: tugqlIdentifier, Text: "ASC"}})
	if err != nil || len(order) != 2 || !order[0].Desc || order[1].Desc {
		t.Fatalf("order=%+v err=%v", order, err)
	}
}

func TestParseTugQLDocumentParametersCTEsAndImports(t *testing.T) {
	source := `parameters (
  @customer integer required
  @fee decimal default 1.2300
  @label string default 'a,b'
  @active boolean default true
)
with First as (
  from Customer as c
  where c.Id = @customer
  select c.Id as Id
)
with Invoices from "./invoice-query.tugql"
  using (
    @customer = @customer
  )
with Final as (
  from First as f
  join Invoice as i
    on i.CustomerId = f.Id
  select f.Id, i.Total
)
from Final as result
select result.Id`
	tree, sourceDoc, diags := parseTugQLDocument(source)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %+v", diags)
	}
	if sourceDoc.Text != source || sourceDoc.Format != "tugql" || sourceDoc.Version != 1 {
		t.Fatalf("source metadata: %+v", sourceDoc)
	}
	if tree.Format != "tugqtree" || tree.Version != 1 || len(tree.Parameters) != 4 || len(tree.Definitions) != 3 {
		t.Fatalf("tree envelope: %+v", tree)
	}
	if !tree.Parameters[0].Required || tree.Parameters[0].Default != nil {
		t.Fatalf("required parameter presence lost: %+v", tree.Parameters[0])
	}
	if got := *tree.Parameters[1].Default; got != "1.2300" {
		t.Fatalf("decimal default lost exact text: %#v", got)
	}
	if tree.Definitions[0].Kind != "cte" || tree.Definitions[0].Query == nil {
		t.Fatalf("first CTE: %+v", tree.Definitions[0])
	}
	if tree.Definitions[1].Kind != "import" || tree.Definitions[1].Path != "./invoice-query.tugql" || len(tree.Definitions[1].Using) != 1 {
		t.Fatalf("import: %+v", tree.Definitions[1])
	}
	if tree.Definitions[2].Query == nil || len(tree.Definitions[2].Query.Query.From.Joins) != 1 {
		t.Fatalf("nested CTE query: %+v", tree.Definitions[2])
	}
}

func TestParseTugQLRejectsPartialSyntaxAndBadBlocks(t *testing.T) {
	for _, tc := range []struct{ name, source, code string }{
		{"unknown line", "from Invoice\nnonsense\nselect Id", "unsupported_statement"},
		{"mixed casing", "FROM Invoice\nwhere Id = 1", "keyword_case"},
		{"mixed indentation", "parameters (\n  @x INTEGER REQUIRED\n)\nwith X as (\n\tfrom T\n)\nFROM T", "indentation"},
		{"parameter comma", "parameters (\n  @x INTEGER REQUIRED,\n)\nfrom T", "invalid_parameter"},
		{"declaration without AS", "with X (\n  from T\n)\nfrom X", "invalid_with"},
		{"inline close", "with X as ( from T )\nfrom X", "invalid_cte"},
		{"multiple clause same line", "from T select id", "multiple_clauses_same_line"},
		{"semicolon statement", "from T; select id", "multiple_statements"},
		{"parameter marker", "from T where id = @", "invalid_parameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, diags := parseTugQLDocument(tc.source)
			found := false
			for _, d := range diags {
				if d.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected %q, got %+v", tc.code, diags)
			}
		})
	}
}

func TestParseTugQLNestedDiagnosticsAndNewlineStyles(t *testing.T) {
	_, _, diags := parseTugQLDocument("with X as (\n  from T\n  where Id =\n)\nfrom X")
	if len(diags) == 0 || diags[0].Span.Start.Line < 2 {
		t.Fatalf("nested diagnostic wasn't positioned in source: %+v", diags)
	}
	crOnly := "from T\rselect Id"
	tree, _, diags := parseTugQLDocument(crOnly)
	if len(diags) != 0 || tree.Query.From.Name != "T" || len(tree.Query.Columns) != 1 {
		t.Fatalf("CR-only newline wasn't parsed as a statement boundary: tree=%+v diagnostics=%+v", tree.Query, diags)
	}
}

func TestTugQLPublicTreeIsVersionedAndKeepsCallsNonExecutable(t *testing.T) {
	doc, diags := ParseTugQL("from Invoice as i\nselect COALESCE(i.Note, 'a,b') as Note")
	if len(diags) != 0 || doc.Tree == nil {
		t.Fatalf("document=%+v diagnostics=%+v", doc, diags)
	}
	if doc.Source != "from Invoice as i\nselect COALESCE(i.Note, 'a,b') as Note" || doc.SourceMetadata.Format != "tugql" || doc.SourceMetadata.Version != 1 {
		t.Fatalf("source identity was lost: %+v", doc)
	}
	treeYAML, err := yaml.Marshal(*doc.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(treeYAML), "format: tugqtree") || !strings.Contains(string(treeYAML), "call:") || !strings.Contains(string(treeYAML), "COALESCE") {
		t.Fatalf("canonical TugQTree omitted call syntax: %s", treeYAML)
	}
	legacy, err := Deserialize([]byte("from: {name: Invoice}\ncolumns: [{call: {function: COALESCE, args: [{field: Note}]}}]\n"))
	if err == nil || legacy != nil {
		t.Fatalf("legacy DTQL made a scalar call executable: %v %v", legacy, err)
	}
	doc.Tree.Query["unsupported"] = true
	if _, err := yaml.Marshal(*doc.Tree); err == nil {
		t.Fatal("unknown sibling query shape was accepted with an unsupported call")
	}
}

func TestResolveTugQLJSONTransportPreservesTypedDefaults(t *testing.T) {
	source := "parameters (\n  @Count integer default 1\n)\nfrom Invoice\nselect InvoiceId"
	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %+v", diagnostics)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var transported TugQLDocument
	if err := json.Unmarshal(data, &transported); err != nil {
		t.Fatal(err)
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{
		Version: "s1",
		Tables:  []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}},
	}}}
	resolved, diagnostics := ResolveTugQL(transported, context)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics after JSON round trip: %+v", diagnostics)
	}
	if resolved.Query == nil || resolved.SchemaVersion != "s1" || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "InvoiceId" {
		t.Fatalf("unexpected resolved transport result: %+v", resolved)
	}
}

func TestResolveTugQLRejectsUnauthorizedExistsPredicateFields(t *testing.T) {
	tree := &TugQLTree{
		Format:  "tugqtree",
		Version: 1,
		Query: map[string]any{
			"from":    map[string]any{"name": "Invoice", "alias": "i"},
			"columns": []any{map[string]any{"field": "InvoiceId", "source": "i"}},
			"where": map[string]any{"notExists": map[string]any{"query": map[string]any{
				"from":    map[string]any{"name": "Customer", "alias": "c"},
				"columns": []any{map[string]any{"field": "Id", "source": "c"}},
				"where": map[string]any{
					"op":    "==",
					"left":  map[string]any{"field": "Secret", "source": "c"},
					"right": map[string]any{"value": "x"},
				},
			}}},
		},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Secret", Type: "string", Authorized: false}}},
	}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(tree), context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("unauthorized EXISTS predicate resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLTreeMarshalPreservesLegacyQueryOrderAndValidatesCalls(t *testing.T) {
	doc, diagnostics := ParseTugQL("from Invoice as i\nwhere i.Id = 1\nselect i.Id as Id")
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("document=%+v diagnostics=%+v", doc, diagnostics)
	}
	data, err := yaml.Marshal(*doc.Tree)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	from := strings.Index(text, "  from:")
	where := strings.Index(text, "  where:")
	columns := strings.Index(text, "  columns:")
	if from < 0 || where < from || columns < where {
		t.Fatalf("canonical query key order changed: %s", text)
	}
	malformed := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
		"from": map[string]any{"name": "T"},
		"columns": []any{map[string]any{"call": map[string]any{"function": "COALESCE", "args": []any{
			map[string]any{"binary": map[string]any{"op": "+", "left": map[string]any{"field": "x"}}},
		}}}},
	}}
	if _, err := yaml.Marshal(malformed); err == nil {
		t.Fatal("malformed expression inside call argument was accepted")
	}
}

func TestTugQLTreeStrictNestedWireValidation(t *testing.T) {
	valid := `format: tugqtree
version: 1
definitions:
  - kind: cte
    name: X
    query:
      query:
        from: {name: T}
query:
  from: {name: X}
`
	var tree TugQLTree
	if err := yaml.Unmarshal([]byte(valid), &tree); err != nil {
		t.Fatalf("valid nested tree: %v", err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, "name: X\n", "name: X\n    surprise: true\n", 1),
		strings.Replace(valid, "kind: cte", "kind: invalid", 1),
		strings.Replace(valid, "name: X\n", "name: X\n    query:\n      query:\n        from: {name: T}\n    path: other\n", 1),
		"format: tugqtree\nversion: 1\nparameters:\n  - name: P\n    type: integer\n    required: true\n    default: 1\nquery: {from: {name: T}}\n",
	} {
		if err := yaml.Unmarshal([]byte(invalid), &tree); err == nil {
			t.Fatalf("invalid nested TugQTree was accepted:\n%s", invalid)
		}
	}
	parsed, diagnostics := ParseTugQL("from T\nselect (\n  Value as (\n    with Local as (\n      from U\n      select Id\n    )\n    from Local\n    select Id\n  )\n)")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	data, err := yaml.Marshal(*parsed.Tree)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip TugQLTree
	if err := yaml.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("scalar body round trip: %v\n%s", err, data)
	}
}

func TestResolveTugQLValidatesAuthorizedProjectionAndCTE(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r7", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "Secret", Type: "string", Authorized: false}}}}}}}
	doc, diagnostics := ParseTugQL("from Invoice as i\nselect i.InvoiceId as ID")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || resolved.SchemaVersion != "schema-r7" || len(resolved.Columns) != 1 || resolved.Columns[0].Type != "integer" || resolved.Columns[0].Lineage[0].Field != "InvoiceId" {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	cte, diagnostics := ParseTugQL("with Invoices as (\n  from Invoice as i\n  select i.InvoiceId\n)\nfrom Invoices as v\nselect v.InvoiceId")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if resolved, diagnostics = ResolveTugQL(cte, context); len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 {
		t.Fatalf("CTE resolution=%+v diagnostics=%+v", resolved, diagnostics)
	}
	unsafe, diagnostics := ParseTugQL("from Invoice as i\nselect i.Secret")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if resolved, diagnostics = ResolveTugQL(unsafe, context); len(diagnostics) == 0 || resolved.Query != nil || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("unauthorized field resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	unsafe.Tree.Query["unknown"] = true
	if resolved, diagnostics = ResolveTugQL(unsafe, context); len(diagnostics) == 0 || resolved.Query != nil {
		t.Fatalf("mutated caller tree resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	filter, diagnostics := ParseTugQL("from Invoice as i\nwhere i.Secret = 'x'\nselect i.InvoiceId")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if result, diagnostics := ResolveTugQL(filter, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("unauthorized filter resolved=%+v diagnostics=%+v", result, diagnostics)
	}
}

func TestResolveTugQLOutputExpressionTypesLineageAndWildcard(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{
		{Name: "InvoiceId", Type: "integer", Authorized: true},
		{Name: "Total", Type: "decimal", Authorized: true},
		{Name: "Secret", Type: "string", Authorized: false},
	}}}}}}
	doc, diagnostics := ParseTugQL("from Invoice as i\ngroup by i.InvoiceId + 1\nselect (\n  i.InvoiceId + 1 as NextId\n  COUNT(*) as InvoiceCount\n)")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 2 || resolved.Columns[0].Type != "number" || len(resolved.Columns[0].Lineage) != 1 || resolved.Columns[1].Type != "integer" {
		t.Fatalf("expression result=%+v diagnostics=%+v", resolved, diagnostics)
	}
	wildcard, diagnostics := ParseTugQL("from Invoice as i\nselect *")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics = ResolveTugQL(wildcard, context)
	if len(diagnostics) != 0 || len(resolved.Columns) != 2 || resolved.Columns[1].Name != "Total" {
		t.Fatalf("wildcard result=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestResolveTugQLCanonicalWildcardExclusion(t *testing.T) {
	doc, diagnostics := ParseTugQL("from Invoice as i\nselect *")
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("parse document=%+v diagnostics=%+v", doc, diagnostics)
	}
	doc.Source = "" // A caller may supply a canonical TugQTree without authoring text.
	doc.Tree.Query["columns"] = []any{map[string]any{"wildcard": map[string]any{
		"source": "i", "exclude": []any{"Secret"},
	}}}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{
		{Name: "InvoiceId", Type: "integer", Authorized: true},
		{Name: "Secret", Type: "string", Authorized: true},
	}}}}}}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "InvoiceId" {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestResolveTugQLSemanticNodeBudget(t *testing.T) {
	columns := make([]any, 100_000)
	wideColumn := map[string]any{"field": "Id"}
	for i := range columns {
		columns[i] = wideColumn
	}
	document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
		"from":    map[string]any{"name": "T"},
		"columns": columns,
	}}}
	resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" || diagnostics[0].Message != "TugQL semantic tree exceeds 5000 nodes" {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestResolveTugQLExpandsOnlyDeclaredExactRelationships(t *testing.T) {
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
			{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
			{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		}}},
		Relationships: []TugQLRelationship{{ID: "fk-invoice-customer", Version: "r1", From: TugQLRelationEndpoint{Source: "i", Table: "Invoice"}, To: TugQLRelationEndpoint{Source: "c", Table: "Customer"}, Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}, ExactTypedEquality: true}},
	}
	for _, source := range []string{
		"from Invoice as i\njoin Customer as c\nselect i.InvoiceId",
		"from Invoice as i\njoin Customer as c\n  on CustomerId\nselect i.InvoiceId",
	} {
		doc, diagnostics := ParseTugQL(source)
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		resolved, diagnostics := ResolveTugQL(doc, context)
		if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Relationships) != 1 || resolved.Relationships[0].ID != "fk-invoice-customer" {
			t.Fatalf("relationship resolution=%+v diagnostics=%+v", resolved, diagnostics)
		}
	}
	context.Relationships[0].ExactTypedEquality = false
	doc, _ := ParseTugQL("from Invoice as i\njoin Customer as c\nselect i.InvoiceId")
	if resolved, diagnostics := ResolveTugQL(doc, context); len(diagnostics) == 0 || resolved.Query != nil {
		t.Fatalf("non-exact relationship resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestResolveTugQLDefaultProjectionMergesOnlyExactInnerJoinKeys(t *testing.T) {
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
			{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "Region", Type: "string", Authorized: true}}},
			{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Region", Type: "string", Authorized: true}, {Name: "Name", Type: "string", Authorized: true}}},
		}}},
		Relationships: []TugQLRelationship{{ID: "fk-invoice-customer", Version: "r1", From: TugQLRelationEndpoint{Source: "i", Table: "Invoice"}, To: TugQLRelationEndpoint{Source: "c", Table: "Customer"}, Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}, {FromField: "Region", ToField: "Region"}}, ExactTypedEquality: true}},
	}
	for _, source := range []string{"from Invoice as i\njoin Customer as c"} {
		doc, diagnostics := ParseTugQL(source)
		if len(diagnostics) != 0 {
			t.Fatalf("parse %q: %+v", source, diagnostics)
		}
		resolved, diagnostics := ResolveTugQL(doc, context)
		if len(diagnostics) != 0 || len(resolved.Columns) != 4 || resolved.Columns[1].Name != "CustomerId" || resolved.Columns[1].Lineage == nil || len(resolved.Columns[1].Lineage) != 2 || resolved.Columns[2].Name != "Region" || len(resolved.Columns[2].Lineage) != 2 || resolved.Columns[3].Name != "Name" {
			t.Fatalf("exact inner projection=%+v diagnostics=%+v", resolved, diagnostics)
		}
	}
	explicit := context
	explicit.Relationships = []TugQLRelationship{{ID: "fk-invoice-customer", Version: "r1", From: TugQLRelationEndpoint{Source: "i", Table: "Invoice"}, To: TugQLRelationEndpoint{Source: "c", Table: "Customer"}, Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}, ExactTypedEquality: true}}
	doc, diagnostics := ParseTugQL("from Invoice as i\njoin Customer as c\n  on (\n    i.CustomerId = c.Id\n  )")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, explicit)
	if len(diagnostics) != 0 || len(resolved.Columns) != 5 || resolved.Columns[1].Name != "CustomerId" || len(resolved.Columns[1].Lineage) != 2 || resolved.Columns[2].Name != "Region" || resolved.Columns[3].Name != "c_Region" {
		t.Fatalf("explicit exact inner projection=%+v diagnostics=%+v", resolved, diagnostics)
	}

	left, diagnostics := ParseTugQL("from Invoice as i\nleft join Customer as c")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics = ResolveTugQL(left, context)
	if len(diagnostics) != 0 || len(resolved.Columns) != 6 || resolved.Columns[1].Name != "CustomerId" || resolved.Columns[3].Name != "Id" || resolved.Columns[4].Name != "c_Region" || len(resolved.Columns[1].Lineage) != 1 {
		t.Fatalf("left join must preserve null-distinct keys: %+v diagnostics=%+v", resolved, diagnostics)
	}

	context.Relationships[0].ExactTypedEquality = false
	inner, diagnostics := ParseTugQL("from Invoice as i\njoin Customer as c")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics = ResolveTugQL(inner, context)
	if len(diagnostics) != 1 || diagnostics[0].Code != "relationship_not_found" || resolved.Query != nil {
		t.Fatalf("non-exact omitted ON resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestResolveTugQLBindsParametersAndRejectsUnsetRequired(t *testing.T) {
	query := "parameters (\n  @Min integer required\n)\nfrom Invoice as i\nwhere i.InvoiceId >= @Min\nselect i.InvoiceId"
	doc, diagnostics := ParseTugQL(query)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	transport, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var transported TugQLDocument
	if err := json.Unmarshal(transport, &transported); err != nil {
		t.Fatal(err)
	}
	doc = transported
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r7", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}},
		Bindings:          []TugQLBinding{{Name: "Min", Set: true, Value: int64(10)}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	serialized, err := Serialize(resolved.Query)
	if err != nil || strings.Contains(string(serialized), "param:") || !strings.Contains(string(serialized), "value: 10") {
		t.Fatalf("parameter was not lowered into an exact value: %s, err=%v", serialized, err)
	}
	context.Bindings[0].Set = false
	if result, diagnostics := ResolveTugQL(doc, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "missing_required_binding" {
		t.Fatalf("unset required binding resolved=%+v diagnostics=%+v", result, diagnostics)
	}
	context.Bindings[0] = TugQLBinding{Name: "Min", Set: true, Value: nil}
	if result, diagnostics := ResolveTugQL(doc, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_binding" {
		t.Fatalf("required explicit null resolved=%+v diagnostics=%+v", result, diagnostics)
	}
}

func TestResolveTugQLExpandsPinnedImportAndRecordsReceipt(t *testing.T) {
	query := "parameters (\n  @MinimumSpend integer required\n)\nwith CustomerTotals from \"./queries/customer-totals\"\n  using (\n    @MinTotal = @MinimumSpend\n  )\nfrom CustomerTotals as c\nselect c.CustomerId"
	doc, diagnostics := ParseTugQL(query)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	imported := "parameters (\n  @MinTotal integer required\n)\nfrom Invoice as i\nwhere i.Total >= @MinTotal\nselect i.CustomerId"
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r7", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "Total", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}}}}},
		ProjectRoot:       "/repo",
		ImportingPath:     "main.tql",
		ProjectRevision:   "revision-42",
		PinnedImports:     []TugQLPinnedImport{{Path: "queries/customer-totals", Revision: "revision-42", Source: imported}},
		Bindings:          []TugQLBinding{{Name: "MinimumSpend", Set: true, Value: 10}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Dependencies) != 1 || resolved.Dependencies[0] != (TugQLDependencyReceipt{Path: "queries/customer-totals", Revision: "revision-42"}) {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	context.PinnedImports[0].Revision = "revision-41"
	if result, diagnostics := ResolveTugQL(doc, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unpinned_import" {
		t.Fatalf("unpinned import resolved=%+v diagnostics=%+v", result, diagnostics)
	}
}

func TestResolveTugQLImportPathBoundaries(t *testing.T) {
	query := "with Saved from \"../shared/query.tql\"\nfrom Saved\nselect Id"
	doc, diagnostics := ParseTugQL(query)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot:       "/repo",
		ImportingPath:     "queries/main.tql",
		ProjectRevision:   "r1",
		PinnedImports:     []TugQLPinnedImport{{Path: "shared/query.tql", Revision: "r1", Source: "from T\nselect Id"}},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Dependencies) != 1 || resolved.Dependencies[0].Path != "shared/query.tql" {
		t.Fatalf("safe parent-relative import resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}

	for _, tc := range []struct{ importPath, code string }{
		{"../../outside.tql", "import_outside_project"},
		{"file:query.tql", "import_outside_project"},
		{"https://example.test/query.tql", "import_outside_project"},
		{"..\\outside.tql", "import_outside_project"},
	} {
		t.Run(tc.importPath, func(t *testing.T) {
			bad, parseDiagnostics := ParseTugQL("with Saved from \"" + tc.importPath + "\"\nfrom Saved\nselect Id")
			if len(parseDiagnostics) != 0 {
				t.Fatal(parseDiagnostics)
			}
			got, resolveDiagnostics := ResolveTugQL(bad, context)
			if got.Query != nil || len(resolveDiagnostics) != 1 || resolveDiagnostics[0].Code != tc.code {
				t.Fatalf("resolved=%+v diagnostics=%+v", got, resolveDiagnostics)
			}
		})
	}

	missingAnchors := context
	missingAnchors.ProjectRoot = ""
	if got, resolveDiagnostics := ResolveTugQL(doc, missingAnchors); got.Query != nil || len(resolveDiagnostics) != 1 || resolveDiagnostics[0].Code != "import_context_required" {
		t.Fatalf("missing anchors resolved=%+v diagnostics=%+v", got, resolveDiagnostics)
	}
}

func TestResolveTugQLExpandsNestedScalarWithAndKeepsOneColumn(t *testing.T) {
	source := "from Customer as c\nselect (\n  RecentTotal as (\n    with Recent as (\n      from Invoice as i\n      select i.Total\n    )\n    from Recent as r\n    select r.Total\n  )\n)"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r7", Tables: []TugQLTable{
		{Name: "Customer", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "Name", Type: "string", Authorized: true}}},
		{Name: "Invoice", Fields: []TugQLField{{Name: "Total", Type: "integer", Authorized: true}, {Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
	}}}}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "RecentTotal" || resolved.Columns[0].Type != "integer" {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	call, diagnostics := ParseTugQL("from Customer\nselect COALESCE(CustomerId, 1)")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if result, diagnostics := ResolveTugQL(call, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_function" {
		t.Fatalf("call resolved=%+v diagnostics=%+v", result, diagnostics)
	}
	correlated, diagnostics := ParseTugQL("from Invoice as i\nselect (\n  i.InvoiceId\n  CustomerName as (\n    from Customer as c\n    where c.CustomerId = i.CustomerId\n    select c.Name\n  )\n)")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if result, diagnostics := ResolveTugQL(correlated, context); len(diagnostics) != 0 || result.Query == nil || len(result.Columns) != 2 || result.Columns[1].Lineage[0].Source != "c" {
		t.Fatalf("correlated scalar resolved=%+v diagnostics=%+v", result, diagnostics)
	}
}

func TestResolveTugQLScalarCardinalityAndCorrelationThroughLocalCTE(t *testing.T) {
	resolveContext := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "Name", Type: "string", Authorized: true}}},
	}}}}
	correlated := "from Invoice as i\nselect (\n  Name as (\n    with Selected as (\n      from Customer as c\n      where c.CustomerId = i.CustomerId\n      select c.Name\n    )\n    from Selected as s\n    select s.Name\n  )\n)"
	doc, diagnostics := ParseTugQL(correlated)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, resolveContext)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "Name" || len(resolved.Columns[0].Lineage) != 1 || resolved.Columns[0].Lineage[0] != (TugQLOutputLineage{Source: "c", Field: "Name"}) {
		t.Fatalf("correlated local CTE resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	for _, source := range []string{
		"from Invoice as i\nselect (\n  N as (\n    from Customer as c\n    select c.Name, c.CustomerId\n  )\n)",
		"from Invoice as i\nselect (\n  N as (\n    from Customer as c\n  )\n)",
	} {
		bad, parseDiagnostics := ParseTugQL(source)
		if len(parseDiagnostics) != 0 {
			t.Fatal(parseDiagnostics)
		}
		got, resolveDiagnostics := ResolveTugQL(bad, resolveContext)
		if got.Query != nil || len(resolveDiagnostics) != 1 || resolveDiagnostics[0].Code != "scalar_column_count" {
			t.Fatalf("invalid scalar resolved=%+v diagnostics=%+v", got, resolveDiagnostics)
		}
	}
}

func TestTugQLScalarExecutionZeroOneAndManyRows(t *testing.T) {
	resolveContext := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "Name", Type: "string", Authorized: true}}},
	}}}}
	source := "from Invoice as i\nselect (\n  i.InvoiceId\n  Name as (\n    from Customer as c\n    where c.CustomerId = i.CustomerId\n    select c.Name\n  )\n)"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, resolveContext)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("resolve=%+v diagnostics=%+v", resolved, diagnostics)
	}
	rows := func(includeMany bool) map[string][]record.Record {
		invoices := []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 1), map[string]any{"InvoiceId": 1, "CustomerId": 1}),
			record.NewRecordWithData(record.NewKeyWithID("Invoice", 2), map[string]any{"InvoiceId": 2, "CustomerId": 2}),
		}
		if includeMany {
			invoices = append(invoices, record.NewRecordWithData(record.NewKeyWithID("Invoice", 3), map[string]any{"InvoiceId": 3, "CustomerId": 3}))
		}
		return map[string][]record.Record{
			"Invoice": invoices,
			"Customer": {
				record.NewRecordWithData(record.NewKeyWithID("Customer", 1), map[string]any{"CustomerId": 1, "Name": "Ada"}),
				record.NewRecordWithData(record.NewKeyWithID("Customer", 2), map[string]any{"CustomerId": 3, "Name": "Bob"}),
				record.NewRecordWithData(record.NewKeyWithID("Customer", 3), map[string]any{"CustomerId": 3, "Name": "Cara"}),
			},
		}
	}
	reader, err := dal.ExecuteRecursiveQuery(context.Background(), fixtureLeafExecutor{tables: rows(false)}, resolved.Query)
	if err == nil {
		var records []record.Record
		records, err = dal.ReadAllToRecords(context.Background(), reader)
		if err == nil {
			for i, row := range records {
				data := row.Data().(map[string]any)
				if i == 0 && data["Name"] != "Ada" || i == 1 && data["Name"] != nil {
					t.Fatalf("scalar zero/one values at row %d = %#v", i, data)
				}
			}
			if len(records) != 2 {
				t.Fatalf("scalar result rows=%d, want 2", len(records))
			}
		}
	}
	if err != nil {
		t.Fatalf("zero/one-row scalar execution failed: %v", err)
	}

	_, err = dal.ExecuteRecursiveQuery(context.Background(), fixtureLeafExecutor{tables: rows(true)}, resolved.Query)
	var cardinality *dal.QueryValidationError
	if !errors.As(err, &cardinality) || cardinality.Category != "cardinality" || cardinality.Path != "columns[1].query" {
		t.Fatalf("two-row scalar error = %v", err)
	}
}

func TestResolveTugQLRetainsLexicalCTEsAndJoinsWhenExpanding(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "Name", Type: "string", Authorized: true}}},
	}}}}
	for _, source := range []string{
		"with First as (\n  from Invoice\n  select InvoiceId\n)\nwith Second as (\n  from First\n  select InvoiceId\n)\nfrom Second",
		"with Invoices as (\n  from Invoice\n)\nfrom Invoices as i\njoin Customer as c\n  on i.CustomerId = c.CustomerId\nselect i.InvoiceId, c.Name",
	} {
		doc, diagnostics := ParseTugQL(source)
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		resolved, diagnostics := ResolveTugQL(doc, context)
		if len(diagnostics) != 0 || resolved.Query == nil {
			t.Fatalf("source:\n%s\nresolved=%+v diagnostics=%+v", source, resolved, diagnostics)
		}
	}
}

func TestResolveTugQLInfersAggregateOutputsThroughCTEs(t *testing.T) {
	doc, diagnostics := ParseTugQL("with Counted as (\n  from Invoice as i\n  select count(*) as Count\n)\nfrom Counted as c\nselect c.Count")
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}}}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "Count" || resolved.Columns[0].Type != "integer" || len(resolved.Columns[0].Lineage) != 0 {
		t.Fatalf("aggregate CTE resolution=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestParseTugQLWithErrorsHasNoSemanticTree(t *testing.T) {
	doc, diags := ParseTugQL("from T\nunknown clause")
	if len(diags) == 0 || doc.Tree != nil || doc.Source == "" {
		t.Fatalf("invalid parse exposed partial semantics: %+v, %+v", doc, diags)
	}
}

func TestFormatTugQLOptionsAndRepairBuffer(t *testing.T) {
	source := "with X as (\n  from T as t\n  select t.Id as Id -- keep select comment\n)\nfrom X\nselect *"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	got, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "tab"})
	if len(diagnostics) != 0 || got != "WITH X AS (\n\tFROM T AS t\n\tSELECT t.Id AS Id -- keep select comment\n)\nFROM X\nSELECT *" {
		t.Fatalf("formatted=%q diagnostics=%+v", got, diagnostics)
	}
	if _, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "Title"}); len(diagnostics) != 1 || diagnostics[0].Code != "invalid_format_option" {
		t.Fatalf("invalid keyword option diagnostics=%+v", diagnostics)
	}
	if _, diagnostics := FormatTugQL(doc, TugQLFormatOptions{Indentation: "spaces"}); len(diagnostics) != 1 || diagnostics[0].Code != "invalid_format_option" {
		t.Fatalf("invalid indentation option diagnostics=%+v", diagnostics)
	}
	invalid := TugQLDocument{Source: "from T\nnot a clause"}
	if got, diagnostics := FormatTugQL(invalid, TugQLFormatOptions{KeywordCase: "preserve-existing", Indentation: "preserve-existing"}); len(diagnostics) == 0 || got != invalid.Source {
		t.Fatalf("repair buffer diagnostics/source=%q %+v", got, diagnostics)
	}
	if _, diagnostics := FormatTugQL(invalid, TugQLFormatOptions{KeywordCase: "uppercase"}); len(diagnostics) == 0 {
		t.Fatal("explicit formatting accepted invalid source")
	}
	if _, diagnostics := FormatTugQL(TugQLDocument{}, TugQLFormatOptions{}); len(diagnostics) != 1 || diagnostics[0].Code != "source_required" {
		t.Fatalf("tree-only formatting diagnostics=%+v", diagnostics)
	}
	precedence := TugQLDocument{Source: "FROM T\nwhere Id = 1\nSELECT Id"}
	formatted, diagnostics := FormatTugQL(precedence, TugQLFormatOptions{UserKeywordCase: "lowercase", DefaultKeywordCase: "uppercase"})
	if len(diagnostics) != 0 || !strings.HasPrefix(formatted, "from T\nwhere") {
		t.Fatalf("user preference should win over source/default: %q %+v", formatted, diagnostics)
	}
	formatted, diagnostics = FormatTugQL(TugQLDocument{Source: "from T\nselect Id"}, TugQLFormatOptions{ProjectTeamKeywordCase: "uppercase", UserKeywordCase: "lowercase"})
	if len(diagnostics) != 0 || !strings.HasPrefix(formatted, "FROM T") {
		t.Fatalf("project/team preference should win: %q %+v", formatted, diagnostics)
	}
	formatted, diagnostics = FormatTugQL(precedence, TugQLFormatOptions{})
	if len(diagnostics) != 0 || !strings.HasPrefix(formatted, "FROM T\nWHERE") {
		t.Fatalf("first source keyword should choose casing: %q %+v", formatted, diagnostics)
	}
	if _, diagnostics = FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "lowercase", ProjectTeamKeywordCase: "uppercase"}); len(diagnostics) != 1 || diagnostics[0].Code != "conflicting_format_preference" {
		t.Fatalf("same-layer conflict diagnostics=%+v", diagnostics)
	}
}

func TestParseTugQLSelectedProjectionBlock(t *testing.T) {
	source := "from Invoice as i\nselect (\n  i.InvoiceId as ID\n  COALESCE(i.Note, 'a,b') as Note\n)"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("document=%+v diagnostics=%+v", doc, diagnostics)
	}
	columns, ok := doc.Tree.Query["columns"].([]any)
	if !ok || len(columns) != 2 {
		t.Fatalf("block columns=%#v", doc.Tree.Query["columns"])
	}
	if _, diagnostics := ParseTugQL("from T\nselect (\n  Id,\n  Name\n)"); len(diagnostics) == 0 {
		t.Fatal("comma-separated select block was accepted")
	}
	if _, diagnostics := ParseTugQL("from T\nselect ( Id )"); len(diagnostics) != 0 {
		t.Fatalf("compact parenthesized expression was rejected: %+v", diagnostics)
	}
}

func TestParseTugQLRelationshipShorthandRemainsUnresolved(t *testing.T) {
	doc, diagnostics := ParseTugQL("from Invoice as i\njoin Customer as c\n  on CustomerId\nselect i.InvoiceId")
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("document=%+v diagnostics=%+v", doc, diagnostics)
	}
	joins, ok := doc.Tree.Query["from"].(map[string]any)["joins"].([]any)
	if !ok || len(joins) != 1 {
		t.Fatalf("joins=%#v", doc.Tree.Query["from"])
	}
	on := joins[0].(map[string]any)["on"].([]any)
	condition := on[0].(map[string]any)
	if condition["op"] != "relationship" {
		t.Fatalf("relationship shorthand was not preserved as unresolved metadata: %#v", condition)
	}
}

func TestParseTugQLScalarSubqueryPreservesNestedCTEs(t *testing.T) {
	source := "from Invoice as i\nselect (\n  i.InvoiceId as ID\n  CustomerName as (\n    with Customers as (\n      from Customer as c\n      select c.CustomerId, c.FirstName\n    )\n    from Customers as c\n    where c.CustomerId = i.CustomerId\n    select c.FirstName\n  )\n)"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("document=%+v diagnostics=%+v", doc, diagnostics)
	}
	columns := doc.Tree.Query["columns"].([]any)
	scalar := columns[1].(map[string]any)
	if scalar["as"] != "CustomerName" {
		t.Fatalf("scalar output name=%#v", scalar)
	}
	body := scalar["query"].(map[string]any)
	if len(body["definitions"].([]any)) != 1 {
		t.Fatalf("nested CTE definitions were dropped: %#v", body)
	}
	if _, diagnostics := ParseTugQL("from Invoice\nselect (\n  CustomerName as (\n    from Customer\n    select Name\n  )\n)"); len(diagnostics) != 0 {
		t.Fatalf("ordinary nested scalar query failed: %+v", diagnostics)
	}
}

func TestTugQLTypedBindingValueValidationBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		value    any
		required bool
		valid    bool
	}{
		{name: "optional null", typeName: "string", value: nil, valid: true},
		{name: "required null", typeName: "string", value: nil, required: true},
		{name: "integer int", typeName: "integer", value: int(7), valid: true},
		{name: "integer int64", typeName: "integer", value: int64(-7), valid: true},
		{name: "integer exact float", typeName: "integer", value: float64(12), valid: true},
		{name: "integer fractional float", typeName: "integer", value: float64(1.25)},
		{name: "integer unsafe float", typeName: "integer", value: float64(1 << 53)},
		{name: "integer wrong type", typeName: "integer", value: "1"},
		{name: "decimal exact text", typeName: "decimal", value: "-12.500", valid: true},
		{name: "decimal invalid text", typeName: "decimal", value: "NaN"},
		{name: "decimal wrong type", typeName: "decimal", value: float64(1)},
		{name: "string", typeName: "string", value: "text", valid: true},
		{name: "string wrong type", typeName: "string", value: true},
		{name: "boolean", typeName: "boolean", value: true, valid: true},
		{name: "boolean wrong type", typeName: "boolean", value: "true"},
		{name: "real date", typeName: "date", value: "2026-10-10", valid: true},
		{name: "invalid date", typeName: "date", value: "2026-02-30"},
		{name: "date wrong type", typeName: "date", value: true},
		{name: "datetime", typeName: "datetime", value: "2026-10-10T12:30:00Z", valid: true},
		{name: "timestamp more than nine fraction digits", typeName: "timestamp", value: "2026-10-10T12:30:00.1234567890Z", valid: true},
		{name: "invalid datetime", typeName: "datetime", value: "not-a-date"},
		{name: "timestamp", typeName: "timestamp", value: "2026-10-10T12:30:00.123Z", valid: true},
		{name: "invalid timestamp", typeName: "timestamp", value: "2026-10-10 12:30:00"},
		{name: "single digit hour timestamp", typeName: "timestamp", value: "2026-01-01T0:00:00Z"},
		{name: "offset hour out of range", typeName: "timestamp", value: "2026-01-01T00:00:00+24:00"},
		{name: "offset minute out of range", typeName: "timestamp", value: "2026-01-01T00:00:00+00:60"},
		{name: "comma timestamp fraction", typeName: "timestamp", value: "2026-01-01T00:00:00,123Z"},
		{name: "unknown type", typeName: "custom", value: "value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTugQLBindingValue(test.typeName, test.value, test.required)
			if (err == nil) != test.valid {
				t.Fatalf("validateTugQLBindingValue(%q, %#v, %v) error=%v, valid=%v", test.typeName, test.value, test.required, err, test.valid)
			}
		})
	}
}

func TestResolveTugQLImportsInNestedFromAndJoinSources(t *testing.T) {
	leaf := TugQLPinnedImport{Path: "queries/leaf.tql", Revision: "r1", Source: "from Invoice as i\nselect i.InvoiceId"}
	context := TugQLResolveContext{
		ProjectRoot:     "/project",
		ImportingPath:   "queries/main.tql",
		ProjectRevision: "r1",
		PinnedImports:   []TugQLPinnedImport{leaf},
	}
	importDefinition := tugqlDefinition{Kind: "import", Name: "Leaf", Path: "./leaf.tql"}
	queryWithImport := func() *document {
		return &document{
			From: fromYAML{Name: "Leaf"},
			Columns: []columnYAML{{exprYAML: exprYAML{tugqlQueryBody: &tugqlBody{
				Definitions: []tugqlDefinition{importDefinition},
				Query:       document{From: fromYAML{Name: "Leaf"}},
			}}}},
		}
	}
	query := &document{From: fromYAML{
		Query: queryWithImport(),
		Joins: []joinYAML{{From: &fromYAML{Query: queryWithImport()}}},
	}}
	definitions, receipts, diagnostics := resolveTugQLImportsInQuery(query, context, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("nested import resolution diagnostics=%+v", diagnostics)
	}
	if len(definitions) != 0 || len(receipts) != 2 {
		t.Fatalf("nested imports definitions=%d receipts=%+v, want imports retained in their local bodies and two receipts", len(definitions), receipts)
	}
	for _, receipt := range receipts {
		if receipt.Path != leaf.Path || receipt.Revision != leaf.Revision {
			t.Fatalf("nested receipt=%+v, want pinned leaf %+v", receipt, leaf)
		}
	}
	for _, nested := range []*document{query.From.Query, query.From.Joins[0].From.Query} {
		body := nested.Columns[0].tugqlQueryBody
		if body == nil || len(body.Definitions) != 1 || body.Definitions[0].Kind != "cte" || body.Definitions[0].Query == nil {
			t.Fatalf("nested import was not retained as a local CTE: %+v", body)
		}
	}
}

func TestWalkTugQLQueryFromPropagatesNestedDiagnostics(t *testing.T) {
	want := []TugQLDiagnostic{formatDiagnostic("nested_failure", "nested query failed")}
	from := &fromYAML{
		Joins: []joinYAML{{From: &fromYAML{Query: &document{}}}},
	}
	got := walkQueryFrom(from, func(*document) []TugQLDiagnostic { return want })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("walkQueryFrom diagnostics=%+v, want %+v", got, want)
	}
}

func TestTugQLExpandedCTENodeCountWalksEveryQueryShape(t *testing.T) {
	if got := tugqlDocumentNodeCount(nil, 10); got != 0 {
		t.Fatalf("nil query node count=%d, want 0", got)
	}
	if got := tugqlDocumentNodeCount(&document{}, 0); got != 1 {
		t.Fatalf("non-positive limit node count=%d, want 1", got)
	}
	field := exprYAML{Field: "Id"}
	query := &document{
		From: fromYAML{
			Query: &document{From: fromYAML{Name: "Nested"}},
			Joins: []joinYAML{{From: &fromYAML{Query: &document{From: fromYAML{Name: "Joined"}}}, On: []condYAML{{Op: "==", Left: &field, Right: &field}}}},
		},
		Where:   &condYAML{And: []condYAML{{Left: &field}, {Or: []condYAML{{Right: &field}, {IsNull: &field}}}}, Exists: &existsYAML{Query: &document{From: fromYAML{Name: "Exists"}}}},
		GroupBy: []exprYAML{{Binary: &binaryYAML{Op: "+", Left: &field, Right: &field}}},
		Money:   &moneyYAML{DivisionScale: 2},
		Having:  &condYAML{IsNotNull: &field},
		OrderBy: []orderYAML{{exprYAML: exprYAML{Aggregate: &aggregateYAML{Function: "SUM", Args: []exprYAML{field}, OrderBy: []orderYAML{{exprYAML: field}}}}}},
		Columns: []columnYAML{
			{exprYAML: exprYAML{tugqlCall: &tugqlCallYAML{Function: "COALESCE", Args: []exprYAML{field}}}},
			{exprYAML: exprYAML{Query: &document{From: fromYAML{Name: "Scalar"}}, tugqlQueryBody: &tugqlBody{Query: document{From: fromYAML{Name: "Scalar"}}}}},
		},
	}
	count := tugqlDocumentNodeCount(query, 1000)
	if count <= 20 {
		t.Fatalf("query node count=%d, want nested source/condition/expression nodes to be included", count)
	}
	for limit := 1; limit < count; limit++ {
		if got := tugqlDocumentNodeCount(query, limit); got != limit+1 {
			t.Fatalf("limited node count at limit %d=%d, want limit+1", limit, got)
		}
	}
}

func TestAppendTugQLRelationshipExpansionDeduplicatesExactReceipts(t *testing.T) {
	base := TugQLRelationshipExpansion{ID: "invoice-customer", Version: "v1", FromSource: "i", ToSource: "c", JoinType: "inner", Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}}
	var expansions []TugQLRelationshipExpansion
	appendTugQLRelationshipExpansion(&expansions, base)
	appendTugQLRelationshipExpansion(&expansions, base)
	if len(expansions) != 1 {
		t.Fatalf("exact duplicate expansion receipts=%d, want one", len(expansions))
	}
	changed := base
	changed.JoinType = "left"
	appendTugQLRelationshipExpansion(&expansions, changed)
	if len(expansions) != 2 || expansions[1].JoinType != "left" {
		t.Fatalf("distinct join type was deduplicated: %+v", expansions)
	}
}
