package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/record"
)

// Inserter defines a function to insert a single record intoRecord a database
type Inserter interface {

	// Insert inserts a single record intoRecord a database
	Insert(ctx context.Context, record record.Record, opts ...InsertOption) error
}

// MultiInserter defines a function to insert multiple records intoRecord a database
type MultiInserter interface {
	// InsertMulti inserts multiple record intoRecord a database at once if possible, or fallback to batch of single inserts
	InsertMulti(ctx context.Context, records []record.Record, opts ...InsertOption) error
}

// IDGenerator defines a contract for ID generator function
type IDGenerator = func(ctx context.Context, record record.Record) error

// InsertOptions defines interface for insert options
type InsertOptions interface {
	IDGenerator() IDGenerator

	// PreferAdapterGeneratedID reports whether WithAdapterGeneratedID was passed.
	// See WithAdapterGeneratedID for the contract adapters must follow.
	PreferAdapterGeneratedID() bool
}

type insertOptions struct {
	idGenerator              IDGenerator
	preferAdapterGeneratedID bool
}

func (v insertOptions) IDGenerator() IDGenerator {
	return v.idGenerator
}

func (v insertOptions) PreferAdapterGeneratedID() bool {
	return v.preferAdapterGeneratedID
}

var _ InsertOptions = (*insertOptions)(nil)

// NewInsertOptions creates insert options
func NewInsertOptions(opts ...InsertOption) InsertOptions {
	var options insertOptions
	for _, o := range opts {
		o(&options)
	}
	return options
}

// InsertOption defines a contract for an insert option
type InsertOption func(options *insertOptions)

// WithAdapterGeneratedID requests that the storage adapter generates the record
// ID natively (e.g. Firestore's client-side auto-generated 20-char document IDs).
//
// Contract for adapters:
//   - An adapter SHOULD use its backend's native ID generation mechanism if it has one.
//   - An adapter that has no native mechanism MUST fall back to the default
//     random-string generator (WithRandomStringKey(DefaultRandomStringIDLength, 5)),
//     so this option never fails on a compliant adapter.
//   - If an explicit ID generator option (e.g. WithRandomStringKey) is supplied
//     alongside this option, the explicit generator wins: adapters must check
//     InsertOptions.IDGenerator() first and only consult
//     InsertOptions.PreferAdapterGeneratedID() when it is nil.
//
// Adapters introspect this option via InsertOptions.PreferAdapterGeneratedID().
func WithAdapterGeneratedID() InsertOption {
	return func(options *insertOptions) {
		options.preferAdapterGeneratedID = true
	}
}

type randomStringOptions struct {
	length int
	prefix string
}

// Length returns a predefined length for a random string. Default is DefaultRandomStringIDLength
func (v randomStringOptions) Length() int {
	if v.length == 0 {
		return DefaultRandomStringIDLength
	}
	return v.length
}

// Prefix returns a predefined prefix for a random string
func (v randomStringOptions) Prefix() string {
	return v.prefix
}

// RandomStringOptions defines settings for random string
type RandomStringOptions interface {
	Prefix() string
	Length() int
}

// Prefix sets prefix for a random string
func Prefix(prefix string) func(options *randomStringOptions) {
	return func(options *randomStringOptions) {
		options.prefix = prefix
	}
}

// RandomLength sets length for a random string
func RandomLength(length int) func(options *randomStringOptions) {
	return func(options *randomStringOptions) {
		options.length = length
	}
}

type randomStringOption func(opts *randomStringOptions)

// WithIDGenerator sets ID generator for a random string (usually random)
func WithIDGenerator(ctx context.Context, g IDGenerator) record.KeyOption {
	return func(key *record.Key) error {
		if key.ID != nil {
			panic("an attempt to set ID generator for a child that already have an ID value")
		}
		return g(ctx, record.NewRecord(key))
	}
}

func InsertWithIdGenerator(
	ctx context.Context,
	r record.Record,
	generateID IDGenerator,
	maxAttempts int,
	exists func(*record.Key) error,
	insert func(record.Record) error,
) error {
	key := r.Key()

	for i := 1; i <= maxAttempts; i++ {
		if err := generateID(ctx, r); err != nil {
			key.ID = nil
			return fmt.Errorf("failed to generate record key ID: %w", err)
		}
		r.SetError(nil)
		if validatableWthKey, ok := r.Data().(interface{ ValidateWithKey(*record.Key) error }); ok {
			if err := validatableWthKey.ValidateWithKey(key); err != nil {
				return fmt.Errorf("failed to validate record key: %w", err)
			}
		}

		if err := exists(key); err != nil {
			if record.IsNotFound(err) {
				return insert(r) // r shares child with tmp
			}
			key.ID = nil
			return fmt.Errorf("failed to check if record exists: %w", err)
		}
	}
	key.ID = nil
	return fmt.Errorf("not able to generate unique id: %w: %d", ErrExceedsMaxNumberOfAttempts, maxAttempts)
}

var ErrExceedsMaxNumberOfAttempts = fmt.Errorf("exceeds maximum number of attempts")
