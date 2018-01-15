package db

import "fmt"

type Changes struct {
	entityHolders []EntityHolder
}

func (changes Changes) IsChanged(entityHolder EntityHolder) bool {
	for i := range changes.entityHolders {
		if changes.entityHolders[i] == entityHolder {
			return true
		}
	}
	return false
}

func (changes *Changes) FlagAsChanged(entityHolder EntityHolder) {
	if entityHolder == nil {
		panic("entityHolder == nil")
	}
	entityHolders := changes.entityHolders
	for i := range entityHolders {
		if eh := entityHolders[i]; eh == entityHolder {
			return
		} else if equalKeys(entityHolder, eh) {
			return
		}
	}
	changes.entityHolders = append(changes.entityHolders, entityHolder)
}

func (changes Changes) EntityHolders() (entityHolders []EntityHolder) {
	entityHolders = make([]EntityHolder, len(changes.entityHolders))
	copy(entityHolders, changes.entityHolders)
	return
}

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
