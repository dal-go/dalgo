package dal

import "testing"

func TestQueryBuilderStartAfterPreservesStartFrom(t *testing.T) {
	q := From(NewRootCollectionRef("states", "")).NewQuery().StartFrom("inclusive").StartAfter("exclusive").SelectKeysOnly(0)
	if q.StartFrom() != "inclusive" || q.StartAfter() != "exclusive" {
		t.Fatalf("cursors = from %q after %q", q.StartFrom(), q.StartAfter())
	}
}
