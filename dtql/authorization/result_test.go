package authorization

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/access"
)

const inspectResultFixture = `{
  "apiVersion":"dtql.org/authorization/v1","requestId":"req-1","mode":"inspect","scope":"request","result":"allow","allowed":true,"hypothetical":false,
  "operations":[{"id":"u1","requestOperationId":"u1","action":"update","resource":{"databaseId":"crm","path":"/customers/101","rowId":"101","columns":[["name"]]},"result":"allow","restrictionIds":[],"allOf":[],"executionClass":"dtql"}],
  "layers":[{"layerId":"ingit-crm","source":{"ownerId":"ingit-local","provider":"ingitdb","databaseId":"crm","kind":"ingitdb"},"aclState":"enabled","result":"allow","decisions":[{"operationId":"u1","result":"allow","scope":"operation","restrictionIds":[]}]}],
  "blockers":[],"coverage":{"evaluation":"complete","disclosure":"full","truncated":false,"unevaluated":[]},"restrictions":[]
}`

func TestFrozenInspectResultRoundTrip(t *testing.T) {
	result, err := ParseResult([]byte(inspectResultFixture))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalResult(result)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(inspectResultFixture), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(want, got) {
		t.Fatalf("round trip differs\n%s", encoded)
	}
}

func TestFrozenResultFixturesValidateAndRoundTrip(t *testing.T) {
	files, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no frozen result fixtures")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ParseResult(data)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := MarshalResult(result)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if json.Unmarshal(data, &want) != nil || json.Unmarshal(encoded, &got) != nil || !jsonEqual(want, got) {
				t.Fatalf("round trip differs\n%s", encoded)
			}
		})
	}
}

func TestFrozenInvalidResultFixturesReject(t *testing.T) {
	files, err := filepath.Glob("testdata/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ParseResult(data); err == nil {
				t.Fatal("invalid frozen fixture accepted")
			}
		})
	}
}

func TestResultStrictValidation(t *testing.T) {
	tests := map[string]string{
		"unknown":              strings.Replace(inspectResultFixture, `"requestId":"req-1"`, `"requestId":"req-1","unknown":true`, 1),
		"duplicate":            strings.Replace(inspectResultFixture, `"requestId":"req-1"`, `"requestId":"req-1","requestId":"req-2"`, 1),
		"allowed deny":         strings.Replace(inspectResultFixture, `"result":"allow","allowed":true`, `"result":"deny","allowed":true`, 1),
		"restriction mismatch": strings.Replace(inspectResultFixture, `"restrictionIds":[],"allOf":[]`, `"restrictionIds":["r1"],"allOf":[]`, 1),
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseResult([]byte(source)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestCompatibleDecodePreservesFutureDenialCode(t *testing.T) {
	source := strings.Replace(inspectResultFixture, `"result":"allow","allowed":true`, `"result":"deny","allowed":false`, 1)
	source = strings.Replace(source, `"blockers":[]`, `"blockers":[{"operationId":"u1","code":"ACL_FUTURE_DENIAL","scope":"operation"}]`, 1)
	if _, err := ParseResult([]byte(source)); err == nil {
		t.Fatal("strict producer parse accepted future code")
	}
	result, err := DecodeResultCompatible([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if result.Blockers[0].Code != "ACL_FUTURE_DENIAL" || result.Allowed {
		t.Fatalf("future denial lost: %+v", result)
	}
}

func TestReferenceAndMaskRestrictionDiscriminators(t *testing.T) {
	base, err := ParseResult([]byte(inspectResultFixture))
	if err != nil {
		t.Fatal(err)
	}
	base.Result, base.Allowed, base.Operations[0].Result, base.Layers[0].Result = OutcomeConditional, false, OutcomeConditional, OutcomeConditional
	base.Coverage.Disclosure = "redacted"
	base.Restrictions = []Restriction{{ID: "r1", OperationID: "u1", LayerID: "ingit-crm", Kind: "row_filter", Representation: "reference", OmissionReason: "not_authorized"}}
	base.Operations[0].RestrictionIDs = []string{"r1"}
	base.Operations[0].AllOf = []string{"r1"}
	base.Layers[0].Decisions[0].RestrictionIDs = []string{"r1"}
	base.Layers[0].Decisions[0].Result = OutcomeConditional
	if _, err := MarshalResult(base); err != nil {
		t.Fatal(err)
	}
	base.Restrictions[0].Expression = &structuralCondition
	if _, err := MarshalResult(base); err == nil {
		t.Fatal("reference with expression accepted")
	}
}

var structuralCondition = func() (condition access.DocumentCondition) { return }()

func jsonEqual(a, b any) bool { return string(mustJSON(a)) == string(mustJSON(b)) }
func mustJSON(v any) []byte   { data, _ := json.Marshal(v); return data }
