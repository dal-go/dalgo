package dalgo

import (
	"fmt"
	"github.com/pkg/errors"
)

// ErrNotSupported - return this if db driver does not support requested operation.
// (for example no support for transactions)
//goland:noinspection GoUnusedGlobalVariable
var ErrNotSupported = errors.New("not supported")

// ErrDuplicateUser indicates there is a duplicate user // TODO: move to strongo/app?
type ErrDuplicateUser struct {
	// TODO: Should it be moved out of this package to strongo/app/user?
	SearchCriteria   string
	DuplicateUserIDs []int64
}

// Error implements error interface
func (err ErrDuplicateUser) Error() string {
	return fmt.Sprintf("Multiple users by given search criteria[%v]: %v", err.SearchCriteria, err.DuplicateUserIDs)
}

var (
	// ErrRecordNotFound is returned when a DB record is not found
	ErrRecordNotFound = errors.New("Record not found")
)

// IsNotFound check if underlying error is ErrRecordNotFound
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(ErrNotFoundByKey)
	return ok || errors.Cause(err) == ErrRecordNotFound
}

// ErrNotFoundByKey indicates error was not found by ID
type ErrNotFoundByKey interface {
	Key() RecordKey
	Cause() error
	error
}

type errNotFoundByKey struct {
	key   RecordKey
	cause error
}

func (e errNotFoundByKey) Key() RecordKey {
	return e.key
}

func (e errNotFoundByKey) Cause() error {
	return e.cause
}

func (e errNotFoundByKey) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("record not found by key=%v", GetRecordKeyPath(e.key))
	}
	return fmt.Sprintf("record not found by key=%v: %v", GetRecordKeyPath(e.key), e.cause)
}

// NewErrNotFoundByKey creates an error that indicates that entity was not found by ID
func NewErrNotFoundByKey(key RecordKey, cause error) error {
	return errNotFoundByKey{key: key, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause == ErrRecordNotFound {
		return ErrRecordNotFound
	}
	return errors.WithMessage(ErrRecordNotFound, cause.Error())
}
