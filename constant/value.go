package constant

import (
	"fmt"
)

type ValueConst struct {
	value any
}

func Value(v any) ValueConst {
	return ValueConst{value: v}
}

func (v ValueConst) String() string {
	return fmt.Sprintf("%+v", v.value)
}
