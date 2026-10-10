package dtql

import (
	"strings"
	"testing"
)

func TestTugQLSyntaxMorePublicClauseDiagnostics(t *testing.T) {
	cases := []struct {
		name, source, code string
	}{
		{"query must begin with from", "where id = 1", "expected_from"},
		{"unknown statement", "from T\nOFFSET 1", "unsupported_statement"},
		{"duplicate where", "from T\nwhere A = 1\nwhere B = 2", "duplicate_clause"},
		{"select must be last", "from T\nselect Id\nwhere Active = true", "clause_order"},
		{"one query body", "from T\nfrom U", "multiple_statements"},
		{"invalid root source", "from 7", "invalid_from"},
		{"invalid join source", "from T\njoin", "invalid_join"},
		{"on requires join", "from T\non A = 1", "indentation"},
		{"malformed join condition", "from T\njoin U\n  on A =", "invalid_condition"},
		{"malformed where condition", "from T\nwhere A =", "invalid_condition"},
		{"malformed having condition", "from T\nhaving A is", "invalid_condition"},
		{"empty group list", "from T\ngroup by", "invalid_group_by"},
		{"empty order list", "from T\norder by", "invalid_order_by"},
		{"fractional limit", "from T\nlimit 1.5", "invalid_limit"},
		{"limit overflows int", "from T\nlimit 999999999999999999999999999", "invalid_limit"},
		{"clause order", "from T\norder by Id\nwhere Active = true", "clause_order"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tugqlSyntaxWantCode(t, tc.source, tc.code) })
	}
}

func TestTugQLSyntaxMoreAcceptedReadableSourceForms(t *testing.T) {
	cases := []struct {
		name, source string
	}{
		{"lowercase compact clauses and comments", "-- from ignored\nfrom \"Order\" as o\nwhere o.Name = 'from,select' -- retained comment\nselect o.Id as Id, COUNT(*) as Total"},
		{"uppercase clauses and multiline select", "PARAMETERS (\n  @limit INTEGER REQUIRED\n)\nWITH Recent AS (\n  FROM Invoice AS i\n  WHERE i.Id > @limit\n  SELECT (\n    i.Id AS Id\n    COUNT(*) AS Total\n  )\n)\nFROM Recent AS r\nSELECT r.Id"},
		{"tab-indented join condition", "FROM Invoice AS i\nJOIN Customer AS c\n\tON c.Id = i.CustomerId\nSELECT i.Id"},
		{"nested scalar query", "from Invoice as i\nselect (\n  Customer as (\n    from Customer as c\n    where c.Id = i.CustomerId\n    select c.Name\n  )\n)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, diagnostics := ParseTugQL(tc.source)
			if len(diagnostics) != 0 || doc.Tree == nil {
				t.Fatalf("accepted source rejected: diagnostics=%+v source=%q", diagnostics, tc.source)
			}
			if doc.Source != tc.source {
				t.Fatalf("ParseTugQL changed authoring source: got %q want %q", doc.Source, tc.source)
			}
		})
	}
}

