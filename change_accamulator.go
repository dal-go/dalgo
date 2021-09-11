package dalgo

// Changes accumulates DB changes
type Changes struct {
	entityHolders []Record
}

// IsChanged returns true if entity changed
func (changes Changes) IsChanged(entityHolder Record) bool {
	for i := range changes.entityHolders {
		if changes.entityHolders[i] == entityHolder {
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
	for _, r := range changes.entityHolders {
		if r == record {
			return
		} else if equalKeys(record.Key(), r.Key()) {
			return
		}
	}
	changes.entityHolders = append(changes.entityHolders, record)
}

// EntityHolders returns list of entity holders
func (changes Changes) EntityHolders() (entityHolders []Record) {
	entityHolders = make([]Record, len(changes.entityHolders))
	copy(entityHolders, changes.entityHolders)
	return
}

// HasChanges returns true if there are changes
func (changes Changes) HasChanges() bool {
	return len(changes.entityHolders) > 0
}

func equalKeys(k1, k2 RecordKey) bool {
	if len(k1) != len(k2) {
		return false
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			return false
		}
	}
	return true
}
