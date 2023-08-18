package dal

// Changes accumulates DB changes
type Changes struct {
	records []Record
}

// IsChanged returns true if entity changed
func (changes *Changes) IsChanged(record Record) bool {
	for _, r := range changes.records {
		if r == record {
			return true
		} else if EqualKeys(r.Key(), record.Key()) {
			return true
		}
	}
	return false
}

// FlagAsChanged flags a record as changed
func (changes *Changes) FlagAsChanged(record Record) {
	if record == nil {
		panic("record == nil")
	}
	record.MarkAsChanged()
	for _, r := range changes.records {
		if r == record {
			return
		} else if EqualKeys(record.Key(), r.Key()) {
			return
		}
	}
	changes.records = append(changes.records, record)
}

// Records returns list of entity holders
func (changes *Changes) Records() (records []Record) {
	records = make([]Record, len(changes.records))
	copy(records, changes.records)
	return records
	//return changes.records[:]
}

// HasChanges returns true if there are changes
func (changes *Changes) HasChanges() bool {
	// TODO: should it be simply len(changes.records) > 0 ?
	for _, r := range changes.records {
		if r.HasChanged() {
			return true
		}
	}
	return false
}

// ChangedRecords returns slice of changed records
func (changes *Changes) ChangedRecords() (changed []Record) {
	// TODO: Can we remove this method if records are always marked as changed?
	for _, r := range changes.records {
		if r.HasChanged() {
			changed = append(changed, r)
		}
	}
	return changed
}
