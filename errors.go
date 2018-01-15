package db

import (
	"fmt"
	"github.com/pkg/errors"
)

type ErrDuplicateUser struct {
	// TODO: Should it be moved out of this package to strongo/app/user?
	SearchCriteria   string
	DuplicateUserIDs []int64
}

func (err ErrDuplicateUser) Error() string {
	return fmt.Sprintf("Multiple users by given search criteria[%v]: %v", err.SearchCriteria, err.DuplicateUserIDs)
}

var (
	ErrRecordNotFound = errors.New("Record not found")
)

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(ErrNotFoundByID)
	return ok || errors.Cause(err) == ErrRecordNotFound
}

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

func NewErrNotFoundID(holder EntityHolder, cause error) error {
	kind := holder.Kind()
	switch {
	case holder.IntID() != 0: return NewErrNotFoundByIntID(kind, holder.IntID(), cause)
	case holder.StrID() != "": return NewErrNotFoundByStrID(kind, holder.StrID(), cause)
	default:
		panic("no ID")
	}
}

func NewErrNotFoundByIntID(kind string, id int64, cause error) error {
	return errNotFoundByID{kind: kind, intID: id, cause: errNotFoundCause(cause)}
}

func NewErrNotFoundByStrID(kind string, id string, cause error) error {
	return errNotFoundByID{kind: kind, strID: id, cause: errNotFoundCause(cause)}
}

func errNotFoundCause(cause error) error {
	if cause == nil || cause == ErrRecordNotFound {
		return ErrRecordNotFound
	} else {
		return errors.WithMessage(ErrRecordNotFound, cause.Error())
	}
}
