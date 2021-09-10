package db

import "strings"

func GetRecordKind(key RecordKey) string {
	var kinds []string
	for _, ref := range key {
		kinds = append(kinds, ref.Kind)
	}
	return strings.Join(kinds, "/")
}

func GetRecordKeyPath(key RecordKey) string {
	var p []string
	for _, ref := range key {
		p = append(p, ref.Kind, ref.ID)
	}
	return strings.Join(p, "/")
}
