package dal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type (
	// Embedded without a name, so encoding/json writes their fields as fields of the row.
	promotedBase   struct{ Created, Shadow time.Time }
	promotedSeen   struct{ Seen *time.Time }
	promotedAbsent struct{ Absent time.Time }
	PromotedMeta   struct {
		Meta struct{ When time.Time } `json:"meta"`
	}
	// Two embedded structs that write the same name at the same depth.
	promotedDupA struct{ Dup time.Time }
	promotedDupB struct{ Dup time.Time }
	// Two that write the same name, one of them because its tag says so.
	promotedTagged struct {
		Pick time.Time `json:"Pick"`
	}
	promotedUntagged struct{ Pick time.Time }
	// An embedded struct with a name of its own is a field of that name.
	promotedNamed struct{ Inner time.Time }
	// An embedded value that is not a struct is not written when its type is unexported.
	promotedCount int
)

type promotedRow struct {
	promotedBase
	*promotedSeen
	*promotedAbsent
	PromotedMeta
	promotedCount
	promotedDupA
	promotedDupB
	promotedTagged
	promotedUntagged
	promotedNamed `json:"named"`
	Shadow        time.Time
	Skipped       time.Time `json:"-"`
	private       time.Time
}

type cyclicRow struct {
	*cyclicRow
	Seen time.Time `json:"seen"`
}

// The sort value of a field of a struct row is the one of the field encoding/json
// writes under that name: a field of an embedded struct is a field of the row, an
// own field shadows it, and a name two embedded structs write is none.
func TestOrderedAggregateSortValuesFollowEmbeddedStructsAsEncodingJSONDoes(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 1, 1, 0, 0, second, 0, time.UTC) }
	seen := at(2)
	row := promotedRow{
		promotedBase:     promotedBase{Created: at(1), Shadow: at(10)},
		promotedSeen:     &promotedSeen{Seen: &seen},
		promotedAbsent:   nil,
		promotedDupA:     promotedDupA{Dup: at(20)},
		promotedDupB:     promotedDupB{Dup: at(21)},
		promotedTagged:   promotedTagged{Pick: at(5)},
		promotedUntagged: promotedUntagged{Pick: at(6)},
		promotedNamed:    promotedNamed{Inner: at(7)},
		Shadow:           at(3),
		Skipped:          at(8),
		private:          at(9),
	}
	row.Meta.When = at(4)

	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	if err := json.Unmarshal(encoded, &written); err != nil {
		t.Fatal(err)
	}
	writes := func(path string) (string, bool) {
		var current any = written
		for _, part := range strings.Split(path, ".") {
			object, ok := current.(map[string]any)
			if !ok {
				return "", false
			}
			if current, ok = object[part]; !ok {
				return "", false
			}
		}
		text, ok := current.(string)
		return text, ok
	}

	for _, tc := range []struct {
		path   string
		second int // the second of the instant the field holds, or 0 when no field of that name is written
	}{
		{"Created", 1},
		{"Seen", 2},
		{"Shadow", 3},
		{"meta.When", 4},
		{"Pick", 5},
		{"named.Inner", 7},
		{"Inner", 0},
		{"Absent", 0},
		{"Dup", 0},
		{"Skipped", 0},
		{"-", 0},
		{"private", 0},
		{"promotedCount", 0},
		{"missing", 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			text, isWritten := writes(tc.path)
			if isWritten != (tc.second != 0) {
				t.Fatalf("test data: encoding/json writes %q = %v, the test expects %v", tc.path, isWritten, tc.second != 0)
			}
			var want map[string]any
			if tc.second != 0 {
				if text != at(tc.second).Format(time.RFC3339) {
					t.Fatalf("test data: encoding/json writes %q as %s", tc.path, text)
				}
				want = map[string]any{tc.path: at(tc.second).UTC().Format(timestampSortLayout)}
			}
			for _, data := range []any{row, &row} {
				got, err := buildSortValues(data, []string{tc.path})
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("sort values = %v, %v; want %v", got, err, want)
				}
			}
		})
	}

	t.Run("a struct that embeds a pointer to itself is searched once", func(t *testing.T) {
		cyclic := &cyclicRow{Seen: at(1)}
		cyclic.cyclicRow = cyclic
		if got, err := buildSortValues(cyclic, []string{"seen"}); err != nil || got["seen"] != at(1).Format(timestampSortLayout) {
			t.Fatalf("sort values = %v, %v", got, err)
		}
		if got, err := buildSortValues(cyclic, []string{"absent"}); err != nil || got != nil {
			t.Fatalf("sort values = %v, %v", got, err)
		}
	})
}
