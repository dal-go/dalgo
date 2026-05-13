package ddl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOption_TypesCompile(t *testing.T) {
	// Per REQ:option-type AC-1.
	var fn Option
	var o Options
	_ = fn
	_ = o
}

func TestIfNotExists_SetsFlag(t *testing.T) {
	// Per REQ:option-constructors AC-1.
	var o Options
	IfNotExists()(&o)
	assert.True(t, o.IfNotExists)
	assert.False(t, o.IfExists)
}

func TestIfExists_SetsFlag(t *testing.T) {
	// Per REQ:option-constructors AC-2.
	var o Options
	IfExists()(&o)
	assert.True(t, o.IfExists)
	assert.False(t, o.IfNotExists)
}

func TestOptions_Independent(t *testing.T) {
	// Per REQ:option-constructors AC-3.
	var o Options
	IfNotExists()(&o)
	IfExists()(&o)
	assert.True(t, o.IfNotExists)
	assert.True(t, o.IfExists)
}

func TestResolveOptions_Helper(t *testing.T) {
	got := ResolveOptions(IfNotExists(), IfExists())
	assert.True(t, got.IfNotExists)
	assert.True(t, got.IfExists)
}
