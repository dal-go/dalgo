package dtql

import "github.com/dal-go/dalgo/dal"

// inScopeComparisonOps is the set of dal comparison operators DTQL represents.
// Their YAML form is the dal.Operator string itself (e.g. "==", "In", ">").
var inScopeComparisonOps = map[dal.Operator]bool{
	dal.Equal:          true, // ==
	dal.In:             true, // In
	dal.GreaterThen:    true, // >
	dal.GreaterOrEqual: true, // >=
	dal.LessThen:       true, // <
	dal.LessOrEqual:    true, // <=
}

// inScopeGroupOps is the set of dal group operators DTQL represents.
var inScopeGroupOps = map[dal.Operator]bool{
	dal.And: true, // AND
	dal.Or:  true, // OR
}
