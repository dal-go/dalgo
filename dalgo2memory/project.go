package dalgo2memory

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// validateColumns rejects a projected column whose expression is not a
// FieldRef, or whose non-empty Source() names no recordset in the query,
// before any rows are produced -- consistent with the WHERE/ORDER BY
// unresolvable-source behavior. An empty columns list is valid (no projection).
func validateColumns(columns []dal.Column, known map[string]bool) error {
	for _, col := range columns {
		f, ok := col.Expression.(dal.FieldRef)
		if !ok {
			return fmt.Errorf("dalgo2memory: projected column %q is not a field reference", col.String())
		}
		if src := f.Source(); src != "" && !known[src] {
			return fmt.Errorf("dalgo2memory: projected column %q references unknown source %q", f.Name(), src)
		}
	}
	return nil
}

// baseSources builds the per-source resolution map for a single-source query
// row: the empty key and the base recordset's name (and alias when set) all
// resolve to the same row data.
func baseSources(base dal.RecordsetSource, data map[string]any) map[string]map[string]any {
	src := map[string]map[string]any{"": data, base.Name(): data}
	if a := base.Alias(); a != "" {
		src[a] = data
	}
	return src
}

// columnKey returns the output map key for a projected column: its Alias,
// falling back to the column's field name when the alias is empty.
func columnKey(col dal.Column, f dal.FieldRef) string {
	if col.Alias != "" {
		return col.Alias
	}
	return f.Name()
}

// projectRow builds the projected output map for one row: one entry per
// selected column keyed by alias/field-name, resolving each column's FieldRef
// against the per-source data (empty Source() -> base; alias/name -> that
// source). Each column expression is expected to be a resolvable FieldRef.
func projectRow(columns []dal.Column, sources map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(columns))
	for _, col := range columns {
		f := col.Expression.(dal.FieldRef)
		out[columnKey(col, f)] = sources[f.Source()][f.Name()]
	}
	return out
}
