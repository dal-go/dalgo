package tugql

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parsedCommentTree(t *testing.T, source string) TugQLTree {
	t.Helper()
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	return *doc.Tree
}

func TestLexTugQLWithCommentsMarkersAndLiterals(t *testing.T) {
	tokens, comments, diagnostics := lexTugQLWithComments("a // one\n'x//y' \"p--q\" `r//s` -- two\n// three")
	if len(diagnostics) != 0 || len(tokens) != 4 {
		t.Fatalf("tokens=%d diagnostics=%+v", len(tokens), diagnostics)
	}
	want := []tugqlComment{
		{Marker: "//", Text: "one", Line: 1, Column: 3, Trailing: true},
		{Marker: "--", Text: "two", Line: 2, Column: 22, Trailing: true},
		{Marker: "//", Text: "three", Line: 3, Column: 1},
	}
	if !reflect.DeepEqual(comments, want) {
		t.Fatalf("comments=%+v want=%+v", comments, want)
	}
	// Division is still division.
	tokens, comments, _ = lexTugQLWithComments("a / b")
	if len(tokens) != 3 || len(comments) != 0 {
		t.Fatalf("tokens=%d comments=%d", len(tokens), len(comments))
	}
}

func TestLexTugQLWithCommentsErrors(t *testing.T) {
	if _, comments, diagnostics := lexTugQLWithComments("// ok\n\xff"); comments != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_utf8" {
		t.Fatalf("comments=%+v diagnostics=%+v", comments, diagnostics)
	}
}

