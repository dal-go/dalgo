package dal

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dal-go/record"
)

// ErrNotSupported - return this if db name does not support requested operation.
// (for example no support for transactions)
var ErrNotSupported = errors.New("not supported")

// ErrNotImplementedYet - return this if db name does not support requested operation yet.
var ErrNotImplementedYet = errors.New("not implemented yet")

// ErrNoMoreRecords indicates there is no more records
var ErrNoMoreRecords = fmt.Errorf("%w: no more errors", io.EOF)

var ErrLimitReached = fmt.Errorf("%w: limit reached", ErrNoMoreRecords)

// ErrDuplicateUser indicates there is a duplicate user // TODO: move to strongo/app?
type ErrDuplicateUser struct {
	// TODO: Should it be moved out of this package to strongo/app/user?
	SearchCriteria   string
	DuplicateUserIDs []string
}

// Error implements error interface
func (err ErrDuplicateUser) Error() string {
	return fmt.Sprintf("multiple users by given search criteria[%v]: %v", err.SearchCriteria, strings.Join(err.DuplicateUserIDs, ","))
}

// ErrNotFoundByKey indicates error was not found by value
type ErrNotFoundByKey interface {
	Key() *record.Key
	Cause() error
	error
}

var _ ErrNotFoundByKey = (*errNotFoundByKey)(nil)

type errNotFoundByKey struct {
	key   *record.Key
	cause error
}

func (e errNotFoundByKey) Key() *record.Key {
	return e.key
}

func (e errNotFoundByKey) Cause() error {
	if e.cause == nil {
		return record.ErrRecordNotFound
	}
	return e.cause
}

func (e errNotFoundByKey) Unwrap() error {
	return e.Cause()
}

func (e errNotFoundByKey) Error() string {
	if errors.Is(e.cause, record.ErrRecordNotFound) {
		return fmt.Sprintf("%v: by key=%v", e.cause, e.key)
	}
	return fmt.Sprintf("%v: not found by key=%v", e.Cause(), e.key)
}

// NewErrNotFoundByKey creates an error that indicates that entity was not found by value
func NewErrNotFoundByKey(key *record.Key, cause error) error {
	return errNotFoundByKey{key: key, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause.(any) == record.ErrRecordNotFound.(any) {
		return record.ErrRecordNotFound
	}
	return fmt.Errorf("%w: %v", record.ErrRecordNotFound, cause)
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
