package dal

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type constantCondition struct {
	Value any `json:"value"`
}

// String returns string representation of a constantCondition
func (v constantCondition) String() string {
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
