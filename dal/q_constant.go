package dal

import (
	"encoding/json"
	"fmt"
	"strconv"
)

var _ Expression = Constant{}
var _ Expression = (*Constant)(nil)

type Constant struct {
	Value any `json:"value"`
}

func (v Constant) Equal(b Constant) bool {
	return v.Value == b.Value
}

// String returns string representation of a Constant
func (v Constant) String() string {
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
