package mockdb

import (
	"github.com/strongo/dalgo"
	"testing"
)

func TestNewMockDB(t *testing.T) {
	var mockDb dalgo.Database = NewMockDB(nil, nil)
	if mockDb == nil {
		t.Errorf("NewMockDB returned null")
	}
}
