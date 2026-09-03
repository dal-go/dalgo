package access

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// residual is one policy's resolved row condition for one request resource,
// with the attribution needed to explain a denial without leaking values.
type residual struct {
	policy       string
	policySource string
	rule         string
	text         string
	resource     Resource
	condition    dal.Condition
}

func (r residual) deny(operation Operations, explanation string) *DeniedError {
	return &DeniedError{Decision: Decision{
		Operation:    operation,
		Resource:     r.resource,
		Policy:       r.policy,
		PolicySource: r.policySource,
		Rule:         r.rule,
		Effect:       effectDeny.String(),
		Condition:    r.text,
		Explanation:  explanation,
	}}
}

// matchResiduals evaluates every residual for one resource against record
// data. The first failing residual is returned as a denial that names its
// rule and condition text, never the record's values.
func matchResiduals(operation Operations, data map[string]any, residuals []residual) error {
	for _, r := range residuals {
		ok, err := condeval.Match(data, r.condition)
		if err != nil {
			return r.deny(operation, fmt.Sprintf("row condition could not be evaluated for rule %q: %v", r.rule, err))
		}
		if !ok {
			return r.deny(operation, fmt.Sprintf("row condition not satisfied for rule %q (where: %s)", r.rule, r.text))
		}
	}
	return nil
}

// checkRecord enforces the residuals of a point read on a record the adapter
// has just loaded. A record that does not exist needs no check. On denial the
// record's data is cleared and its error set, so the caller receives nothing
// but the denial.
func checkRecord(operation Operations, rec record.Record, residuals []residual) error {
	if len(residuals) == 0 || !rec.Exists() {
		return nil
	}
	data, err := condeval.ToMap(rec.Data())
	if err != nil {
		denied := residuals[0].deny(operation, fmt.Sprintf("row condition could not be evaluated: %v", err))
		clearRecord(rec, denied)
		return denied
	}
	if err := matchResiduals(operation, data, residuals); err != nil {
		clearRecord(rec, err)
		return err
	}
	return nil
}

// clearRecord removes the loaded data from a denied record: a pointer target
// is zeroed, a map is emptied, and the record's error is set to the denial. A
// record that is not in the loaded state (not found, or already denied) has
// no data to clear and only receives the error.
func clearRecord(rec record.Record, denied error) {
	if err := rec.Error(); err != nil && !errors.Is(err, record.ErrNoError) {
		rec.SetError(denied)
		return
	}
	value := reflect.ValueOf(rec.Data())
	switch value.Kind() {
	case reflect.Pointer:
		if !value.IsNil() {
			value.Elem().Set(reflect.Zero(value.Elem().Type()))
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			value.SetMapIndex(key, reflect.Value{})
		}
	}
	rec.SetError(denied)
}

// rewriteQuery applies the residuals of a Query request. Only the base source
// (resource index 0) may carry a residual in this version; a residual on a
// joined source is refused. A residual on a non-structured query cannot occur,
// because conditional rules are not valid on opaque-query scopes, but it is
// refused as well rather than assumed away.
func rewriteQuery(query dal.Query, residuals [][]residual) (dal.Query, error) {
	if len(residuals) == 0 {
		return query, nil
	}
	for i := 1; i < len(residuals); i++ {
		for _, r := range residuals[i] {
			return nil, r.deny(Query, fmt.Sprintf("conditional rule %q applies to a joined source; conditional rules on joins are not supported in this version", r.rule))
		}
	}
	base := residuals[0]
	if len(base) == 0 {
		return query, nil
	}
	structured, ok := query.(dal.StructuredQuery)
	if !ok {
		return nil, base[0].deny(Query, "row conditions require a structured query")
	}
	conditions := make([]dal.Condition, len(base))
	for i, r := range base {
		conditions[i] = r.condition
	}
	condition := conditions[0]
	if len(conditions) > 1 {
		condition = dal.NewGroupCondition(dal.And, conditions...)
	}
	if existing := structured.Where(); existing != nil {
		condition = dal.NewGroupCondition(dal.And, existing, condition)
	}
	return dal.WithWhere(structured, condition), nil
}

// existsThroughRead upgrades an existence check to a read so a residual can be
// evaluated. The record is read into a private map, never into caller memory.
func existsThroughRead(ctx context.Context, session dal.ReadSession, key *record.Key, residuals []residual) (bool, error) {
	shadow := record.NewRecordWithData(key, &map[string]any{})
	if err := session.Get(ctx, shadow); err != nil {
		return false, err
	}
	if !shadow.Exists() {
		return false, nil
	}
	if err := checkRecord(Exists, shadow, residuals); err != nil {
		return false, err
	}
	return true, nil
}

// checkRecords enforces residuals on a multi-record read. Any denial clears
// every record, so a batch never returns a partial result.
func checkRecords(operation Operations, records []record.Record, residuals [][]residual) error {
	if len(residuals) == 0 {
		return nil
	}
	for i, rec := range records {
		if err := checkRecord(operation, rec, residuals[i]); err != nil {
			for _, other := range records {
				clearRecord(other, err)
			}
			return err
		}
	}
	return nil
}
