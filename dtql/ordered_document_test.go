package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestOrderedAggregateDocumentRoundTrip(t *testing.T) {
	const input = "from:\n  name: Invoice\ncolumns:\n  - aggregate:\n      function: last\n      args:\n        - field: Total\n      orderBy:\n        - field: InvoiceDate\n        - field: InvoiceId\n          desc: true\n    as: last_total\n"
	q, err := Deserialize([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	aggregate, ok := q.Columns()[0].Expression.(dal.OrderedAggregateFunc)
	if !ok || len(aggregate.AggregateOrder()) != 2 {
		t.Fatalf("ordered aggregate = %T, want two order keys", q.Columns()[0].Expression)
	}
	output, err := Serialize(q)
	if err != nil || string(output) != input {
		t.Fatalf("serialized = %q, err = %v; want %q", output, err, input)
	}
}

func TestOrderedAggregateDocumentRejectsInvalidKeys(t *testing.T) {
	for name, tc := range map[string]struct{ aggregate, want string }{
		"empty order":    {"{function: last, args: [{field: Total}], orderBy: []}", "orderBy"},
		"nonfield order": {"{function: last, args: [{field: Total}], orderBy: [{value: 1}]}", "orderBy"},
		"extra key":      {"{function: last, args: [{field: Total}], orderBy: [{field: InvoiceDate, typo: true}]}", "typo"},
		"other function": {"{function: sum, args: [{field: Total}], orderBy: [{field: InvoiceDate}]}", "orderBy"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Deserialize([]byte("from: {name: Invoice}\ncolumns: [{aggregate: " + tc.aggregate + "}]\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
