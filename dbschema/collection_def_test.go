package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestCollectionDef_Compiles(t *testing.T) {
	// Per REQ:collection-def-struct AC-1.
	c := CollectionDef{
		Name: "users",
		Fields: []FieldDef{
			{Name: "id", Type: Int},
			{Name: "email", Type: String},
		},
		PrimaryKey: []dal.FieldName{"id"},
	}
	assert.Equal(t, "users", c.Name)
	assert.Len(t, c.Fields, 2)
	assert.Len(t, c.PrimaryKey, 1)
}

func TestCollectionDef_CompositePK(t *testing.T) {
	// Per REQ:collection-def-struct AC-2.
	c := CollectionDef{
		Name:       "tenant_users",
		Fields:     []FieldDef{{Name: "tenant_id", Type: Int}, {Name: "user_id", Type: Int}},
		PrimaryKey: []dal.FieldName{"tenant_id", "user_id"},
	}
	assert.Len(t, c.PrimaryKey, 2)
	assert.Equal(t, dal.FieldName("tenant_id"), c.PrimaryKey[0])
	assert.Equal(t, dal.FieldName("user_id"), c.PrimaryKey[1])
}

func TestCollectionDef_WithIndexes(t *testing.T) {
	// Per REQ:collection-def-struct AC-3.
	c := CollectionDef{
		Name:   "users",
		Fields: []FieldDef{{Name: "id", Type: Int}, {Name: "email", Type: String}},
		Indexes: []IndexDef{
			{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true},
		},
	}
	assert.Len(t, c.Fields, 2)
	assert.Len(t, c.Indexes, 1)
}

func TestCollectionDef_ZeroValue(t *testing.T) {
	// Per REQ:collection-def-struct AC-4.
	var c CollectionDef
	assert.Empty(t, c.Name)
	assert.Nil(t, c.Fields)
	assert.Nil(t, c.PrimaryKey)
	assert.Nil(t, c.Indexes)
}
