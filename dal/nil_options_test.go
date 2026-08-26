package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTransactionOptions_SkipsNil is the regression test for a panic found
// by a fleet sweep: callers pass a literal nil (or an unset option variable)
// among the variadic options — RunReadwriteTransaction(ctx, f, nil) exists at
// several call sites in the wild — and the constructor invoked it. The
// constructor guards every adapter at once, including dalgo2firestore, which
// reads these options in production.
func TestNewTransactionOptions_SkipsNil(t *testing.T) {
	require.NotPanics(t, func() {
		o := NewTransactionOptions(nil)
		assert.NotNil(t, o)
	})
	// Real options interleaved with nils still apply.
	o := NewTransactionOptions(nil, TxWithAttempts(3), nil)
	assert.Equal(t, 3, o.Attempts())
}

// TestNewInsertOptions_SkipsNil applies the same guard, and reason, to the
// insert-option constructor.
func TestNewInsertOptions_SkipsNil(t *testing.T) {
	require.NotPanics(t, func() {
		_ = NewInsertOptions(nil)
	})
	o := NewInsertOptions(nil, WithRandomStringKey(16, 5), nil)
	assert.NotNil(t, o.IDGenerator())
}
