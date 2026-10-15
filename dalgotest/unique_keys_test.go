package dalgotest

import (
	"strings"
	"testing"

	"github.com/dal-go/record"
)

type recordingTB struct {
	testing.TB
	failed  bool
	message string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Error(args ...any) {
	r.failed = true
	for _, a := range args {
		r.message += a.(error).Error()
	}
}

func rec(collection string, id any) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID(collection, id), map[string]any{})
}

func TestCheckUniqueKeys(t *testing.T) {
	child := func(parent string, id any) record.Record {
		return record.NewRecordWithData(record.NewKeyWithParentAndID(record.NewKeyWithID("p", parent), "c", id), map[string]any{})
	}
	for _, tc := range []struct {
		name    string
		records []record.Record
		want    string // a fragment of the violation, "" for none
	}{
		{"no records", nil, ""},
		{"distinct keys", []record.Record{rec("a", 1), rec("a", 2), rec("a", "3")}, ""},
		{"the same key in two collections", []record.Record{rec("a", 1), rec("b", 1)}, ""},
		{"the same child ID under two parents", []record.Record{child("x", 1), child("y", 1)}, ""},
		{"the same child under one parent", []record.Record{child("x", 1), child("x", 1)}, "records 0 and 1 of one read share the key"},
		{"a repeated integer", []record.Record{rec("a", 1), rec("a", 2), rec("a", 1)}, "records 0 and 2"},
		{"a repeated constant key", []record.Record{rec("a", "__id"), rec("a", "__id")}, "records 0 and 1"},
		{"an integer and its text are one key", []record.Record{rec("a", 1), rec("a", "1")}, "records 0 and 1"},
		{"the zero number is a key", []record.Record{rec("a", 0), rec("a", 0)}, "records 0 and 1"},
		{"empty and nil IDs may repeat", []record.Record{rec("a", ""), rec("a", ""), rec("a", nil), rec("a", nil)}, ""},
		{"nil records may repeat", []record.Record{nil, nil}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckUniqueKeys(tc.records)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("CheckUniqueKeys() = %v, want none", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CheckUniqueKeys() = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestAssertUniqueKeys(t *testing.T) {
	ok := &recordingTB{}
	AssertUniqueKeys(ok, []record.Record{rec("a", 1), rec("a", 2)})
	if ok.failed {
		t.Fatalf("unique keys failed: %s", ok.message)
	}
	bad := &recordingTB{}
	AssertUniqueKeys(bad, []record.Record{rec("a", 1), rec("a", 1)})
	if !bad.failed || !strings.Contains(bad.message, "records 0 and 1") {
		t.Fatalf("repeated keys: failed = %v, message = %q", bad.failed, bad.message)
	}
}

func TestHasNoKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  *record.Key
		want bool
	}{
		{"nil key", nil, true},
		{"nil ID", record.NewKeyWithID[any]("a", nil), true},
		{"empty ID", record.NewKeyWithID("a", ""), true},
		{"a text ID", record.NewKeyWithID("a", "x"), false},
		{"the zero number", record.NewKeyWithID("a", 0), false},
		{"false", record.NewKeyWithID("a", false), false},
	} {
		if got := HasNoKey(tc.key); got != tc.want {
			t.Errorf("%s: HasNoKey() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