func TestTugQLSyntaxMoreParameterDeclarationContracts(t *testing.T) {
	valid := "parameters (\n" +
		"  @whole integer default +42\n" +
		"  @fraction decimal default -0.1250\n" +
		"  @label string default 'it''s ready'\n" +
		"  @enabled boolean default false\n" +
		"  @day date default '2024-02-29'\n" +
		"  @created datetime default '2024-02-29T12:30:00Z'\n" +
		"  @updated timestamp default '2024-02-29T12:30:00.125Z'\n" +
		")\nfrom T\nselect Id"
	doc, diagnostics := ParseTugQL(valid)
	if len(diagnostics) != 0 || doc.Tree == nil || len(doc.Tree.Parameters) != 7 {
		t.Fatalf("all supported parameter types should parse: tree=%+v diagnostics=%+v", doc.Tree, diagnostics)
	}
	if got := *doc.Tree.Parameters[2].Default; got != "it's ready" {
		t.Fatalf("escaped string default=%#v", got)
	}
	if got := *doc.Tree.Parameters[3].Default; got != false {
		t.Fatalf("false default=%#v", got)
	}

	cases := []struct {
		name, declaration string
	}{
		{"unknown type", "@p float required"},
		{"invalid name", "@p$ integer required"},
		{"no mode", "@p integer"},
		{"required and default", "@p integer required default 1"},
		{"extra default literal", "@p integer default 1 2"},
		{"integer receives decimal", "@p integer default 1.25"},
		{"decimal receives malformed number", "@p decimal default 1.2.3"},
		{"string must be quoted", "@p string default value"},
		{"boolean must be boolean", "@p boolean default 0"},
		{"date must be quoted", "@p date default 123"},
		{"date must exist", "@p date default '2024-02-30'"},
		{"datetime timestamp must parse", "@p datetime default 'not-a-time'"},
		{"timestamp must be quoted", "@p timestamp default 2024"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "parameters (\n  " + tc.declaration + "\n)\nfrom T"
			tugqlSyntaxWantCode(t, source, "invalid_parameter")
		})
	}
	for _, tc := range []struct {
		name, source, code string
	}{
		{"duplicate names ignore case", "parameters (\n  @Id integer required\n  @id string required\n)\nfrom T", "duplicate_parameter"},
		{"parameters block missing close", "parameters (\n  @p integer required\nfrom T", "unclosed_parameters"},
		{"parameters declaration comma forbidden", "parameters (\n  @p integer required,\n)\nfrom T", "invalid_parameter"},
		{"parameter indentation one level", "parameters (\n @p integer required\n)\nfrom T", "indentation"},
	} {
		t.Run(tc.name, func(t *testing.T) { tugqlSyntaxWantCode(t, tc.source, tc.code) })
	}
}

func TestTugQLSyntaxMoreDefinitionAndBlockContracts(t *testing.T) {
	cases := []struct {
		name, source, code string
	}{
		{"with declaration needs a name and kind", "with\nfrom T", "invalid_with"},
		{"as parenthesis stays on header", "with C as ( from T )\nfrom C", "invalid_cte"},
		{"cte must close on its own line", "with C as (\n  from T\nfrom C", "unclosed_cte"},
		{"cte is top level", "  with C as (\n    from T\n  )\nfrom C", "indentation"},
		{"nested parameters are forbidden", "with C as (\n  parameters (\n    @p integer required\n  )\n  from T\n)\nfrom C", "nested_parameters"},
		{"import path is quoted", "with I from path.tugql\nfrom I", "invalid_with"},
		{"using header has no trailing expression", "with I from \"i.tugql\"\n  using ( extra\n    @p = 1\n  )\nfrom I", "invalid_using"},
		{"using block must close", "with I from \"i.tugql\"\n  using (\n    @p = 1\nfrom I", "unclosed_using"},
		{"using maps need a structural level", "with I from \"i.tugql\"\n  using (\n  @p = 1\n  )\nfrom I", "indentation"},
		{"using expression must parse", "with I from \"i.tugql\"\n  using (\n    @p =\n  )\nfrom I", "invalid_mapping"},
		{"document needs main query after definitions", "with C as (\n  from T\n)", "expected_from"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tugqlSyntaxWantCode(t, tc.source, tc.code) })
	}
}

func TestTugQLSyntaxMoreFormattingPreservesSourceAndIsIdempotent(t *testing.T) {
	source := "-- SELECT words in comments and 'FROM' text stay untouched\r\nfrom \"select\" as s\r\nselect s.Name as Name, 'where' as Literal"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("source parse diagnostics: %+v", diagnostics)
	}
	formatted, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "tab"})
	if len(diagnostics) != 0 {
		t.Fatalf("format diagnostics: %+v", diagnostics)
	}
	if !strings.Contains(formatted, "FROM \"select\"") || !strings.Contains(formatted, "'where'") || !strings.Contains(formatted, "-- SELECT words") {
		t.Fatalf("formatter changed non-keyword text: %q", formatted)
	}
	formattedDoc, diagnostics := ParseTugQL(formatted)
	if len(diagnostics) != 0 {
		t.Fatalf("formatted source did not parse: %+v\n%s", diagnostics, formatted)
	}
	again, diagnostics := FormatTugQL(formattedDoc, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "tab"})
	if len(diagnostics) != 0 || again != formatted {
		t.Fatalf("formatter not idempotent: first=%q second=%q diagnostics=%+v", formatted, again, diagnostics)
	}
}

