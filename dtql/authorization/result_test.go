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

func TestResultValidationRejectsMalformedContractStates(t *testing.T) {
	valid := func(t *testing.T) Result {
		t.Helper()
		result, err := ParseResult([]byte(inspectResultFixture))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	tests := map[string]func(*Result){
		"api":     func(r *Result) { r.APIVersion = "future" },
		"request": func(r *Result) { r.RequestID = "" },
		"mode":    func(r *Result) { r.Mode = "future" },
		"allowed blockers": func(r *Result) {
			r.Blockers = []Blocker{{OperationID: "u1", Code: "ACCESS_DENIED", Scope: "operation"}}
		},
		"sample missing":             func(r *Result) { r.Mode, r.Scope = ModeSample, ScopeSample },
		"sample forbidden":           func(r *Result) { r.Sample = &Sample{} },
		"nil operations":             func(r *Result) { r.Operations = nil },
		"coverage evaluation":        func(r *Result) { r.Coverage.Evaluation = "future" },
		"coverage disclosure":        func(r *Result) { r.Coverage.Disclosure = "future" },
		"duplicate operation":        func(r *Result) { r.Operations = append(r.Operations, r.Operations[0]) },
		"nil operation restrictions": func(r *Result) { r.Operations[0].RestrictionIDs = nil },
		"bad operation":              func(r *Result) { r.Operations[0].Action = "future" },
		"callable mismatch":          func(r *Result) { r.Operations[0].Callable = &Callable{} },
		"unknown operation restriction": func(r *Result) {
			r.Operations[0].RestrictionIDs, r.Operations[0].AllOf = []string{"missing"}, []string{"missing"}
		},
		"bad layer":                    func(r *Result) { r.Layers[0].ACLState = "future" },
		"nil decisions":                func(r *Result) { r.Layers[0].Decisions = nil },
		"nil decision restrictions":    func(r *Result) { r.Layers[0].Decisions[0].RestrictionIDs = nil },
		"bad decision":                 func(r *Result) { r.Layers[0].Decisions[0].Scope = "future" },
		"unknown decision restriction": func(r *Result) { r.Layers[0].Decisions[0].RestrictionIDs = []string{"missing"} },
		"bad blocker": func(r *Result) {
			r.Result, r.Allowed, r.Operations[0].Result, r.Layers[0].Result = OutcomeDeny, false, OutcomeDeny, OutcomeDeny
			r.Blockers = []Blocker{{OperationID: "", Code: "ACCESS_DENIED", Scope: "operation"}}
		},
		"bad blocker slot": func(r *Result) {
			r.Result, r.Allowed, r.Operations[0].Result, r.Layers[0].Result = OutcomeDeny, false, OutcomeDeny, OutcomeDeny
			r.Blockers = []Blocker{{OperationID: "u1", Code: "ACCESS_DENIED", Scope: "operation", Slot: "future"}}
		},
		"bad unevaluated": func(r *Result) { r.Coverage.Unevaluated = []Unevaluated{{OperationID: "u1", Reason: "future"}} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := valid(t)
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("malformed result accepted")
			}
		})
	}
}

func TestRestrictionRepresentationsValidate(t *testing.T) {
	base := func() Result {
		r, _ := ParseResult([]byte(inspectResultFixture))
		r.Result, r.Allowed, r.Operations[0].Result, r.Layers[0].Result, r.Layers[0].Decisions[0].Result = OutcomeConditional, false, OutcomeConditional, OutcomeConditional, OutcomeConditional
		r.Operations[0].RestrictionIDs, r.Operations[0].AllOf = []string{"r1"}, []string{"r1"}
		r.Layers[0].Decisions[0].RestrictionIDs = []string{"r1"}
		return r
	}
	valid := []Restriction{
		{ID: "r1", OperationID: "u1", Enforced: true, Representation: "expression", Kind: "row_filter", Expression: &access.DocumentCondition{Op: "==", Left: &access.DocumentExpression{Field: "id"}, Right: &access.DocumentExpression{Value: "1"}}},
		{ID: "r1", OperationID: "u1", Enforced: true, Representation: "fields", Kind: "field_allowlist", Fields: []string{"name"}},
		{ID: "r1", OperationID: "u1", Enforced: true, Representation: "reference", Kind: "opaque", OmissionReason: "private"},
		{ID: "r1", OperationID: "u1", Enforced: true, Representation: "mask", Kind: "field_mask", Mask: &access.Mask{Stages: []access.MaskStage{{Include: []string{"*"}}}}},
	}
	for _, restriction := range valid {
		result := base()
		result.Restrictions = []Restriction{restriction}
		if err := result.Validate(); err != nil {
			t.Fatalf("valid restriction %+v: %v", restriction, err)
		}
	}
	invalid := []Restriction{
		{},
		{ID: "r1", OperationID: "u1", Representation: "future", Kind: "opaque"},
		{ID: "r1", OperationID: "u1", Representation: "expression", Kind: "opaque", Expression: &structuralCondition},
		{ID: "r1", OperationID: "u1", Representation: "fields", Kind: "field_allowlist"},
		{ID: "r1", OperationID: "u1", Representation: "reference", Kind: "opaque", OmissionReason: "future"},
		{ID: "r1", OperationID: "u1", Representation: "mask", Kind: "field_mask", Mask: &access.Mask{}},
	}
	for _, restriction := range invalid {
		result := base()
		result.Restrictions = []Restriction{restriction}
		if err := result.Validate(); err == nil {
			t.Fatalf("invalid restriction accepted: %+v", restriction)
		}
	}
}

var structuralCondition = func() (condition access.DocumentCondition) { return }()

func jsonEqual(a, b any) bool { return string(mustJSON(a)) == string(mustJSON(b)) }
func mustJSON(v any) []byte   { data, _ := json.Marshal(v); return data }
