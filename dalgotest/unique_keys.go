package dalgotest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

// AssertUniqueKeys fails t when two records of one read share a key.
//
// The invariant every provider keeps: within one read (one result set of one query
// against one recordset) no two records have the same key. The only records allowed to
// repeat are those with no key at all (see HasNoKey). A provider with no key of its own
// for a row, such as one of a view, a join or a GROUP BY, keys the rows by ordinal, the
// position of the row in the read counted from 0 and written as decimal text, which is
// unique by construction.
//
// Keys are compared as text, with their parents and collections: the integer 1 and the
// string "1" are one key, as they are one path.
func AssertUniqueKeys(t testing.TB, records []record.Record) {
	t.Helper()
	if err := CheckUniqueKeys(records); err != nil {
		t.Error(err)
	}
}

// CheckUniqueKeys is AssertUniqueKeys returning the violation, naming both rows (by their
// position in records, from 0) and the key they share, or nil.
func CheckUniqueKeys(records []record.Record) error {
	seen := make(map[string]int, len(records))
	for row, r := range records {
		if r == nil || HasNoKey(r.Key()) {
			continue
		}
		identity := keyIdentity(r.Key())
		if first, repeated := seen[identity]; repeated {
			return fmt.Errorf("dalgotest: records %d and %d of one read share the key %q: a key is unique within a read", first, row, identity)
		}
		seen[identity] = row
	}
	return nil
}

// HasNoKey reports whether key is no key: it is nil, or its ID is nil or the empty
// string. A record with no key may repeat within a read. Every other value, the zero
// number and false included, is a key.
func HasNoKey(key *record.Key) bool {
	if key == nil {
		return true
	}
	switch id := key.ID.(type) {
	case nil:
		return true
	case string:
		return id == ""
	}
	return false
}

// keyIdentity is the text a key is compared by: its collections and IDs from the root.
func keyIdentity(key *record.Key) string {
	var parts []string
	for k := key; k != nil; k = k.Parent() {
		parts = append(parts, fmt.Sprintf("%s/%v", k.Collection(), k.ID))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}