func TestTugQLSyntaxMoreFormattingPreferenceValidation(t *testing.T) {
	invalid := []struct {
		name    string
		doc     TugQLDocument
		options TugQLFormatOptions
		code    string
	}{
		{"keyword settings conflict", TugQLDocument{Source: "from T"}, TugQLFormatOptions{KeywordCase: "lowercase", ProjectTeamKeywordCase: "uppercase"}, "conflicting_format_preference"},
		{"indent settings conflict", TugQLDocument{Source: "from T"}, TugQLFormatOptions{Indentation: "tab", ProjectTeamIndentation: "two-spaces"}, "conflicting_format_preference"},
		{"invalid keyword option", TugQLDocument{Source: "from T"}, TugQLFormatOptions{KeywordCase: "title"}, "invalid_format_option"},
		{"invalid indentation option", TugQLDocument{Source: "from T"}, TugQLFormatOptions{Indentation: "four-spaces"}, "invalid_format_option"},
		{"source required", TugQLDocument{}, TugQLFormatOptions{}, "source_required"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			formatted, diagnostics := FormatTugQL(tc.doc, tc.options)
			if formatted != "" || len(diagnostics) != 1 || diagnostics[0].Code != tc.code {
				t.Fatalf("formatted=%q diagnostics=%+v", formatted, diagnostics)
			}
		})
	}
	invalidSource := "from T\nwhere A ="
	doc, _ := ParseTugQL(invalidSource)
	formatted, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "preserve-existing", Indentation: "preserve-existing"})
	if formatted != invalidSource || len(diagnostics) == 0 {
		t.Fatalf("preserve-existing must keep repairable source: formatted=%q diagnostics=%+v", formatted, diagnostics)
	}

	defaulted, diagnostics := FormatTugQL(TugQLDocument{Source: "fRom T\nsElect Id"}, TugQLFormatOptions{DefaultKeywordCase: "uppercase", DefaultIndentation: "tab"})
	if len(diagnostics) != 0 || !strings.HasPrefix(defaulted, "FROM T\nSELECT Id") {
		t.Fatalf("application defaults were not used when source styles were ambiguous: %q %+v", defaulted, diagnostics)
	}
}

func TestTugQLSyntaxMoreExpressionParserEdges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tokens []tugqlToken
		want   string
	}{
		{"empty input", nil, "expected expression"},
		{"binary right side error propagates", []tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: "+"}, {Kind: tugqlSymbol, Text: ","}}, "unexpected token"},
		{"function call must close", []tugqlToken{{Kind: tugqlIdentifier, Text: "F"}, {Kind: tugqlSymbol, Text: "("}, {Kind: tugqlIdentifier, Text: "x"}}, "missing closing parenthesis"},
		{"qualifier needs a field", []tugqlToken{{Kind: tugqlIdentifier, Text: "x"}, {Kind: tugqlSymbol, Text: "."}, {Kind: tugqlNumber, Text: "1"}}, "expected field name"},
		{"unsupported token kind", []tugqlToken{{Kind: tugqlTokenKind(255), Text: "?"}}, "expected expression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTugQLExpression(tc.tokens)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want substring %q", err, tc.want)
			}
		})
	}
	falseValue, err := parseTugQLExpression([]tugqlToken{{Kind: tugqlIdentifier, Text: "false"}})
	if err != nil || falseValue.Value == nil || *falseValue.Value != false {
		t.Fatalf("false literal=%+v error=%v", falseValue, err)
	}
}

