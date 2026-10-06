package datarights

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOptionalMetadataWireAndSnapshots(t *testing.T) {
	empty, err := json.Marshal(QueryMetadata{})
	if err != nil || string(empty) != "{}" {
		t.Fatalf("legacy omission: %s, %v", empty, err)
	}
	knownEmpty, err := json.Marshal(QueryMetadata{SourceRights: []SourceRight{}, UsedSourceIDs: []string{}})
	if err != nil || string(knownEmpty) != `{"sourceRights":[],"usedSourceIds":[]}` {
		t.Fatalf("known empty lost: %s %v", knownEmpty, err)
	}
	source := QueryMetadata{SourceRights: []SourceRight{
		{SourceID: "a", Source: Source{ServerID: "provider", DatabaseID: "db", Recordset: "view"}, Declaration: Declaration{Name: "Custom terms", URL: "https://example.com/terms", Text: "Source conditions", SPDX: "MIT"}, DeclarationScope: "database", DeclaredAt: Source{ServerID: "provider", DatabaseID: "db"}, EvidenceOrigin: "provider-policy", Pins: []Pin{{Role: "input", Repository: "org/source", Revision: "abc", Path: "raw.json", SHA256: "hash", Bytes: 5}}, Attribution: &Notice{Text: "Source credit"}, FreeSource: &LinkNotice{Text: "Free data", URL: "https://example.com/data"}, Transformations: []string{"projection"}},
		{SourceID: "b", Source: Source{ServerID: "other"}, Declaration: Declaration{URL: "https://example.com/other"}, DeclarationScope: "server", DeclaredAt: Source{ServerID: "other"}, EvidenceOrigin: "server-declared", Pins: []Pin{}, Transformations: []string{}},
	}, UsedSourceIDs: []string{"a"}}
	snapshot := source.Clone()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded QueryMetadata
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, decoded) {
		t.Fatalf("wire changed metadata: %s", encoded)
	}
	source.SourceRights[0].Pins[0].Path = "changed"
	source.SourceRights[0].Attribution.Text = "changed"
	source.SourceRights[0].FreeSource.URL = "changed"
	source.SourceRights[0].Transformations[0] = "changed"
	source.UsedSourceIDs[0] = "changed"
	if snapshot.SourceRights[0].Pins[0].Path != "raw.json" || snapshot.SourceRights[0].Attribution.Text != "Source credit" || snapshot.SourceRights[0].FreeSource.URL != "https://example.com/data" || snapshot.SourceRights[0].Transformations[0] != "projection" || snapshot.UsedSourceIDs[0] != "a" {
		t.Fatal("snapshot aliases mutable input")
	}
	if (QueryMetadata{}).Clone().SourceRights != nil {
		t.Fatal("absence became an empty inventory")
	}
}

func TestFreeSourceLinkIsRequiredOnWire(t *testing.T) {
	data, err := json.Marshal(LinkNotice{Text: "Original source", URL: "https://example.com/source"})
	if err != nil || string(data) != `{"text":"Original source","url":"https://example.com/source"}` {
		t.Fatalf("required free-source link: %s %v", data, err)
	}
	data, err = json.Marshal(LinkNotice{Text: "Original source"})
	if err != nil || string(data) != `{"text":"Original source","url":""}` {
		t.Fatalf("required field was omitted: %s %v", data, err)
	}
	// An empty URL is invalid provider evidence; the core does not certify terms.
}
