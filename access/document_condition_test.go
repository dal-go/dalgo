package access

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

const conditionalPolicyYAML = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: customers
default: deny
scopes:
  - path: /customers/*
    rules:
      - id: read-all
        effect: allow
        operations: [read]
      - id: edit-assigned
        effect: allow
        operations: [update, delete]
        where:
          op: "=="
          left: { field: assignedTo }
          right: { param: currentUser }
  - path: /customers
    rules:
      - id: query-open-or-mine
        effect: allow
        operations: [query]
        where:
          or:
            - op: "=="
              left: { field: status }
              right: { value: open }
            - and:
                - op: In
                  left: { field: region }
                  right: { values: [eu, uk] }
                - op: ">="
                  left: { field: score }
                  right: { value: 10 }
`

func TestConditionalDocumentRoundTrip(t *testing.T) {
	policy, err := UnmarshalAccessPolicyYAML([]byte(conditionalPolicyYAML))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ctx := WithCurrentUser(context.Background(), "u1")
	resource := RecordResourceForKey(record.NewKeyWithID("customers", "c1"))
	decision := policy.Decide(ctx, Request{Operation: Update, Resources: []Resource{resource}})
	if !decision.Allowed || decision.Condition != "assignedTo = $currentUser" {
		t.Fatalf("decoded conditional rule = %+v", decision)
	}
	collection := CollectionResourceFor(nil, "customers")
	decision = policy.Decide(ctx, Request{Operation: Query, Resources: []Resource{collection}})
	if !decision.Allowed || decision.Residuals[0].String() != "(status = 'open' OR (region In ('eu','uk') AND score >= 10))" {
		t.Fatalf("decoded group rule = %+v", decision)
	}
	for name, marshal := range map[string]func(*AccessPolicy) ([]byte, error){"yaml": MarshalAccessPolicyYAML, "json": MarshalAccessPolicyJSON} {
		encoded, err := marshal(policy)
		if err != nil {
			t.Fatalf("%s encode: %v", name, err)
		}
		if !strings.Contains(string(encoded), "currentUser") {
			t.Errorf("%s encoding lost the parameter: %s", name, encoded)
		}
		var again *AccessPolicy
		if name == "yaml" {
			again, err = UnmarshalAccessPolicyYAML(encoded)
		} else {
			again, err = UnmarshalAccessPolicyJSON(encoded)
		}
		if err != nil {
			t.Fatalf("%s decode again: %v", name, err)
		}
		for _, request := range []Request{
			{Operation: Update, Resources: []Resource{resource}},
			{Operation: Query, Resources: []Resource{collection}},
			{Operation: Get, Resources: []Resource{resource}},
		} {
			first, second := policy.Decide(ctx, request), again.Decide(ctx, request)
			if first.Allowed != second.Allowed || first.Condition != second.Condition || first.Rule != second.Rule {
				t.Errorf("%s: decisions differ for %s: %+v vs %+v", name, request.Operation, first, second)
			}
		}
	}
	// A document with no conditions still round-trips unchanged.
	plain := MustPolicy("plain", Root(Allow(Read, "all")))
	encoded, err := MarshalAccessPolicyYAML(plain)
	if err != nil || strings.Contains(string(encoded), "where") {
		t.Errorf("plain policy: err=%v encoded=%s", err, encoded)
	}
}

func TestDocumentConditionErrors(t *testing.T) {
	field := func(name string) *DocumentExpression { return &DocumentExpression{Field: name} }
	value := func(v any) *DocumentExpression { return &DocumentExpression{Value: v} }
	for name, condition := range map[string]DocumentCondition{
		"empty":            {},
		"mixed forms":      {Op: "==", Left: field("a"), Right: value(1), And: []DocumentCondition{{Op: "==", Left: field("a"), Right: value(1)}}},
		"unknown operator": {Op: "!=", Left: field("a"), Right: value(1)},
		"missing right":    {Op: "==", Left: field("a")},
		"bad left":         {Op: "==", Left: &DocumentExpression{}, Right: value(1)},
		"bad right":        {Op: "==", Left: field("a"), Right: &DocumentExpression{Field: "b", Value: 1}},
		"bad param":        {Op: "==", Left: field("a"), Right: &DocumentExpression{Param: "1x"}},
		"empty and":        {And: []DocumentCondition{}},
		"empty or":         {Or: []DocumentCondition{}},
		"nested error":     {Or: []DocumentCondition{{}}},
		"deny condition":   {Op: "==", Left: field("a"), Right: value(1)}, // valid condition on a deny rule below
	} {
		effect := "allow"
		if name == "deny condition" {
			effect = "deny"
		}
		document := Document{APIVersion: DocumentAPIVersion, Kind: AccessPolicyKind, Metadata: DocumentMetadata{Name: "x"}, Default: "deny",
			Scopes: []DocumentScope{{Path: "/x/*", Rules: []DocumentRule{{ID: "r", Effect: effect, Operations: []string{"get"}, Where: &condition}}}}}
		if _, err := DecodeAccessPolicy(strings.NewReader(""), staticCodec{d: document}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Encoding rejects conditions outside the portable subset.
	for name, condition := range map[string]dal.Condition{
		"fake":           fakeCond{},
		"operator":       dal.Comparison{Operator: dal.Operator("!="), Left: dal.Field("a"), Right: dal.Constant{Value: 1}},
		"left":           dal.Comparison{Operator: dal.Equal, Left: fakeExpr{}, Right: dal.Constant{Value: 1}},
		"right":          dal.Comparison{Operator: dal.Equal, Left: dal.Field("a"), Right: fakeExpr{}},
		"group operator": dal.NewGroupCondition(dal.Operator("XOR"), dal.WhereField("a", dal.Equal, dal.Constant{Value: 1})),
		"group member":   dal.NewGroupCondition(dal.And, fakeCond{}),
	} {
		if _, err := documentFromCondition(condition); err == nil {
			t.Errorf("encode %s: expected an error", name)
		}
	}
	// A rule whose condition cannot be encoded is reported as not serializable.
	policy := &AccessPolicy{name: "p", rules: []Rule{Root(Rule{kind: directiveRule, effect: effectAllow, operations: Get, name: "r", where: fakeCond{}})}}
	if _, err := MarshalAccessPolicyYAML(policy); !errors.Is(err, ErrNotSerializable) {
		t.Errorf("unserialisable condition = %v", err)
	}
}

type fakeExpr struct{}

func (fakeExpr) String() string { return "fake" }
