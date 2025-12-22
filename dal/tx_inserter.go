package dal

import (
	"context"
	"fmt"
)

// Inserter defines a function to insert a single record into a database
type Inserter interface {

	// Insert inserts a single record into a database
	Insert(ctx context.Context, record Record, opts ...InsertOption) error
}

// MultiInserter defines a function to insert multiple records into a database
type MultiInserter interface {
	// InsertMulti inserts multiple record into a database at once if possible, or fallback to batch of single inserts
	InsertMulti(ctx context.Context, records []Record, opts ...InsertOption) error
}

// IDGenerator defines a contract for ID generator function
type IDGenerator = func(ctx context.Context, record Record) error

// InsertOptions defines interface for insert options
type InsertOptions interface {
	IDGenerator() IDGenerator
}

type insertOptions struct {
	idGenerator IDGenerator
}

func (v insertOptions) IDGenerator() IDGenerator {
	return v.idGenerator
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
func WithIDGenerator(ctx context.Context, g IDGenerator) KeyOption {
	return func(key *Key) error {
		if key.ID != nil {
			panic("an attempt to set ID generator for a child that already have an ID value")
		}
		return g(ctx, &record{key: key})
	}
}

func InsertWithIdGenerator(
	ctx context.Context,
	r Record,
	generateID IDGenerator,
	maxAttempts int,
	exists func(*Key) error,
	insert func(Record) error,
) error {
	key := r.Key()

	for i := 1; i <= maxAttempts; i++ {
		if err := generateID(ctx, r); err != nil {
			key.ID = nil
			return fmt.Errorf("failed to generate record key ID: %w", err)
		}
		r.SetError(nil)
		if validatableWthKey, ok := r.Data().(interface{ ValidateWithKey(*Key) error }); ok {
			if err := validatableWthKey.ValidateWithKey(key); err != nil {
				return fmt.Errorf("failed to validate record key: %w", err)
			}
		}
		if err := exists(key); err == nil {
			continue
		} else if IsNotFound(err) {
			return insert(r) // r shares child with tmp
		}
		key.ID = nil
		return fmt.Errorf("failed to check if record exists: %w", err)
	}
	key.ID = nil
	return fmt.Errorf("not able to generate unique id: %w: %d", ErrExceedsMaxNumberOfAttempts, maxAttempts)
}

var ErrExceedsMaxNumberOfAttempts = fmt.Errorf("exceeds maximum number of attempts")
