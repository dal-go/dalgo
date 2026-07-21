package dal_test

import (
	"go/build"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDalImportsRecord guards the intended dependency direction: DAL operates
// on record envelopes supplied by the standalone record module.
func TestDalImportsRecord(t *testing.T) {
	const recordPkg = "github.com/dal-go/record"

	pkg, err := build.ImportDir(".", 0)
	require.NoError(t, err)

	assert.Contains(t, pkg.Imports, recordPkg,
		"package dal must import the record package")
}
