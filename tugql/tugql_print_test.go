package tugql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusSources collects every TugQL source text found in the shared corpus.
func corpusSources(t *testing.T) []string {
	t.Helper()
	var sources []string
	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if text, ok := item.(string); ok && (key == "source" || key == "text") && text != "" {
					sources = append(sources, text)
					continue
				}
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	for _, name := range []string{"suite", "resolve", "execute", "format", "comments"} {
		data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		walk(decoded)
	}
	return sources
}

func TestTugQLPrintRoundTripsCorpus(t *testing.T) {
	ok, unsupported, bad := 0, 0, 0
	for _, source := range corpusSources(t) {
		doc, diagnostics := ParseTugQL(source)
		if len(diagnostics) != 0 {
			continue
		}
		y, d := TugQLToYAML(source)
		if len(d) != 0 {
			t.Errorf("yaml %q: %+v", source, d)
			continue
		}
		text, d := YAMLToTugQL(y)
		if len(d) != 0 {
			unsupported++
			t.Logf("UNSUPPORTED %q: %s", source, d[0].Message)
			continue
		}
		again, ad := ParseTugQL(text)
		if len(ad) != 0 {
			bad++
			t.Errorf("reparse %q -> %q: %+v", source, text, ad)
			continue
		}
		a := *doc.Tree
		a.Comments = nil
		b := *again.Tree
		b.Comments = nil
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			bad++
			t.Errorf("tree differs for %q -> %q\n%s\n%s", source, text, ja, jb)
			continue
		}
		ok++
	}
	t.Logf("ok=%d unsupported=%d bad=%d", ok, unsupported, bad)
}

const printFeatureSource = "parameters (\n  @cap integer default 5\n  @neg integer default -2\n  @who string required\n  @price decimal default 1.50\n  @flag boolean default true\n  @off boolean default false\n  @day date default '2026-01-02'\n  @at timestamp default '2026-01-02T03:04:05Z'\n  @name string default 'it''s'\n)\nwith A as (\n  with Local as (\n    from U\n    select Id\n  )\n  from Local\n  select Id\n)\nwith B from 'lib/b.tugql'\n  using (\n    @x = 1\n    @y = @cap\n  )\nwith C from 'lib/c.tugql'\nfrom s.t.T as t\nleft join A as a\n  on t.Id = a.Id\njoin B\n  on Rel\nwhere \"My Col\" = 'it''s' and (\"order\" = 1 or Z is null) and x <> 2 and y != 3 and z <= 4 and w >= 5\ngroup by t.Id, lower(Name)\nhaving count(*) > 1\norder by Id asc, Name desc\nlimit 10\nselect t.Id, count(*) as n, sum(a + b * c) as s, (a + b) * c as p, a - (b - c) as q, null as nothing, true as yes, false as no, @cap as cap, lower(Name) as lname"

const printScalarSource = "from T\nselect (\n  Id\n  Total as (\n    with W as (\n      from U\n      select Id\n    )\n    from W\n    select Id\n  )\n)"

func TestTugQLPrintFeatures(t *testing.T) {
	// The canonical layout differs from printFeatureSource only in the dropped
	// redundant ASC.
	want := strings.Replace(printFeatureSource, "order by Id asc, Name desc", "order by Id, Name desc", 1)
	for source, wantText := range map[string]string{printFeatureSource: want, printScalarSource: printScalarSource} {
		doc, diagnostics := ParseTugQL(source)
		if len(diagnostics) != 0 {
			t.Fatalf("%+v", diagnostics)
		}
		data, _ := TugQLToYAML(source)
		text, diagnostics := YAMLToTugQL(data)
		if len(diagnostics) != 0 || text != wantText {
			t.Fatalf("text=%q diagnostics=%+v\nwant %q", text, diagnostics, wantText)
		}
		// PrintTugQL on the parsed tree gives the same text.
		direct, diagnostics := PrintTugQL(*doc.Tree)
		if len(diagnostics) != 0 || direct != wantText {
			t.Fatalf("direct=%q diagnostics=%+v", direct, diagnostics)
		}
	}
}

