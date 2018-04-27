package db

import "testing"

func TestNoStrID(t *testing.T) {
	t.Parallel()
	t.Run("StrID", func(t *testing.T) {
		t.Parallel()
		strID := NoStrID{}.StrID()
		if strID != "" {
			t.Fatalf("strID is not empty string: %v", strID)
		}
	})
	t.Run("SetStrID", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if err := recover(); err == nil {
				t.Errorf("Panic expected")
			}
		}()
		(&NoStrID{}).SetStrID("test")
	})
}

func TestIntegerID(t *testing.T) {
	t.Parallel()
	t.Run("NewIntID", func(t *testing.T) {
		t.Parallel()
		integerID := NewIntID(5)
		if integerID.ID != 5 {
			t.Fatalf("unexpected ID value: %v", integerID.ID)
		}
		if integerID.TypeOfID() != IsIntID {
			t.Fatalf("TypeOfID() is no integer: %v", integerID.TypeOfID())
		}
	})
}
