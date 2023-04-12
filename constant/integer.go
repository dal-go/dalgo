package constant

import (
	"strconv"
)

type IntConst struct {
	value int
}

func (i IntConst) String() string {
	return strconv.Itoa(i.value)
}

func Int(v int) IntConst {
	return IntConst{value: v}
}
