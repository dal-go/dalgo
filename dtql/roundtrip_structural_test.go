package dtql

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestStructuralRoundTrip(t *testing.T) {
	cases := map[string]dal.StructuredQuery{
		"from only": fakeQuery{from: rootFrom()},
		"from with alias": fakeQuery{
			from: dal.From(dal.NewRootCollectionRef("users", "u")),
		},
		"single int comparison": fakeQuery{
			from:  rootFrom(),
			where: dal.WhereField("age", dal.GreaterOrEqual, 18),
		},
		"string equality": fakeQuery{
			from:  rootFrom(),
			where: dal.WhereField("country", dal.Equal, "US"),
		},
		"bool and float": fakeQuery{
			from: rootFrom(),
			where: dal.NewGroupCondition(dal.And,
				dal.WhereField("active", dal.Equal, true),
				dal.WhereField("score", dal.LessThen, 4.5),
			),
		},
		"in string array": fakeQuery{
			from:  rootFrom(),
			where: dal.WhereField("status", dal.In, []string{"active", "pending"}),
		},
		"in int array": fakeQuery{
			from:  rootFrom(),
			where: dal.WhereField("level", dal.In, []int{1, 2, 3}),
		},
		"columns with alias": fakeQuery{
			from: rootFrom(),
			columns: []dal.Column{
				{Expression: dal.Field("name")},
				{Expression: dal.Field("age"), Alias: "years"},
			},
		},
		"order limit offset": fakeQuery{
			from: rootFrom(),
			orderBy: []dal.OrderExpression{
				dal.AscendingField("name"),
				dal.DescendingField("created"),
			},
			limit:  25,
			offset: 50,
		},
		"all operators": fakeQuery{
			from: rootFrom(),
			where: dal.NewGroupCondition(dal.Or,
				dal.WhereField("a", dal.GreaterThen, 1),
				dal.WhereField("b", dal.GreaterOrEqual, 2),
				dal.WhereField("c", dal.LessThen, 3),
				dal.WhereField("d", dal.LessOrEqual, 4),
			),
		},
		"nested groups": fullQuery(),
	}

	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			got, err := Deserialize(data)
			if err != nil {
				t.Fatalf("Deserialize: %v\nYAML:\n%s", err, data)
			}
			if !Equal(q, got) {
				t.Fatalf("deserialize(serialize(q)) not structurally equal to q.\nYAML:\n%s", data)
			}
		})
	}
}

// TestEqual_detectsDifferences guards the comparator against false positives.
func TestEqual_detectsDifferences(t *testing.T) {
	base := fakeQuery{from: rootFrom(), where: dal.WhereField("age", dal.Equal, 18)}
	diffs := []dal.StructuredQuery{
		fakeQuery{from: rootFrom(), where: dal.WhereField("age", dal.Equal, 19)},           // value
		fakeQuery{from: rootFrom(), where: dal.WhereField("years", dal.Equal, 18)},         // field
		fakeQuery{from: rootFrom(), where: dal.WhereField("age", dal.GreaterThen, 18)},     // operator
		fakeQuery{from: rootFrom(), limit: 1, where: dal.WhereField("age", dal.Equal, 18)}, // limit
		fakeQuery{from: dal.From(dal.NewRootCollectionRef("orders", ""))},                  // from
	}
	for i, d := range diffs {
		if Equal(base, d) {
			t.Errorf("case #%d: expected Equal to report a difference", i)
		}
	}
}
