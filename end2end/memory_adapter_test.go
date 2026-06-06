package end2end_test

import (
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/end2end"
)

func TestDalgo2Memory(t *testing.T) {
	end2end.TestDalgoDB(t, dalgo2memory.NewDB(), nil, false)
}
