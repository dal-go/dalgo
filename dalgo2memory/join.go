package dalgo2memory

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"

	"github.com/dal-go/dalgo/dal"
)

// joinedRow is one row of a join result: the base record id, the per-source
// data used to resolve qualified fields, and a flat merge of both sources
// used as the output record data. For an unmatched LEFT row the joined
// source's data is nil.
type joinedRow struct {
	baseID  string
	sources map[string]map[string]any
	merged  map[string]any
}

// executeJoinQuery executes a StructuredQuery whose From carries a single
// INNER or LEFT equi-join, via a nested loop over the two in-memory
// collections with source-qualified field resolution for ON and WHERE.
func (s session) executeJoinQuery(q dal.StructuredQuery) (dal.RecordsReader, error) {
	from := q.From()
	joins := from.Joins()
	if len(joins) != 1 {
		return nil, fmt.Errorf("dalgo2memory: only a single join is supported per query, got %d", len(joins))
	}
	join := joins[0]
	switch join.JoinType() {
	case dal.JoinInner, dal.JoinLeft:
		// supported
	default:
		return nil, fmt.Errorf("dalgo2memory: unsupported join type %q (only INNER and LEFT are supported)", join.JoinType())
	}

	base := from.Base()
	baseKey := sourceKey(base)
	joinKey := sourceKey(join)
	known := map[string]bool{"": true, baseKey: true, joinKey: true}

	if err := validateOrderSources(q.OrderBy(), known); err != nil {
		return nil, err
	}

	baseRows, err := s.loadRows(base.Name())
	if err != nil {
		return nil, err
	}
	joinRows, err := s.loadRows(join.Name())
	if err != nil {
		return nil, err
	}
	sortByID(baseRows)
	sortByID(joinRows)

	combined := make([]joinedRow, 0, len(baseRows))
	for _, br := range baseRows {
		matched := false
		for _, jr := range joinRows {
			sources := map[string]map[string]any{"": br.data, baseKey: br.data, joinKey: jr.data}
			ok, err := allConditionsMatch(join.On(), sources, known)
			if err != nil {
				return nil, err
			}
			if ok {
				combined = append(combined, joinedRow{baseID: br.id, sources: sources, merged: mergeData(br.data, jr.data)})
				matched = true
			}
		}
		if !matched && join.JoinType() == dal.JoinLeft {
			sources := map[string]map[string]any{"": br.data, baseKey: br.data, joinKey: nil}
			combined = append(combined, joinedRow{baseID: br.id, sources: sources, merged: mergeData(br.data, nil)})
		}
	}

	filtered := make([]joinedRow, 0, len(combined))
	for _, row := range combined {
		ok, err := matchesJoinCondition(q.Where(), row.sources, known)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, row)
		}
	}

	orderJoinedRows(filtered, q.OrderBy())

	if limit := q.Limit(); limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	records := make([]dal.Record, 0, len(filtered))
	for _, row := range filtered {
		key := dal.NewKeyWithID(base.Name(), row.baseID)
		if q.IntoRecord() == nil {
			records = append(records, dal.NewRecord(key).SetError(nil))
			continue
		}
		records = append(records, dal.NewRecordWithData(key, row.merged).SetError(nil))
	}
	return dal.NewRecordsReader(records), nil
}

// sourceKey is the key a qualified FieldRef.Source() must match to resolve
// against this recordset: its alias if set, otherwise its name.
func sourceKey(rs dal.RecordsetSource) string {
	if a := rs.Alias(); a != "" {
		return a
	}
	return rs.Name()
}

// validateOrderSources rejects an ORDER BY FieldRef whose non-empty Source()
// names no recordset in the query, before sorting (the sort callback cannot
// return an error). A non-FieldRef key is not an error here — it is skipped
// during the sort.
func validateOrderSources(orderBy []dal.OrderExpression, known map[string]bool) error {
	for _, oe := range orderBy {
		f, ok := oe.Expression().(dal.FieldRef)
		if !ok {
			continue
		}
		if src := f.Source(); src != "" && !known[src] {
			return fmt.Errorf("dalgo2memory: ORDER BY field %q references unknown source %q", f.Name(), src)
		}
	}
	return nil
}

