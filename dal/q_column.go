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

// Excludes reports whether name is excluded by this projection. Exclusions
// without a '*' match exactly and case-sensitively. An exclusion containing
// one or more '*' is a case-insensitive mask that must match the entire column
// name; '*' may occur anywhere and matches zero or more characters. No other
// mask characters have special meaning.
func (v WildcardProjection) Excludes(name string) bool {
	for _, exclusion := range v.Exclude {
		if strings.ContainsRune(exclusion, '*') {
			if wildcardMaskMatches(exclusion, name) {
				return true
			}
			continue
		}
		if exclusion == name {
			return true
		}
	}
	return false
}

// wildcardMaskMatches reports whether mask matches name. Both values are
// iterated as runes so masks work for Unicode column names as well as ASCII.
func wildcardMaskMatches(mask, name string) bool {
	value := make([]rune, 0, len(name))
	for _, runeValue := range name {
		value = append(value, runeValue)
	}
	states := make([]bool, len(value)+1)
	states[0] = true

	for _, token := range mask {
		next := make([]bool, len(value)+1)
		if token == '*' {
			for i := range states {
				if states[i] {
					next[i] = true
				}
				if i > 0 && next[i-1] {
					next[i] = true
				}
			}
		} else {
			for i := 1; i < len(states); i++ {
				if states[i-1] && strings.EqualFold(string(token), string(value[i-1])) {
					next[i] = true
				}
			}
		}
		states = next
	}
	return states[len(value)]
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
