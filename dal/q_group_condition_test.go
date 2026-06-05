package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupCondition(t *testing.T) {
	for _, tt := range []struct {
		name           string
		gc             GroupCondition
		expectedString string
	}{
		{name: "empty", gc: GroupCondition{}, expectedString: "()"},
		{name: "2items", gc: GroupCondition{
			operator: Equal,
			conditions: []Condition{
				String("a"),
				String("b"),
			},
		}, expectedString: "('a' == 'b')"},
		{name: "AND-2items", gc: GroupCondition{
			operator: And,
			conditions: []Condition{
				String("a"),
				String("b"),
			},
		}, expectedString: "('a' AND 'b')"},
		{name: "OR-3items", gc: GroupCondition{
			operator: Or,
			conditions: []Condition{
				String("a"),
				String("b"),
				String("c"),
			},
		}, expectedString: "('a' OR 'b' OR 'c')"},
		{name: "AND-1item", gc: GroupCondition{
			operator: And,
			conditions: []Condition{
				String("a"),
			},
		}, expectedString: "('a')"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("String", func(t *testing.T) {
				assert.Equal(t, tt.expectedString, tt.gc.String())
			})
			t.Run("Conditions", func(t *testing.T) {
				assert.Equal(t, tt.gc.conditions, tt.gc.Conditions())
			})
			t.Run("Operator", func(t *testing.T) {
				assert.Equal(t, tt.gc.operator, tt.gc.Operator())
			})
		})
	}
}

func TestNewGroupCondition(t *testing.T) {
	a, b := String("a"), String("b")
	gc := NewGroupCondition(And, a, b)
	assert.Equal(t, Operator(And), gc.Operator())
	assert.Equal(t, []Condition{a, b}, gc.Conditions())
	assert.Equal(t, "('a' AND 'b')", gc.String())
}
