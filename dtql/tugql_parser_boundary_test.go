package dtql

import (
	"strings"
	"testing"
)

func TestTugQLParserBoundaryRFC3339Defaults(t *testing.T) {
	valid := []string{
		"2024-02-29T23:59:59Z",
		"2024-02-29T23:59:59.123456789+23:59",
	}
	for _, value := range valid {
		if !validTugQLRFC3339Timestamp(value) {
			t.Errorf("valid RFC3339 timestamp rejected: %q", value)
		}
	}
	invalid := []string{
		"2024-02-29T23:59:59",            // timezone required
		"2024-02-29T23:59:59.Z",          // fraction requires digits
		"2024-02-29T23:59:59.1Ztail",     // Z must end the value
		"2024-02-29T23:59:59+24:00",      // timezone hour out of range
		"2024-02-29T23:59:59+00:60",      // timezone minute out of range
		"2024-02-29T24:00:00Z",           // hour out of range
		"2024-02-29T23:60:00Z",           // minute out of range
		"2024-02-29T23:59:60Z",           // parser rejects leap seconds
		"2023-02-29T12:00:00Z",           // impossible calendar date
		"2024-02-29T12:00:00+00:00extra", // trailing timezone data
		"2024-02-29T12:00:00+0x:00",      // non-ASCII/non-digit timezone
		"2024-02-29t12:00:00Z",           // RFC3339 separator is uppercase T
	}
	for _, value := range invalid {
		if validTugQLRFC3339Timestamp(value) {
			t.Errorf("invalid RFC3339 timestamp accepted: %q", value)
		}
	}
}

func TestTugQLParserBoundaryClauseDiagnostics(t *testing.T) {
	cases := []struct {
		name, source, code string
	}{
		{"unsupported statement", "FROM T\nOFFSET 2", "unsupported_statement"},
		{"select before lower-ranked clause", "FROM T\nSELECT Id\nWHERE Id = 1", "clause_order"},
		{"select must be last with duplicate select", "FROM T\nSELECT A\nSELECT B", "select_must_be_last"},
		{"join body missing source", "FROM T\nJOIN\n  ON Id = 1", "invalid_join"},
		{"group expression parse failure", "FROM T\nGROUP BY Id +", "invalid_group_by"},
		{"order expression parse failure", "FROM T\nORDER BY Id +", "invalid_order_by"},
		{"nested boolean child parse failure", "FROM T\nWHERE Id = 1 OR", "invalid_condition"},
		{"invalid limit token count", "FROM T\nLIMIT 1 2", "invalid_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, diagnostics := parseTugQLDocument(tc.source)
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == tc.code {
					return
				}
			}
			t.Fatalf("missing %s diagnostic: %+v", tc.code, diagnostics)
		})
	}
}

func TestTugQLParserBoundaryParameterIndentAndTypedDefaults(t *testing.T) {
	cases := []struct {
		name, source, code string
	}{
		{"parameter indent styles must agree", "PARAMETERS (\n  @a INTEGER REQUIRED\n\t@b STRING REQUIRED\n)\nFROM T", "indentation"},
		{"decimal default accepts exact notation", "PARAMETERS (\n  @rate DECIMAL DEFAULT -12.3400\n)\nFROM T", ""},
		{"timestamp default reports invalid offset", "PARAMETERS (\n  @at TIMESTAMP DEFAULT '2024-02-29T12:00:00+24:00'\n)\nFROM T", "invalid_parameter"},
		{"timestamp default reports missing zone", "PARAMETERS (\n  @at DATETIME DEFAULT '2024-02-29T12:00:00'\n)\nFROM T", "invalid_parameter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, diagnostics := parseTugQLDocument(tc.source)
			if tc.code == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("valid parameter block rejected: %+v", diagnostics)
				}
				return
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == tc.code {
					return
				}
			}
			t.Fatalf("missing %s diagnostic: %+v", tc.code, diagnostics)
		})
	}
}

func TestTugQLParserBoundaryConditionPrecedenceAndDepth(t *testing.T) {
	source := "FROM T\nWHERE (A = 1 OR B = 2) AND C = 3"
	doc, _, diagnostics := parseTugQLDocument(source)
	if len(diagnostics) != 0 || doc.Query.Where == nil || len(doc.Query.Where.And) != 2 || len(doc.Query.Where.And[0].Or) != 2 {
		t.Fatalf("condition tree lost precedence: query=%+v diagnostics=%+v", doc.Query, diagnostics)
	}

	deep := "FROM T\nWHERE " + strings.Repeat("(", maxTugQLDepth) + "A = 1" + strings.Repeat(")", maxTugQLDepth)
	_, _, diagnostics = parseTugQLDocument(deep)
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "invalid_condition" {
			return
		}
	}
	t.Fatalf("maximum-depth condition should be rejected: %+v", diagnostics)
}

