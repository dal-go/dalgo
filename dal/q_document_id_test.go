package dal

import "testing"

func TestDocumentIDAndCursor(t *testing.T) {
	field := DocumentID()
	if !field.IsID() || field.Name() != "__name__" {
		t.Fatalf("document ID field = %+v", field)
	}
	if cursor, err := NewDocumentIDCursor("qualification-state-1"); err != nil || cursor != "qualification-state-1" {
		t.Fatalf("document ID cursor = %q, %v", cursor, err)
	}
	if _, err := NewDocumentIDCursor(""); err == nil {
		t.Fatal("invalid document ID cursor was accepted")
	}
}
