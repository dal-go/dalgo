package dbschema

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// NotSupportedError is the typed error returned by dbschema and ddl
// helper functions when a driver does not support a given operation
// (either because the driver does not implement the relevant
// capability interface at all, or because the specific operation is
// unsupported).
//
// NotSupportedError.Unwrap returns the existing [dal.ErrNotSupported]
// sentinel so callers can do a coarse errors.Is(err, dal.ErrNotSupported)
// check or extract detail via errors.As(err, &ue) with ue of type
// *dbschema.NotSupportedError.
//
// The same error type is used by both the read side (this package's
// helpers) and the write side ([ddl] package helpers) so consumers
// have a single typed error to handle across the whole DDL surface.
type NotSupportedError struct {
	// Op names the operation that was not supported (e.g.
	// "CreateCollection", "DescribeCollection", "DropIndex").
	Op string
	// Backend optionally identifies the driver (e.g.
	// "dalgo2sql/sqlite"). When the helper function constructs the
	// error after a failed type assertion, it sets Backend from
	// db.Adapter().Name() if Adapter() returns a non-nil dal.Adapter;
	// otherwise Backend is left empty.
	Backend string
	// Reason is an optional human-readable explanation.
	Reason string
}

// Error returns a readable single-line message.
func (e *NotSupportedError) Error() string {
	parts := []string{"dbschema: operation not supported"}
	if e.Op != "" {
		parts = append(parts, fmt.Sprintf("op=%s", e.Op))
	}
	if e.Backend != "" {
		parts = append(parts, fmt.Sprintf("backend=%s", e.Backend))
	}
	if e.Reason != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", e.Reason))
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "; " + p
	}
	return out
}

// Unwrap returns the dal.ErrNotSupported sentinel so callers can use
// errors.Is(err, dal.ErrNotSupported) for a coarse check.
func (e *NotSupportedError) Unwrap() error {
	return dal.ErrNotSupported
}
