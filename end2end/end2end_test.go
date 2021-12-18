package end2end

import "testing"

func TestEndToEnd(t *testing.T) {

	defer func() {
		if err := recover(); err == nil {
			t.Fatal("should panic on nil parameters")
		}
	}()
	TestDalgoDB(nil, nil)
}
