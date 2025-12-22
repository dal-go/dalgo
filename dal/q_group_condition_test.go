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
		}, expectedString: "('a'=='b')"},
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
