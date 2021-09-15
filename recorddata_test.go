package dalgo

import "testing"

func TestRecordData_Validate(t *testing.T) {
	var v Validatable = new(RecordData)
	if err := v.Validate(); err != nil {
		t.Errorf("unexpected error")
	}
}
