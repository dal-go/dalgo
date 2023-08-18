package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSumAs(t *testing.T) {
	expression := Field("id")
	alias := "c1"
	sumAs := SumAs(expression, alias)
	assert.Equal(t, alias, sumAs.Alias)
	assert.Equal(t, "SUM(id) AS c1", sumAs.String())
}

func TestCountAs(t *testing.T) {
	expression := Field("id")
	alias := "c1"
	countAs := CountAs(expression, alias)
	assert.Equal(t, alias, countAs.Alias)
	assert.Equal(t, "COUNT(id) AS c1", countAs.String())
}

func TestMinAs(t *testing.T) {
	expression := Field("id")
	alias := "c1"
	minAs := MinAs(expression, alias)
	assert.Equal(t, alias, minAs.Alias)
	assert.Equal(t, "MIN(id) AS c1", minAs.String())
}

func TestMaxAs(t *testing.T) {
	expression := Field("id")
	alias := "c1"
	maxAs := MaxAs(expression, alias)
	assert.Equal(t, alias, maxAs.Alias)
	assert.Equal(t, "MAX(id) AS c1", maxAs.String())
}

func TestAverageAs(t *testing.T) {
	expression := Field("id")
	alias := "c1"
	averageAs := AverageAs(expression, alias)
	assert.Equal(t, alias, averageAs.Alias)
	assert.Equal(t, "AVG(id) AS c1", averageAs.String())
}
