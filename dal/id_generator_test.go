package dal

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWithRandomStringKey(t *testing.T) {
	type args struct {
		length      int
		maxAttempts int
	}
	tests := []struct {
		name string
		args args
		want InsertOption
	}{
		{
			name: "1/1",
			args: args{
				length:      1,
				maxAttempts: 1,
			},
		},
		{
			name: "3/1",
			args: args{
				length:      1,
				maxAttempts: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var io insertOptions
			WithRandomStringKey(tt.args.length, tt.args.maxAttempts)(&io)
			idGen := io.IDGenerator()
			if idGen == nil {
				t.Errorf("WithRandomStringKeyPrefixedByUnixTime() insertOptions.idGen = nil")
			}
			data := struct{}{}
			rec := NewRecordWithIncompleteKey("recordsetSource", reflect.String, &data)
			if err := idGen(context.Background(), rec); err != nil {
				t.Fatalf("idGen returend errr: %v", err)
			}
			id := rec.Key().ID.(string)
			if id == "" {
				t.Fatalf("generated id is empty string")
			}
			if len(id) != tt.args.length {
				t.Errorf("length of generated id expected to be %d, got %d", tt.args.length, len(id))
			}
		})
	}
}

func TestWithRandomStringKeyPrefixedByUnixTime(t *testing.T) {
	type args struct {
		length      int
		maxAttempts int
	}
	tests := []struct {
		name string
		args args
		want InsertOption
	}{
		{
			name: "1/1",
			args: args{
				length:      1,
				maxAttempts: 1,
			},
		},
		{
			name: "3/1",
			args: args{
				length:      3,
				maxAttempts: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var io insertOptions
			WithRandomStringKeyPrefixedByUnixTime(tt.args.length, tt.args.maxAttempts)(&io)
			idGen := io.IDGenerator()
			if idGen == nil {
				t.Errorf("WithRandomStringKeyPrefixedByUnixTime() insertOptions.idGen = nil")
			}
			data := struct{}{}
			rec := NewRecordWithIncompleteKey("recordsetSource", reflect.String, &data)
			if err := idGen(context.Background(), rec); err != nil {
				t.Fatalf("idGen returend errr: %v", err)
			}
			id := rec.Key().ID.(string)
			if id == "" {
				t.Fatalf("generated id is empty string")
			}
			if !strings.Contains(id, "_") {
				t.Fatalf("generated id does not contain \"_\"")
			}
			parts := strings.Split(id, "_")
			if len(parts) != 2 {
				t.Fatalf("generated id expected to have 2 parts, got %d", len(parts))
			}
			if part0, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
				t.Errorf("first part of id expected to be int64, got %s", parts[0])
			} else {
				unixTime := time.Unix(part0, 0)
				if unixTime.IsZero() {
					t.Errorf("first part of id expected to be non-zero, got %s", unixTime)
				}
			}
			if len(parts[1]) != tt.args.length {
				t.Errorf("length of second part of id expected to be %d, got %d", tt.args.length, len(parts[1]))
			}
		})
	}
}

func TestIdGenerator_GenerateID(t *testing.T) {
	tests := []struct {
		name        string
		maxAttempts int
		expectError bool
	}{
		{
			name:        "success on first attempt",
			maxAttempts: 1,
			expectError: false,
		},
		{
			name:        "exceeds max attempts",
			maxAttempts: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &idGenerator{
				maxAttempts: tt.maxAttempts,
				attempts:    0,
				f: func(ctx context.Context, record Record) error {
					record.Key().ID = "test-id"
					return nil
				},
			}

			data := struct{}{}
			rec := NewRecordWithIncompleteKey("test", reflect.String, &data)
			
			err := generator.GenerateID(context.Background(), rec)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				if rec.Key().ID != nil {
					t.Errorf("expected ID to be nil when error occurs, got %v", rec.Key().ID)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if rec.Key().ID != "test-id" {
					t.Errorf("expected ID to be 'test-id', got %v", rec.Key().ID)
				}
			}
		})
	}
}

func TestWithTimeStampStringID(t *testing.T) {
	tests := []struct {
		name        string
		accuracy    TimeStampAccuracy
		base        int
		maxAttempts int
	}{
		{
			name:        "nano accuracy base 10",
			accuracy:    TimeStampAccuracyNano,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "microsecond accuracy base 10",
			accuracy:    TimeStampAccuracyMicrosecond,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "millisecond accuracy base 10",
			accuracy:    TimeStampAccuracyMillisecond,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "second accuracy base 10",
			accuracy:    TimeStampAccuracySecond,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "minute accuracy base 10",
			accuracy:    TimeStampAccuracyMinute,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "hour accuracy base 10",
			accuracy:    TimeStampAccuracyHour,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "day accuracy base 10",
			accuracy:    TimeStampAccuracyDay,
			base:        10,
			maxAttempts: 1,
		},
		{
			name:        "second accuracy base 16",
			accuracy:    TimeStampAccuracySecond,
			base:        16,
			maxAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var io insertOptions
			WithTimeStampStringID(tt.accuracy, tt.base, tt.maxAttempts)(&io)
			idGen := io.IDGenerator()
			if idGen == nil {
				t.Errorf("WithTimeStampStringID() insertOptions.idGen = nil")
			}
			
			data := struct{}{}
			rec := NewRecordWithIncompleteKey("test", reflect.String, &data)
			if err := idGen(context.Background(), rec); err != nil {
				t.Fatalf("idGen returned error: %v", err)
			}
			
			id := rec.Key().ID.(string)
			if id == "" {
				t.Fatalf("generated id is empty string")
			}
			
			// Verify the ID can be parsed as an integer in the specified base
			if _, err := strconv.ParseInt(id, tt.base, 64); err != nil {
				t.Errorf("generated id %s cannot be parsed as base %d integer: %v", id, tt.base, err)
			}
		})
	}
}

func TestWithTimeStampStringID_InvalidAccuracy(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for invalid accuracy")
		}
	}()
	
	var io insertOptions
	WithTimeStampStringID(TimeStampAccuracy(999), 10, 1)(&io)
	idGen := io.IDGenerator()
	
	data := struct{}{}
	rec := NewRecordWithIncompleteKey("test", reflect.String, &data)
	idGen(context.Background(), rec)
}
