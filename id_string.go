package db

type NoIntID struct{}

func (NoIntID) IntID() int64 {
	return 0
}

func (*NoIntID) SetIntID(id int64) {
	panic("Integer ID is not supported")
}


func NewStrID(id string) StringID {
	return StringID{ID: id}
}

type StringID struct {
	NoIntID
	ID string
}

func (v *StringID) SetStrID(id string) {
	v.ID = id
}

func (v StringID) StrID() string {
	return v.ID
}

func (v StringID) TypeOfID() TypeOfID {
	return IsStringID
}

