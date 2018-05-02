package db

import "fmt"

// Changes accumulates DB changes
type Changes struct {
	entityHolders []EntityHolder
}

// IsChanged returns true if entity changed
func (changes Changes) IsChanged(entityHolder EntityHolder) bool {
	for i := range changes.entityHolders {
		if changes.entityHolders[i] == entityHolder {
			return true
		}
	}
	return false
}

// FlagAsChanged falgs entity as changed
func (changes *Changes) FlagAsChanged(entityHolder EntityHolder) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	for _, eh := range changes.entityHolders {
		if eh == entityHolder {
			return
		} else if equalKeys(entityHolder, eh) {
			return
		}
	}
	changes.entityHolders = append(changes.entityHolders, entityHolder)
}

// EntityHolders returns list of entity holders
func (changes Changes) EntityHolders() (entityHolders []EntityHolder) {
	entityHolders = make([]EntityHolder, len(changes.entityHolders))
	copy(entityHolders, changes.entityHolders)
	return
}

// HasChanges returns true if there are changes
func (changes Changes) HasChanges() bool {
	return len(changes.entityHolders) > 0
}

func equalKeys(eh1, eh2 EntityHolder) bool {
	if eh1.Kind() == eh2.Kind() {
		switch eh1.TypeOfID() {
		case IsIntID:
			return eh1.IntID() == eh2.IntID()
		case IsStringID:
			return eh1.StrID() == eh2.StrID()
		case IsComplexID:
			panic("complex IDs are not supported yet")
		default:
			panic(fmt.Sprintf("Unknown ID type: %v", eh1.TypeOfID()))
		}
	}
	return false
}