func TestTugQLParserBoundaryMalformedComparisons(t *testing.T) {
	for _, tc := range []struct {
		name, condition string
	}{
		{"invalid left expression", "+ = 1"},
		{"invalid right expression", "Id = +"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, diagnostics := parseTugQLDocument("FROM T\nWHERE " + tc.condition)
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "invalid_condition" {
					return
				}
			}
			t.Fatalf("missing invalid_condition: %+v", diagnostics)
		})
	}
}

func TestTugQLParserBoundaryDedentDiagnostics(t *testing.T) {
	cases := []struct {
		name, source, message string
	}{
		{"missing block indent", "from T", "block body must be indented one structural level"},
		{"indent must use a structural unit", " from T", "block body must add one tab or two spaces"},
		{"indent uses complete levels", "  from T\n   where Id = 1", "block body indentation must use consistent structural levels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Split(tc.source, "\n")
			tokens, lexDiags := lexTugQL(tc.source)
			if len(lexDiags) != 0 {
				t.Fatalf("test input must lex: %+v", lexDiags)
			}
			lines := linesFromTokens(tc.source, tokens)
			_, diagnostics := dedentTugQLBlock(raw, lines, 0, len(lines), "")
			if len(diagnostics) != 1 || diagnostics[0].Code != "indentation" || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("dedent diagnostics=%+v, want %q", diagnostics, tc.message)
			}
		})
	}

	raw := []string{"", "  "}
	tokens, lexDiags := lexTugQL(strings.Join(raw, "\n"))
	if len(lexDiags) != 0 {
		t.Fatalf("blank input must lex: %+v", lexDiags)
	}
	body, diagnostics := dedentTugQLBlock(raw, linesFromTokens(strings.Join(raw, "\n"), tokens), 0, len(raw), "")
	if len(diagnostics) != 0 || body != "\n" {
		t.Fatalf("blank block body=%q diagnostics=%+v", body, diagnostics)
	}
}

func TestTugQLParserBoundarySourceAndColumnHelpers(t *testing.T) {
	tokens, diagnostics := lexTugQL("sales.1")
	if len(diagnostics) != 0 {
		t.Fatalf("test input must lex: %+v", diagnostics)
	}
	if _, err := parseTugQLFrom(tokens); err == nil || !strings.Contains(err.Error(), "expected identifier after '.'") {
		t.Fatalf("source path error=%v", err)
	}

	columns, err := parseTugQLColumns(tokens[:1])
	if err != nil || len(columns) != 1 || columns[0].Field != "sales" {
		t.Fatalf("single field column=%+v error=%v", columns, err)
	}
	commaTokens, diagnostics := lexTugQL("Id,")
	if len(diagnostics) != 0 {
		t.Fatalf("test input must lex: %+v", diagnostics)
	}
	if _, err := parseTugQLColumns(commaTokens); err == nil || !strings.Contains(err.Error(), "empty list item") {
		t.Fatalf("trailing comma error=%v", err)
	}

	if _, err := parseTugQLDefault("OTHER", tugqlToken{Kind: tugqlIdentifier, Text: "x"}); err == nil || !strings.Contains(err.Error(), "unsupported parameter type") {
		t.Fatalf("unsupported default type error=%v", err)
	}
}

func TestTugQLParserBoundaryBlankLinesAndKeywordStyle(t *testing.T) {
	valid := "PARAMETERS (\n  @a INTEGER REQUIRED\n\n  @b STRING REQUIRED\n)\n\nWITH First AS (\n  FROM T\n)\n\nWITH Second AS (\n  FROM First\n)\nFROM Second"
	tree, _, diagnostics := parseTugQLDocument(valid)
	if len(diagnostics) != 0 || len(tree.Definitions) != 2 || len(tree.Parameters) != 2 {
		t.Fatalf("blank lines between declarations/definitions changed parse: definitions=%d parameters=%d diagnostics=%+v", len(tree.Definitions), len(tree.Parameters), diagnostics)
	}

	_, _, diagnostics = parseTugQLDocument("fRoM T")
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "keyword_case" {
			return
		}
	}
	t.Fatalf("mixed keyword case was accepted: %+v", diagnostics)
}

func TestTugQLParserBoundaryDefaultHelperTokens(t *testing.T) {
	tokens, diagnostics := lexTugQL("@a INTEGER DEFAULT 1 2 3")
	if len(diagnostics) != 0 {
		t.Fatalf("test input must lex: %+v", diagnostics)
	}
	if _, err := parseTugQLParameter(tokens); err == nil || !strings.Contains(err.Error(), "exactly one typed literal") {
		t.Fatalf("extra default values error=%v", err)
	}

	for _, value := range []string{
		"2024-02-29T12:00:00.1",     // fractional seconds still require a zone
		"2024-02-29T12:00:00+0x:00", // malformed timezone digit
	} {
		if validTugQLRFC3339Timestamp(value) {
			t.Errorf("invalid timestamp accepted: %q", value)
		}
	}
}

