package recordset

// Evaluator computes a value for a computed column from the stored (non-computed)
// values of a row. The stored map is keyed by column name.
type Evaluator interface {
	Eval(stored map[string]any) (value any, err error)
}
