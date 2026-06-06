package dalgo2memory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// rowSources is one input row's per-source resolution map — the same shape the
// join WHERE/ORDER BY resolver and baseSources produce (empty key -> base data;
// alias/name -> that source's data). The grouping engine consumes rows of this
// shape so a single implementation serves both the single-source and join paths.
type rowSources = map[string]map[string]any

// aggGroup is one GROUP BY partition: the member rows (for aggregate
// evaluation) and the projected output row (set during projection, then read
// back by HAVING/ORDER BY when an operand references a SELECT alias).
type aggGroup struct {
	rows []rowSources
	out  map[string]any
}

// executeGroupedReader runs the grouping/aggregation pass shared by the
// single-source and join executors. rows are the WHERE-matched input rows in
// per-source shape; collection names the result key collection; known is the
// set of resolvable source qualifiers.
func executeGroupedReader(q dal.StructuredQuery, rows []rowSources, collection string, known map[string]bool) (dal.RecordsReader, error) {
	groupBy := q.GroupBy()
	columns := q.Columns()

	// Validate GROUP BY field sources up front.
	for _, ge := range groupBy {
		if f, ok := ge.(dal.FieldRef); ok {
			if src := f.Source(); src != "" && !known[src] {
				return nil, fmt.Errorf("dalgo2memory: GROUP BY field %q references unknown source %q", f.Name(), src)
			}
		}
	}

	// Validate selected columns: each must be an aggregate or a GROUP BY
	// expression (the SQL grouping rule), rejected before any group is emitted.
	groupKeys := make(map[string]bool, len(groupBy))
	for _, ge := range groupBy {
		groupKeys[ge.String()] = true
	}
	for _, col := range columns {
		if _, ok := col.Expression.(dal.AggregateFunc); ok {
			continue
		}
		if groupKeys[col.Expression.String()] {
			continue
		}
		return nil, fmt.Errorf("dalgo2memory: selected column %q is neither an aggregate nor in GROUP BY", col.String())
	}

	// Partition into groups, preserving first-seen order.
	groupOrder := make([]string, 0)
	byKey := make(map[string]*aggGroup)
	for _, r := range rows {
		key, err := groupKey(groupBy, r, known)
		if err != nil {
			return nil, err
		}
		g := byKey[key]
		if g == nil {
			g = &aggGroup{}
			byKey[key] = g
			groupOrder = append(groupOrder, key)
		}
		g.rows = append(g.rows, r)
	}
	groups := make([]*aggGroup, len(groupOrder))
	for i, k := range groupOrder {
		groups[i] = byKey[k]
	}

	// Project each group's selected columns into its output row.
	for _, g := range groups {
		out := make(map[string]any, len(columns))
		for _, col := range columns {
			val, err := resolveGroupValue(col.Expression, g, known)
			if err != nil {
				return nil, err
			}
			out[columnOutKey(col)] = val
		}
		g.out = out
	}

	// HAVING: post-aggregation filter over each group's row.
	if having := q.Having(); having != nil {
		kept := make([]*aggGroup, 0, len(groups))
		for _, g := range groups {
			ok, err := matchesHaving(having, g, known)
			if err != nil {
				return nil, err
			}
			if ok {
				kept = append(kept, g)
			}
		}
		groups = kept
	}

	// ORDER BY over the grouped rows. Sort keys are resolved once per group
	// up front so any resolution error surfaces before sorting (the comparator
	// itself cannot return an error).
	if orderBy := q.OrderBy(); len(orderBy) > 0 {
		keys := make([][]any, len(groups))
		for i, g := range groups {
			row := make([]any, len(orderBy))
			for k, oe := range orderBy {
				v, err := resolveGroupValue(oe.Expression(), g, known)
				if err != nil {
					return nil, err
				}
				row[k] = v
			}
			keys[i] = row
		}
		indexed := make([]int, len(groups))
		for i := range indexed {
			indexed[i] = i
		}
		sort.SliceStable(indexed, func(a, b int) bool {
			ki, kj := keys[indexed[a]], keys[indexed[b]]
			for k, oe := range orderBy {
				c := compare(ki[k], kj[k])
				if oe.Descending() {
					c = -c
				}
				if c != 0 {
					return c < 0
				}
			}
			return false
		})
		sorted := make([]*aggGroup, len(groups))
		for i, idx := range indexed {
			sorted[i] = groups[idx]
		}
		groups = sorted
	}

	// OFFSET then LIMIT on the grouped rows.
	if offset := q.Offset(); offset > 0 {
		if offset >= len(groups) {
			groups = nil
		} else {
			groups = groups[offset:]
		}
	}
	if limit := q.Limit(); limit > 0 && limit < len(groups) {
		groups = groups[:limit]
	}

	records := make([]dal.Record, len(groups))
	for i, g := range groups {
		key := dal.NewKeyWithID(collection, fmt.Sprint(i))
		records[i] = dal.NewRecordWithData(key, g.out).SetError(nil)
	}
	return dal.NewRecordsReader(records), nil
}

