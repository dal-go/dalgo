package dal

import (
	"errors"
	"fmt"
)

// ErrNotSupported - return this if db driver does not support requested operation.
// (for example no support for transactions)
//
//goland:noinspection GoUnusedGlobalVariable
var ErrNotSupported = errors.New("not supported")

// ErrNoMoreRecords indicates there is no more records
var ErrNoMoreRecords = errors.New("no more errors")

// ErrDuplicateUser indicates there is a duplicate user // TODO: move to strongo/app?
type ErrDuplicateUser struct {
	// TODO: Should it be moved out of this package to strongo/app/user?
	SearchCriteria   string
	DuplicateUserIDs []int64
}

var NoError = errors.New("no error")

// Error implements error interface
func (err ErrDuplicateUser) Error() string {
	return fmt.Sprintf("Multiple users by given search criteria[%v]: %v", err.SearchCriteria, err.DuplicateUserIDs)
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
	if _, ok := err.(ErrNotFoundByKey); ok {
		return true
	}
	if errors.Is(err, ErrRecordNotFound) {
		return true
	}
	return false
}

// ErrNotFoundByKey indicates error was not found by Value
type ErrNotFoundByKey interface {
	Key() Key
	Cause() error
	error
}

type errNotFoundByKey struct {
	key   *Key
	cause error
}

func (e errNotFoundByKey) Key() *Key {
	return e.key
}

func (e errNotFoundByKey) Cause() error {
	return e.cause
}

func (e errNotFoundByKey) Unwrap() error {
	return e.cause
}

func (e errNotFoundByKey) Error() string {
	if e.cause == ErrRecordNotFound {
		return fmt.Sprintf("%v: by key=%v", e.cause, e.key)
	}
	s := fmt.Sprintf("record not found by key=%v", e.key)
	if e.cause == nil {
		return s
	}
	return fmt.Sprintf("%v: %v", s, e.cause)
}

// NewErrNotFoundByKey creates an error that indicates that entity was not found by Value
func NewErrNotFoundByKey(key *Key, cause error) error {
	return errNotFoundByKey{key: key, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause == ErrRecordNotFound {
		return ErrRecordNotFound
	}
	return fmt.Errorf("%w: %v", ErrRecordNotFound, cause)
}

type errRollbackFailed struct {
	originalError error
	rollbackError error
}

func (v errRollbackFailed) Error() string {
	if v.originalError == nil {
		return fmt.Sprintf("rollback failed: %v", v.rollbackError)
	}
	return fmt.Sprintf("rollback failed: %v: original error: %v", v.rollbackError, v.originalError)
}

func (v errRollbackFailed) OriginalError() error {
	return v.originalError
}

func (v errRollbackFailed) RollbackError() error {
	return v.rollbackError
}

// NewRollbackError creates a rollback error
func NewRollbackError(rollbackError, originalError error) error {
	return errRollbackFailed{originalError: originalError, rollbackError: rollbackError}
}
