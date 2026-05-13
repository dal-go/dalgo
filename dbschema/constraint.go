package dbschema

// ConstraintDef is the portable description of one constraint on a
// collection. Tier-1 keeps this minimal — Name and Type only. The
// richer shape required for specific constraint kinds (check
// expression, unique field list, foreign-key target + cascade
// actions, etc.) is intentionally deferred. Engine-specific reader
// extensions (Tier 2) MAY define richer constraint types in their
// own packages without waiting for Tier 1 to grow.
type ConstraintDef struct {
	// Name is the constraint name.
	Name string
	// Type is the engine-neutral kind:
	// "check", "unique", "primary-key", "foreign-key".
	Type string
}
