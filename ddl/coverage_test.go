package ddl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAlterOpMarkers exercises the unexported sealed marker method on
// each AlterOp concrete type so coverage reflects them as touched. The
// methods are intentional no-ops — they exist only to seal the
// interface at the package boundary.
func TestAlterOpMarkers(_ *testing.T) {
	addFieldOp{}.alterOp()
	dropFieldOp{}.alterOp()
	modifyFieldOp{}.alterOp()
	renameFieldOp{}.alterOp()
	addIndexOp{}.alterOp()
	dropIndexOp{}.alterOp()
}

// TestBackendName_NilDB covers the early-return branch when db is nil.
func TestBackendName_NilDB(t *testing.T) {
	assert.Equal(t, "", backendName(nil))
}

// TestBackendName_NilAdapter covers the db.Adapter() == nil branch.
func TestBackendName_NilAdapter(t *testing.T) {
	db := newMinStubDBNilAdapter()
	assert.Equal(t, "", backendName(db))
}
