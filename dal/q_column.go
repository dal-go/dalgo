package dal

import "fmt"

// Column reference a column in a SELECT statement
type Column struct {
	Alias      string     `json:"Alias"`
	Expression Expression `json:"expression"`
}

// String stringifies column value
func (v Column) String() string {
	if v.Alias == "" {
		return v.Expression.String()
	}
	expr := v.Expression.String()
	if expr == v.Alias {
		return expr
	}
	return fmt.Sprintf("%v AS %v", expr, v.Alias)
}
