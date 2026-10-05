package end2end

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/stretchr/testify/assert"
)

func TestDenialOf(t *testing.T) {
	denial := &access.DeniedError{Decision: access.Decision{
		Policy:   "sources-allowed",
		Resource: access.CollectionResourceFor(nil, hiddenSourceCollection),
	}}
	for name, c := range map[string]struct {
		err      error
		policy   string
		resource string
	}{
		"a denial":         {denial, "sources-allowed", "/" + hiddenSourceCollection},
		"a wrapped denial": {fmt.Errorf("read: %w", denial), "sources-allowed", "/" + hiddenSourceCollection},
		"another error":    {errors.New("boom"), "", ""},
		"no error":         {nil, "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			policy, resource := denialOf(c.err)
			assert.Equal(t, c.policy, policy)
			assert.Equal(t, c.resource, resource)
		})
	}
}
