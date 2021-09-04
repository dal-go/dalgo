package db

import "fmt"

func CreateEntityHoldersWithIntIDs(ids []int64, newEntityHolder func() EntityHolder) (entityHolders []EntityHolder) {
	entityHolders = make([]EntityHolder, len(ids))
	for i := range entityHolders {
		eh := newEntityHolder()
		id := ids[i]
		if id == 0 {
			panic(fmt.Sprintf("ids[%v] == 0", i))
		}
		eh.SetIntID(ids[i])
		entityHolders[i] = eh
	}
	return
}
