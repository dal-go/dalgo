package dalgo

import (
	"testing"
	"time"
)

func TestWithExistsPrecondition(t *testing.T) {
	if WithExistsPrecondition() == nil {
		t.Fatalf("expected to be != nil")
	}
}
func TestPreconditions_Exists(t *testing.T) {
	preconditions := preConditions{}
	if preconditions.Exists() {
		t.Fatalf("preconditions.Exists() exoected to be false")
	}
	v := WithExistsPrecondition()
	v(&preconditions)
	if !preconditions.Exists() {
		t.Errorf("preconditions.Exists() exoected to be true")
	}
}

func TestWithLastUpdateTimePrecondition(t *testing.T) {
	preconditions := preConditions{}
	if !preconditions.lastUpdateTime.IsZero() {
		t.Fatalf("preconditions.lastUpdateTime exoected to be zero")
	}
	expected := time.Now()
	v := WithLastUpdateTimePrecondition(expected)
	v(&preconditions)
	actual := preconditions.lastUpdateTime
	if !actual.Equal(expected) {
		t.Errorf("actual != expected")
	}
}
