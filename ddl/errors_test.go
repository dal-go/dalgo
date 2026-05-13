package ddl

import (
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestPartialSuccessError_Fields(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-1.
	e := &PartialSuccessError{
		Op:           "AlterCollection",
		Collection:   "users",
		Backend:      "dalgo2sql/sqlite",
		Applied:      []AlterOp{nil, nil},
		FirstFailed:  nil,
		NotAttempted: []AlterOp{nil},
		Cause:        errors.New("inner failure"),
	}
	assert.Equal(t, "AlterCollection", e.Op)
	assert.Equal(t, "users", e.Collection)
	assert.Equal(t, "dalgo2sql/sqlite", e.Backend)
	assert.Len(t, e.Applied, 2)
	assert.Len(t, e.NotAttempted, 1)
	assert.NotNil(t, e.Cause)
}

func TestPartialSuccessError_ErrorString(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-2.
	e := &PartialSuccessError{
		Op:           "AlterCollection",
		Collection:   "users",
		Backend:      "dalgo2sql/sqlite",
		Applied:      []AlterOp{nil, nil},
		FirstFailed:  nil,
		NotAttempted: []AlterOp{nil},
		Cause:        errors.New("inner failure"),
	}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "AlterCollection"))
	assert.True(t, strings.Contains(s, "users"))
}

func TestPartialSuccessError_Unwrap(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-3.
	cause := errors.New("inner")
	e := &PartialSuccessError{Cause: cause}
	assert.Same(t, cause, errors.Unwrap(e))
}

func TestPartialSuccessError_ErrorsIs_ViaCause(t *testing.T) {
	// Per REQ:partial-success-error-struct AC-4.
	cause := dal.ErrNotSupported
	e := &PartialSuccessError{Cause: cause}
	assert.True(t, errors.Is(e, dal.ErrNotSupported))
}
