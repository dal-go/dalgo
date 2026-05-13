package dbschema

import (
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestNotSupportedError_Fields(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-1.
	e := NotSupportedError{Op: "CreateCollection", Backend: "dalgo2sql/sqlite", Reason: "read-only"}
	assert.Equal(t, "CreateCollection", e.Op)
	assert.Equal(t, "dalgo2sql/sqlite", e.Backend)
	assert.Equal(t, "read-only", e.Reason)
}

func TestNotSupportedError_ErrorString(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-2.
	e := &NotSupportedError{Op: "CreateCollection", Backend: "dalgo2sql/sqlite", Reason: "read-only mode"}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "CreateCollection"), "missing Op in: %q", s)
	assert.True(t, strings.Contains(s, "dalgo2sql/sqlite"), "missing Backend in: %q", s)
	assert.True(t, strings.Contains(s, "read-only mode"), "missing Reason in: %q", s)
}

func TestNotSupportedError_ErrorStringWithEmptyFields(t *testing.T) {
	// Per REQ:not-supported-error-struct AC-3 — non-empty, no panic, no verbatim empties.
	e := &NotSupportedError{Op: "DescribeCollection"}
	s := e.Error()
	assert.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "DescribeCollection"), "missing Op in: %q", s)
}

func TestNotSupportedError_ErrorsIs(t *testing.T) {
	// Per REQ:unwrap-to-sentinel AC-1.
	err := &NotSupportedError{Op: "CreateCollection"}
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
}

func TestNotSupportedError_ErrorsAs(t *testing.T) {
	// Per REQ:unwrap-to-sentinel AC-2.
	var err error = &NotSupportedError{Op: "DropIndex", Backend: "x"}
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "DropIndex", ue.Op)
	assert.Equal(t, "x", ue.Backend)
}
