package dal

import "errors"

// SelectAllIDs is a helper method that for a given reader returns all IDs as a strongly typed slice.
func SelectAllIDs[T comparable](reader Reader, limit int) (ids []T, err error) {
	for i := 0; limit <= 0 || i < limit; i++ {
		var record Record
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return
		}
		ids = append(ids, record.Key().ID.(T))
	}
	return
}

// SelectAllRecords	is a helper method that for a given reader returns all records as a slice.
func SelectAllRecords(reader Reader, limit int) (records []Record, err error) {
	for i := 0; limit <= 0 || i < limit; i++ {
		var record Record
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return
		}
		records = append(records, record)
	}
	return
}
