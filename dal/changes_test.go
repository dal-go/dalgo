package dal

import "testing"

func TestChanges_IsChanged(t *testing.T) {
	type test struct {
		name     string
		changes  Changes
		record   Record
		expected bool
	}

	r1unchanged := record{key: &Key{ID: "r1", collection: "records"}}
	r2changed := record{changed: true, key: &Key{ID: "r2", collection: "records"}}

	for _, tt := range []test{
		{
			name:     "empty_nil",
			changes:  Changes{},
			record:   nil,
			expected: false,
		},
		{
			name:     "empty_not_nil",
			changes:  Changes{},
			record:   new(record),
			expected: false,
		},
		{
			name:     "empty_not_nil",
			changes:  Changes{},
			record:   new(record),
			expected: false,
		},
		{
			name:     "unchanged",
			changes:  Changes{records: []Record{&r1unchanged, &r2changed}},
			record:   &r1unchanged,
			expected: false,
		},
		{
			name:     "changed",
			changes:  Changes{records: []Record{&r1unchanged, &r2changed}},
			record:   &r2changed,
			expected: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected != tt.changes.IsChanged(tt.record) {
				t.Errorf("should be %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

func TestChanges_FlagAsChanged(t *testing.T) {
	r := record{key: &Key{ID: "r1", collection: "records"}}
	c := Changes{}
	c.FlagAsChanged(&r)
	if !r.changed {
		t.Errorf("record should be marked as changed")
	}
	if len(c.records) != 1 {
		t.Errorf("should be 1 record in changes, got %d", len(c.records))
	}
}

func TestChanges_Records(t *testing.T) {
	c := Changes{
		records: []Record{
			&record{key: &Key{ID: "r1", collection: "records"}},
			&record{key: &Key{ID: "r2", collection: "records"}, changed: true},
		},
	}
	records := c.Records()
	const expectedCount = 2
	if count := len(records); count != expectedCount {
		t.Fatalf("should be %d records in changes, got %d", expectedCount, count)
	}
	if SlicesShareSameBackingArray(c.records, records) {
		t.Errorf("Records() returned internal slice, should be a copy")
	}
}

// TODO: move to slice package
func SlicesShareSameBackingArray[T any](a, b []T) bool {
	return &a[cap(a)-1] == &b[cap(b)-1]
}

func TestChanges_HasChanges(t *testing.T) {
	type test struct {
		name     string
		changes  Changes
		expected bool
	}

	r1changed := record{key: &Key{ID: "r1", collection: "records"}, changed: true}
	r2changed := record{key: &Key{ID: "r2", collection: "records"}, changed: true}
	//r2changed := record{changed: true, key: &Key{ID: "r2", collection: "records"}}

	for _, tt := range []test{
		{
			name:     "empty",
			changes:  Changes{},
			expected: false,
		},
		{
			name:     "1changed",
			changes:  Changes{records: []Record{&r1changed}},
			expected: true,
		},
		{
			name:     "2changed",
			changes:  Changes{records: []Record{&r1changed, &r2changed}},
			expected: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected != tt.changes.HasChanges() {
				t.Errorf("should be %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

//func TestChanges_ChangedRecords(t *testing.T) {
//	r1unchanged := record{key: &Key{ID: "r1", collection: "records"}}
//	r2changed := record{changed: true, key: &Key{ID: "r2", collection: "records"}}
//	r3changed := record{changed: true, key: &Key{ID: "r3", collection: "records"}}
//	r4unchanged := record{key: &Key{ID: "r4", collection: "records"}}
//
//	records := []Record{&r1unchanged, &r2changed, &r3changed, &r4unchanged}
//
//	changes := Changes{records: records}
//
//	unchanged := changes.ChangedRecords()
//
//	const expectedCount = 2
//	if count := len(unchanged); count != expectedCount {
//		t.Fatalf("should be %d records in changes, got %d", expectedCount, count)
//	}
//	if SlicesShareSameBackingArray(records, unchanged) {
//		t.Errorf("ChangedRecords() returned internal slice, should be a new one")
//	}
//	if unchanged[0] != &r2changed {
//		t.Errorf("first record should be r2changed, got %v", unchanged[0])
//	}
//	if unchanged[1] != &r3changed {
//		t.Errorf("second record should be r3changed, got %v", unchanged[1])
//	}
//}