func TestTugQLSyntaxMoreIdentifierAndDecimalPredicates(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  bool
		want bool
	}{
		{"empty parameter name", validTugQLParamName(""), false},
		{"leading digit", validTugQLParamName("1x"), false},
		{"punctuation", validTugQLParamName("p$"), false},
		{"unicode identifier", validTugQLParamName("値2"), true},
		{"empty decimal", validExactDecimal(""), false},
		{"sign without digits", validExactDecimal("+"), false},
		{"signed decimal", validExactDecimal("-0.1250"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

func TestTugQLSyntaxMoreLexerUnicodeQuotesAndLimits(t *testing.T) {
	if tokens, diagnostics := lexTugQL("x\r\xff"); len(tokens) != 0 || len(diagnostics) != 1 || diagnostics[0].Span.Start != (tugqlPosition{Line: 2, Column: 1}) {
		t.Fatalf("invalid UTF-8 after CR tokens=%+v diagnostics=%+v", tokens, diagnostics)
	}
	for _, source := range []string{"from T\nselect 'open\nstill open'", "from T\nselect \"open\nstill open\"", "from T\nselect `open\nstill open`"} {
		_, diagnostics := lexTugQL(source)
		if len(diagnostics) == 0 || diagnostics[0].Code != "unterminated_quote" || diagnostics[0].Span.Start.Line != 2 {
			t.Fatalf("unterminated multiline quote %q diagnostics=%+v", source, diagnostics)
		}
	}
	if tokens, diagnostics := lexTugQL("\r"); len(tokens) != 0 || len(diagnostics) != 0 {
		t.Fatalf("lone CR at EOF tokens=%+v diagnostics=%+v", tokens, diagnostics)
	}
	if tokens, diagnostics := lexTugQL("@"); len(tokens) != 0 || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_parameter" {
		t.Fatalf("bare parameter marker tokens=%+v diagnostics=%+v", tokens, diagnostics)
	}
	_, diagnostics := lexTugQL(strings.Repeat("x ", maxTugQLTokens+1))
	if len(diagnostics) != 1 || diagnostics[0].Code != "too_many_tokens" {
		t.Fatalf("token limit diagnostics=%+v", diagnostics)
	}

	if got := unquoteString("x"); got != "x" {
		t.Fatalf("single-rune malformed string token should pass through unchanged, got %q", got)
	}
	if got := spanForLine(tugqlLine{Number: 7}); got != (tugqlSpan{Start: tugqlPosition{Line: 7, Column: 1}, End: tugqlPosition{Line: 7, Column: 1}}) {
		t.Fatalf("empty line span=%+v", got)
	}
}

func TestTugQLSyntaxMoreNestedAndIndentedBlockBoundaries(t *testing.T) {
	validWithUsing := "with External from \"external.tugql\"\n" +
		"\tusing (\n\t\t@tenant = @tenant\n\t)\n" +
		"from External\nselect Id"
	doc, diagnostics := ParseTugQL(validWithUsing)
	if len(diagnostics) != 0 || doc.Tree == nil || len(doc.Tree.Definitions) != 1 || len(doc.Tree.Definitions[0].Using) != 1 {
		t.Fatalf("tab-indented import mapping rejected: tree=%+v diagnostics=%+v", doc.Tree, diagnostics)
	}

	for _, tc := range []struct {
		name, source, code string
	}{
		{"parameter declarations change indent", "parameters (\n  @a integer required\n    @b string required\n)\nfrom T", "indentation"},
		{"parameters must be top level", "  parameters (\n    @a integer required\n  )\nfrom T", "indentation"},
		{"parameters requires open delimiter on header", "parameters\n  @a integer required\nfrom T", "invalid_parameters"},
		{"tab and spaces are not mixed", "with C as (\n\tfrom T\n  )\nfrom C", "indentation"},
		{"mixed prefix whitespace rejected", "from T\n \twhere A = 1", "indentation"},
		{"cte query body needs one structural indent", "with C as (\nfrom T\n)\nfrom C", "indentation"},
		{"using blank rows are ignored", "with I from \"i.tugql\"\n  using (\n\n    @p = 1\n  )\nfrom I", ""},
		{"mapping expression error is surfaced", "with I from \"i.tugql\"\n  using (\n    @p = 1 +\n  )\nfrom I", "invalid_mapping"},
		{"multiline select items reject commas", "from T\nselect (\n  A,\n  B\n)", "invalid_select"},
		{"multiline select requires an item", "from T\nselect (\n)", "invalid_select"},
		{"multiline select close must align", "from T\nselect (\n  A\n  )", "select_block_close"},
		{"multiline select item must indent", "from T\nselect (\nA\n)", "indentation"},
		{"multiline select must close", "from T\nselect (\n  A", "unclosed_select_block"},
		{"condition depth limited", "from T\nwhere " + strings.Repeat("(", maxTugQLDepth+1) + "A = 1" + strings.Repeat(")", maxTugQLDepth+1), "invalid_condition"},
		{"group expression depth limited", "from T\ngroup by " + strings.Repeat("(", maxTugQLDepth+1) + "1" + strings.Repeat(")", maxTugQLDepth+1), "invalid_group_by"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code == "" {
				parsed, diags := ParseTugQL(tc.source)
				if len(diags) != 0 || parsed.Tree == nil {
					t.Fatalf("valid block rejected: %+v", diags)
				}
				return
			}
			tugqlSyntaxWantCode(t, tc.source, tc.code)
		})
	}
}

func TestTugQLSyntaxMoreOptionalClauseAndExpressionBoundaries(t *testing.T) {
	valid := "from sales.Invoice as i\n" +
		"join Customer as c\n  on c.Id = i.CustomerId\n" +
		"where (i.Total >= 0 and c.Active = true)\n" +
		"group by i.CustomerId\n" +
		"having COUNT(*) > 0\n" +
		"order by i.Total desc, i.Id asc\n" +
		"limit 25\nselect i.Id as Id, COALESCE(i.Note, 'unknown') as Note"
	doc, diagnostics := ParseTugQL(valid)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("valid complete clause sequence rejected: %+v", diagnostics)
	}
	if len(doc.Tree.Query["from"].(map[string]any)) == 0 || len(doc.Tree.Query["columns"].([]any)) != 2 {
		t.Fatalf("parsed query omitted source/projections: %#v", doc.Tree.Query)
	}

	for _, tc := range []struct {
		name, source, code string
	}{
		{"source alias requires AS", "from T x", "invalid_from"},
		{"source alias requires one identifier", "from T as x y", "invalid_from"},
		{"qualified source needs identifier", "from sales.", "invalid_from"},
		{"empty expression around AND", "from T\nwhere and A = 1", "invalid_condition"},
		{"missing expression after OR", "from T\nwhere A = 1 or", "invalid_condition"},
		{"comparison requires both sides", "from T\nwhere = 1", "invalid_condition"},
		{"qualified wildcard is unsupported", "from T\nselect t.*", "invalid_select"},
		{"alias requires AS and one identifier", "from T\nselect Id as", "invalid_select"},
		{"invalid scalar subquery body reports syntax", "from T\nselect (\n  Child as (\n    from U\n    where A =\n  )\n)", "invalid_select"},
		{"scalar subquery needs multiline body", "from T\nselect (\n  Child as ()\n)", "invalid_select"},
	} {
		t.Run(tc.name, func(t *testing.T) { tugqlSyntaxWantCode(t, tc.source, tc.code) })
	}
}

