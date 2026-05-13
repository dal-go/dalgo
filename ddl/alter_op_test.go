package ddl

import (
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

func TestAlterOp_InterfaceExists(t *testing.T) {
	// Per REQ:alter-op-interface AC-1.
	var op AlterOp
	assert.Nil(t, op)
}

func TestAlterOp_HasUnexportedMarker(t *testing.T) {
	// Per REQ:alter-op-interface AC-2.
	// Interface now has two methods: the unexported sealing marker and
	// the exported ApplyTo added by REQ:alter-op-interface-extended.
	typ := reflect.TypeOf((*AlterOp)(nil)).Elem()
	assert.Equal(t, reflect.Interface, typ.Kind())
	var unexported []reflect.Method
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.PkgPath != "" {
			unexported = append(unexported, m)
		}
	}
	assert.Len(t, unexported, 1, "expected exactly one unexported (sealing) method on AlterOp")
	assert.NotEmpty(t, unexported[0].PkgPath, "sealing method should be unexported")
}

func TestAddField_Constructs(t *testing.T) {
	// Per REQ:add-field-constructor AC-1.
	op := AddField(dbschema.FieldDef{Name: "email", Type: dbschema.String})
	assert.NotNil(t, op)
}

func TestAddField_PreservesField(t *testing.T) {
	// Per REQ:add-field-constructor AC-2.
	f := dbschema.FieldDef{Name: "email", Type: dbschema.String}
	op := AddField(f, IfNotExists())
	concrete, ok := op.(addFieldOp)
	assert.True(t, ok)
	assert.Equal(t, f, concrete.field)
	assert.True(t, concrete.options.IfNotExists)
}

func TestDropField_Constructs(t *testing.T) {
	// Per REQ:drop-field-constructor AC-1.
	op := DropField("legacy_user_code", IfExists())
	assert.NotNil(t, op)
	concrete, ok := op.(dropFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("legacy_user_code"), concrete.name)
	assert.True(t, concrete.options.IfExists)
}

func TestModifyField_Constructs(t *testing.T) {
	// Per REQ:modify-field-constructor AC-1.
	op := ModifyField("created_at", dbschema.FieldDef{Name: "created_at", Type: dbschema.Time, Nullable: false})
	assert.NotNil(t, op)
	concrete, ok := op.(modifyFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("created_at"), concrete.name)
	assert.Equal(t, dbschema.Time, concrete.newDef.Type)
}

func TestRenameField_Constructs(t *testing.T) {
	// Per REQ:rename-field-constructor AC-1.
	op := RenameField("user_name", "username")
	assert.NotNil(t, op)
	concrete, ok := op.(renameFieldOp)
	assert.True(t, ok)
	assert.Equal(t, dal.FieldName("user_name"), concrete.oldName)
	assert.Equal(t, dal.FieldName("username"), concrete.newName)
}

func TestAddIndex_Constructs(t *testing.T) {
	// Per REQ:add-index-constructor AC-1.
	idx := dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true}
	op := AddIndex(idx, IfNotExists())
	assert.NotNil(t, op)
	concrete, ok := op.(addIndexOp)
	assert.True(t, ok)
	assert.Equal(t, idx, concrete.index)
	assert.True(t, concrete.options.IfNotExists)
}

func TestDropIndex_Constructs(t *testing.T) {
	// Per REQ:drop-index-constructor AC-1.
	op := DropIndex("ix_users_legacy", IfExists())
	assert.NotNil(t, op)
	concrete, ok := op.(dropIndexOp)
	assert.True(t, ok)
	assert.Equal(t, "ix_users_legacy", concrete.name)
	assert.True(t, concrete.options.IfExists)
}
