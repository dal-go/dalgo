// specscore: feat-recordops/diff
package recordops

// Option configures Diff/DiffFunc behavior. The package exports four
// orthogonal options:
//
//   - WithIgnoreFields(names...) — exclude named fields from comparison.
//   - WithIncludeMatched() — emit IDDiff for every ID, including fully matched.
//   - WithOnlyChangedFields() — trim Baseline.Fields to only fields with deltas.
//   - WithAbsentEqualsNil() — treat field-absent as equivalent to field-with-nil-value during comparison.
type Option func(*options)

// options is the internal aggregated configuration. Unexported.
type options struct {
	ignoreFields      map[string]struct{}
	includeMatched    bool
	onlyChangedFields bool
	absentEqualsNil   bool
}

// WithIgnoreFields instructs Diff to omit named fields from comparison.
// Matching is by Go struct field name (when Record.Data() returns a struct)
// or by map key (when Record.Data() returns a map[string]any). Case-sensitive.
// Multiple calls compose additively. Unknown names are silently ignored.
//
// Canonical use case: WithIgnoreFields("UpdatedAt") drops a timestamp field
// that always changes between snapshots.
func WithIgnoreFields(names ...string) Option {
	return func(o *options) {
		if o.ignoreFields == nil {
			o.ignoreFields = make(map[string]struct{}, len(names))
		}
		for _, n := range names {
			o.ignoreFields[n] = struct{}{}
		}
	}
}

// WithIncludeMatched instructs Diff to emit IDDiff for every ID touched
// by any input — including IDs where every candidate is Matched. Default
// is to skip those.
func WithIncludeMatched() Option {
	return func(o *options) {
		o.includeMatched = true
	}
}

// WithOnlyChangedFields trims IDDiff.Baseline.Fields to only the fields
// that have a delta on at least one candidate. Default is to populate
// the full baseline record snapshot for context.
func WithOnlyChangedFields() Option {
	return func(o *options) {
		o.onlyChangedFields = true
	}
}

// WithAbsentEqualsNil instructs Diff to treat "field absent from a record"
// as equivalent to "field present with nil value" during comparison.
// Default is to distinguish the two via FieldValue.Absent. Use this when
// the dataset is sourced from heterogeneous backends where one stores
// "no value" as an absent column and another stores it as NULL.
//
// When set: a baseline field with nil value and a candidate that lacks
// the field (or vice versa) produces no delta. Records whose differences
// all reduce to absent-vs-nil report Status == Matched.
func WithAbsentEqualsNil() Option {
	return func(o *options) {
		o.absentEqualsNil = true
	}
}

// resolveOptions applies the option functions and returns the aggregated config.
func resolveOptions(opts ...Option) options {
	o := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
