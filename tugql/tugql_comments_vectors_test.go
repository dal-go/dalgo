package tugql

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type commentVectorSQL struct {
	Forbid                    string   `json:"forbid"`
	IgnoringStringLiterals    bool     `json:"ignoringStringLiterals"`
	IgnoringQuotedIdentifiers bool     `json:"ignoringQuotedIdentifiers"`
	StringLiterals            []string `json:"stringLiterals"`
	QuotedIdentifiers         []string `json:"quotedIdentifiers"`
}

type commentVectorToken struct {
	Marker   string `json:"marker"`
	Text     string `json:"text"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Trailing bool   `json:"trailing"`
}

type commentVectors struct {
	Format    string                  `json:"format"`
	Version   int                     `json:"version"`
	Schema    []TugQLAuthorizedSchema `json:"schema"`
	TextCases []struct {
		Name          string             `json:"name"`
		Text          string             `json:"text"`
		FormatOptions TugQLFormatOptions `json:"formatOptions"`
		Expected      struct {
			Diagnostics   []TugQLDiagnostic    `json:"diagnostics"`
			CommentTokens []commentVectorToken `json:"commentTokens"`
			Format        *string              `json:"format"`
			YAML          *string              `json:"yaml"`
			RoundTripText *string              `json:"roundTripText"`
			SQL           *commentVectorSQL    `json:"sql"`
		} `json:"expected"`
	} `json:"textCases"`
	YAMLCases []struct {
		Name     string `json:"name"`
		YAML     string `json:"yaml"`
		Expected struct {
			Diagnostics []TugQLDiagnostic `json:"diagnostics"`
			Text        *string           `json:"text"`
			SQL         *commentVectorSQL `json:"sql"`
		} `json:"expected"`
	} `json:"yamlCases"`
}

func loadCommentVectors(t *testing.T) commentVectors {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "tugql", "v1", "comments.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors commentVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Format != "tugql-comments-suite" || vectors.Version != 1 || len(vectors.TextCases) == 0 || len(vectors.YAMLCases) == 0 {
		t.Fatalf("invalid comment vectors header: %s %d", vectors.Format, vectors.Version)
	}
	return vectors
}

// withoutQuoted removes the listed literals so a remaining forbidden marker
// would be a comment marker that leaked outside a literal or identifier.
func withoutQuoted(text string, sql *commentVectorSQL) string {
	for _, literal := range append(append([]string(nil), sql.StringLiterals...), sql.QuotedIdentifiers...) {
		text = strings.ReplaceAll(text, literal, "")
	}
	return text
}

func assertNoCommentInResolved(t *testing.T, vectors commentVectors, text string, sql *commentVectorSQL) {
	t.Helper()
	doc, diagnostics := ParseTugQL(text)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %+v", diagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{AuthorizedSchemas: vectors.Schema})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %+v", diagnostics)
	}
	encoded, err := Serialize(resolved.Query)
	if err != nil {
		t.Fatal(err)
	}
	// Resolution output is the only input a SQL emitter sees. It must carry no
	// comment text and no comment marker outside literals and identifiers.
	if got := withoutQuoted(string(encoded), sql); strings.Contains(got, sql.Forbid) {
		t.Fatalf("resolved query carries %q outside literals:\n%s", sql.Forbid, encoded)
	}
	// Comments do not influence resolution: the same tree without them
	// resolves to byte-identical output.
	stripped := *doc.Tree
	stripped.Comments = nil
	strippedResolved, _ := ResolveTugQL(tugqlTestDocument(&stripped), TugQLResolveContext{AuthorizedSchemas: vectors.Schema})
	strippedEncoded, _ := Serialize(strippedResolved.Query)
	if string(strippedEncoded) != string(encoded) {
		t.Fatalf("comments changed the resolved query:\n%s\nvs\n%s", encoded, strippedEncoded)
	}
	if body, _ := json.Marshal(resolved); strings.Contains(string(body), "comment") {
		t.Fatalf("resolved output mentions comments: %s", body)
	}
}

func TestTugQLCommentVectorsText(t *testing.T) {
	vectors := loadCommentVectors(t)
	for _, c := range vectors.TextCases {
		t.Run(c.Name, func(t *testing.T) {
			_, comments, _ := lexTugQLWithComments(c.Text)
			tokens := []commentVectorToken{}
			for _, comment := range comments {
				tokens = append(tokens, commentVectorToken(comment))
			}
			if !reflect.DeepEqual(tokens, c.Expected.CommentTokens) {
				t.Fatalf("comment tokens=%+v want=%+v", tokens, c.Expected.CommentTokens)
			}
			doc, diagnostics := ParseTugQL(c.Text)
			if !reflect.DeepEqual(diagnostics, c.Expected.Diagnostics) {
				t.Fatalf("diagnostics=%+v want=%+v", diagnostics, c.Expected.Diagnostics)
			}
			if len(diagnostics) != 0 {
				if _, yamlDiagnostics := TugQLToYAML(c.Text); !reflect.DeepEqual(yamlDiagnostics, diagnostics) {
					t.Fatalf("TugQLToYAML diagnostics=%+v", yamlDiagnostics)
				}
				return
			}
			formatted, formatDiagnostics := FormatTugQL(doc, c.FormatOptions)
			if len(formatDiagnostics) != 0 || formatted != *c.Expected.Format {
				t.Fatalf("format=%q diagnostics=%+v want=%q", formatted, formatDiagnostics, *c.Expected.Format)
			}
			yamlText, yamlDiagnostics := TugQLToYAML(c.Text)
			if len(yamlDiagnostics) != 0 || string(yamlText) != *c.Expected.YAML {
				t.Fatalf("yaml=\n%s\ndiagnostics=%+v want=\n%s", yamlText, yamlDiagnostics, *c.Expected.YAML)
			}
			roundTrip, roundTripDiagnostics := YAMLToTugQL(yamlText)
			if len(roundTripDiagnostics) != 0 || roundTrip != *c.Expected.RoundTripText {
				t.Fatalf("round trip=%q diagnostics=%+v want=%q", roundTrip, roundTripDiagnostics, *c.Expected.RoundTripText)
			}
			// Converting the round-trip text again is stable: every comment
			// already sits on an anchor.
			again, againDiagnostics := TugQLToYAML(roundTrip)
			if len(againDiagnostics) != 0 || string(again) != string(yamlText) {
				t.Fatalf("second conversion differs:\n%s\nwant:\n%s\ndiagnostics=%+v", again, yamlText, againDiagnostics)
			}
			assertNoCommentInResolved(t, vectors, c.Text, c.Expected.SQL)
			// The formatter output parses to the same comments.
			formattedDoc, _ := ParseTugQL(formatted)
			if !reflect.DeepEqual(formattedDoc.Tree.Comments, doc.Tree.Comments) {
				t.Fatalf("formatting changed comments: %+v vs %+v", formattedDoc.Tree.Comments, doc.Tree.Comments)
			}
		})
	}
}

func TestTugQLCommentVectorsYAML(t *testing.T) {
	vectors := loadCommentVectors(t)
	for _, c := range vectors.YAMLCases {
		t.Run(c.Name, func(t *testing.T) {
			text, diagnostics := YAMLToTugQL([]byte(c.YAML))
			if !reflect.DeepEqual(diagnostics, c.Expected.Diagnostics) {
				t.Fatalf("diagnostics=%+v want=%+v", diagnostics, c.Expected.Diagnostics)
			}
			if c.Expected.Text == nil {
				return
			}
			if text != *c.Expected.Text {
				t.Fatalf("text=%q want=%q", text, *c.Expected.Text)
			}
			assertNoCommentInResolved(t, vectors, text, c.Expected.SQL)
		})
	}
}

func TestTugQLCorpusManifest(t *testing.T) {
	dir := filepath.Join("testdata", "tugql", "v1")
	data, err := os.ReadFile(filepath.Join(dir, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid manifest line %q", line)
		}
		content, err := os.ReadFile(filepath.FromSlash(strings.TrimPrefix(fields[1], "tugql/")))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != fields[0] {
			t.Fatalf("%s digest = %x, manifest = %s", fields[1], sum, fields[0])
		}
		listed[filepath.Base(fields[1])] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "manifest.sha256" && !listed[entry.Name()] {
			t.Fatalf("%s is not listed in manifest.sha256", entry.Name())
		}
	}
	if !listed["comments.json"] {
		t.Fatal("comments.json missing from manifest.sha256")
	}
}
