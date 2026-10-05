package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// Serialize returns on a query whose condition holds itself. The validation of the aggregates of a
// query ends on it as the inspection of the query tree does: a group held by pointer ends where it
// meets itself and is refused as a condition DTQL does not cover, and a group held by value in
// where is followed to the bound of the walk and refused as nested too deeply to be checked.
func TestSerializeEndsOnAConditionGroupThatHoldsItself(t *testing.T) {
	byPointer := make([]dal.Condition, 2)
	pointerGroup := dal.NewGroupCondition(dal.Or, byPointer...)
	byPointer[0], byPointer[1] = &pointerGroup, dal.WhereField("a", dal.Equal, 1)

	byValue := make([]dal.Condition, 2)
	valueGroup := dal.NewGroupCondition(dal.Or, byValue...)
	byValue[0], byValue[1] = valueGroup, dal.WhereField("a", dal.Equal, 1)

	for name, tc := range map[string]struct {
		q    dal.StructuredQuery
		want string
	}{
		"held by pointer, in where":  {fakeQuery{from: rootFrom(), where: pointerGroup}, "unsupported condition"},
		"held by value, in where":    {fakeQuery{from: rootFrom(), where: valueGroup}, "too deep to be checked"},
		"held by pointer, in having": {fakeQuery{from: rootFrom(), having: pointerGroup, columns: []dal.Column{{Alias: "n", Expression: dal.Count().Expression}}}, "unsupported condition"},
	} {
		t.Run(name, func(t *testing.T) {
			if data, err := Serialize(tc.q); err == nil || !strings.Contains(err.Error(), tc.want) || data != nil {
				t.Fatalf("data = %q, err = %v, want an error that says %q", data, err, tc.want)
			}
		})
	}
}
