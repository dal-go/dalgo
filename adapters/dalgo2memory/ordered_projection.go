package dalgo2memory

// promoteOrderedSourceTimestamps materializes a declared record only for an
// ordered aggregate source. DALgo supplies its JSON-aware timestamp field
// selection; the adapter leaves every other projected value untouched.
func promoteOrderedSourceTimestamps(projected map[string]any, row memoryRow, factory func() any, values func(any) map[string]any) error {
	if factory == nil {
		return nil
	}
	raw := factory()
	if raw == nil {
		return nil
	}
	if err := row.materialize(raw); err != nil {
		return err
	}
	for name, value := range values(raw) {
		projected[name] = value
	}
	return nil
}