// groupKey builds a stable partition key from the resolved GROUP BY expression
// values of a single row. The type tag avoids collisions between values of
// different types that share a textual form (e.g. int 1 vs string "1").
func groupKey(groupBy []dal.Expression, r rowSources, known map[string]bool) (string, error) {
	parts := make([]string, len(groupBy))
	for i, ge := range groupBy {
		v, _, err := resolveJoinExpr(ge, r, known)
		if err != nil {
			return "", err
		}
		parts[i] = fmt.Sprintf("%T:%v", v, v)
	}
	return strings.Join(parts, "\x00"), nil
}

// resolveGroupValue resolves an expression to a single per-group value: an
// aggregate is evaluated over the group's rows; a FieldRef resolves first
// against the already-projected output row (so HAVING/ORDER BY can reference a
// SELECT alias), then as a GROUP BY field from a member row; a Constant yields
// its value.
func resolveGroupValue(e dal.Expression, g *aggGroup, known map[string]bool) (any, error) {
	switch ex := e.(type) {
	case dal.AggregateFunc:
		return evalAggregate(ex, g.rows, known)
	case dal.FieldRef:
		if ex.Source() == "" && g.out != nil {
			if v, ok := g.out[ex.Name()]; ok {
				return v, nil
			}
		}
		// A group always has at least one member row.
		v, _, err := resolveJoinExpr(ex, g.rows[0], known)
		return v, err
	case dal.Constant:
		return ex.Value, nil
	default:
		return nil, fmt.Errorf("dalgo2memory: unsupported grouped expression %T", e)
	}
}

// evalAggregate computes one aggregate over a group's rows with standard SQL
// null handling (NULLs skipped). COUNT(*) (a non-field argument) counts all
// rows; COUNT(field) counts non-null values; SUM/AVG operate on numeric values
// (all-null -> nil); MIN/MAX reduce by the shared comparator (all-null -> nil).
func evalAggregate(af dal.AggregateFunc, rows []rowSources, known map[string]bool) (any, error) {
	args := af.FuncArgs()
	switch af.FuncName() {
	case dal.COUNT:
		if len(args) == 1 {
			if _, isField := args[0].(dal.FieldRef); !isField {
				return len(rows), nil
			}
		}
		n := 0
		for _, r := range rows {
			v, present, err := resolveJoinExpr(args[0], r, known)
			if err != nil {
				return nil, err
			}
			if present && v != nil {
				n++
			}
		}
		return n, nil
	case dal.SUM, dal.AVERAGE:
		sum, cnt := 0.0, 0
		for _, r := range rows {
			v, present, err := resolveJoinExpr(args[0], r, known)
			if err != nil {
				return nil, err
			}
			if !present || v == nil {
				continue
			}
			f, ok := number(v)
			if !ok {
				continue
			}
			sum += f
			cnt++
		}
		if cnt == 0 {
			return nil, nil
		}
		if af.FuncName() == dal.AVERAGE {
			return sum / float64(cnt), nil
		}
		return sum, nil
	case dal.MIN, dal.MAX:
		var best any
		found := false
		for _, r := range rows {
			v, present, err := resolveJoinExpr(args[0], r, known)
			if err != nil {
				return nil, err
			}
			if !present || v == nil {
				continue
			}
			if !found {
				best, found = v, true
				continue
			}
			c := compare(v, best)
			if (af.FuncName() == dal.MIN && c < 0) || (af.FuncName() == dal.MAX && c > 0) {
				best = v
			}
		}
		if !found {
			return nil, nil
		}
		return best, nil
	default:
		return nil, fmt.Errorf("dalgo2memory: unsupported aggregate %q", af.FuncName())
	}
}

// matchesHaving evaluates a HAVING condition over a projected group. A
// Comparison resolves both operands to per-group values (alias or aggregate
// expression) and applies the operator; a GroupCondition composes AND/OR.
func matchesHaving(cond dal.Condition, g *aggGroup, known map[string]bool) (bool, error) {
	switch c := cond.(type) {
	case dal.Comparison:
		l, err := resolveGroupValue(c.Left, g, known)
		if err != nil {
			return false, err
		}
		r, err := resolveGroupValue(c.Right, g, known)
		if err != nil {
			return false, err
		}
		return compareOp(c.Operator, l, r), nil
	case dal.GroupCondition:
		conds := c.Conditions()
		if c.Operator() == dal.Or {
			for _, sub := range conds {
				ok, err := matchesHaving(sub, g, known)
				if err != nil {
					return false, err
				}
				if ok {
					return true, nil
				}
			}
			return false, nil
		}
		for _, sub := range conds {
			ok, err := matchesHaving(sub, g, known)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("dalgo2memory: unsupported HAVING condition %T", cond)
	}
}

// compareOp applies a comparison operator to two values via the shared
// numeric/string comparator.
func compareOp(op dal.Operator, l, r any) bool {
	c := compare(l, r)
	switch op {
	case dal.Equal:
		return c == 0
	case dal.GreaterThen:
		return c > 0
	case dal.GreaterOrEqual:
		return c >= 0
	case dal.LessThen:
		return c < 0
	case dal.LessOrEqual:
		return c <= 0
	default:
		return false
	}
}

// columnOutKey is the output map key for a selected column: its Alias, else a
// FieldRef's name, else the expression's textual form.
func columnOutKey(col dal.Column) string {
	if col.Alias != "" {
		return col.Alias
	}
	if f, ok := col.Expression.(dal.FieldRef); ok {
		return f.Name()
	}
	return col.Expression.String()
}
