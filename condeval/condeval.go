// Package condeval evaluates dal.Condition trees against record values.
//
// It is the shared, adapter-independent evaluator behind row-level access
// conditions: a condition is validated once when a policy is compiled
// (Validate), its parameters are substituted from runtime values
// (Substitute), and the resolved condition is matched against a record's data
// (Match). Record data is normalised through a JSON round trip (ToMap), so
// struct fields are addressed by their JSON names, numbers compare as float64,
// and times compare as RFC 3339 strings — the same shape the in-memory adapter
// stores and queries.
//
// The supported subset mirrors the core query model: comparisons of a field
// reference with a constant, an array (for In) or a parameter, combined with
// And/Or groups. Anything else is rejected by Validate and reported as an error
// by Match, never silently treated as a match.
package condeval

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// Info describes the field and parameter names a condition references.
type Info struct {
	Fields []string
	Params []string
}

var comparisonOperators = map[dal.Operator]bool{
	dal.Equal:          true,
	dal.In:             true,
	dal.GreaterThen:    true,
	dal.GreaterOrEqual: true,
	dal.LessThen:       true,
	dal.LessOrEqual:    true,
}

// Validate checks that condition uses only the supported subset and returns
// the field and parameter names it references, each sorted and de-duplicated.
func Validate(condition dal.Condition) (Info, error) {
	if condition == nil {
		return Info{}, fmt.Errorf("condeval: condition is required")
	}
	fields := map[string]struct{}{}
	params := map[string]struct{}{}
	if err := validate(condition, fields, params); err != nil {
		return Info{}, err
	}
	return Info{Fields: sortedKeys(fields), Params: sortedKeys(params)}, nil
}

func validate(condition dal.Condition, fields, params map[string]struct{}) error {
	switch c := condition.(type) {
	case dal.GroupCondition:
		if c.Operator() != dal.And && c.Operator() != dal.Or {
			return fmt.Errorf("condeval: unsupported group operator %q", c.Operator())
		}
		if len(c.Conditions()) == 0 {
			return fmt.Errorf("condeval: %s group must have at least one condition", c.Operator())
		}
		for _, sub := range c.Conditions() {
			if sub == nil {
				return fmt.Errorf("condeval: %s group contains a nil condition", c.Operator())
			}
			if err := validate(sub, fields, params); err != nil {
				return err
			}
		}
		return nil
	case dal.Comparison:
		if !comparisonOperators[c.Operator] {
			return fmt.Errorf("condeval: unsupported comparison operator %q", c.Operator)
		}
		field, ok := c.Left.(dal.FieldRef)
		if !ok {
			return fmt.Errorf("condeval: comparison left operand must be a field reference, got %T", c.Left)
		}
		if field.Source() != "" {
			return fmt.Errorf("condeval: field reference %q must not be source-qualified", field)
		}
		if field.Name() == "" {
			return fmt.Errorf("condeval: field reference has an empty name")
		}
		fields[field.Name()] = struct{}{}
		switch right := c.Right.(type) {
		case dal.Constant:
			if c.Operator == dal.In {
				return fmt.Errorf("condeval: In requires an array or a parameter on the right, got a constant")
			}
		case dal.Array:
			if c.Operator != dal.In {
				return fmt.Errorf("condeval: an array on the right requires the In operator, got %q", c.Operator)
			}
			if !isSlice(right.Value) {
				return fmt.Errorf("condeval: array value must be a slice or array, got %T", right.Value)
			}
		case dal.Param:
			if !dal.ValidParamName(right.Name) {
				return fmt.Errorf("condeval: invalid parameter name %q", right.Name)
			}
			params[right.Name] = struct{}{}
		default:
			return fmt.Errorf("condeval: comparison right operand must be a constant, an array or a parameter, got %T", c.Right)
		}
		return nil
	default:
		return fmt.Errorf("condeval: unsupported condition %T", condition)
	}
}

// Substitute returns a copy of condition with every parameter replaced by the
// value resolve returns for its name: a slice or array becomes a dal.Array,
// anything else a dal.Constant. An unresolved parameter is an error that names
// it, so callers can fail closed and explain why.
func Substitute(condition dal.Condition, resolve func(name string) (any, bool)) (dal.Condition, error) {
	switch c := condition.(type) {
	case dal.GroupCondition:
		subs := make([]dal.Condition, 0, len(c.Conditions()))
		for _, sub := range c.Conditions() {
			resolved, err := Substitute(sub, resolve)
			if err != nil {
				return nil, err
			}
			subs = append(subs, resolved)
		}
		return dal.NewGroupCondition(c.Operator(), subs...), nil
	case dal.Comparison:
		param, ok := c.Right.(dal.Param)
		if !ok {
			return c, nil
		}
		value, found := resolve(param.Name)
		if !found {
			return nil, fmt.Errorf("condeval: unresolved parameter $%s", param.Name)
		}
		var right dal.Expression = dal.Constant{Value: value}
		if isSlice(value) {
			right = dal.Array{Value: value}
		}
		return dal.Comparison{Operator: c.Operator, Left: c.Left, Right: right}, nil
	default:
		return nil, fmt.Errorf("condeval: unsupported condition %T", condition)
	}
}

