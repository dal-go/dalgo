package end2end

import (
	"github.com/dal-go/dalgo/dal"
	"testing"
)

func assertRecordsMustExist(t *testing.T, records []dal.Record) {
	t.Helper()
	notFound := 0
	for _, record := range records {
		if err := record.Error(); err != nil {
			t.Errorf("not able to check record for existence as it has unexpected error: %v", err)
		}
		if !record.Exists() {
			t.Errorf("record was expected to exist, key: %v", record.Key())
			notFound++
		}
	}
	if notFound > 0 {
		t.Fatalf("out of %d records that must exists %d were not found", len(records), notFound)
	}
}

func assertRecordsMustNotExist(t *testing.T, records []dal.Record) {
	t.Helper()
	for i, record := range records {
		if err := record.Error(); err != nil {
			t.Errorf("record with key=[%v] has unexpected error: %v", record.Key(), err)
		} else if record.Exists() {
			t.Errorf("for record #%v of %v Exists() returned true, but expected false; key: %v",
				i+1, len(records), record.Key())
		}
	}
}
