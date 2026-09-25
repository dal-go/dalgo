package dbschema

import "github.com/dal-go/dalgo/dal"

// ForeignKeyEnforcement describes whether a database foreign key is enforced
// for the connection used to inspect it. Unknown means the provider cannot
// establish the effective state; it must not be interpreted as enabled.
type ForeignKeyEnforcement string

const (
	ForeignKeyEnforcementUnknown  ForeignKeyEnforcement = "unknown"
	ForeignKeyEnforcementEnabled  ForeignKeyEnforcement = "enabled"
	ForeignKeyEnforcementDisabled ForeignKeyEnforcement = "disabled"
)

// ForeignKeyDef is one outgoing database foreign key. Fields and
// ReferencedFields are ordered in matching positions, including for composite
// keys. ReferencedFields may be empty when the database does not expose the
// implicit primary-key target columns.
type ForeignKeyDef struct {
	Name                 string
	Fields               []dal.FieldName
	ReferencedCollection string
	// ReferencedNamespace is set when the target is outside the source
	// collection's default namespace (for example another SQL schema).
	ReferencedNamespace string
	ReferencedFields    []dal.FieldName
	Enforcement         ForeignKeyEnforcement
	// OnUpdate and OnDelete are database referential actions (for example
	// CASCADE, RESTRICT, SET NULL). Empty means the provider did not report them.
	OnUpdate string
	OnDelete string
}