func queryYAML(body string) string {
	return "format: tugqtree\nversion: 1\nquery:\n" + body
}

func TestTugQLYAMLToTugQLRejectsWhatTextCannotSay(t *testing.T) {
	cases := map[string]string{
		"as":                            queryYAML("    as: x\n    from:\n        name: T\n"),
		"money":                         queryYAML("    from:\n        name: T\n    money:\n        minorUnitScale: 2\n        divisionScale: 4\n        rounding: half-even\n"),
		"offset":                        queryYAML("    from:\n        name: T\n    offset: 3\n"),
		"from database":                 queryYAML("    from:\n        database: d\n        name: T\n"),
		"from scan":                     queryYAML("    from:\n        name: T\n        scan:\n            limit: 1\n"),
		"from query":                    queryYAML("    from:\n        name: T\n        query:\n            from:\n                name: U\n"),
		"from without name":             queryYAML("    from:\n        schema: s\n"),
		"join without from":             queryYAML("    from:\n        name: T\n        joins:\n            - on: []\n"),
		"join type":                     queryYAML("    from:\n        name: T\n        joins:\n            - type: right\n              from:\n                name: U\n              on: []\n"),
		"join nested":                   queryYAML("    from:\n        name: T\n        joins:\n            - from:\n                name: U\n                joins:\n                    - from:\n                        name: V\n                      on: []\n              on: []\n"),
		"join source":                   queryYAML("    from:\n        name: T\n        joins:\n            - from:\n                schema: s\n              on: []\n"),
		"join condition":                queryYAML("    from:\n        name: T\n        joins:\n            - from:\n                name: U\n              on:\n                - op: like\n                  left:\n                    field: a\n                  right:\n                    value: x\n"),
		"where":                         queryYAML("    from:\n        name: T\n    where:\n        op: like\n        left:\n            field: a\n        right:\n            value: x\n"),
		"group by":                      queryYAML("    from:\n        name: T\n    groupBy:\n        - value: -1\n"),
		"having":                        queryYAML("    from:\n        name: T\n    having:\n        op: like\n        left:\n            field: a\n        right:\n            value: x\n"),
		"order by":                      queryYAML("    from:\n        name: T\n    orderBy:\n        - value: -1\n"),
		"columns":                       queryYAML("    from:\n        name: T\n    columns:\n        - value: -1\n"),
		"wildcard":                      queryYAML("    from:\n        name: T\n    columns:\n        - star: true\n          wildcard:\n              exclude:\n                  - a\n"),
		"scalar no alias":               queryYAML("    from:\n        name: T\n    columns:\n        - query:\n            query:\n                from:\n                    name: U\n                columns:\n                    - field: Id\n"),
		"scalar body":                   queryYAML("    from:\n        name: T\n    columns:\n        - query:\n            query:\n                from:\n                    name: U\n                columns:\n                    - value: -1\n          as: s\n"),
		"scalar plain item":             queryYAML("    from:\n        name: T\n    columns:\n        - value: -1\n          as: s\n        - query:\n            query:\n                from:\n                    name: U\n                columns:\n                    - field: Id\n          as: t\n"),
		"aggregate distinct":            queryYAML("    from:\n        name: T\n    columns:\n        - aggregate:\n            function: count\n            distinct: true\n            args:\n                - star: true\n"),
		"aggregate args":                queryYAML("    from:\n        name: T\n    columns:\n        - aggregate:\n            function: sum\n            args:\n                - value: -1\n"),
		"binary operator":               queryYAML("    from:\n        name: T\n    columns:\n        - binary:\n            op: '%'\n            left:\n                field: a\n            right:\n                field: b\n"),
		"binary left":                   queryYAML("    from:\n        name: T\n    columns:\n        - binary:\n            op: '+'\n            left:\n                value: -1\n            right:\n                field: b\n"),
		"binary right":                  queryYAML("    from:\n        name: T\n    columns:\n        - binary:\n            op: '+'\n            left:\n                field: b\n            right:\n                value: -1\n"),
		"float value":                   queryYAML("    from:\n        name: T\n    columns:\n        - value: 1.5\n"),
		"newline value":                 queryYAML("    from:\n        name: T\n    columns:\n        - value: \"a\\nb\"\n"),
		"values":                        queryYAML("    from:\n        name: T\n    columns:\n        - values: [1, 2]\n"),
		"exists":                        queryYAML("    from:\n        name: T\n    where:\n        exists:\n            query:\n                from:\n                    name: U\n"),
		"is not null":                   queryYAML("    from:\n        name: T\n    where:\n        isNotNull:\n            field: a\n"),
		"or child":                      queryYAML("    from:\n        name: T\n    where:\n        or:\n            - op: like\n              left:\n                  field: a\n              right:\n                  value: x\n"),
		"and child":                     queryYAML("    from:\n        name: T\n    where:\n        and:\n            - op: like\n              left:\n                  field: a\n              right:\n                  value: x\n"),
		"comparison left":               queryYAML("    from:\n        name: T\n    where:\n        op: ==\n        left:\n            value: -1\n        right:\n            value: 1\n"),
		"comparison right":              queryYAML("    from:\n        name: T\n    where:\n        op: ==\n        left:\n            field: a\n        right:\n            value: -1\n"),
		"is null operand":               queryYAML("    from:\n        name: T\n    where:\n        isNull:\n            value: -1\n"),
		"parameter neither":             "format: tugqtree\nversion: 1\nparameters:\n    - name: p\n      type: integer\nquery:\n    from:\n        name: T\n",
		"parameter default newline":     "format: tugqtree\nversion: 1\nparameters:\n    - name: p\n      type: string\n      default: \"a\\nb\"\nquery:\n    from:\n        name: T\n",
		"cte unsupported":               "format: tugqtree\nversion: 1\ndefinitions:\n    - kind: cte\n      name: C\n      query:\n          query:\n              from:\n                  name: U\n              offset: 1\nquery:\n    from:\n        name: T\n",
		"nested definition unsupported": "format: tugqtree\nversion: 1\ndefinitions:\n    - kind: cte\n      name: C\n      query:\n          definitions:\n              - kind: import\n                name: I\n                path: \"a\\nb\"\n          query:\n              from:\n                  name: U\nquery:\n    from:\n        name: T\n",
		"wildcard exclusion":            queryYAML("    from:\n        name: T\n    columns:\n        - wildcard:\n              exclude:\n                  - a\n"),
		"import path":                   "format: tugqtree\nversion: 1\ndefinitions:\n    - kind: import\n      name: I\n      path: \"a\\nb\"\nquery:\n    from:\n        name: T\n",
		"import mapping":                "format: tugqtree\nversion: 1\ndefinitions:\n    - kind: import\n      name: I\n      path: p\n      using:\n          - name: x\n            expression:\n                value: 1.5\nquery:\n    from:\n        name: T\n",
		"cte body":                      "format: tugqtree\nversion: 1\ndefinitions:\n    - kind: cte\n      name: C\n      query:\n          query:\n              from:\n                  schema: s\nquery:\n    from:\n        name: T\n",
		"unspellable expression":        queryYAML("    from:\n        name: T\n    columns:\n        - as: x\n"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			text, diagnostics := YAMLToTugQL([]byte(data))
			if len(diagnostics) != 1 || (diagnostics[0].Code != "unsupported_print" && diagnostics[0].Code != "invalid_tree") {
				t.Fatalf("text=%q diagnostics=%+v", text, diagnostics)
			}
			t.Log(diagnostics[0].Code, diagnostics[0].Message)
		})
	}
}
