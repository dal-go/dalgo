package dal

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dal-go/record"
	"github.com/strongo/random"
)

type idGenerator struct {
	maxAttempts int
	attempts    int
	f           func(ctx context.Context, record record.Record) error
}

// GenerateID calls the wrapped generator.
// When the number of calls exceeds maxAttempts it clears the record key ID and returns an error wrapping ErrExceedsMaxNumberOfAttempts.
func (v *idGenerator) GenerateID(ctx context.Context, record record.Record) error {
	if v.attempts++; v.attempts > v.maxAttempts {
		record.Key().ID = nil
		return fmt.Errorf("%w to generate a record ID: %d", ErrExceedsMaxNumberOfAttempts, v.maxAttempts)
	}
	return v.f(ctx, record)
}

// NewIDGenerator stores f and maxAttempts on an idGenerator and returns f.
func NewIDGenerator(f IDGenerator, maxAttempts int) IDGenerator {
	v := &idGenerator{f: f, maxAttempts: maxAttempts}
	return v.f
}

// WithRandomStringKey returns an InsertOption that sets the ID generator through NewIDGenerator.
// The generator assigns random.ID(length) to the record key ID.
func WithRandomStringKey(length, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record record.Record) error {
				record.Key().ID = random.ID(length)
				return nil
			},
			maxAttempts,
		)
	}
}

// WithRandomStringKeyPrefixedByUnixTime returns an InsertOption that sets the ID generator through NewIDGenerator.
// The generator assigns the current UTC Unix time, an underscore, and random.ID(randomLength) to the record key ID.
func WithRandomStringKeyPrefixedByUnixTime(randomLength, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record record.Record) error {
				record.Key().ID = fmt.Sprintf("%d_%s", time.Now().UTC().Unix(), random.ID(randomLength))
				return nil
			},
			maxAttempts,
		)
	}
}

// TimeStampAccuracy selects how finely WithTimeStampStringID reads the current time.
type TimeStampAccuracy int

const (
	// TimeStampAccuracyNano reads the time in nanoseconds.
	TimeStampAccuracyNano TimeStampAccuracy = iota
	// TimeStampAccuracyMicrosecond reads the time in microseconds.
	TimeStampAccuracyMicrosecond
	// TimeStampAccuracyMillisecond reads the time in milliseconds.
	TimeStampAccuracyMillisecond
	// TimeStampAccuracySecond reads the time in seconds.
	TimeStampAccuracySecond
	// TimeStampAccuracyMinute reads the time in whole minutes.
	TimeStampAccuracyMinute
	// TimeStampAccuracyHour reads the time in whole hours.
	TimeStampAccuracyHour
	// TimeStampAccuracyDay reads the time in whole days.
	TimeStampAccuracyDay
)

// WithTimeStampStringID returns an InsertOption that sets the ID generator through NewIDGenerator.
// The generator assigns the current UTC time, at the given accuracy, formatted in base.
// An accuracy outside these constants panics.
func WithTimeStampStringID(accuracy TimeStampAccuracy, base, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record record.Record) error {
				now := time.Now().UTC()
				var timestamp int64
				switch accuracy {
				case TimeStampAccuracyNano:
					timestamp = now.UnixNano()
				case TimeStampAccuracyMicrosecond:
					timestamp = now.UnixMicro()
				case TimeStampAccuracyMillisecond:
					timestamp = now.UnixMilli()
				case TimeStampAccuracySecond:
					timestamp = now.Unix()
				case TimeStampAccuracyMinute:
					timestamp = now.Unix() / 60
				case TimeStampAccuracyHour:
					timestamp = now.Unix() / 60 / 60
				case TimeStampAccuracyDay:
					timestamp = now.Unix() / 60 / 60 / 24
				default:
					panic(fmt.Sprintf("invalid timeStampAccuracy: %v", accuracy))
				}
				record.Key().ID = strconv.FormatInt(timestamp, base)
				return nil
			},
			maxAttempts,
		)
	}
}
