package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQualifiedRootCollectionRef(t *testing.T) {
	t.Run("requires schema", func(t *testing.T) {
		assert.Panics(t, func() {
			NewQualifiedRootCollectionRef("", "Customer", "")
		})
	})

	ref := NewQualifiedRootCollectionRef("main", "Customer", "c")
	assert.Equal(t, "main", ref.Schema())
	assert.Equal(t, "Customer", ref.Name())
	assert.Equal(t, "c", ref.Alias())
	assert.Nil(t, ref.Parent())
	assert.Equal(t, "main.Customer", ref.Path())
	assert.Equal(t, "main.Customer AS c", ref.String())
}

func TestCollectionRefSchemaCompatibility(t *testing.T) {
	t.Run("unqualified reference", func(t *testing.T) {
		ref := NewRootCollectionRef("Customer", "")
		assert.Empty(t, ref.Schema())
		assert.Equal(t, "Customer", ref.Name())
		assert.Equal(t, "Customer", ref.Path())
	})

	t.Run("dotted name remains an unqualified name", func(t *testing.T) {
		ref := NewRootCollectionRef("main.Customer", "")
		assert.Empty(t, ref.Schema())
		assert.Equal(t, "main.Customer", ref.Name())
		assert.Equal(t, "main.Customer", ref.Path())
	})

	t.Run("equality includes schema", func(t *testing.T) {
		mainCustomer := NewQualifiedRootCollectionRef("main", "Customer", "c")
		assert.True(t, mainCustomer.Equal(NewQualifiedRootCollectionRef("main", "Customer", "c"), false))
		assert.True(t, mainCustomer.Equal(NewQualifiedRootCollectionRef("main", "Customer", "other"), true))
		assert.False(t, mainCustomer.Equal(NewQualifiedRootCollectionRef("dbo", "Customer", "c"), false))
		assert.False(t, mainCustomer.Equal(NewRootCollectionRef("Customer", "c"), false))
	})
}
