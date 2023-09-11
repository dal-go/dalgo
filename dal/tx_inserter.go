package dal

import (
	"context"
	"fmt"
)

// Inserter defines a function to insert a single record into database
type Inserter interface {

	// Insert inserts a single record in database
	Insert(ctx context.Context, record Record, opts ...InsertOption) error
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

// RandomLength sets prefix for a random string
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

// InsertWithRandomID inserts a record with a random ID
func InsertWithRandomID(
	ctx context.Context,
	r Record,
	generateID IDGenerator,
	attempts int,
	exists func(*Key) error,
	insert func(Record) error,
) error {
	key := r.Key()
	// We need a temp record to make sure we do not overwrite data during exists() check
	tmp := &record{key: key}
	for i := 1; i <= attempts; i++ {
		if err := generateID(ctx, tmp); err != nil {
			return fmt.Errorf("failed to generate random value: %w", err)
		}
		if err := exists(key); err == nil {
			continue
		} else if IsNotFound(err) {
			return insert(r) // r shares child with tmp
		} else {
			r.Key().ID = nil
			return fmt.Errorf("failed to check if record exists: %w", err)
		}
	}
	r.Key().ID = nil
	return fmt.Errorf("not able to generate unique id: %w: %d", ErrExceedsMaxNumberOfAttempts, attempts)
}

var ErrExceedsMaxNumberOfAttempts = fmt.Errorf("exceeds max number of attempts")
