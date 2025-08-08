package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQuery_String_from_types(t *testing.T) {
	// from as CollectionRef value
	q1 := theQuery{from: CollectionRef{name: "Users"}}
	assert.Equal(t, "SELECT * FROM [Users]", q1.String())

	// from as *CollectionGroupRef pointer
	grp := &CollectionGroupRef{name: "UserGroup"}
	q2 := theQuery{from: grp}
	assert.Equal(t, "SELECT * FROM [UserGroup]", q2.String())

	// from as CollectionGroupRef value
	q3 := theQuery{from: CollectionGroupRef{name: "OrdersGroup"}}
	assert.Equal(t, "SELECT * FROM [OrdersGroup]", q3.String())
}
