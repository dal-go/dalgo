package dal

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTextQuery_Basic(t *testing.T) {
	// Arrange
	text := "SELECT * FROM books WHERE author = @author AND year > @year"
	args := []QueryArg{{Name: "author", Value: "Asimov"}, {Name: "year", Value: 1950}}

	// getKey uses args to build a key deterministically so we can verify it is stored and usable
	getKey := func(data any, a []QueryArg) *Key {
		// Compose ID from args in order
		id := ""
		for i, qa := range a {
			if i > 0 {
				id += ":"
			}
			id += qa.Name + "=" + toString(qa.Value)
		}
		return NewKeyWithID("queries", id)
	}

	// Act
	tq := NewTextQuery(text, getKey, args...)

	// Assert: interface is not nil and methods work
	assert.NotNil(t, tq)
	assert.Equal(t, text, tq.Text())
	assert.Equal(t, text, tq.String())

	// Args are preserved and in the same order
	gotArgs := tq.Args()
	assert.Equal(t, len(args), len(gotArgs))
	assert.Equal(t, args, gotArgs)

	// Verify the getKey function was stored by asserting to concrete type
	if impl, ok := tq.(*textQuery); assert.True(t, ok, "expected *textQuery implementation") {
		key := impl.getKey(nil, impl.args)
		assert.NotNil(t, key)
		assert.Equal(t, "queries", key.Collection())
		// Rebuild expected id string
		expectedID := "author=Asimov:year=1950"
		assert.Equal(t, expectedID, key.ID)
	}
}

func TestTextQuery_Methods(t *testing.T) {
	tq := &textQuery{
		offset: 10,
		limit:  20,
	}
	assert.Equal(t, 10, tq.Offset())
	assert.Equal(t, 20, tq.Limit())

	t.Run("GetRecordsetReader", func(t *testing.T) {
		_, _ = tq.GetRecordsetReader(context.Background(), mockQueryExecutor{})
	})
	t.Run("GetRecordsReader", func(t *testing.T) {
		_, _ = tq.GetRecordsReader(context.Background(), mockQueryExecutor{})
	})
}

func TestTextQuery_NoArgs(t *testing.T) {
	tq := NewTextQuery("DELETE FROM t WHERE 1=1", func(data any, args []QueryArg) *Key { return nil })
	assert.NotNil(t, tq)
	assert.Equal(t, 0, len(tq.Args()))
}

func TestTextQuery_ArgsAreKeptOrder(t *testing.T) {
	args := []QueryArg{{Name: "a", Value: 1}, {Name: "b", Value: 2}, {Name: "c", Value: 3}}
	tq := NewTextQuery("", nil, args...)
	got := tq.Args()
	assert.Equal(t, args, got)
}

func TestTextQuery_StringEqualsText(t *testing.T) {
	text := "UPDATE t SET x=@x"
	tq := NewTextQuery(text, nil)
	assert.Equal(t, text, tq.Text())
	assert.Equal(t, text, tq.String())
}

// toString is a tiny helper for building IDs in tests
func toString(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case int:
		return fmtInt64(int64(vv))
	case int64:
		return fmtInt64(vv)
	case int32:
		return fmtInt64(int64(vv))
	default:
		return fmtAny(v)
	}
}

func fmtInt64(v int64) string { return fmtAny(v) }

func fmtAny(v any) string { return fmt.Sprintf("%v", v) }
