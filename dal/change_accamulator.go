package dal

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

func equalKeys(k1, k2 *Key) bool {
	if k1.Collection() != k2.Collection() {
		return false
	}
	p1 := k1.Parent()
	p2 := k2.Parent()
	if p1 != nil && p2 != nil {
		if !equalKeys(p1, p2) {
			return false
		}
	}
	if k1.ID == nil && k2.ID != nil || k2.ID == nil && k1.ID != nil {
		return false
	}
	if k1.ID != k2.ID {
		return false
	}
	return true
}
