package dal

import (
	"context"
	"math"
)

// ReadAll reads all records from a reader
func ReadAll(_ context.Context, reader Reader, limit int) (records []Record, err error) {
	var record Record
	if limit <= 0 {
		limit = math.MaxInt64
	}
	for i := 0; i < limit; i++ {
		if i >= limit {
			break
		}
		if record, err = reader.Next(); err != nil {
			if err == ErrNoMoreRecords {
				break
			}
			records = append(records, record)
		}
	}
	return records, err
}
