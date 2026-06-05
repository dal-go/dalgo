package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryBuilder_SelectColumns(t *testing.T) {
	cols := []Column{
		{Expression: NewFieldRef("", "id")},
		{Alias: "s", Expression: NewFieldRef("", "status")},
	}
	q := NewQueryBuilder(nil).SelectColumns(cols...)
	assert.Equal(t, cols, q.Columns())

	// A query finalized by any other terminal has an empty Columns().
	q2 := NewQueryBuilder(nil).SelectIntoRecord(nil)
	assert.Empty(t, q2.Columns())
}
