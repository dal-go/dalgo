package dal

import "testing"

func TestStructuredQuery_Text(t *testing.T) {
	q := structuredQuery{
		from:  From(&CollectionRef{name: "User"}),
		where: ID("SomeID", 123),
		limit: 5,
	}
	if q.Text() != q.String() {
		t.Fatalf("Text should delegate to String; got %q vs %q", q.Text(), q.String())
	}
}
