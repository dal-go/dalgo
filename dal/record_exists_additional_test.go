package dal

import (
	"testing"

	"github.com/dal-go/record"
)

func TestRecord_Exists_panics_on_other_error(t *testing.T) {
	r := record.NewRecord(record.NewKeyWithID("Kind1", "k1")).SetError(assertErr("boom"))
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when Exists() called with non-notfound error set")
		}
	}()
	_ = r.Exists()
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
