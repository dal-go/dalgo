// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"testing"
)

// Smoke tests: every exported type/sentinel exists and has the expected zero value.
// Semantic tests for options live in Task 5.

func TestSentinels_Exist(t *testing.T) {
	for _, e := range []error{
		ErrUnsortedInput,
		ErrDuplicateID,
		ErrIncomparableField,
		ErrInvalidArgument,
	} {
		if e == nil {
			t.Fatalf("nil sentinel error")
		}
		// Self-Is invariant
		if !errors.Is(e, e) {
			t.Fatalf("sentinel %v is not errors.Is-itself", e)
		}
	}
}

func TestRecordStatus_Constants(t *testing.T) {
	if Missing != 0 || Extra != 1 || Matched != 2 || Changed != 3 {
		t.Fatalf("RecordStatus constant order is load-bearing: Missing=%d Extra=%d Matched=%d Changed=%d", Missing, Extra, Matched, Changed)
	}
}

func TestOptionsConstructors_DoNotPanic(t *testing.T) {
	_ = WithIgnoreFields()
	_ = WithIgnoreFields("a", "b")
	_ = WithIncludeMatched()
	_ = WithOnlyChangedFields()
	_ = WithAbsentEqualsNil()
}
