package dal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dal-go/record"
)

// sortValuesKey is the reserved key under which a row, or the row of one source
// of a join, carries the sort values of its timestamp fields for the in-memory
// ordered aggregates. It is set only for a query that holds an ordered
// aggregate, only for the fields that aggregate reads, and no row a query
// returns holds it.
const sortValuesKey = "\x00dalgo_sort_values"

// timestampSortLayout writes an instant in UTC with a fixed width, so that the
// byte order of two texts is the time order of the two instants.
const timestampSortLayout = "2006-01-02T15:04:05.000000000Z"

// timestampSortValue is the sort value of a timestamp. A year outside 0000 to
// 9999 would change the width of the text, so it is refused.
func timestampSortValue(instant time.Time) (string, error) {
	utc := instant.UTC()
	if year := utc.Year(); year < 0 || year > 9999 {
		return "", errors.New("an aggregate order key holds a timestamp outside the supported years")
	}
	return utc.Format(timestampSortLayout), nil
}

// buildSortValues reads the fields of a provider's raw row, before it becomes
// JSON values, and returns the sort value of each one that holds a timestamp.
// It returns nil when no field does. A struct row is read as encoding/json writes
// it, so a field of an embedded struct is a field of the row.
func buildSortValues(raw any, fields []string) (map[string]any, error) {
	var values map[string]any
	for _, name := range fields {
		var instant time.Time
		switch value := rawField(raw, name, true).(type) {
		case time.Time:
			instant = value
		case *time.Time:
			if value == nil {
				continue
			}
			instant = *value
		default:
			continue
		}
		text, err := timestampSortValue(instant)
		if err != nil {
			return nil, err
		}
		if values == nil {
			values = map[string]any{}
		}
		values[name] = text
	}
	return values, nil
}

// sortValue reads a field the way evalScalar does, through the sort value of
// its source row when that row carries one for it, and through the ordinary
// value otherwise.
func sortValue(field FieldRef, row map[string]any) any {
	holder, ok := fieldHolder(field, row)
	if !ok {
		return nil
	}
	if values, ok := holder[sortValuesKey].(map[string]any); ok {
		if value, ok := values[field.Name()]; ok {
			return value
		}
	}
	value, _ := lookupAggregationField(holder, field.Name())
	return value
}

// evalSortKey evaluates an order key of an ordered aggregate for a row. A key is
// a field; the other branch serves an expression that validation would refuse.
func evalSortKey(expression Expression, row map[string]any) (any, error) {
	if field, ok := expression.(FieldRef); ok {
		return sortValue(field, row), nil
	}
	return evalScalar(expression, row)
}

// orderedAggregateFieldRefs lists the fields the ordered aggregates of q compare
// by: every order key, and the argument when it is a field, which breaks ties.
func orderedAggregateFieldRefs(q StructuredQuery) []FieldRef {
	var refs []FieldRef
	seen := map[string]bool{}
	add := func(field FieldRef) {
		id := field.Source() + "\x00" + field.Name()
		if !seen[id] {
			seen[id] = true
			refs = append(refs, field)
		}
	}
	walkAggregates(q, func(a AggregateFunc) {
		order := aggregateOrder(a)
		if len(order) == 0 {
			return
		}
		for _, key := range order {
			if key == nil {
				continue
			}
			if field, ok := key.Expression().(FieldRef); ok {
				add(field)
			}
		}
		for _, arg := range a.FuncArgs() {
			if field, ok := arg.(FieldRef); ok {
				add(field)
			}
		}
	})
	return refs
}

// orderedAggregateSortNames lists the names of the fields of a query over one
// source that need a sort value.
func orderedAggregateSortNames(q StructuredQuery) []string {
	var names []string
	seen := map[string]bool{}
	for _, field := range orderedAggregateFieldRefs(q) {
		if !seen[field.Name()] {
			seen[field.Name()] = true
			names = append(names, field.Name())
		}
	}
	return names
}

