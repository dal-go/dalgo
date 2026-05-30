package end2end

import (
	"errors"
	"testing"

	"github.com/dal-go/dalgo/mocks/mock_dal"
	"go.uber.org/mock/gomock"
)

// Ensures the else-branch with t.Skip in TestDalgoDB is executed when queries are unsupported.
func TestQueryUnsupportedIsSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// No expectations are needed because we will skip single/multi
	db := mock_dal.NewMockDB(ctrl)

	// Disable single/multi to reach the query branch only
	runSingleAndMulti = false
	t.Cleanup(func() { runSingleAndMulti = true })

	TestDalgoDB(t, db, errors.New("queries not supported"), true)
}
