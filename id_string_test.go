package db

import "testing"

func TestNoIntID(t *testing.T) {
	t.Parallel()
	t.Run("IntID", func(t *testing.T) {
		t.Parallel()
		intID := NoIntID{}.IntID()
		if intID != 0 {
			t.Fatalf("intID != 0: %v", intID)
		}
	})
	t.Run("SetIntID", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if err := recover(); err == nil {
				t.Errorf("Panic expected")
			}
		}()
		(&NoIntID{}).SetIntID(123)
	})
}
