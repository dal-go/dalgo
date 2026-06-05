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
// used to build the output record. For an unmatched LEFT row the joined
// source's data is nil.
type joinedRow struct {
	baseID  string
	sources map[string]map[string]any
	merged  map[string]any
}

// executeJoinQuery executes a StructuredQuery whose From carries a single
// INNER or LEFT equi-join, via a nested loop over the two in-memory
// collections with source-qualified field resolution.
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
	joinKey := sourceKey(join.RecordsetSource)
	known := map[string]bool{"": true, baseKey: true, joinKey: true}

	baseRows, err := s.loadRows(base.Name())
	if err != nil {
		return nil, err
	}
	joinRows, err := s.loadRows(join.RecordsetSource.Name())
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
			ok, err := evalJoinConditions(join.On(), sources, known)
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
		ok, err := matchesWhereJoined(q.Where(), row.sources, known)
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
		template := q.IntoRecord()
		if template == nil {
			records = append(records, dal.NewRecord(key).SetError(nil))
			continue
		}
		b, err := json.Marshal(row.merged)
		if err != nil {
			return nil, err
		}
		data := template.Data()
		if err := json.Unmarshal(b, data); err != nil {
			return nil, err
		}
		records = append(records, dal.NewRecordWithData(key, data).SetError(nil))
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
// source that names no recordset in the query is treated as absent here; Task
// 6 turns that into a descriptive error.
func resolveJoinExpr(e dal.Expression, sources map[string]map[string]any, known map[string]bool) (any, bool, error) {
	switch v := e.(type) {
	case dal.FieldRef:
		src := v.Source()
		if !known[src] {
			return nil, false, nil
		}
		val, present := sources[src][v.Name()]
		return val, present, nil
	case dal.Constant:
		return v.Value, true, nil
	default:
		return nil, false, fmt.Errorf("dalgo2memory: unsupported expression %T in join query", e)
	}
}

func evalJoinConditions(conds []dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	for _, c := range conds {
		ok, err := matchesWhereJoined(c, sources, known)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func matchesWhereJoined(cond dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	if cond == nil {
		return true, nil
	}
	switch c := cond.(type) {
	case dal.Comparison:
		if c.Operator != dal.Equal {
			return false, nil
		}
		l, lok, err := resolveJoinExpr(c.Left, sources, known)
		if err != nil {
			return false, err
		}
		r, rok, err := resolveJoinExpr(c.Right, sources, known)
		if err != nil {
			return false, err
		}
		return lok && rok && valuesEqual(l, r), nil
	case dal.GroupCondition:
		return matchesGroupJoined(c, sources, known)
	default:
		return false, nil
	}
}

func matchesGroupJoined(g dal.GroupCondition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	conds := g.Conditions()
	if g.Operator() == dal.Or {
		for _, c := range conds {
			ok, err := matchesWhereJoined(c, sources, known)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	for _, c := range conds { // And (default)
		ok, err := matchesWhereJoined(c, sources, known)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func valuesEqual(a, b any) bool {
	if af, aok := number(a); aok {
		if bf, bok := number(b); bok {
			return af == bf
		}
	}
	return a == b
}

func orderJoinedRows(rows []joinedRow, orderBy []dal.OrderExpression) {
	if len(orderBy) == 0 {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].baseID < rows[j].baseID })
		return
	}
	field, ok := orderBy[0].Expression().(dal.FieldRef)
	if !ok {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		cmp := compare(rows[i].merged[field.Name()], rows[j].merged[field.Name()])
		if orderBy[0].Descending() {
			return cmp > 0
		}
		return cmp < 0
	})
}
