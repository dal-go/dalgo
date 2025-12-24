package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithOffset(t *testing.T) {
	ro := new(ReaderOptions)
	WithOffset(3)(ro)
	assert.Equal(t, 3, ro.offset)
	ro.offset = 0
	assert.Equal(t, ReaderOptions{}, *ro)
}

func TestWithLimit(t *testing.T) {
	ro := new(ReaderOptions)
	WithLimit(4)(ro)
	assert.Equal(t, 4, ro.limit)
	ro.limit = 0
	assert.Equal(t, ReaderOptions{}, *ro)
}

func TestNewReaderOptions(t *testing.T) {
	ro := newReaderOptions(
		WithLimit(10),
		WithOffset(5),
	)
	assert.Equal(t, 10, ro.Limit())
	assert.Equal(t, 5, ro.Offset())
}