func TestTugQLParserBoundaryIndentationStylesAndSelectItems(t *testing.T) {
	cases := []struct {
		name, source, code string
	}{
		{"query clause indentation styles cannot mix", "FROM T\n\tWHERE Id = 1\n  ORDER BY Id", "indentation"},
		{"tab continuation is parsed", "FROM T\nJOIN U\n\tON A =\n\t\tB", ""},
		{"tab-indented select block is parsed", "FROM T\nSELECT (\n\tId\n)", ""},
		{"blank line in select block is ignored", "FROM T\nSELECT (\n  Id\n\n  Name\n)", ""},
		{"select items share one structural column", "FROM T\nSELECT (\n  Id\n    Name\n)", "invalid_select"},
		{"multiline select column expression error", "FROM T\nSELECT (\n  Id AS\n)", "invalid_select"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diagnostics := ParseTugQL(tc.source)
			if tc.code == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("valid source rejected: %+v", diagnostics)
				}
				return
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == tc.code {
					return
				}
			}
			t.Fatalf("missing %s diagnostic: %+v", tc.code, diagnostics)
		})
	}
}

func TestTugQLParserBoundaryConditionBranchErrors(t *testing.T) {
	cases := []string{
		"+ IS NULL",      // invalid expression before IS NULL
		"A = +",          // invalid comparison right side
		"A = 1 OR + = 2", // nested boolean child fails parsing
	}
	for _, condition := range cases {
		t.Run(condition, func(t *testing.T) {
			_, _, diagnostics := parseTugQLDocument("FROM T\nWHERE " + condition)
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "invalid_condition" {
					return
				}
			}
			t.Fatalf("missing invalid_condition for %q: %+v", condition, diagnostics)
		})
	}

	tokens, diagnostics := lexTugQL("A = 1 OR")
	if len(diagnostics) != 0 {
		t.Fatalf("test input must lex: %+v", diagnostics)
	}
	if _, err := parseTugQLCondition(tokens); err == nil {
		t.Fatal("trailing OR must be rejected by condition parser")
	}
}

func TestTugQLParserBoundaryLexFailureAndStyleTokenRange(t *testing.T) {
	_, _, diagnostics := parseTugQLDocument("FROM 'unterminated")
	if len(diagnostics) == 0 {
		t.Fatal("unterminated string should be rejected by the lexer")
	}
	_, queryDiagnostics := parseTugQLQueryAt("FROM 'unterminated", 0)
	if len(queryDiagnostics) == 0 {
		t.Fatal("query parser helper should preserve its lexical failure")
	}
	for _, index := range []int{-1, 1} {
		if isTugQLStyleToken([]tugqlToken{{Kind: tugqlIdentifier, Text: "FROM"}}, index) {
			t.Errorf("style token index %d unexpectedly accepted", index)
		}
	}
	if validTugQLRFC3339Timestamp("2024-0x-29T12:00:00Z") {
		t.Fatal("timestamp with non-digit date field should be rejected")
	}
}

func TestTugQLParserBoundaryScalarSubqueryBlockFailures(t *testing.T) {
	cases := []struct {
		name, source string
	}{
		{"scalar closing delimiter aligns with name", "FROM T\nSELECT (\n  Child AS (\n    FROM U\n   )\n)"},
		{"scalar nested body has complete indentation levels", "FROM T\nSELECT (\n  Child AS (\n    FROM U\n     WHERE Id = 1\n  )\n)"},
		{"tab scalar reports nested query diagnostic", "FROM T\nSELECT (\n\tChild AS (\n\t\tFROM U\n\t\tWHERE Id =\n\t)\n)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diagnostics := ParseTugQL(tc.source)
			if len(diagnostics) == 0 {
				t.Fatal("malformed scalar query was accepted")
			}
		})
	}
}

func TestTugQLParserBoundaryScalarHelperRejectsMalformedNestedIndent(t *testing.T) {
	source := "FROM T\nSELECT (\n  Child AS (\n    FROM U\n     WHERE Id = 1\n  )\n)"
	tokens, diagnostics := lexTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("test input must lex: %+v", diagnostics)
	}
	selectAt := -1
	for i, token := range tokens {
		if isKeyword(token, "select") {
			selectAt = i
			break
		}
	}
	if selectAt < 0 || selectAt+1 >= len(tokens) || tokens[selectAt+1].Text != "(" {
		t.Fatalf("missing multiline SELECT header: %+v", tokens)
	}
	depth, closeAt := 1, -1
	for i := selectAt + 2; i < len(tokens); i++ {
		switch tokens[i].Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				closeAt = i
			}
		}
		if closeAt >= 0 {
			break
		}
	}
	if closeAt < 0 {
		t.Fatal("missing SELECT block close")
	}
	items, err := splitTugQLSelectBlock(tokens[selectAt+2 : closeAt])
	if err != nil || len(items) != 1 || !isTugQLScalarItem(items[0]) {
		t.Fatalf("scalar item=%+v error=%v", items, err)
	}
	rawLines := strings.Split(normalizeTugQLNewlines(source), "\n")
	_, err = parseTugQLScalarColumn(items[0], rawLines, linesFromTokens(source, tokens), 0)
	if err == nil || !strings.Contains(err.Error(), "block body indentation must use consistent structural levels") {
		t.Fatalf("malformed nested indentation error=%v", err)
	}
}
