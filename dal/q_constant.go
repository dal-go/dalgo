package dal

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type constantExpression struct {
	Value any `json:"value"`
}

func (v constantExpression) Equal(b constantExpression) bool {
	return v.Value == b.Value
}

// String returns string representation of a constantExpression
func (v constantExpression) String() string {
	switch v.Value.(type) {
	case int:
		return strconv.Itoa(v.Value.(int))
	case string:
		return fmt.Sprintf("'%v'", v.Value)
	default:
		s, _ := json.Marshal(v.Value)
		return string(s)
	}
}
