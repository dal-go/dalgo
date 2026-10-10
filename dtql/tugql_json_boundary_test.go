package dtql

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTugQLDocumentJSONRejectsIncompleteOrTrailingInput(t *testing.T) {
	var nilDocument *TugQLDocument
	if err := nilDocument.UnmarshalJSON([]byte(`{}`)); err == nil || !strings.Contains(err.Error(), "nil TugQL document receiver") {
		t.Fatalf("nil receiver error = %v", err)
	}
	for _, input := range []string{
		`{"unknown":true}`,
		`{"sourceMetadata":{"format":"tugql","version":1,"unknown":true}}`,
		`{"source":`,
		`{} {}`,
		`{} null`,
		`{} malformed`,
	} {
		t.Run(input, func(t *testing.T) {
			original := TugQLDocument{Source: "retained draft"}
			if err := original.UnmarshalJSON([]byte(input)); err == nil {
				t.Fatal("invalid JSON document accepted")
			}
			if original.Source != "retained draft" {
				t.Fatal("failed decode replaced the retained draft")
			}
		})
	}
}

func TestTugQLDocumentJSONRoundTripPreservesSourceAndTree(t *testing.T) {
	source := "from Invoice as i -- billing records\nselect (\n  i.InvoiceId as ID\n)\n"
	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics = %#v", diagnostics)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var restored TugQLDocument
	if err := json.Unmarshal(append(encoded, '\n'), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Source != source || restored.SourceMetadata != document.SourceMetadata || restored.Tree == nil {
		t.Fatal("JSON round-trip lost source, format metadata, or tree")
	}
	reencoded, err := json.Marshal(restored)
	if err != nil || string(reencoded) != string(encoded) {
		t.Fatalf("semantic tree changed on round-trip: %s (%v)", reencoded, err)
	}
}
