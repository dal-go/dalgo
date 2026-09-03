package condeval

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
)

func field(name string) dal.FieldRef { return dal.Field(name) }

func cmp(name string, op dal.Operator, right dal.Expression) dal.Comparison {
	return dal.Comparison{Operator: op, Left: field(name), Right: right}
}

func TestValidate(t *testing.T) {
	good := dal.NewGroupCondition(dal.And,
		cmp("createdBy", dal.Equal, dal.NewParam("currentUser")),
		dal.NewGroupCondition(dal.Or,
			cmp("status", dal.In, dal.Array{Value: []string{"draft", "review"}}),
			cmp("age", dal.GreaterOrEqual, dal.Constant{Value: 18}),
			cmp("role", dal.In, dal.NewParam("principal.roles")),
		),
	)
	info, err := Validate(good)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if want := []string{"age", "createdBy", "role", "status"}; !reflect.DeepEqual(info.Fields, want) {
		t.Errorf("Fields = %v, want %v", info.Fields, want)
	}
	if want := []string{"currentUser", "principal.roles"}; !reflect.DeepEqual(info.Params, want) {
		t.Errorf("Params = %v, want %v", info.Params, want)
	}
	for name, bad := range map[string]dal.Condition{
		"nil":               nil,
		"unknown node":      fakeCondition{},
		"group operator":    dal.NewGroupCondition(dal.Operator("XOR"), cmp("a", dal.Equal, dal.Constant{Value: 1})),
		"empty group":       dal.NewGroupCondition(dal.And),
		"nil in group":      dal.NewGroupCondition(dal.And, nil),
		"bad nested":        dal.NewGroupCondition(dal.And, fakeCondition{}),
		"operator":          cmp("a", dal.Operator("!="), dal.Constant{Value: 1}),
		"left constant":     dal.Comparison{Operator: dal.Equal, Left: dal.Constant{Value: 1}, Right: field("a")},
		"left qualified":    dal.Comparison{Operator: dal.Equal, Left: dal.NewFieldRef("t", "a"), Right: dal.Constant{Value: 1}},
		"left empty":        dal.Comparison{Operator: dal.Equal, Left: dal.NewFieldRef("", ""), Right: dal.Constant{Value: 1}},
		"in with constant":  cmp("a", dal.In, dal.Constant{Value: 1}),
		"array without in":  cmp("a", dal.Equal, dal.Array{Value: []int{1}}),
		"array not a slice": cmp("a", dal.In, dal.Array{Value: 1}),
		"array nil":         cmp("a", dal.In, dal.Array{Value: nil}),
		"bad param name":    cmp("a", dal.Equal, dal.Param{Name: "1x"}),
		"right unknown":     dal.Comparison{Operator: dal.Equal, Left: field("a"), Right: field("b")},
	} {
		if _, err := Validate(bad); err == nil {
			t.Errorf("Validate(%s) = nil error, want error", name)
		}
	}
}

type fakeCondition struct{}

func (fakeCondition) String() string { return "fake" }

func TestSubstitute(t *testing.T) {
	resolve := func(name string) (any, bool) {
		switch name {
		case "currentUser":
			return "u1", true
		case "principal.roles":
			return []string{"admin", "member"}, true
		}
		return nil, false
	}
	condition := dal.NewGroupCondition(dal.And,
		cmp("createdBy", dal.Equal, dal.NewParam("currentUser")),
		cmp("role", dal.In, dal.NewParam("principal.roles")),
		cmp("status", dal.Equal, dal.Constant{Value: "active"}),
	)
	resolved, err := Substitute(condition, resolve)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if got := resolved.String(); got != "(createdBy = 'u1' AND role In ('admin','member') AND status = 'active')" {
		t.Errorf("String() = %q", got)
	}
	if _, err := Substitute(cmp("a", dal.Equal, dal.NewParam("missing")), resolve); err == nil || !strings.Contains(err.Error(), "$missing") {
		t.Errorf("unresolved parameter error = %v", err)
	}
	if _, err := Substitute(dal.NewGroupCondition(dal.And, cmp("a", dal.Equal, dal.NewParam("missing"))), resolve); err == nil {
		t.Error("unresolved parameter inside a group must fail")
	}
	if _, err := Substitute(fakeCondition{}, resolve); err == nil {
		t.Error("unknown condition must fail")
	}
}

