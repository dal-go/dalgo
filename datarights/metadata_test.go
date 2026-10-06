package datarights

import (
	"bytes"
	"encoding/json"
	"os"
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

func TestSharedSourceRightsWireAndNullPolicy(t *testing.T) {
	fixture, err := os.ReadFile("testdata/source-rights-wire.json")
	if err != nil {
		t.Fatal(err)
	}
	original := QueryMetadata{SourceRights: []SourceRight{{SourceID: "a", Source: Source{ServerID: "provider", DatabaseID: "db", Recordset: "A"}, Declaration: Declaration{URL: "https://example.com/terms"}, DeclarationScope: "database", DeclaredAt: Source{ServerID: "provider", DatabaseID: "db"}, EvidenceOrigin: "server-declared"}}, UsedSourceIDs: []string{}}
	encoded, err := json.Marshal(original)
	if err != nil || !bytes.Equal(encoded, bytes.TrimSpace(fixture)) {
		t.Fatalf("Go/JS required-array wire mismatch: %s %v", encoded, err)
	}
	var metadata QueryMetadata
	if err := json.Unmarshal(fixture, &metadata); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := json.Marshal(metadata)
	if err != nil || !bytes.Equal(roundtrip, encoded) {
		t.Fatalf("wire roundtrip: %s %v", roundtrip, err)
	}
	for _, invalid := range []string{`{`, `[]`, `null`, `{"sourceRights":null}`, `{"usedSourceIds":null}`, `{"usedSourceIds":"invalid"}`, `{"sourceRights":[{"pins":null,"transformations":[]}]}`, `{"sourceRights":[{"pins":[],"transformations":null}]}`, `{"sourceRights":[{"pins":[],"transformations":"invalid"}]}`, `{"sourceRights":[{}]}`} {
		if err := json.Unmarshal([]byte(invalid), &metadata); err == nil {
			t.Fatalf("invalid metadata accepted: %s", invalid)
		}
	}
	for _, invalid := range []string{`{`, `[]`, `null`, `{"pins":[],"transformations":null}`} {
		var right SourceRight
		if err := json.Unmarshal([]byte(invalid), &right); err == nil {
			t.Fatalf("invalid right accepted: %s", invalid)
		}
	}
}
