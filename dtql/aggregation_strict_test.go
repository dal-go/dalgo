package dtql

import (
	"strings"
	"testing"
)

func TestAggregationYAMLRejectsUnknownNestedKeys(t *testing.T) {
	for name, document := range map[string]string{
		"aggregate": "from: {name: orders}\ncolumns: [{aggregate: {function: count, args: [{star: true}], typo: true}}]\n",
		"binary":    "from: {name: orders}\ncolumns: [{aggregate: {function: sum, args: [{binary: {op: '*', left: {field: a}, right: {field: b}, typo: true}}]}}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Deserialize([]byte(document))
			if err == nil || !strings.Contains(err.Error(), "field typo not found") {
				t.Fatalf("got %v", err)
			}
		})
	}
}
