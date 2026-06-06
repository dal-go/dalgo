package dal

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC group-by-recorded: GroupBy records its expressions; a query without a
// GroupBy call reports an empty GroupBy().
func TestQueryBuilder_GroupBy(t *testing.T) {
	grouped := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		SelectColumns(Column{Expression: Field("category")})
	require.Len(t, grouped.GroupBy(), 1)
	assert.Equal(t, "category", grouped.GroupBy()[0].String())

	ungrouped := From(NewRootCollectionRef("sales", "")).NewQuery().
		SelectColumns(Column{Expression: Field("category")})
	assert.Empty(t, ungrouped.GroupBy())
}

// AC having-recorded-and-rendered: Having records its condition and String()
// renders a HAVING clause positioned after the GROUP BY clause.
func TestQueryBuilder_Having(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		Having(NewComparison(Count().Expression, GreaterThen, NewConstant(2))).
		SelectColumns(Column{Expression: Field("category")})

	require.NotNil(t, q.Having())

	s := q.String()
	groupAt := strings.Index(s, "GROUP BY")
	havingAt := strings.Index(s, "HAVING")
	require.NotEqual(t, -1, groupAt, "expected GROUP BY in %q", s)
	require.NotEqual(t, -1, havingAt, "expected HAVING in %q", s)
	assert.Greater(t, havingAt, groupAt, "HAVING must come after GROUP BY in %q", s)
	assert.Contains(t, s, "HAVING COUNT(*) > 2")
}

// AC having-recorded-and-rendered (multi-condition): multiple Having conditions
// AND-combine, mirroring Where.
func TestQueryBuilder_Having_MultipleConditions(t *testing.T) {
	q := From(NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(Field("category")).
		Having(NewComparison(Count().Expression, GreaterThen, NewConstant(2))).
		Having(NewComparison(Field("total"), LessThen, NewConstant(100))).
		SelectColumns(Column{Expression: Field("category")})

	gc, ok := q.Having().(GroupCondition)
	require.True(t, ok, "expected GroupCondition, got %T", q.Having())
	assert.Equal(t, Operator(And), gc.Operator())
	require.Len(t, gc.Conditions(), 2)
}

// AC count-star-expressible: Count() is COUNT(*), renders "COUNT(*)", and is
// distinct from CountAs(field, alias).
func TestCount_Star(t *testing.T) {
	c := Count()
	assert.Empty(t, c.Alias)
	assert.Equal(t, "COUNT(*)", c.String())

	af, ok := c.Expression.(AggregateFunc)
	require.True(t, ok, "Count() expression must be an AggregateFunc, got %T", c.Expression)
	assert.Equal(t, COUNT, af.FuncName())
	require.Len(t, af.FuncArgs(), 1)
	_, isField := af.FuncArgs()[0].(FieldRef)
	assert.False(t, isField, "COUNT(*) argument must not be a field reference")

	field := CountAs(Field("amount"), "n")
	assert.Equal(t, "COUNT(amount) AS n", field.String())
	assert.NotEqual(t, c.String(), field.String())
}
