package dal

import "testing"

func TestSchema_KeyToFields_nil_func_returns_nil(t *testing.T) {
	s := NewSchema(nil, nil)
	fields, err := s.KeyToFields(NewKeyWithID("Kind", "1"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields != nil {
		t.Fatalf("expected nil fields when keyToFields is nil, got: %#v", fields)
	}
}
