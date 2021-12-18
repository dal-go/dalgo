package dalmock

import (
	"testing"
)

func TestNewDbMock(t *testing.T) {
	if got := NewDbMock(); got.onSelectFrom == nil {
		t.Error("NewDbMock(): dbMock.onSelectFrom is nil")
	}
}
