package dal

import "errors"

// SelectAllIDs is a helper method that for a given reader returns all IDs as a strongly typed slice.
func SelectAllIDs[T comparable](reader Reader, limit int) (ids []T, err error) {
	if reader == nil {
		panic("reader is a required parameter, got nil")
	}
	ids = make([]T, 0, limit)
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
	return ids, reader.Close()
}

// SelectAllRecords	is a helper method that for a given reader returns all records as a slice.
func SelectAllRecords(reader Reader, limit int) (records []Record, err error) {
	records = make([]Record, 0, limit)
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
	return records, reader.Close()
}
