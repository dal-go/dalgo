package dalgo

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/random"
)

// Inserter is an interface that describe DB provider that can insert a single entity with a specific or random Value
type Inserter interface {
	Insert(c context.Context, record Record, opts ...InsertOption) error
}

type IDGenerator = func(ctx context.Context, record Record) error

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

func NewInsertOptions(opts ...InsertOption) InsertOptions {
	var options insertOptions
	for _, o := range opts {
		o(&options)
	}
	return options
}

type InsertOption func(options *insertOptions)

type randomStringOptions struct {
	prefix string
}

type RandomStringOptions interface {
	Prefix() string
}

func WithPrefix(prefix string) func(options *randomStringOptions) {
	return func(options *randomStringOptions) {
		options.prefix = prefix
	}
}

func (v randomStringOptions) Prefix() string {
	return v.prefix
}

type RandomStringOption func(opts *randomStringOptions)

func WithIDGenerator(g IDGenerator) KeyOption {
	return func(key *Key) {
		if key.ID != nil {
			panic("an attempt to set ID generator for a child that already have an ID value")
		}
		key.ID = g
	}
}

func WithRandomStringID(length int, options ...RandomStringOption) KeyOption {
	var rso randomStringOptions
	for _, setOption := range options {
		setOption(&rso)
	}
	return func(key *Key) {
		key.ID = WithIDGenerator(func(ctx context.Context, record Record) error {
			key.ID = random.ID(length)
			return nil
		})
	}
}

func WithParent(kind string, id interface{}, options ...KeyOption) KeyOption {
	return func(key *Key) {
		key.parent = NewKeyWithID(kind, id, options...)
	}
}

func WithParentKey(parent *Key) KeyOption {
	return func(key *Key) {
		key.parent = parent
	}
}

func WithStringID(id string) KeyOption {
	return func(key *Key) {
		key.ID = id
	}
}

func InsertWithRandomID(
	c context.Context,
	r Record,
	generateID IDGenerator,
	attempts int,
	exists func(*Key) error,
	insert func(Record) error,
) error {
	// We need a temp record to make sure we do not overwrite data during exists() check
	tmp := record{key: r.Key(), data: nil}
	for i := 1; i <= attempts; i++ {
		if err := generateID(c, tmp); err != nil {
			return errors.Wrap(err, "failed to generate random Value")
		}
		if err := exists(tmp.key); err == nil {
			continue
		} else if IsNotFound(err) {
			return insert(r) // r shares child with tmp
		} else {
			return fmt.Errorf("failed to check if record exists: %w", err)
		}
	}
	return fmt.Errorf("not able to generate unique Value in %v attempts", attempts)
}
