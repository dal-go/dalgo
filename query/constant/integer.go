package constant

import (
	"fmt"
	"strconv"
	"strings"
)

type integer struct {
	value int
}

func (i integer) String() string {
	return strconv.Itoa(i.value)
}

func Int(v int) integer {
	return integer{value: v}
}

type str struct {
	value string
}

func Str(v string) str {
	return str{value: v}
}

func (v str) String() string {
	return fmt.Sprintf("'%v'", strings.Replace(v.value, "'", "''", -1))
}
