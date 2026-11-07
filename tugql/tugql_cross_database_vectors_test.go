package tugql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type crossDatabaseVectors struct {
	Format    string              `json:"format"`
	Version   int                 `json:"version"`
	Context   TugQLResolveContext `json:"context"`
	TextCases []struct {
		Name     string `json:"name"`
		Text     string `json:"text"`
		Expected struct {
			Tree json.RawMessage `json:"tree"`
			YAML string          `json:"yaml"`
			Text string          `json:"text"`
		} `json:"expected"`
	} `json:"textCases"`
	ErrorCases []struct {
		Name     string `json:"name"`
		Text     string `json:"text"`
		Expected struct {
			Code   string `json:"code"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"expected"`
	} `json:"errorCases"`
	TreeCases []struct {
		Name      string    `json:"name"`
		Operation string    `json:"operation"`
		Tree      TugQLTree `json:"tree"`
		Expected  struct {
			Code  string  `json:"code"`
			Query *string `json:"query"`
			Text  *string `json:"text"`
		} `json:"expected"`
	} `json:"treeCases"`
	YAMLCases []struct {
		Name     string `json:"name"`
		YAML     string `json:"yaml"`
		Expected struct {
			Code string  `json:"code"`
			Text *string `json:"text"`
		} `json:"expected"`
	} `json:"yamlCases"`
	FormatCases []struct {
		Name     string             `json:"name"`
		Text     string             `json:"text"`
		Options  TugQLFormatOptions `json:"options"`
		Expected struct {
			Text string `json:"text"`
		} `json:"expected"`
	} `json:"formatCases"`
	ResolveCases []struct {
		Name     string          `json:"name"`
		Text     string          `json:"text"`
		Context  json.RawMessage `json:"context"`
		Expected struct {
			Code          string              `json:"code"`
			Query         string              `json:"query"`
			SchemaVersion string              `json:"schemaVersion"`
			Columns       []string            `json:"columns"`
			OutputColumns []TugQLOutputColumn `json:"outputColumns"`
		} `json:"expected"`
	} `json:"resolveCases"`
}

func loadCrossDatabaseVectors(t *testing.T) crossDatabaseVectors {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "cross-database.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors crossDatabaseVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Format != "tugql-cross-database-suite" || vectors.Version != 1 || len(vectors.TextCases) == 0 || len(vectors.ErrorCases) == 0 || len(vectors.TreeCases) == 0 || len(vectors.YAMLCases) == 0 || len(vectors.ResolveCases) == 0 || len(vectors.FormatCases) == 0 {
		t.Fatalf("invalid cross-database vectors header: %s %d", vectors.Format, vectors.Version)
	}
	return vectors
}

func sameJSON(t *testing.T, got any, want []byte) bool {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var left, right any
	_ = json.Unmarshal(encoded, &left)
	_ = json.Unmarshal(want, &right)
	return reflect.DeepEqual(left, right)
}

func TestTugQLCrossDatabaseVectorsText(t *testing.T) {
	for _, c := range loadCrossDatabaseVectors(t).TextCases {
		t.Run(c.Name, func(t *testing.T) {
			doc, diagnostics := ParseTugQL(c.Text)
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics: %+v", diagnostics)
			}
			if !sameJSON(t, doc.Tree, c.Expected.Tree) {
				encoded, _ := json.Marshal(doc.Tree)
				t.Fatalf("tree=%s\nwant=%s", encoded, c.Expected.Tree)
			}
			yamlText, diagnostics := TugQLToYAML(c.Text)
			if len(diagnostics) != 0 || string(yamlText) != c.Expected.YAML {
				t.Fatalf("yaml=\n%s\ndiagnostics=%+v want=\n%s", yamlText, diagnostics, c.Expected.YAML)
			}
			printed, diagnostics := PrintTugQL(*doc.Tree)
			if len(diagnostics) != 0 || printed != c.Expected.Text {
				t.Fatalf("print=%q diagnostics=%+v want=%q", printed, diagnostics, c.Expected.Text)
			}
			back, diagnostics := YAMLToTugQL(yamlText)
			if len(diagnostics) != 0 || back != c.Expected.Text {
				t.Fatalf("YAMLToTugQL=%q diagnostics=%+v", back, diagnostics)
			}
			again, diagnostics := ParseTugQL(printed)
			if len(diagnostics) != 0 || !reflect.DeepEqual(again.Tree.Databases, doc.Tree.Databases) || !reflect.DeepEqual(again.Tree.Query, doc.Tree.Query) || !reflect.DeepEqual(again.Tree.Definitions, doc.Tree.Definitions) {
				t.Fatalf("printed text does not reparse to the same tree: %+v", diagnostics)
			}
		})
	}
}

func TestTugQLCrossDatabaseVectorsErrors(t *testing.T) {
	for _, c := range loadCrossDatabaseVectors(t).ErrorCases {
		t.Run(c.Name, func(t *testing.T) {
			doc, diagnostics := ParseTugQL(c.Text)
			if doc.Tree != nil || len(diagnostics) == 0 {
				t.Fatalf("parsed without diagnostics")
			}
			first := diagnostics[0]
			if first.Code != c.Expected.Code || first.Span.Start.Line != c.Expected.Line || first.Span.Start.Column != c.Expected.Column {
				t.Fatalf("first diagnostic=%+v want %+v", first, c.Expected)
			}
		})
	}
}

