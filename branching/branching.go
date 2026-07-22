// Package branching defines optional database checkpoint and branching capabilities.
//
// The contracts deliberately live beside dal rather than inside dal.DB. A
// provider can opt in without widening the mandatory data-access interface.
package branching

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

var (
	// ErrUnsupportedCapability identifies a database configuration a provider
	// cannot safely checkpoint or branch.
	ErrUnsupportedCapability = errors.New("dalgo branching: unsupported capability")

	ErrNilSourceDB     = errors.New("dalgo branching: source database is nil")
	ErrNilBranchDB     = errors.New("dalgo branching: branch database is nil")
	ErrReleased        = errors.New("dalgo branching: checkpoint is released")
	ErrEmptyGeneration = errors.New("dalgo branching: checkpoint generation is empty")
)

// UnsupportedError names the provider mode rejected by an optional capability.
type UnsupportedError struct {
	Provider string
	Mode     string
	Reason   string
}

func (e *UnsupportedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("%s: provider=%q mode=%q", ErrUnsupportedCapability, e.Provider, e.Mode)
	}
	return fmt.Sprintf("%s: provider=%q mode=%q: %s", ErrUnsupportedCapability, e.Provider, e.Mode, e.Reason)
}

// Unwrap supports errors.Is(err, ErrUnsupportedCapability).
func (e *UnsupportedError) Unwrap() error { return ErrUnsupportedCapability }

// Capability is stable provider metadata recorded in a state-holder manifest.
type Capability struct {
	Provider string
	Version  string
	Mode     string
}

// Provider captures immutable state from one source database.
type Provider interface {
	Capability() Capability
	Capture(context.Context, dal.DB) (Checkpoint, error)
}

// Checkpoint is immutable provider state from which fresh branches are created.
// Release must be idempotent.
type Checkpoint interface {
	Generation() string
	Branch(context.Context) (Branch, error)
	Release(context.Context) error
}

// Branch owns a fresh database handle. Close must be idempotent.
type Branch interface {
	DB() dal.DB
	Close(context.Context) error
}
