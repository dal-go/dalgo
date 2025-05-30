package dal

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotSupported - return this if db name does not support requested operation.
// (for example no support for transactions)
var ErrNotSupported = errors.New("not supported")

// ErrNotImplementedYet - return this if db name does not support requested operation yet.
var ErrNotImplementedYet = errors.New("not implemented yet")

// ErrNoMoreRecords indicates there is no more records
var ErrNoMoreRecords = errors.New("no more errors")

// ErrDuplicateUser indicates there is a duplicate user // TODO: move to strongo/app?
type ErrDuplicateUser struct {
	// TODO: Should it be moved out of this package to strongo/app/user?
	SearchCriteria   string
	DuplicateUserIDs []string
}

var ErrNoError = errors.New("no error")

// Error implements error interface
func (err ErrDuplicateUser) Error() string {
	return fmt.Sprintf("multiple users by given search criteria[%v]: %v", err.SearchCriteria, strings.Join(err.DuplicateUserIDs, ","))
}

var (
	// ErrRecordNotFound is returned when a DB record is not found
	ErrRecordNotFound = errors.New("record not found")
)

// IsNotFound check if underlying error is ErrRecordNotFound
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRecordNotFound) {
		return true
	}
	return false
}

// ErrNotFoundByKey indicates error was not found by value
type ErrNotFoundByKey interface {
	Key() *Key
	Cause() error
	error
}

var _ ErrNotFoundByKey = (*errNotFoundByKey)(nil)

type errNotFoundByKey struct {
	key   *Key
	cause error
}

func (e errNotFoundByKey) Key() *Key {
	return e.key
}

func (e errNotFoundByKey) Cause() error {
	if e.cause == nil {
		return ErrRecordNotFound
	}
	return e.cause
}

func (e errNotFoundByKey) Unwrap() error {
	return e.Cause()
}

func (e errNotFoundByKey) Error() string {
	if errors.Is(e.cause, ErrRecordNotFound) {
		return fmt.Sprintf("%v: by key=%v", e.cause, e.key)
	}
	return fmt.Sprintf("%v: not found by key=%v", e.Cause(), e.key)
}

// NewErrNotFoundByKey creates an error that indicates that entity was not found by value
func NewErrNotFoundByKey(key *Key, cause error) error {
	return errNotFoundByKey{key: key, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause == ErrRecordNotFound {
		return ErrRecordNotFound
	}
	return fmt.Errorf("%w: %v", ErrRecordNotFound, cause)
}

type rollbackError struct {
	originalErr error
	rollbackErr error
}

func (v rollbackError) Error() string {
	if v.originalErr == nil {
		return fmt.Sprintf("rollback failed: %v", v.rollbackErr)
	}
	return fmt.Sprintf("rollback failed: %v: original error: %v", v.rollbackErr, v.originalErr)
}

func (v rollbackError) OriginalError() error {
	return v.originalErr
}

func (v rollbackError) RollbackError() error {
	return v.rollbackErr
}

// NewRollbackError creates a rollback error
func NewRollbackError(rollbackErr, originalErr error) error {
	return &rollbackError{originalErr: originalErr, rollbackErr: rollbackErr}
}

// ErrHookFailed indicates that error occurred during hook execution
var ErrHookFailed = errors.New("failed in dalgo hook")
