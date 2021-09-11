package dalgo

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/random"
)

// Inserter is an interface that describe DB provider that can insert a single entity with a specific or random ID
type Inserter interface {
	Insert(c context.Context, record Record, options InsertOptions) error
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

func WithIDGenerator(idGenerator IDGenerator) InsertOption {
	return func(options *insertOptions) {
		if options.idGenerator != nil {
			panic("an attempt to add an ID generator while insert options already have one")
		}
		options.idGenerator = idGenerator
	}
}

type randomStringOptions struct {
	length int
	prefix string
}

type RandomStringOptions interface {
	Length() int
}

func (v randomStringOptions) Length() int {
	return v.length
}

type randomStringOption func(opts *randomStringOptions)

func WithRandomStringID(length int) InsertOption {
	return func(options *insertOptions) {
		if options.idGenerator != nil {
			panic("an attempt to add random string ID generator while insert options already have one")
		}
		options.idGenerator = func(ctx context.Context, record Record) error {
			key := record.Key()
			key[len(key)-1].ID = random.ID(length)
			return nil
		}
	}
}

type o struct {
}

func (o) Validate() error {
	return nil
}

func VoidData() Validatable {
	return new(o)
}

func InsertWithRandomID(
	c context.Context,
	r Record,
	generateID IDGenerator,
	attempts int,
	exists func(RecordKey) error,
	insert func(Record) error,
) error {
	// We need a temp record to make sure we do not overwrite data during exists() check
	tmp := record{key: r.Key(), data: new(o)}
	for i := 1; i <= attempts; i++ {
		if err := generateID(c, tmp); err != nil {
			return errors.Wrap(err, "failed to generate random ID")
		}
		if err := exists(tmp.key); err == nil {
			continue
		} else if IsNotFound(err) {
			return insert(r) // r shares key with tmp
		} else {
			return fmt.Errorf("failed to check if record exists: %w", err)
		}
	}
	return fmt.Errorf("not able to generate unique ID in %v attempts", attempts)
}
