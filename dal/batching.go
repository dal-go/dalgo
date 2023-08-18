package dal

//
//func CreateEntityHoldersWithIntIDs(ids []int64, newRecordWithOnlyKey func() RecordWithIntID) (records []Record) {
//	records = make([]Record, len(ids))
//	for i := range records {
//		record := newRecordWithOnlyKey()
//		id := ids[i]
//		if id == 0 {
//			panic(fmt.Sprintf("ids[%v] == 0", i))
//		}
//		record.SetIntID(ids[i])
//		records[i] = record
//	}
//	return
//}
