package tugql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type slashPathVectors struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Cases   []struct {
		Name     string `json:"name"`
		Text     string `json:"text"`
		Expected struct {
			From     map[string]any `json:"from"`
			Alias    string         `json:"alias"`
			Print    *string        `json:"print"`
			Comments []string       `json:"comments"`
			Error    *struct {
				Code   string `json:"code"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"error"`
		} `json:"expected"`
	} `json:"cases"`
}

func loadSlashPathVectors(t *testing.T) slashPathVectors {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "slash-paths", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors slashPathVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Format != "tugql-slash-paths-suite" || vectors.Version != 1 || len(vectors.Cases) == 0 {
		t.Fatalf("invalid slash-path vectors header: %s %d", vectors.Format, vectors.Version)
	}
	return vectors
}

func TestTugQLSlashPathVectors(t *testing.T) {
	for _, tc := range loadSlashPathVectors(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			doc, diags := ParseTugQL(tc.Text)
			if tc.Expected.Error != nil {
				want := tc.Expected.Error
				for _, d := range diags {
					if d.Code == want.Code && d.Span.Start.Line == want.Line && d.Span.Start.Column == want.Column {
						return
					}
				}
				t.Fatalf("want diagnostic %s at %d:%d, got %#v", want.Code, want.Line, want.Column, diags)
			}
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
			from := treeFromObject(t, doc.Tree.Query)
			if !reflect.DeepEqual(from, tc.Expected.From) {
				t.Fatalf("from = %#v, want %#v", from, tc.Expected.From)
			}
			alias, _ := from["alias"].(string)
			if alias == "" {
				alias, _ = from["name"].(string)
			}
			if alias != tc.Expected.Alias {
				t.Fatalf("alias = %q, want %q", alias, tc.Expected.Alias)
			}
			var comments []string
			if doc.Tree.Comments != nil {
				keys := make([]string, 0, len(doc.Tree.Comments.Anchors))
				for key := range doc.Tree.Comments.Anchors {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					unit := doc.Tree.Comments.Anchors[key]
					for _, c := range unit.Leading {
						comments = append(comments, c.Text)
					}
					if unit.Trailing != nil {
						comments = append(comments, unit.Trailing.Text)
					}
				}
				for _, c := range doc.Tree.Comments.End {
					comments = append(comments, c.Text)
				}
			}
			if !reflect.DeepEqual(comments, tc.Expected.Comments) {
				t.Fatalf("comments = %#v, want %#v", comments, tc.Expected.Comments)
			}
			if tc.Expected.Print == nil {
				t.Fatal("expected.print is required for a successful case")
			}
			printed, printDiags := PrintTugQL(*doc.Tree)
			if len(printDiags) != 0 || printed != *tc.Expected.Print {
				t.Fatalf("print = %q %#v, want %q", printed, printDiags, *tc.Expected.Print)
			}
			reparsed, reparseDiags := ParseTugQL(printed)
			if len(reparseDiags) != 0 || !reflect.DeepEqual(treeFromObject(t, reparsed.Tree.Query), from) {
				t.Fatalf("printed text does not re-parse to the same from: %#v", reparseDiags)
			}
		})
	}
}

// treeFromObject returns query.from as plain JSON values, the shape the
// TypeScript parser also produces.
func treeFromObject(t *testing.T, query map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(query["from"])
	if err != nil {
		t.Fatal(err)
	}
	var from map[string]any
	if err := json.Unmarshal(data, &from); err != nil {
		t.Fatal(err)
	}
	return from
}

func TestTugQLSlashPathInJoin(t *testing.T) {
	doc, diags := ParseTugQL("FROM spaces/{s}/ext/datatug/projects AS p\nJOIN spaces/{s}/ext/datatug/members AS m\n  ON m.project = p.id\nSELECT p.id")
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	printed, printDiags := PrintTugQL(*doc.Tree)
	want := "from spaces/{s}/ext/datatug/projects as p\njoin spaces/{s}/ext/datatug/members as m\n  on m.project = p.id\nselect p.id"
	if len(printDiags) != 0 || printed != want {
		t.Fatalf("print = %q %#v, want %q", printed, printDiags, want)
	}
	_, diags = ParseTugQL("FROM a\nJOIN b//c\n  ON b.x = a.x")
	if len(diags) == 0 || diags[0].Code != "empty_path_segment" || diags[0].Span.Start != (TugQLPosition{Line: 2, Column: 8}) {
		t.Fatalf("join diagnostics = %#v", diags)
	}
}

func TestTugQLSlashPathErrorText(t *testing.T) {
	if err := pathError("code", "message", tugqlSpan{}); err.Error() != "message" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestTugQLPrintSourceSlashFormFallback(t *testing.T) {
	schema := func(s string) *string { return &s }
	for _, tc := range []struct {
		from fromYAML
		want string
	}{
		{fromYAML{Schema: schema("a/b"), Name: "c"}, "a/b/c"},
		{fromYAML{Schema: schema("a/b{c"), Name: "d"}, `"a/b{c".d`},
		{fromYAML{Schema: schema("a/b"), Name: "x/y"}, `"a/b"."x/y"`},
		{fromYAML{Schema: schema("a//b"), Name: "c"}, `"a//b".c`},
		{fromYAML{Schema: schema("a"), Name: "{1a}"}, `a."{1a}"`},
		{fromYAML{Schema: schema("a"), Name: "{a-b}"}, `a."{a-b}"`},
		{fromYAML{Schema: schema("a"), Name: "{}"}, `a."{}"`},
		{fromYAML{Schema: schema("{_s}"), Name: "from"}, `{_s}/"from"`},
	} {
		if got := printTugQLSource(tc.from); got != tc.want {
			t.Errorf("printTugQLSource(%v/%s) = %s, want %s", *tc.from.Schema, tc.from.Name, got, tc.want)
		}
		if _, diags := ParseTugQL("FROM " + tc.want); len(diags) != 0 {
			t.Errorf("%s does not re-parse: %#v", tc.want, diags)
		}
	}
}
