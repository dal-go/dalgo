package recordset

// Evaluator computes a value for a computed column from the stored (non-computed)
// values of a row. The stored map is keyed by column name.
type Evaluator interface {
	// Eval returns the computed value for stored, a map of column name to non-computed value.
	Eval(stored map[string]any) (value any, err error)
}
