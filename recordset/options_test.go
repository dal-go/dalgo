package recordset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithName(t *testing.T) {
	o := &options{}
	WithName("test")(o)
	assert.Equal(t, "test", o.name)
	assert.Equal(t, "test", o.Name())
}

func TestNewOptions(t *testing.T) {
	o := NewOptions(WithName("test"))
	assert.Equal(t, "test", o.Name())
}
