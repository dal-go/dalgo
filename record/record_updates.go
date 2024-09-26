package record

import "github.com/dal-go/dalgo/dal"

// Updates defines updates for a record
type Updates struct {
	Record  dal.Record
	Updates []dal.Update
}
