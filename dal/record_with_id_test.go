package dal_test

import (
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestRecordWithID_String(t *testing.T) {
	tests := []struct {
		name  string
		input fmt.Stringer
		want  string
	}{
		{
			name:  "Empty FullID",
			input: dal.RecordWithID[string]{ID: "1", FullID: "", Key: nil, Record: nil},
			want:  "{ID=1, FullID=nil, Key=<nil>, Record=<nil>}",
		},
		{
			name:  "FullID is not empty, ID is string",
			input: dal.RecordWithID[string]{ID: "1", FullID: "custom-1", Key: nil, Record: nil},
			want:  `{ID="1", FullID="custom-1", Key=<nil>, Record=<nil>}`,
		},
		{
			name:  "FullID is not empty, ID is integer",
			input: dal.RecordWithID[int]{ID: 1, FullID: "custom-1", Key: nil, Record: nil},
			want:  `{ID=1, FullID="custom-1", Key=<nil>, Record=<nil>}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.String()
			if got != tt.want {
				t.Errorf("RecordWithID.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRecordWithID(t *testing.T) {
	type data struct{ Title string }
	key := dal.NewKeyWithID("things", "t1")
	d := &data{Title: "x"}
	got := dal.NewRecordWithID("t1", key, d)
	if got.ID != "t1" {
		t.Errorf("ID = %q, want t1", got.ID)
	}
	if got.Key != key {
		t.Errorf("Key not set")
	}
	if got.Record == nil {
		t.Fatalf("Record not set")
	}
	got.Record.SetError(nil)
	if got.Record.Data() != d {
		t.Errorf("Record data not wired")
	}
}
