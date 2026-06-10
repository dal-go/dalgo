package record_test

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
)

// TestNewWithID covers the backward-compatible record.NewWithID forwarder.
// The full RecordWithID behavior is tested in package dal.
func TestNewWithID(t *testing.T) {
	type data struct{ Title string }
	key := dal.NewKeyWithID("things", "t1")
	d := &data{Title: "x"}

	got := record.NewWithID("t1", key, d) // returns record.WithID = dal.RecordWithID alias

	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, key, got.Key)
	assert.NotNil(t, got.Record)
	got.Record.SetError(nil)
	assert.Equal(t, d, got.Record.Data())
}
