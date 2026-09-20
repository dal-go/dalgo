package dal

import "strings"

// WildcardProjection selects every column from Source except the named
// exclusions. Source is optional; when set it names the source alias or
// collection whose wildcard is being projected. Exclusions deliberately retain
// their requested spelling and order so executors can report unmatched names in
// diagnostic metadata in the future.
type WildcardProjection struct {
	Source  string   `json:"source,omitempty"`
	Exclude []string `json:"exclude"`
}

// String renders the wildcard projection in DTQL's compact notation.
func (v WildcardProjection) String() string {
	prefix := "*"
	if v.Source != "" {
		prefix = v.Source + ".*"
	}
	return prefix + "-(" + strings.Join(v.Exclude, ", ") + ")"
}

// Column reference a column in a SELECT statement
type Column struct {
	Alias      string              `json:"Alias"`
	Expression Expression          `json:"expression"`
	Wildcard   *WildcardProjection `json:"wildcard,omitempty"`
}

// AllColumnsExcept returns an unqualified wildcard projection that excludes
// the supplied column names.
func AllColumnsExcept(excluded ...string) Column {
	return AllColumnsExceptFrom("", excluded...)
}

// AllColumnsExceptFrom returns a wildcard projection scoped to source that
// excludes the supplied unqualified column names.
func AllColumnsExceptFrom(source string, excluded ...string) Column {
	return Column{Wildcard: &WildcardProjection{
		Source:  source,
		Exclude: append([]string(nil), excluded...),
	}}
}

// String stringifies column value
func (v Column) String() string {
	var expr string
	if v.Wildcard != nil {
		expr = v.Wildcard.String()
	} else if v.Expression == nil {
		expr = "NULL"
	} else {
		expr = v.Expression.String()
	}
	if v.Alias == "" {
		return expr
	}
	return expr + " AS " + v.Alias
}
