package dal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
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

func NewConstant(v any) Constant {
	switch v := v.(type) {
	case
		nil,
		bool,
		string,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		time.Time:
		return Constant{Value: v}
	case Constant:
		return v
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
}
