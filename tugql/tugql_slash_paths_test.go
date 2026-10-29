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
				if len(diags) == 0 || diags[0].Code != want.Code || diags[0].Span.Start != (TugQLPosition{Line: want.Line, Column: want.Column}) {
					t.Fatalf("want first diagnostic %s at %d:%d, got %#v", want.Code, want.Line, want.Column, diags)
				}
				return
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

func TestTugQLSlashPathErrorText(t *testing.T) {
	if err := pathError("code", "message", tugqlSpan{}); err.Error() != "message" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

// TestTugQLPrintSourcePaths covers trees built directly (not parsed from
// text): segments the text form cannot spell fail closed instead of printing
// text that would re-parse to a different tree or not at all.
func TestTugQLPrintSourcePaths(t *testing.T) {
	schema := func(s string) *string { return &s }
	for _, tc := range []struct {
		from fromYAML
		want string
	}{
		{fromYAML{Name: "c"}, "c"},
		{fromYAML{Schema: schema("a/b"), Name: "c"}, "a/b/c"},
		{fromYAML{Schema: schema("a.b"), Name: "c"}, "a.b.c"},
		{fromYAML{Schema: schema("{_s}"), Name: "from"}, `{_s}/"from"`},
		{fromYAML{Schema: schema("a/b{c"), Name: "d"}, ""},
		{fromYAML{Schema: schema("a/b"), Name: "x/y"}, ""},
		{fromYAML{Schema: schema("a//b"), Name: "c"}, ""},
		{fromYAML{Schema: schema("a"), Name: "{1a}"}, ""},
		{fromYAML{Schema: schema("a"), Name: "{a-b}"}, ""},
		{fromYAML{Schema: schema("a"), Name: "{}"}, ""},
		{fromYAML{Schema: schema("{x}.y"), Name: "z"}, ""},
		{fromYAML{Name: "{x}"}, ""},
		{fromYAML{Schema: schema(""), Name: "c"}, ""},
	} {
		got, err := printTugQLSource(tc.from)
		if tc.want == "" {
			if err == nil {
				t.Errorf("printTugQLSource(%+v) = %q, want an error", tc.from, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("printTugQLSource(%+v) = %q, %v, want %q", tc.from, got, err, tc.want)
		}
		if _, diags := ParseTugQL("FROM " + tc.want); len(diags) != 0 {
			t.Errorf("%s does not re-parse: %#v", tc.want, diags)
		}
	}
}

func TestTugQLPrintRejectsUnspellableSourcePaths(t *testing.T) {
	for _, query := range []map[string]any{
		{"from": map[string]any{"schema": "a", "name": "{}"}},
		{"from": map[string]any{"name": "a", "joins": []any{map[string]any{"from": map[string]any{"schema": "a", "name": "x/y"}}}}},
	} {
		if text, diags := PrintTugQL(TugQLTree{Format: "tugqtree", Version: 1, Query: query}); len(diags) == 0 || diags[0].Code != "unsupported_print" {
			t.Errorf("PrintTugQL(%v) = %q, %#v, want unsupported_print", query, text, diags)
		}
	}
}
