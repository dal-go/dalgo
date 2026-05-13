package ddl

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/stretchr/testify/assert"
)

// schemaModifierStub satisfies SchemaModifier; used by tests in this
// task and later in operations_test.go.
type schemaModifierStub struct {
	*minStubDB
	createCollectionCalls []recordedCall
	dropCollectionCalls   []recordedCall
	alterCollectionCalls  []recordedAlter
}

type recordedCall struct {
	ctx  context.Context
	name string
	cdef *dbschema.CollectionDef
	opts []Option
}

type recordedAlter struct {
	ctx  context.Context
	name string
	ops  []AlterOp
}

func (s *schemaModifierStub) CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error {
	s.createCollectionCalls = append(s.createCollectionCalls, recordedCall{ctx: ctx, name: c.Name, cdef: &c, opts: opts})
	return nil
}

func (s *schemaModifierStub) DropCollection(ctx context.Context, name string, opts ...Option) error {
	s.dropCollectionCalls = append(s.dropCollectionCalls, recordedCall{ctx: ctx, name: name, opts: opts})
	return nil
}

func (s *schemaModifierStub) AlterCollection(ctx context.Context, name string, ops ...AlterOp) error {
	s.alterCollectionCalls = append(s.alterCollectionCalls, recordedAlter{ctx: ctx, name: name, ops: ops})
	return nil
}

func newSchemaModifierStub(name string) *schemaModifierStub {
	return &schemaModifierStub{minStubDB: newMinStubDB(name)}
}

func TestSchemaModifier_InterfaceExists(t *testing.T) {
	// Per REQ:schema-modifier-interface AC-1 + AC-2.
	var _ SchemaModifier = (*schemaModifierStub)(nil)
}

func TestSchemaModifier_NotEmbeddedInDB(t *testing.T) {
	// Per REQ:opt-in-not-embedded AC-1 + AC-2.
	var db dal.DB = newMinStubDB("x")
	_, ok := db.(SchemaModifier)
	assert.False(t, ok)
}
