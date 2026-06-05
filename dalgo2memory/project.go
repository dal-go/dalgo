package dalgo2memory

import "github.com/dal-go/dalgo/dal"

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
