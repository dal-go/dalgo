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
			rec := NewRecordWithIncompleteKey("collection", reflect.String, &data)
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
			rec := NewRecordWithIncompleteKey("collection", reflect.String, &data)
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