func TestMatch(t *testing.T) {
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	data, err := ToMap(map[string]any{
		"ownerID": "u1",
		"age":     42,
		"tags":    []string{"a", "b"},
		"when":    now,
		"address": map[string]any{"city": "Limerick"},
		"flag":    true,
	})
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	for name, tc := range map[string]struct {
		condition dal.Condition
		want      bool
	}{
		"nil":                      {nil, true},
		"equal string":             {cmp("ownerID", dal.Equal, dal.Constant{Value: "u1"}), true},
		"equal mismatch":           {cmp("ownerID", dal.Equal, dal.Constant{Value: "u2"}), false},
		"equal int vs float":       {cmp("age", dal.Equal, dal.Constant{Value: 42}), true},
		"equal bool":               {cmp("flag", dal.Equal, dal.Constant{Value: true}), true},
		"equal time":               {cmp("when", dal.Equal, dal.Constant{Value: now}), true},
		"missing field":            {cmp("nope", dal.Equal, dal.Constant{Value: "u1"}), false},
		"nested":                   {cmp("address.city", dal.Equal, dal.Constant{Value: "Limerick"}), true},
		"nested missing":           {cmp("address.zip", dal.Equal, dal.Constant{Value: "x"}), false},
		"nested not a map":         {cmp("ownerID.x", dal.Equal, dal.Constant{Value: "x"}), false},
		"in scalar":                {cmp("ownerID", dal.In, dal.Array{Value: []string{"u2", "u1"}}), true},
		"in scalar miss":           {cmp("ownerID", dal.In, dal.Array{Value: []string{"u2"}}), false},
		"in array overlap":         {cmp("tags", dal.In, dal.Array{Value: []string{"z", "b"}}), true},
		"in array miss":            {cmp("tags", dal.In, dal.Array{Value: []string{"z"}}), false},
		"gt":                       {cmp("age", dal.GreaterThen, dal.Constant{Value: 41}), true},
		"ge":                       {cmp("age", dal.GreaterOrEqual, dal.Constant{Value: 42}), true},
		"lt":                       {cmp("age", dal.LessThen, dal.Constant{Value: 42}), false},
		"le":                       {cmp("age", dal.LessOrEqual, dal.Constant{Value: 42}), true},
		"time before":              {cmp("when", dal.LessThen, dal.Constant{Value: now.Add(time.Hour)}), true},
		"ordered mixed":            {cmp("age", dal.GreaterThen, dal.Constant{Value: "x"}), false},
		"ordered string vs number": {cmp("ownerID", dal.GreaterThen, dal.Constant{Value: 1}), false},
		"ordered unsupported type": {cmp("flag", dal.GreaterThen, dal.Constant{Value: true}), false},
		"and": {dal.NewGroupCondition(dal.And,
			cmp("ownerID", dal.Equal, dal.Constant{Value: "u1"}),
			cmp("age", dal.GreaterThen, dal.Constant{Value: 40})), true},
		"and short-circuit": {dal.NewGroupCondition(dal.And,
			cmp("ownerID", dal.Equal, dal.Constant{Value: "u2"}),
			cmp("age", dal.GreaterThen, dal.Constant{Value: 40})), false},
		"or": {dal.NewGroupCondition(dal.Or,
			cmp("ownerID", dal.Equal, dal.Constant{Value: "u2"}),
			cmp("age", dal.GreaterThen, dal.Constant{Value: 40})), true},
		"or none": {dal.NewGroupCondition(dal.Or,
			cmp("ownerID", dal.Equal, dal.Constant{Value: "u2"}),
			cmp("age", dal.GreaterThen, dal.Constant{Value: 50})), false},
	} {
		got, err := Match(data, tc.condition)
		if err != nil {
			t.Errorf("Match(%s): %v", name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Match(%s) = %v, want %v", name, got, tc.want)
		}
	}
	for name, bad := range map[string]dal.Condition{
		"unknown node":     fakeCondition{},
		"group operator":   dal.NewGroupCondition(dal.Operator("XOR"), cmp("a", dal.Equal, dal.Constant{Value: 1})),
		"and nested error": dal.NewGroupCondition(dal.And, fakeCondition{}),
		"or nested error":  dal.NewGroupCondition(dal.Or, fakeCondition{}),
		"left not a field": dal.Comparison{Operator: dal.Equal, Left: dal.Constant{Value: 1}, Right: dal.Constant{Value: 1}},
		"unresolved param": cmp("ownerID", dal.Equal, dal.NewParam("currentUser")),
		"right unknown":    dal.Comparison{Operator: dal.Equal, Left: field("ownerID"), Right: field("x")},
		"in without array": cmp("ownerID", dal.In, dal.Constant{Value: "u1"}),
		"unknown operator": cmp("ownerID", dal.Operator("!="), dal.Constant{Value: "u1"}),
	} {
		if _, err := Match(data, bad); err == nil {
			t.Errorf("Match(%s) = nil error, want error", name)
		}
	}
}

func TestMatchNonSerialisableValues(t *testing.T) {
	data := map[string]any{"ch": make(chan int), "n": 1.0}
	if ok, _ := Match(data, cmp("ch", dal.Equal, dal.Constant{Value: 1})); ok {
		t.Error("a non-serialisable field value must not equal anything")
	}
	if ok, _ := Match(data, cmp("n", dal.Equal, dal.Constant{Value: make(chan int)})); ok {
		t.Error("a non-serialisable constant must not equal anything")
	}
	if ok, _ := Match(data, cmp("ch", dal.GreaterThen, dal.Constant{Value: 1})); ok {
		t.Error("a non-serialisable field value must not be ordered")
	}
	if ok, _ := Match(data, cmp("n", dal.GreaterThen, dal.Constant{Value: make(chan int)})); ok {
		t.Error("a non-serialisable constant must not be ordered")
	}
}

func TestToMap(t *testing.T) {
	type user struct {
		Name  string `json:"name"`
		Email string `json:"email,omitempty"`
	}
	if m, err := ToMap(nil); err != nil || len(m) != 0 {
		t.Errorf("ToMap(nil) = %v, %v", m, err)
	}
	if m, err := ToMap((*map[string]any)(nil)); err != nil || len(m) != 0 {
		t.Errorf("ToMap(nil pointer) = %v, %v", m, err)
	}
	source := map[string]any{"a": 1}
	if m, err := ToMap(&source); err != nil || m["a"] != 1.0 {
		t.Errorf("ToMap(*map) = %v, %v", m, err)
	}
	if m, err := ToMap(user{Name: "Ada"}); err != nil || m["name"] != "Ada" || len(m) != 1 {
		t.Errorf("ToMap(struct) = %v, %v", m, err)
	}
	if m, err := ToMap(&user{Name: "Ada"}); err != nil || m["name"] != "Ada" {
		t.Errorf("ToMap(*struct) = %v, %v", m, err)
	}
	if _, err := ToMap(make(chan int)); err == nil {
		t.Error("ToMap(chan) must fail")
	}
	if _, err := ToMap([]int{1}); err == nil {
		t.Error("ToMap(slice) must fail: not an object")
	}
	if m, err := ToMap((*user)(nil)); err != nil || len(m) != 0 {
		t.Errorf("ToMap(nil struct pointer) = %v, %v", m, err)
	}
}
