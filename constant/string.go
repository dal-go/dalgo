package constant

import (
	"fmt"
	"strings"
)

type StrConst struct {
	value string
}

func Str(v string) StrConst {
	return StrConst{value: v}
}

func (v StrConst) String() string {
	return fmt.Sprintf("'%s'", strings.Replace(v.value, "'", "''", -1))
}