func TestTugQLSyntaxMoreNestedQueryDepthLimit(t *testing.T) {
	query := "from Base"
	for i := 0; i < maxTugQLDepth+1; i++ {
		indented := "  " + strings.ReplaceAll(query, "\n", "\n  ")
		query = "with C" + strings.Repeat("x", i%7) + " as (\n" + indented + "\n)\nfrom C" + strings.Repeat("x", i%7)
	}
	_, diagnostics := ParseTugQL(query)
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "nesting_limit" {
			return
		}
	}
	t.Fatalf("expected nested query depth diagnostic, got %d diagnostics (first: %+v)", len(diagnostics), diagnostics[:minInt(len(diagnostics), 3)])
}

func TestTugQLSyntaxMoreFormatterRepairsMixedCaseAndNestedBlocks(t *testing.T) {
	mixed := TugQLDocument{Source: "fRom T\nsElect Id"}
	repaired, diagnostics := FormatTugQL(mixed, TugQLFormatOptions{KeywordCase: "preserve-existing", DefaultKeywordCase: "uppercase", Indentation: "preserve-existing", DefaultIndentation: "tab"})
	if len(diagnostics) != 0 || !strings.HasPrefix(repaired, "FROM T\nSELECT Id") {
		t.Fatalf("mixed reserved-word case was not repaired via defaults: %q %+v", repaired, diagnostics)
	}
	if parsed, diagnostics := ParseTugQL(repaired); len(diagnostics) != 0 || parsed.Tree == nil {
		t.Fatalf("repaired source did not parse: %+v", diagnostics)
	}

	source := "parameters (\n  @tenant integer required\n)\n" +
		"with Imported from \"saved.tugql\"\n  using (\n\n    @tenant = @tenant\n  )\n" +
		"with Joined as (\n  from Invoice as i\n  join Customer as c\n    on c.Id = i.CustomerId\n  select (\n    i.Id as Id\n    Customer as (\n      from Customer as nested\n      select nested.Name\n    )\n  )\n)\n" +
		"from Joined\nselect Id\n"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("nested formatter source rejected: %+v", diagnostics)
	}
	formatted, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "lowercase", Indentation: "two-spaces"})
	if len(diagnostics) != 0 {
		t.Fatalf("nested formatting diagnostics: %+v", diagnostics)
	}
	formattedDoc, diagnostics := ParseTugQL(formatted)
	if len(diagnostics) != 0 {
		t.Fatalf("formatted nested source rejected: %+v\n%s", diagnostics, formatted)
	}
	again, diagnostics := FormatTugQL(formattedDoc, TugQLFormatOptions{KeywordCase: "lowercase", Indentation: "two-spaces"})
	if len(diagnostics) != 0 || again != formatted {
		t.Fatalf("nested formatter is not idempotent: diagnostics=%+v\nfirst:\n%s\nsecond:\n%s", diagnostics, formatted, again)
	}
}

