package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWithParentKey(t *testing.T) {
	t.Run("nil_should_panic", func(t *testing.T) {
		assert.Panics(t, func() {
			WithParentKey(nil)
		})
	})
	t.Run("not_nil_should_pass", func(t *testing.T) {
		parentKey := NewKeyWithID("parent1", "id1")
		option := WithParentKey(parentKey)
		key := new(Key)
		err := option(key)
		assert.Nil(t, err)
		assert.Same(t, parentKey, key.parent)
	})
}

func TestWithStringID(t *testing.T) {
	const id = "id1"
	option := WithStringID(id)
	key := new(Key)
	err := option(key)
	assert.Nil(t, err)
	assert.Equal(t, id, key.ID)
}

func TestWithIntID(t *testing.T) {
	const id = 123
	option := WithIntID(id)
	key := new(Key)
	err := option(key)
	assert.Nil(t, err)
	assert.Equal(t, id, key.ID)
}
