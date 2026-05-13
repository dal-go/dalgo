package ddl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// transactionalStub implements TransactionalDDL with a configurable answer.
type transactionalStub struct {
	*minStubDB
	supports bool
}

func (s *transactionalStub) SupportsTransactionalDDL() bool {
	return s.supports
}

func TestTransactionalDDL_InterfaceExists(t *testing.T) {
	// Per REQ:transactional-ddl-interface AC-1.
	var _ TransactionalDDL = (*transactionalStub)(nil)
}

func TestTransactionalDDL_StableAnswer(t *testing.T) {
	// Per REQ:transactional-ddl-interface AC-3.
	s := &transactionalStub{supports: true}
	first := s.SupportsTransactionalDDL()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, s.SupportsTransactionalDDL(), "call %d", i)
	}
}

func TestSupportsTransactionalDDL_TrueOnImplementer(t *testing.T) {
	// Per REQ:helper-function AC-1.
	s := &transactionalStub{minStubDB: newMinStubDB("x"), supports: true}
	assert.True(t, SupportsTransactionalDDL(s))
}

func TestSupportsTransactionalDDL_FalseOnNonImplementer(t *testing.T) {
	// Per REQ:helper-function AC-2.
	s := newMinStubDB("x")
	assert.False(t, SupportsTransactionalDDL(s))
}