func TestTugQLSyntaxMoreFormatterRepairsIndentStyleAndBoundsNesting(t *testing.T) {
	for _, source := range []string{
		"from T\n where A = 1\n select Id",
		"from T\n  where A = 1\n\tselect Id",
	} {
		doc := TugQLDocument{Source: source}
		formatted, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "preserve-existing", Indentation: "preserve-existing", DefaultIndentation: "two-spaces"})
		if len(diagnostics) != 0 || formatted != "from T\nwhere A = 1\nselect Id" {
			t.Fatalf("malformed indentation wasn't repaired from configured default: source=%q formatted=%q diagnostics=%+v", source, formatted, diagnostics)
		}
	}

	query := "from T"
	for i := 0; i < maxTugQLDepth+1; i++ {
		name := "C" + strings.Repeat("x", i%5)
		query = "with " + name + " as (\n  " + strings.ReplaceAll(query, "\n", "\n  ") + "\n)\nfrom " + name
	}
	formatted, diagnostics := FormatTugQL(TugQLDocument{Source: query}, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "two-spaces"})
	if len(diagnostics) == 0 || formatted == "" {
		t.Fatalf("deep formatter input should return a formatted buffer and parse diagnostics: formatted=%q diagnostics=%+v", formatted, diagnostics)
	}
}

func TestTugQLSyntaxMoreFormatterPreservesLexerFatalBuffer(t *testing.T) {
	source := "fRom T\nselect 'unfinished"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) == 0 || doc.Tree != nil {
		t.Fatalf("unterminated string did not remain a failed parse: doc=%+v diagnostics=%+v", doc, parseDiagnostics)
	}
	formatted, diagnostics := FormatTugQL(doc, TugQLFormatOptions{KeywordCase: "uppercase", Indentation: "tab"})
	if formatted != source || len(diagnostics) == 0 {
		t.Fatalf("formatter altered lexer-fatal source: got=%q want=%q diagnostics=%+v", formatted, source, diagnostics)
	}
}

func TestTugQLSyntaxMoreExpressionContinuationIndentation(t *testing.T) {
	valid := "from T\nwhere (Id =\n  1)\nselect Id"
	if _, diagnostics := ParseTugQL(valid); len(diagnostics) != 0 {
		t.Fatalf("two-space continuation rejected: %+v", diagnostics)
	}
	invalid := "from T\nwhere (Id =\n 1)\nselect Id"
	tugqlSyntaxWantCode(t, invalid, "indentation")
}

func tugqlSyntaxWantCode(t *testing.T, source, code string) {
	t.Helper()
	_, diagnostics := ParseTugQL(source)
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("source %q: expected diagnostic %q, got %+v", source, code, diagnostics)
}