// orderBySources is the shared ORDER BY comparator for both the single-source
// and join paths. It stably sorts rows by the ORDER BY expressions in declared
// order, resolving each FieldRef key against sourcesOf(row) (empty Source() ->
// the "" entry; non-empty -> the matching source), honoring Descending() per
// key, skipping non-FieldRef keys, with idOf(row) as the final tiebreak.
func orderBySources[T any](rows []T, orderBy []dal.OrderExpression, sourcesOf func(T) map[string]map[string]any, idOf func(T) string) {
	sort.SliceStable(rows, func(i, j int) bool {
		si, sj := sourcesOf(rows[i]), sourcesOf(rows[j])
		for _, oe := range orderBy {
			f, ok := oe.Expression().(dal.FieldRef)
			if !ok {
				continue
			}
			c := compare(si[f.Source()][f.Name()], sj[f.Source()][f.Name()])
			if oe.Descending() {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return idOf(rows[i]) < idOf(rows[j])
	})
}

// orderJoinedRows orders join result rows via the shared comparator.
func orderJoinedRows(rows []joinedRow, orderBy []dal.OrderExpression) {
	orderBySources(rows, orderBy,
		func(r joinedRow) map[string]map[string]any { return r.sources },
		func(r joinedRow) string { return r.baseID })
}

func (s session) loadRows(collectionName string) ([]memoryRow, error) {
	collection := s.db.collections[collectionName]
	rows := make([]memoryRow, 0, len(collection))
	for id, b := range collection {
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			return nil, err
		}
		rows = append(rows, memoryRow{id: id, data: data, raw: b})
	}
	return rows, nil
}

func sortByID(rows []memoryRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
}

func mergeData(base, join map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(join))
	maps.Copy(merged, base)
	maps.Copy(merged, join)
	return merged
}

// resolveJoinExpr resolves a FieldRef against the per-source data, or returns
// a Constant's value. An empty source denotes the From base. A non-empty
// source that names no recordset in the query is a descriptive error.
func resolveJoinExpr(e dal.Expression, sources map[string]map[string]any, known map[string]bool) (any, bool, error) {
	switch v := e.(type) {
	case dal.FieldRef:
		src := v.Source()
		if !known[src] {
			return nil, false, fmt.Errorf("dalgo2memory: field %q references unknown source %q", v.Name(), src)
		}
		val, present := sources[src][v.Name()]
		return val, present, nil
	case dal.Constant:
		return v.Value, true, nil
	default:
		return nil, false, fmt.Errorf("dalgo2memory: unsupported expression %T in join query", e)
	}
}

// allConditionsMatch reports whether every ON condition holds (AND).
func allConditionsMatch(conds []dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	for _, c := range conds {
		ok, err := matchesJoinCondition(c, sources, known)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// matchesJoinCondition evaluates a single equality Comparison over the
// per-source data. A nil condition matches; any non-equality shape does not
// (mirroring the in-memory adapter's single-source WHERE support).
func matchesJoinCondition(cond dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	if cond == nil {
		return true, nil
	}
	cmp, ok := cond.(dal.Comparison)
	if !ok || cmp.Operator != dal.Equal {
		return false, nil
	}
	l, lok, err := resolveJoinExpr(cmp.Left, sources, known)
	if err != nil {
		return false, err
	}
	r, rok, err := resolveJoinExpr(cmp.Right, sources, known)
	if err != nil {
		return false, err
	}
	return lok && rok && valuesEqual(l, r), nil
}

func valuesEqual(a, b any) bool {
	if af, aok := number(a); aok {
		if bf, bok := number(b); bok {
			return af == bf
		}
	}
	return a == b
}
