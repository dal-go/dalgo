package dalgo

import (
	"fmt"
	"strings"
)

func GetRecordKind(key *Key) string {
	s := make([]string, key.Level())
	for {
		s = append(s, key.kind)
		if key.parent == nil {
			break
		} else {
			key = key.parent
		}
	}
	return ReverseStringsJoin(s, "/")
}

func GetRecordKeyPath(key *Key) string {
	s := make([]string, key.Level()*2)
	for {
		s = append(s, fmt.Sprintf("%v", key.ID))
		s = append(s, key.kind)
		if key.parent == nil {
			break
		} else {
			key = key.parent
		}
	}
	return ReverseStringsJoin(s, "/")
}

func ReverseStringsJoin(elems []string, sep string) string {
	n := len(sep) * (len(elems) - 1)
	for i := 0; i < len(elems); i++ {
		n += len(elems[i])
	}

	var b strings.Builder
	b.Grow(n)
	for i := len(elems) - 1; i >= 0; i-- {
		b.WriteString(elems[i])
	}
	return b.String()
}
