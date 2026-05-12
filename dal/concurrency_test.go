package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubConcurrency is a minimal type used only in tests to verify the
// ConcurrencyAware interface contract.
type stubConcurrency struct {
	value bool
}

func (s stubConcurrency) SupportsConcurrentConnections() bool {
	return s.value
}

// Compile-time assertion: stubConcurrency satisfies ConcurrencyAware.
var _ ConcurrencyAware = stubConcurrency{}

func TestConcurrencyAware_Interface(t *testing.T) {
	var c ConcurrencyAware = stubConcurrency{value: true}
	assert.True(t, c.SupportsConcurrentConnections())

	c = stubConcurrency{value: false}
	assert.False(t, c.SupportsConcurrentConnections())
}

func TestConcurrencyAware_MethodIsStable(t *testing.T) {
	// Per REQ:concurrency-aware-interface AC-2, repeated calls on the same
	// value return the same bool.
	c := stubConcurrency{value: true}
	first := c.SupportsConcurrentConnections()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, c.SupportsConcurrentConnections(), "call %d", i)
	}
}
