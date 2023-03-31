package end2end

import "testing"

func TestEndToEnd(t *testing.T) {

	t.Run("db=nil", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("should panic on nil parameters")
			}
		}()
		TestDalgoDB(t, nil)
	})
}
