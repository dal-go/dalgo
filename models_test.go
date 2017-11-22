package db

import "testing"

//func TestNoStrID_SetStrID(t *testing.T) {
//	defer func() {
//		if err := recover(); err == nil {
//			t.Errorf("Panic expected")
//		}
//	}()
//	(&NoStrID{}).SetStrID("test")
//}

func TestNoIntID_SetIntID(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Errorf("Panic expected")
		}
	}()
	(&NoIntID{}).SetIntID(123)
}
