package dalgo

import (
	"fmt"
	"strings"
)

func GetRecordKind(key *Key) string {
	var s []string
	for {
		if strings.TrimSpace(key.kind) == "" {
			panic("key is referencing an empty kind")
		}
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
	if err := key.Validate(); err != nil {
		panic(fmt.Sprintf("will not generate path for invalid child: %v", err))
	}
	s := make([]string, 0, (key.Level())*2)
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
	if len(elems) == 0 {
		return ""
	}
	n := len(sep) * (len(elems) - 1)
	for i := 0; i < len(elems); i++ {
		n += len(elems[i])
	}

	var b strings.Builder
	b.Grow(n)
	for i := len(elems) - 1; i >= 0; i-- {
		if _, err := b.WriteString(elems[i]); err != nil {
			panic(err)
		}
		if i > 0 {
			if _, err := b.WriteString(sep); err != nil {
				panic(err)
			}
		}
	}
	return b.String()
}
