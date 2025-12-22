package dal

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/strongo/random"
)

type idGenerator struct {
	maxAttempts int
	attempts    int
	f           func(ctx context.Context, record Record) error
}

func (v *idGenerator) GenerateID(ctx context.Context, record Record) error {
	if v.attempts++; v.attempts > v.maxAttempts {
		record.Key().ID = nil
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

func WithRandomStringKeyPrefixedByUnixTime(randomLength, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record Record) error {
				record.Key().ID = fmt.Sprintf("%d_%s", time.Now().UTC().Unix(), random.ID(randomLength))
				return nil
			},
			maxAttempts,
		)
	}
}

type TimeStampAccuracy int

const (
	TimeStampAccuracyNano TimeStampAccuracy = iota
	TimeStampAccuracyMicrosecond
	TimeStampAccuracyMillisecond
	TimeStampAccuracySecond
	TimeStampAccuracyMinute
	TimeStampAccuracyHour
	TimeStampAccuracyDay
)

func WithTimeStampStringID(accuracy TimeStampAccuracy, base, maxAttempts int) InsertOption {
	return func(options *insertOptions) {
		options.idGenerator = NewIDGenerator(
			func(ctx context.Context, record Record) error {
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
