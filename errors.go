package db

import (
	"fmt"
	"github.com/pkg/errors"
)

// ErrNotSupported - return this if db driver does not support requested operation.
// (for example no support for transactions)
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
	_, ok := err.(ErrNotFoundByID)
	return ok || errors.Cause(err) == ErrRecordNotFound
}

// ErrNotFoundByID indicates error was not found by ID
type ErrNotFoundByID interface {
	error
	IntIdentifier
	StrIdentifier
}

type errNotFoundByID struct {
	intID int64
	strID string
	kind  string
	cause error
}

func (e errNotFoundByID) IntID() int64 {
	return e.intID
}

func (e errNotFoundByID) StrID() string {
	return e.strID
}

func (e errNotFoundByID) ID() interface{} {
	if e.intID == 0 {
		if e.strID == "" {
			return nil
		}
		return e.strID
	} else if e.strID == "" {
		return e.intID
	}
	panic("intID != 0 && strID is not empty string")
}

func (e errNotFoundByID) Cause() error {
	return e.cause
}

func (e errNotFoundByID) Error() string {
	return fmt.Sprintf("'%v' not found by id=%v: %v", e.kind, e.ID(), e.cause)
}

// NewErrNotFoundID creates an error that indicates that entity was not found by ID
func NewErrNotFoundID(holder EntityHolder, cause error) error {
	kind := holder.Kind()
	switch {
	case holder.IntID() != 0:
		return NewErrNotFoundByIntID(kind, holder.IntID(), cause)
	case holder.StrID() != "":
		return NewErrNotFoundByStrID(kind, holder.StrID(), cause)
	default:
		panic(fmt.Sprintf("entity Holder has no ID: %+v", holder))
	}
}

// NewErrNotFoundByIntID creates an error that indicates that entity was not found by integer ID
func NewErrNotFoundByIntID(kind string, id int64, cause error) error {
	return errNotFoundByID{kind: kind, intID: id, cause: errNotFoundCause(cause)}
}

// NewErrNotFoundByStrID creates an error that indicates that entity was not found by string ID
func NewErrNotFoundByStrID(kind string, id string, cause error) error {
	return errNotFoundByID{kind: kind, strID: id, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause == ErrRecordNotFound {
		return ErrRecordNotFound
	}
	return errors.WithMessage(ErrRecordNotFound, cause.Error())
}
