package dal_test

import (
	"go/build"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDalDoesNotImportRecord guards the additivity invariant of the typed
// Collection[T] layer: package dal must remain free of any import of the
// record package, so no dal -> record import cycle is introduced. The record
// package imports dal, so a reverse edge would also break the build outright;
// this test pins the invariant explicitly.
func TestDalDoesNotImportRecord(t *testing.T) {
	const recordPkg = "github.com/dal-go/dalgo/record"

	pkg, err := build.ImportDir(".", 0)
	require.NoError(t, err)

	assert.NotContains(t, pkg.Imports, recordPkg,
		"package dal must not import the record package")
}
