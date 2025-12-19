package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuery_String_from_types(t *testing.T) {
	// from as CollectionRef value
	q1 := structuredQuery{from: From(CollectionRef{name: "Users"})}
	assert.Equal(t, "SELECT * FROM [Users]", q1.String())

	// from as *CollectionGroupRef pointer
	grp := &CollectionGroupRef{name: "UserGroup"}
	q2 := structuredQuery{from: From(grp)}
	assert.Equal(t, "SELECT * FROM [UserGroup]", q2.String())

	// from as CollectionGroupRef value
	q3 := structuredQuery{from: From(CollectionGroupRef{name: "OrdersGroup"})}
	assert.Equal(t, "SELECT * FROM [OrdersGroup]", q3.String())
}
