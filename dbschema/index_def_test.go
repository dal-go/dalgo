package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestIndexDef_Compiles(t *testing.T) {
	// Per REQ:index-def-struct AC-1.
	idx := IndexDef{
		Name:       "ix_users_email",
		Collection: "users",
		Fields:     []dal.FieldName{"email"},
		Unique:     true,
	}
	assert.Equal(t, "ix_users_email", idx.Name)
	assert.Equal(t, "users", idx.Collection)
	assert.Len(t, idx.Fields, 1)
	assert.True(t, idx.Unique)
}

func TestIndexDef_CompositeIndex(t *testing.T) {
	// Per REQ:index-def-struct AC-2.
	idx := IndexDef{
		Name:       "ix_orders_status_created",
		Collection: "orders",
		Fields:     []dal.FieldName{"status", "created_at"},
		Unique:     false,
	}
	assert.Len(t, idx.Fields, 2)
	assert.Equal(t, dal.FieldName("status"), idx.Fields[0])
	assert.Equal(t, dal.FieldName("created_at"), idx.Fields[1])
}

func TestIndexDef_ZeroValue(t *testing.T) {
	// Per REQ:index-def-struct AC-3.
	var idx IndexDef
	assert.Empty(t, idx.Name)
	assert.Empty(t, idx.Collection)
	assert.Nil(t, idx.Fields)
	assert.False(t, idx.Unique)
}
