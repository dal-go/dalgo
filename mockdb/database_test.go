package mockdb

import (
	"github.com/strongo/db"
	"testing"
)

func TestNewMockDB(t *testing.T) {
	var mockDb db.Database = NewMockDB(nil, nil)
	if mockDb == nil {
		t.Errorf("NewMockDB returned null")
	}
}