// orderedAggregateSortFields lists, for each source alias of a join, the names of
// the fields that need a sort value. A field with no source belongs to the base
// source, as it does when the join engine reads it.
func orderedAggregateSortFields(q StructuredQuery) map[string][]string {
	fields := map[string][]string{}
	refs := orderedAggregateFieldRefs(q)
	if len(refs) == 0 {
		return fields
	}
	base := ""
	if from := q.From(); from != nil && from.Base() != nil {
		base = joinAlias(from.Base())
	}
	for _, field := range refs {
		alias := field.Source()
		if alias == "" {
			alias = base
		}
		fields[alias] = append(fields[alias], field.Name())
	}
	return fields
}

// answerEncoding is the text that tells apart two answers DALgo's comparison puts
// together: an array and an object are compared by their printed form there, so
// ["a b"] and ["a","b"] tie, and so do 0 and -0. Their JSON encodings differ, and
// encoding/json writes the keys of an object in sorted order, so the text is a
// function of the value alone. A value that cannot be encoded is told apart by its
// printed form and type.
func answerEncoding(answer any) string {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return fmt.Sprintf("%T:%v", answer, answer)
	}
	return string(encoded)
}

// compareOrderedTuples orders two candidate rows of an ordered aggregate: by the
// order keys in the stated directions, then by the argument ascending, then by the
// answer each one returns. The argument's sort value is the instant of a timestamp,
// so two rows of one instant written in two zones still tie on it; the next step
// tells them apart by their text. The last step tells apart two answers that DALgo
// compares as equal by their JSON encodings, so the answer depends on the group's
// data alone.
func compareOrderedTuples(order []OrderExpression, a []any, aTie, aAnswer any, b []any, bTie, bAnswer any) int {
	for i, key := range order {
		comparison := compareAggregationValues(a[i], b[i])
		if key.Descending() {
			comparison = -comparison
		}
		if comparison != 0 {
			return comparison
		}
	}
	if comparison := compareAggregationValues(aTie, bTie); comparison != 0 {
		return comparison
	}
	if comparison := compareAggregationValues(aAnswer, bAnswer); comparison != 0 {
		return comparison
	}
	return strings.Compare(answerEncoding(aAnswer), answerEncoding(bAnswer))
}

// updateOrderedState lets one row compete for the answer of an ordered first or
// last. The state keeps the best row seen so far as a tuple of the sort values of
// the order keys and of the argument, beside the answer, and the tuple is charged
// to the retained-byte budget. A new row replaces it when it is earlier (first) or
// later (last); the result does not depend on the order rows arrive in.
func (r *localAggregationReader) updateOrderedState(group *localGroup, state *aggregateState, order []OrderExpression, arg Expression, row map[string]any, value any) error {
	keys := make([]any, len(order))
	for i, key := range order {
		sortKey, err := evalSortKey(key.Expression(), row)
		if err != nil {
			return err
		}
		keys[i] = sortKey
	}
	tie := value
	if field, ok := arg.(FieldRef); ok {
		tie = sortValue(field, row)
	}
	if state.hasValue {
		comparison := compareOrderedTuples(order, keys, tie, value, state.orderKeys, state.orderTie, state.value)
		if strings.EqualFold(state.expression.FuncName(), LAST) {
			if comparison <= 0 {
				return nil
			}
		} else if comparison >= 0 {
			return nil
		}
	}
	// The row replaces the tuple: the answer, the keys and the argument are what the state holds.
	bytes := aggregationValueBytes(value) + aggregationValueBytes(tie)
	for _, key := range keys {
		bytes += aggregationValueBytes(key)
	}
	if err := r.setAggregateState(group, state, value, bytes); err != nil {
		return err
	}
	state.orderKeys, state.orderTie = keys, tie
	return nil
}

