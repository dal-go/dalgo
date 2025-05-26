package dal

import (
	"context"
	"fmt"
	"github.com/strongo/random"
)

type idGenerator struct {
	maxAttempts int
	attempts    int
	f           func(ctx context.Context, record Record) error
}

func (v *idGenerator) GenerateID(ctx context.Context, record Record) error {
	if v.attempts++; v.attempts > v.maxAttempts {
		return fmt.Errorf("%w to generate a record ID: %d", ErrExceedsMaxNumberOfAttempts, v.maxAttempts)
	}
	return v.f(ctx, record)
}

func NewIDGenerator(f IDGenerator, maxAttempts int) IDGenerator {
	v := &idGenerator{f: f, maxAttempts: maxAttempts}
	return v.f
}

func WithRandomStringKey(length, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record Record) error {
				record.Key().ID = random.ID(length)
				return nil
			},
			maxAttempts,
		)
	}
}
