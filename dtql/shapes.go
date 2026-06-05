package dtql

// document is the YAML representation of an in-scope dal.StructuredQuery.
// Field order here defines the canonical key order of a DTQL-YAML document.
type document struct {
	From    fromYAML     `yaml:"from"`
	Columns []columnYAML `yaml:"columns,omitempty"`
	Where   *condYAML    `yaml:"where,omitempty"`
	OrderBy []orderYAML  `yaml:"orderBy,omitempty"`
	Limit   int          `yaml:"limit,omitempty"`
	Offset  int          `yaml:"offset,omitempty"`
}

// fromYAML is the YAML representation of the root dal.CollectionRef source.
type fromYAML struct {
	Name  string `yaml:"name"`
	Alias string `yaml:"alias,omitempty"`
}

// exprYAML is the YAML representation of an in-scope dal.Expression.
// Exactly one of Field / Value / Values is set, which discriminates a
// FieldRef, a Constant or an Array respectively.
type exprYAML struct {
	Field  string `yaml:"field,omitempty"`  // dal.FieldRef
	Value  any    `yaml:"value,omitempty"`  // dal.Constant (inline scalar)
	Values any    `yaml:"values,omitempty"` // dal.Array (inline sequence)
}

// columnYAML is the YAML representation of a dal.Column.
type columnYAML struct {
	exprYAML `yaml:",inline"`
	As       string `yaml:"as,omitempty"` // dal.Column.Alias
}

// orderYAML is the YAML representation of a dal.OrderExpression.
type orderYAML struct {
	exprYAML `yaml:",inline"`
	Desc     bool `yaml:"desc,omitempty"` // descending order
}

// condYAML is the YAML representation of a dal.Condition.
// A Comparison sets Op/Left/Right; a GroupCondition sets And or Or.
type condYAML struct {
	Op    string     `yaml:"op,omitempty"`    // dal.Comparison.Operator
	Left  *exprYAML  `yaml:"left,omitempty"`  // dal.Comparison.Left
	Right *exprYAML  `yaml:"right,omitempty"` // dal.Comparison.Right
	And   []condYAML `yaml:"and,omitempty"`   // dal.GroupCondition (And)
	Or    []condYAML `yaml:"or,omitempty"`    // dal.GroupCondition (Or)
}
