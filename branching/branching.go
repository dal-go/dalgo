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

	// ErrNilSourceDB identifies a nil source database.
	ErrNilSourceDB = errors.New("dalgo branching: source database is nil")
	// ErrNilBranchDB identifies a nil branch database.
	ErrNilBranchDB = errors.New("dalgo branching: branch database is nil")
	// ErrReleased identifies a checkpoint that has been released.
	ErrReleased = errors.New("dalgo branching: checkpoint is released")
	// ErrEmptyGeneration identifies an empty checkpoint generation.
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
	// Capability returns this provider's stable metadata.
	Capability() Capability
	// Capture captures immutable state from the source database.
	Capture(context.Context, dal.DB) (Checkpoint, error)
}

// Checkpoint is immutable provider state from which fresh branches are created.
// Release must be idempotent.
type Checkpoint interface {
	// Generation returns the checkpoint generation.
	Generation() string
	// Branch returns a fresh branch created from this checkpoint.
	Branch(context.Context) (Branch, error)
	// Release releases this checkpoint. Release must be idempotent.
	Release(context.Context) error
}

// Branch owns a fresh database handle. Close must be idempotent.
type Branch interface {
	// DB returns the branch's database handle.
	DB() dal.DB
	// Close closes the branch. Close must be idempotent.
	Close(context.Context) error
}
