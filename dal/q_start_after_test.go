package dal

import "testing"

func TestQueryBuilderStartCursorsAreMutuallyExclusive(t *testing.T) {
	q := From(NewRootCollectionRef("states", "")).NewQuery().StartFrom("inclusive").StartAfter("exclusive").SelectKeysOnly(0)
	if q.StartFrom() != "" || q.StartAfter() != "exclusive" {
		t.Fatalf("StartAfter cursors = from %q after %q", q.StartFrom(), q.StartAfter())
	}
	q = From(NewRootCollectionRef("states", "")).NewQuery().StartAfter("exclusive").StartFrom("inclusive").SelectKeysOnly(0)
	if q.StartFrom() != "inclusive" || q.StartAfter() != "" {
		t.Fatalf("StartFrom cursors = from %q after %q", q.StartFrom(), q.StartAfter())
	}
}