// Match reports whether data satisfies condition. A nil condition matches. A
// field the record lacks never satisfies a comparison. A condition outside the
// supported subset, or one still carrying a parameter, is an error.
func Match(data map[string]any, condition dal.Condition) (bool, error) {
	switch c := condition.(type) {
	case nil:
		return true, nil
	case dal.GroupCondition:
		switch c.Operator() {
		case dal.And:
			for _, sub := range c.Conditions() {
				ok, err := Match(data, sub)
				if err != nil || !ok {
					return false, err
				}
			}
			return true, nil
		case dal.Or:
			for _, sub := range c.Conditions() {
				ok, err := Match(data, sub)
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		default:
			return false, fmt.Errorf("condeval: unsupported group operator %q", c.Operator())
		}
	case dal.Comparison:
		return matchComparison(data, c)
	default:
		return false, fmt.Errorf("condeval: unsupported condition %T", condition)
	}
}

func matchComparison(data map[string]any, c dal.Comparison) (bool, error) {
	field, ok := c.Left.(dal.FieldRef)
	if !ok {
		return false, fmt.Errorf("condeval: comparison left operand must be a field reference, got %T", c.Left)
	}
	var rightValue any
	switch right := c.Right.(type) {
	case dal.Constant:
		rightValue = right.Value
	case dal.Array:
		rightValue = right.Value
	case dal.Param:
		return false, fmt.Errorf("condeval: unresolved parameter $%s", right.Name)
	default:
		return false, fmt.Errorf("condeval: comparison right operand must be a constant or an array, got %T", c.Right)
	}
	fieldValue, found := Lookup(data, field.Name())
	if !found {
		return false, nil
	}
	switch c.Operator {
	case dal.Equal:
		return equal(fieldValue, rightValue), nil
	case dal.In:
		if !isSlice(rightValue) {
			return false, fmt.Errorf("condeval: In requires an array on the right, got %T", rightValue)
		}
		return contains(fieldValue, rightValue), nil
	case dal.GreaterThen, dal.GreaterOrEqual, dal.LessThen, dal.LessOrEqual:
		return ordered(c.Operator, fieldValue, rightValue), nil
	default:
		return false, fmt.Errorf("condeval: unsupported comparison operator %q", c.Operator)
	}
}

// Lookup resolves a dotted field path ("address.city") in nested maps.
func Lookup(data map[string]any, path string) (any, bool) {
	current := any(data)
	for _, part := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// ToMap renders record data as the JSON-normalised map Match evaluates: struct
// fields under their JSON names, numbers as float64, times as RFC 3339 strings.
// Nil data is an empty map.
func ToMap(data any) (map[string]any, error) {
	if data == nil {
		return map[string]any{}, nil
	}
	if pointer, ok := data.(*map[string]any); ok {
		if pointer == nil {
			return map[string]any{}, nil
		}
		data = *pointer
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("condeval: record data is not JSON-serialisable: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, fmt.Errorf("condeval: record data is not an object: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// normalize renders a value the way ToMap renders record data, so a constant
// written as int or time.Time compares against stored float64 or string.
func normalize(value any) (any, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var out any
	_ = json.Unmarshal(encoded, &out)
	return out, true
}

func equal(a, b any) bool {
	na, ok := normalize(a)
	if !ok {
		return false
	}
	nb, ok := normalize(b)
	if !ok {
		return false
	}
	return reflect.DeepEqual(na, nb)
}

// contains implements In: a scalar field value must be one of values; an
// array field value must share at least one element with values.
func contains(fieldValue, values any) bool {
	candidates := reflect.ValueOf(values)
	if isSlice(fieldValue) {
		elements := reflect.ValueOf(fieldValue)
		for i := 0; i < elements.Len(); i++ {
			for j := 0; j < candidates.Len(); j++ {
				if equal(elements.Index(i).Interface(), candidates.Index(j).Interface()) {
					return true
				}
			}
		}
		return false
	}
	for j := 0; j < candidates.Len(); j++ {
		if equal(fieldValue, candidates.Index(j).Interface()) {
			return true
		}
	}
	return false
}

func ordered(operator dal.Operator, a, b any) bool {
	na, ok := normalize(a)
	if !ok {
		return false
	}
	nb, ok := normalize(b)
	if !ok {
		return false
	}
	var cmp int
	switch left := na.(type) {
	case float64:
		right, ok := nb.(float64)
		if !ok {
			return false
		}
		cmp = compareFloats(left, right)
	case string:
		right, ok := nb.(string)
		if !ok {
			return false
		}
		cmp = strings.Compare(left, right)
	default:
		return false
	}
	switch operator {
	case dal.GreaterThen:
		return cmp > 0
	case dal.GreaterOrEqual:
		return cmp >= 0
	case dal.LessThen:
		return cmp < 0
	default: // dal.LessOrEqual
		return cmp <= 0
	}
}

func compareFloats(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func isSlice(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