// normalizedRow turns a provider's row into JSON values and, when the query
// holds an ordered aggregate, adds the sort values of the timestamps in it. The
// sort values are built first, from the raw row. Only the engine sets the key that
// carries them: a value the provider's own row holds under that name is removed.
func (r *localAggregationReader) normalizedRow(rec record.Record) (map[string]any, error) {
	values, err := buildSortValues(rec.Data(), r.sortFields)
	if err != nil {
		return nil, err
	}
	row, err := normalizedRecordMap(rec)
	if err != nil {
		return nil, err
	}
	if len(r.sortFields) > 0 {
		delete(row, sortValuesKey)
	}
	if values != nil {
		row[sortValuesKey] = values
	}
	return row, nil
}

// hashFallbackPlan is the plan of an aggregation that DALgo computes after the
// provider refused to run it.
var hashFallbackPlan = AggregationPlan{Strategy: AggregationHash, Reason: "provider refused the native aggregation as not supported"}

// refusedAsNotSupported reports whether a provider's native read of a query that
// holds an ordered aggregate failed because the provider cannot run it. The
// provider declared the capability and then declined this query, for example
// because of the type of a column, and no row was delivered, so DALgo computes it.
func refusedAsNotSupported(q StructuredQuery, err error) bool {
	return err != nil && errors.Is(err, ErrNotSupported) && holdsOrderedAggregate(q)
}

// executeAggregationLocal asks the executor for plain rows and aggregates them in
// DALgo's engine under plan.
func executeAggregationLocal(ctx context.Context, executor QueryExecutor, q StructuredQuery, plan AggregationPlan) (RecordsReader, error) {
	rawQuery := newAggregationSourceQuery(q, plan.Strategy == AggregationStreaming)
	raw, err := executor.ExecuteQueryToRecordsReader(ctx, rawQuery)
	if err != nil {
		return nil, err
	}
	return newLocalAggregationReader(ctx, q, raw, plan), nil
}

// executeAggregationAfterRefusal computes in DALgo's engine a query the provider
// refused as not supported. When the plain-row read fails too, the caller gets the
// provider's refusal.
func executeAggregationAfterRefusal(ctx context.Context, executor QueryExecutor, q StructuredQuery, refusal error) (RecordsReader, error) {
	reader, err := executeAggregationLocal(ctx, executor, q, hashFallbackPlan)
	if err != nil {
		return nil, refusal
	}
	return reader, nil
}

// promotedStructField finds the field encoding/json writes under name in a struct,
// among its own fields and those of the structs it embeds without a name of their
// own, depth by depth: a field of the struct shadows one of an embedded struct, and
// where several fields of one depth carry the name, a single one given the name by
// its tag wins and otherwise none does, as in encoding/json.
func promotedStructField(root reflect.Value, name string) (reflect.Value, bool) {
	level := []reflect.Value{root}
	explored := map[reflect.Type]bool{}
	for len(level) > 0 {
		var next, found, tagged []reflect.Value
		for _, current := range level {
			if explored[current.Type()] {
				continue
			}
			for i := 0; i < current.NumField(); i++ {
				field := current.Type().Field(i)
				fieldType := field.Type
				if fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				embeddedStruct := field.Anonymous && fieldType.Kind() == reflect.Struct
				// An unexported field is not written, but the fields of an unexported embedded struct are.
				if field.Tag.Get("json") == "-" || !field.IsExported() && !embeddedStruct {
					continue
				}
				written := strings.Split(field.Tag.Get("json"), ",")[0]
				if written == "" && embeddedStruct {
					embedded := current.Field(i)
					if embedded.Kind() == reflect.Pointer {
						if embedded.IsNil() {
							continue
						}
						embedded = embedded.Elem()
					}
					next = append(next, embedded)
					continue
				}
				isTagged := written != ""
				if !isTagged {
					written = field.Name
				}
				if written != name {
					continue
				}
				found = append(found, current.Field(i))
				if isTagged {
					tagged = append(tagged, current.Field(i))
				}
			}
		}
		switch {
		case len(found) == 1:
			return found[0], true
		case len(tagged) == 1:
			return tagged[0], true
		case len(found) > 1:
			return reflect.Value{}, false
		}
		for _, current := range level {
			explored[current.Type()] = true
		}
		level = next
	}
	return reflect.Value{}, false
}
