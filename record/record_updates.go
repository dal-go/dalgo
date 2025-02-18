package record

import (
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
)

// Updates defines updates for a record
type Updates struct {
	Record  dal.Record
	Updates []update.Update
}
