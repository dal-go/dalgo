package dal

// Column reference a column in a SELECT statement
type Column struct {
	Alias      string     `json:"Alias"`
	Expression Expression `json:"expression"`
}

// String stringifies column value
func (v Column) String() string {
	var expr string
	if v.Expression == nil {
		expr = "NULL"
	} else {
		expr = v.Expression.String()
	}
	if v.Alias == "" {
		return expr
	}
	return expr + " AS " + v.Alias
}
