package dal

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadAll(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		reader               Reader
		shouldPanic          bool
		expectedRecordsCount int
	}{
		{name: "nil_reader_no_limit", reader: nil, shouldPanic: true},
		{name: "empty_reader", reader: &EmptyReader{}, expectedRecordsCount: 0},
		{name: "2records", reader: &RecordsReader{
			records: []Record{
				NewRecord(NewKeyWithID("kind", 1)),
				NewRecord(NewKeyWithID("kind", 2)),
			},
		}, expectedRecordsCount: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("panic expected")
					}
				}()
			}
			records, err := ReadAll(context.Background(), tt.reader, -1)
			assert.Nil(t, err)
			assert.Equal(t, tt.expectedRecordsCount, len(records))
		})
	}
}
