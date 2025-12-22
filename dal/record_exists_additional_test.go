package dal

import "testing"

func TestRecord_Exists_panics_on_other_error(t *testing.T) {
	r := (&record{key: NewKeyWithID("Kind1", "k1")}).setError(assertErr("boom"))
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when Exists() called with non-notfound error set")
		}
	}()
	_ = r.Exists()
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