func TestTugQLCrossDatabaseVectorsTrees(t *testing.T) {
	vectors := loadCrossDatabaseVectors(t)
	for _, c := range vectors.TreeCases {
		t.Run(c.Name, func(t *testing.T) {
			tree := c.Tree
			var diagnostics []TugQLDiagnostic
			switch c.Operation {
			case "print":
				var text string
				text, diagnostics = PrintTugQL(tree)
				if c.Expected.Text != nil && text != *c.Expected.Text {
					t.Fatalf("text=%q want %q", text, *c.Expected.Text)
				}
			case "resolve":
				var resolved TugQLResolved
				resolved, diagnostics = ResolveTugQL(tugqlTestDocument(&tree), vectors.Context)
				if c.Expected.Query != nil {
					data, err := Serialize(resolved.Query)
					if err != nil || string(data) != *c.Expected.Query {
						t.Fatalf("query=%s err=%v want %s", data, err, *c.Expected.Query)
					}
				}
			default:
				t.Fatalf("unknown operation %q", c.Operation)
			}
			if c.Expected.Code == "" && len(diagnostics) != 0 || c.Expected.Code != "" && (len(diagnostics) == 0 || diagnostics[0].Code != c.Expected.Code) {
				t.Fatalf("diagnostics=%+v want %q", diagnostics, c.Expected.Code)
			}
		})
	}
}

func TestTugQLCrossDatabaseVectorsYAML(t *testing.T) {
	for _, c := range loadCrossDatabaseVectors(t).YAMLCases {
		t.Run(c.Name, func(t *testing.T) {
			text, diagnostics := YAMLToTugQL([]byte(c.YAML))
			if c.Expected.Text != nil {
				if len(diagnostics) != 0 || text != *c.Expected.Text {
					t.Fatalf("text=%q diagnostics=%+v want %q", text, diagnostics, *c.Expected.Text)
				}
				return
			}
			if len(diagnostics) == 0 || diagnostics[0].Code != c.Expected.Code {
				t.Fatalf("diagnostics=%+v want %q", diagnostics, c.Expected.Code)
			}
		})
	}
}

func TestTugQLCrossDatabaseVectorsResolve(t *testing.T) {
	vectors := loadCrossDatabaseVectors(t)
	for _, c := range vectors.ResolveCases {
		t.Run(c.Name, func(t *testing.T) {
			// The case context replaces whole top-level keys of the shared one.
			merged := map[string]json.RawMessage{}
			_ = json.Unmarshal(mustJSON(t, vectors.Context), &merged)
			if len(c.Context) != 0 {
				if err := json.Unmarshal(c.Context, &merged); err != nil {
					t.Fatal(err)
				}
			}
			var context TugQLResolveContext
			if err := json.Unmarshal(mustJSON(t, merged), &context); err != nil {
				t.Fatal(err)
			}
			doc, diagnostics := ParseTugQL(c.Text)
			if len(diagnostics) != 0 {
				t.Fatalf("parse diagnostics: %+v", diagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, context)
			if c.Expected.Code != "" {
				if resolved.Query != nil || len(diagnostics) == 0 || diagnostics[0].Code != c.Expected.Code {
					t.Fatalf("resolved=%v diagnostics=%+v want %q", resolved.Query, diagnostics, c.Expected.Code)
				}
				return
			}
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics: %+v", diagnostics)
			}
			data, err := Serialize(resolved.Query)
			if err != nil || string(data) != c.Expected.Query || resolved.SchemaVersion != c.Expected.SchemaVersion {
				t.Fatalf("query=%s schemaVersion=%q err=%v\nwant %s %q", data, resolved.SchemaVersion, err, c.Expected.Query, c.Expected.SchemaVersion)
			}
			var names []string
			for _, column := range resolved.Columns {
				names = append(names, column.Name)
			}
			if !reflect.DeepEqual(names, c.Expected.Columns) || c.Expected.OutputColumns == nil || !sameJSON(t, normalizedTugQLColumns(resolved.Columns), mustJSON(t, normalizedTugQLColumns(c.Expected.OutputColumns))) {
				t.Fatalf("columns=%+v want %v %+v", resolved.Columns, c.Expected.Columns, c.Expected.OutputColumns)
			}
		})
	}
}

func TestTugQLCrossDatabaseVectorsFormat(t *testing.T) {
	for _, c := range loadCrossDatabaseVectors(t).FormatCases {
		t.Run(c.Name, func(t *testing.T) {
			formatted, diagnostics := FormatTugQL(TugQLDocument{Source: c.Text, SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}}, c.Options)
			if len(diagnostics) != 0 || formatted != c.Expected.Text {
				t.Fatalf("format=%q diagnostics=%+v want %q", formatted, diagnostics, c.Expected.Text)
			}
		})
	}
}

// normalizedTugQLColumns treats a null lineage like an empty one.
func normalizedTugQLColumns(columns []TugQLOutputColumn) []TugQLOutputColumn {
	out := append([]TugQLOutputColumn(nil), columns...)
	for i := range out {
		if out[i].Lineage == nil {
			out[i].Lineage = []TugQLOutputLineage{}
		}
	}
	return out
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
