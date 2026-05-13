package ddl

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

func TestHelpers_Compile(t *testing.T) {
	// Per REQ:helper-signatures AC-1.
	_ = CreateCollection
	_ = DropCollection
	_ = AlterCollection
}

func TestCreateCollection_Dispatches(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-1.
	stub := newSchemaModifierStub("stub-driver")
	ctx := context.Background()
	c := dbschema.CollectionDef{Name: "users"}
	err := CreateCollection(ctx, stub, c, IfNotExists())
	assert.NoError(t, err)
	assert.Len(t, stub.createCollectionCalls, 1)
	call := stub.createCollectionCalls[0]
	assert.Equal(t, "users", call.name)
	assert.Len(t, call.opts, 1)
}

func TestDropCollection_Dispatches(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-2.
	stub := newSchemaModifierStub("stub-driver")
	err := DropCollection(context.Background(), stub, "users", IfExists())
	assert.NoError(t, err)
	assert.Len(t, stub.dropCollectionCalls, 1)
	assert.Equal(t, "users", stub.dropCollectionCalls[0].name)
}

func TestAlterCollection_DispatchesMixedOps(t *testing.T) {
	// Per REQ:dispatch-on-implementer AC-3.
	stub := newSchemaModifierStub("stub-driver")
	f := dbschema.FieldDef{Name: "email", Type: dbschema.String}
	idx := dbschema.IndexDef{Name: "ix", Collection: "users", Fields: []dal.FieldName{"email"}}
	err := AlterCollection(context.Background(), stub, "users",
		AddField(f),
		AddIndex(idx),
		DropField("legacy"),
	)
	assert.NoError(t, err)
	assert.Len(t, stub.alterCollectionCalls, 1)
	call := stub.alterCollectionCalls[0]
	assert.Equal(t, "users", call.name)
	assert.Len(t, call.ops, 3)
	// Verify order preserved
	_, isAdd := call.ops[0].(addFieldOp)
	_, isAddIdx := call.ops[1].(addIndexOp)
	_, isDrop := call.ops[2].(dropFieldOp)
	assert.True(t, isAdd, "ops[0] should be addFieldOp")
	assert.True(t, isAddIdx, "ops[1] should be addIndexOp")
	assert.True(t, isDrop, "ops[2] should be dropFieldOp")
}

func TestCreateCollection_NotImplementer(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-1.
	db := newMinStubDB("stub-driver")
	err := CreateCollection(context.Background(), db, dbschema.CollectionDef{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "CreateCollection", ue.Op)
}

func TestAlterCollection_BackendFromAdapter(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-2.
	db := newMinStubDB("stub-driver")
	err := AlterCollection(context.Background(), db, "users", AddField(dbschema.FieldDef{Name: "x", Type: dbschema.Int}))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "stub-driver", ue.Backend)
	assert.Equal(t, "AlterCollection", ue.Op)
}

func TestDropCollection_BackendEmptyWhenAdapterNil(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-3.
	db := newMinStubDBNilAdapter()
	err := DropCollection(context.Background(), db, "x")
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "", ue.Backend)
	assert.NotEmpty(t, ue.Error())
}

func TestAlterCollection_NotImplementer(t *testing.T) {
	// Per REQ:not-supported-on-non-implementer AC-4.
	db := newMinStubDB("stub-driver")
	err := AlterCollection(context.Background(), db, "users",
		AddField(dbschema.FieldDef{Name: "x", Type: dbschema.Int}),
	)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *dbschema.NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "AlterCollection", ue.Op)
}
