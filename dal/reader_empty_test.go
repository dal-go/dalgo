package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmptyReader(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		reader := &EmptyReader{}
		err := reader.Close()
		assert.Nil(t, err)
	})
	t.Run("Cursor", func(t *testing.T) {
		reader := &EmptyReader{}
		cursor, err := reader.Cursor()
		assert.Equal(t, ErrNotSupported, err)
		assert.Equal(t, "", cursor)
	})
	t.Run("Next", func(t *testing.T) {
		reader := &EmptyReader{}
		record, err := reader.Next()
		assert.Equal(t, ErrNoMoreRecords, err)
		assert.Nil(t, record)
	})
}
