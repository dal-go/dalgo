package mock_ddl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

func TestNewMockTransactionalDDL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockTransactionalDDL(ctrl)
	assert.NotNil(t, m)
	assert.NotNil(t, m.EXPECT())
}

func TestMockTransactionalDDL_SupportsTransactionalDDL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockTransactionalDDL(ctrl)
	m.EXPECT().SupportsTransactionalDDL().Return(true)
	assert.True(t, m.SupportsTransactionalDDL())
}

func TestNewMockSchemaModifier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockSchemaModifier(ctrl)
	assert.NotNil(t, m)
	assert.NotNil(t, m.EXPECT())
}

func TestMockSchemaModifier_CreateCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockSchemaModifier(ctrl)
	c := dbschema.CollectionDef{Name: "users"}
	ctx := context.Background()

	t.Run("no opts", func(t *testing.T) {
		m.EXPECT().CreateCollection(ctx, c).Return(nil)
		assert.NoError(t, m.CreateCollection(ctx, c))
	})

	t.Run("with opts", func(t *testing.T) {
		m.EXPECT().CreateCollection(ctx, c, gomock.Any()).Return(nil)
		assert.NoError(t, m.CreateCollection(ctx, c, ddl.IfNotExists()))
	})

	t.Run("error", func(t *testing.T) {
		want := errors.New("boom")
		m.EXPECT().CreateCollection(ctx, c).Return(want)
		assert.ErrorIs(t, m.CreateCollection(ctx, c), want)
	})
}

func TestMockSchemaModifier_DropCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockSchemaModifier(ctrl)
	ctx := context.Background()

	t.Run("no opts", func(t *testing.T) {
		m.EXPECT().DropCollection(ctx, "users").Return(nil)
		assert.NoError(t, m.DropCollection(ctx, "users"))
	})

	t.Run("with opts", func(t *testing.T) {
		m.EXPECT().DropCollection(ctx, "users", gomock.Any()).Return(nil)
		assert.NoError(t, m.DropCollection(ctx, "users", ddl.IfExists()))
	})
}

func TestMockSchemaModifier_AlterCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockSchemaModifier(ctrl)
	ctx := context.Background()

	t.Run("no ops", func(t *testing.T) {
		m.EXPECT().AlterCollection(ctx, "users").Return(nil)
		assert.NoError(t, m.AlterCollection(ctx, "users"))
	})

	t.Run("with ops", func(t *testing.T) {
		op := ddl.AddField(dbschema.FieldDef{Name: "email", Type: dbschema.String})
		m.EXPECT().AlterCollection(ctx, "users", gomock.Any()).Return(nil)
		assert.NoError(t, m.AlterCollection(ctx, "users", op))
	})
}

func TestNewMockApplier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockApplier(ctrl)
	assert.NotNil(t, m)
	assert.NotNil(t, m.EXPECT())
}

func TestMockApplier_AllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockApplier(ctrl)
	ctx := context.Background()
	opts := ddl.Options{}
	f := dbschema.FieldDef{Name: "email", Type: dbschema.String}
	idx := dbschema.IndexDef{Name: "ix"}
	var fn dal.FieldName = "email"

	m.EXPECT().ApplyAddField(ctx, f, opts).Return(nil)
	assert.NoError(t, m.ApplyAddField(ctx, f, opts))

	m.EXPECT().ApplyDropField(ctx, fn, opts).Return(nil)
	assert.NoError(t, m.ApplyDropField(ctx, fn, opts))

	m.EXPECT().ApplyModifyField(ctx, fn, f, opts).Return(nil)
	assert.NoError(t, m.ApplyModifyField(ctx, fn, f, opts))

	m.EXPECT().ApplyRenameField(ctx, dal.FieldName("old"), dal.FieldName("new"), opts).Return(nil)
	assert.NoError(t, m.ApplyRenameField(ctx, "old", "new", opts))

	m.EXPECT().ApplyAddIndex(ctx, idx, opts).Return(nil)
	assert.NoError(t, m.ApplyAddIndex(ctx, idx, opts))

	m.EXPECT().ApplyDropIndex(ctx, "ix", opts).Return(nil)
	assert.NoError(t, m.ApplyDropIndex(ctx, "ix", opts))
}
