package db

import (
	"context"
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
