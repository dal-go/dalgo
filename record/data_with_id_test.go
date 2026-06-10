package record_test

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
)

// TestNewDataWithID covers the backward-compatible record.NewDataWithID
// forwarder. The full validation matrix is tested in package dal
// (TestNewRecordWithDataAndID).
func TestNewDataWithID(t *testing.T) {
	type data struct{ Title string }
	key := dal.NewKeyWithID("r1", "SomeCollection")
	d := &data{Title: "test"}

	got := record.NewDataWithID("r1", key, d) // returns record.DataWithID = dal.RecordWithDataAndID alias

	assert.Equal(t, "r1", got.ID)
	assert.Equal(t, key, got.Key)
	assert.Equal(t, d, got.Data)

	// The forwarder still applies dal's validation (nil key panics).
	assert.PanicsWithValue(t, "key is nil for (id=r1)", func() {
		record.NewDataWithID("r1", nil, d)
	})
}