func TestParseTugQLCommentsNotSeparatedByBlankLinesWhenCodeIsBetween(t *testing.T) {
	tree := parsedCommentTree(t, "-- cte\nwith A as (\n  from U // inner\n  select Id\n)\n\n-- own\nfrom A\nselect Id")
	got := tree.Comments.Anchors["query.from"].Leading
	want := []TugQLComment{{Text: "inner"}, {Text: "own"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("leading=%+v want=%+v", got, want)
	}
}

func TestParseTugQLWithoutCommentsHasNoCommentSet(t *testing.T) {
	if tree := parsedCommentTree(t, "from T\nselect Id"); tree.Comments != nil {
		t.Fatalf("comments=%+v", tree.Comments)
	}
}

func TestTugQLTreeCommentsJSON(t *testing.T) {
	tree := parsedCommentTree(t, "-- a\nfrom T // b\nselect Id\n// z")
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"comments":{"anchors":{"query.from":{"leading":[{"text":"a"}],"trailing":{"text":"b"}}},"end":[{"text":"z"}]}`) {
		t.Fatalf("json=%s", data)
	}
	var decoded TugQLTree
	if err := json.Unmarshal(data, &decoded); err != nil || !reflect.DeepEqual(decoded.Comments, tree.Comments) {
		t.Fatalf("decoded=%+v err=%v", decoded.Comments, err)
	}
}

func TestTugQLTreeYAMLCarriesCommentsBothWays(t *testing.T) {
	tree := parsedCommentTree(t, "-- a\nfrom T // b\nselect Id\n// z")
	data, err := yaml.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var back TugQLTree
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	// yaml.Unmarshal hands the decoder only the root mapping, so the
	// document-level end comment is not visible; YAMLToTugQL reads it.
	want := *tree.Comments
	want.End = nil
	if back.Comments == nil || !reflect.DeepEqual(back.Comments.Anchors, want.Anchors) || len(back.Comments.End) != 0 {
		t.Fatalf("comments=%+v want=%+v\n%s", back.Comments, want, data)
	}
	tree.Comments = nil
	plain, err := yaml.Marshal(tree)
	if err != nil || strings.Contains(string(plain), "#") {
		t.Fatalf("plain yaml has comments: %s %v", plain, err)
	}
}

func TestTugQLTreeMarshalRejectsMultilineComments(t *testing.T) {
	tree := parsedCommentTree(t, "from T\nselect Id")
	for name, comments := range map[string]*TugQLComments{
		"leading":  {Anchors: map[string]TugQLCommentGroup{"query.from": {Leading: []TugQLComment{{Text: "a\nb"}}}}},
		"trailing": {Anchors: map[string]TugQLCommentGroup{"query.from": {Trailing: &TugQLComment{Text: "a\rb"}}}},
		"end":      {End: []TugQLComment{{Text: "a\nb"}}},
	} {
		tree.Comments = comments
		if _, err := yaml.Marshal(tree); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if _, diagnostics := PrintTugQL(tree); len(diagnostics) != 1 || diagnostics[0].Code != "invalid_comments" {
			t.Fatalf("%s: diagnostics=%+v", name, diagnostics)
		}
	}
}

func TestTugQLCommentsWithoutAnchorMoveToDocumentEnd(t *testing.T) {
	tree := parsedCommentTree(t, "from T\nselect Id")
	tree.Comments = &TugQLComments{
		Anchors: map[string]TugQLCommentGroup{
			"query.from":             {Leading: []TugQLComment{{Text: "kept"}}},
			"query.from.joins[4]":    {Leading: []TugQLComment{{Text: "no join"}}, Trailing: &TugQLComment{Text: "no join trail"}},
			"query.where.left":       {Leading: []TugQLComment{{Text: "below scalar"}}},
			"query.from.name.x":      {Leading: []TugQLComment{{Text: "past scalar"}}},
			"query.from.name[0]":     {Leading: []TugQLComment{{Text: "scalar index"}}},
			"definitions[0]":         {Leading: []TugQLComment{{Text: "no definitions"}}},
			"query.missing":          {Trailing: &TugQLComment{Text: "missing"}},
			"parameters[bad]":        {Leading: []TugQLComment{{Text: "bad index"}}},
			"parameters[-1]":         {Leading: []TugQLComment{{Text: "negative index"}}},
			"query.from.joins[0]":    {Leading: []TugQLComment{{Text: "no joins"}}},
			"query.columns[0].field": {Leading: []TugQLComment{{Text: "not an anchor"}}},
		},
		End: []TugQLComment{{Text: "end"}},
	}
	text, diagnostics := PrintTugQL(tree)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
	for _, want := range []string{"-- kept\nfrom T", "-- end", "-- no join", "-- no join trail", "-- below scalar", "-- past scalar", "-- scalar index", "-- no definitions", "-- missing", "-- bad index", "-- negative index", "-- no joins", "-- not an anchor"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	data, err := yaml.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# kept\n    from:", "# end\n", "# no join trail"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in:\n%s", want, data)
		}
	}
}

func TestIsTugQLAnchorPath(t *testing.T) {
	anchors := []string{"parameters", "definitions[0]", "definitions[12]", "query.from", "query.from.joins[0]", "query.from.joins[3].on[1]", "query.where", "query.groupBy", "query.having", "query.orderBy", "query.limit", "query.columns"}
	for _, path := range anchors {
		if !isTugQLAnchorPath(path) {
			t.Errorf("%s should be an anchor", path)
		}
	}
	others := []string{"", "format", "query", "query.from.name", "definitions", "definitions[0].name", "query.from.joins", "query.from.joins[0", "query.from.joins[0]x", "query.from.joins[0].on", "query.from.joins[0].on[1].op", "query.from.joins[0].from", "query.columns[0]", "definitions[0].query.query.from"}
	for _, path := range others {
		if isTugQLAnchorPath(path) {
			t.Errorf("%s should not be an anchor", path)
		}
	}
}

func TestTugQLYAMLSlotLineNode(t *testing.T) {
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "x"}
	block := &yaml.Node{Kind: yaml.MappingNode}
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "k"}
	if (tugqlYAMLSlot{key: key, value: scalar}).lineNode() != scalar || (tugqlYAMLSlot{key: key, value: block}).lineNode() != key {
		t.Fatal("entry line node")
	}
	itemScalar := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, scalar}}
	itemBlock := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, block}}
	if (tugqlYAMLSlot{value: itemScalar}).lineNode() != scalar || (tugqlYAMLSlot{value: itemBlock}).lineNode() != key {
		t.Fatal("item line node")
	}
	if (tugqlYAMLSlot{value: scalar}).lineNode() != scalar || (tugqlYAMLSlot{value: block}).lineNode() != block {
		t.Fatal("fallback line node")
	}
}

func TestReadTugQLCommentsKeepsUnexpectedYAMLComments(t *testing.T) {
	data := "# top\n\nformat: tugqtree # on format\nversion: 1\nquery:\n    from:\n        name: T # on name\n        # inside from\n    limit: 5 # c1\n    columns:\n        - field: Id\n        # after item\n# foot\n\n# last\n"
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(data), &document); err != nil {
		t.Fatal(err)
	}
	comments := readTugQLCommentsFromNode(&document, document.Content[0])
	if comments == nil {
		t.Fatal("no comments read")
	}
	var texts []string
	for _, c := range comments.Anchors["query.from"].Leading {
		texts = append(texts, c.Text)
	}
	if !reflect.DeepEqual(texts, []string{"top", "on format"}) {
		t.Fatalf("from leading=%v", texts)
	}
	if got := comments.Anchors["query.limit"]; len(got.Leading) != 2 || got.Leading[0].Text != "on name" || got.Leading[1].Text != "inside from" || got.Trailing == nil || got.Trailing.Text != "c1" {
		t.Fatalf("limit=%+v", got)
	}
	var end []string
	for _, c := range comments.End {
		end = append(end, c.Text)
	}
	if !reflect.DeepEqual(end, []string{"after item", "foot", "last"}) {
		t.Fatalf("end=%v", end)
	}
}

func TestReadTugQLCommentsMultilineLineCommentIsKept(t *testing.T) {
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "limit"}
	value := &yaml.Node{Kind: yaml.ScalarNode, Value: "5", LineComment: "# one\n# two"}
	from := &yaml.Node{Kind: yaml.ScalarNode, Value: "from"}
	fromValue := &yaml.Node{Kind: yaml.MappingNode}
	query := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{from, fromValue, key, value}}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "query"}, query}}
	comments := readTugQLCommentsFromNode(nil, root)
	if comments.Anchors["query.limit"].Trailing.Text != "one" || len(comments.End) != 1 || comments.End[0].Text != "two" {
		t.Fatalf("comments=%+v", comments)
	}
}

func TestReadTugQLCommentsNone(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("format: tugqtree\nversion: 1\n"), &document); err != nil {
		t.Fatal(err)
	}
	if comments := readTugQLCommentsFromNode(&document, document.Content[0]); comments != nil {
		t.Fatalf("comments=%+v", comments)
	}
}

func TestYAMLToTugQLRejectsBadDocuments(t *testing.T) {
	for name, data := range map[string]string{
		"syntax":        "a: [",
		"empty":         "",
		"sequence":      "- a\n",
		"two documents": "format: tugqtree\n---\nformat: tugqtree\n",
		"bad version":   "format: tugqtree\nversion: 2\nquery:\n    from:\n        name: T\n",
	} {
		if text, diagnostics := YAMLToTugQL([]byte(data)); len(diagnostics) != 1 || text != "" {
			t.Fatalf("%s: text=%q diagnostics=%+v", name, text, diagnostics)
		}
	}
}

func TestTugQLToYAMLReportsDiagnostics(t *testing.T) {
	if data, diagnostics := TugQLToYAML("from"); data != nil || len(diagnostics) == 0 {
		t.Fatalf("data=%q diagnostics=%+v", data, diagnostics)
	}
}

func TestPrintTugQLRejectsInvalidTree(t *testing.T) {
	if _, diagnostics := PrintTugQL(TugQLTree{}); len(diagnostics) != 1 || diagnostics[0].Code != "invalid_tree" {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

func TestPrintTugQLParameterDefaultSpellings(t *testing.T) {
	for _, bad := range []struct {
		typeName string
		value    any
	}{{"integer", "x"}, {"decimal", 1}, {"boolean", "x"}, {"string", 1}} {
		if text, err := printTugQLParameterDefault(bad.typeName, bad.value); err == nil {
			t.Fatalf("%v: text=%q", bad, text)
		}
	}
	if text, err := printTugQLParameterDefault("integer", 7); err != nil || text != "7" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if _, err := printTugQLParameterDefault("string", "a\nb"); err == nil {
		t.Fatal("expected a line break error")
	}
	if text, err := printTugQLValue(int64(3)); err != nil || text != "3" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if text, err := printTugQLValue(4); err != nil || text != "4" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestIdentText(t *testing.T) {
	for in, want := range map[string]string{
		"Id": "Id", "a_b$1": "a_b$1", "": `""`, "1a": `"1a"`, "a b": `"a b"`, `a"b`: `"a""b"`, "select": `"select"`, "TRUE": `"TRUE"`, "null": `"null"`,
	} {
		if got := identText(in, true); got != want {
			t.Errorf("identText(%q)=%s want %s", in, got, want)
		}
	}
	if got := identText("select", false); got != "select" {
		t.Errorf("function names skip the keyword check, got %s", got)
	}
}

func TestPrintTugQLExpressionBranches(t *testing.T) {
	if _, err := printTugQLExpression(exprYAML{Binary: &binaryYAML{Op: "+"}}, 0); err == nil {
		t.Fatal("binary without operands")
	}
	if text, err := printTugQLCondition(condYAML{IsNull: &exprYAML{Field: "a"}}, "and"); err != nil || text != "a is null" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if _, err := printTugQLCondition(condYAML{Op: "==", Left: &exprYAML{Field: "a"}, Right: &exprYAML{}}, ""); err == nil {
		t.Fatal("empty right operand")
	}
	if _, err := printTugQLCall("f", []exprYAML{{}}); err == nil {
		t.Fatal("empty call argument")
	}
}
