package db

type NoStrID struct{}

func (NoStrID) StrID() string {
	return ""
}

func (*NoStrID) SetStrID(id string) {
	panic("String ID is not supported")
}

type IntegerID struct {
	ID int64
	NoStrID
}

func NewIntID(id int64) IntegerID {
	return IntegerID{ID: id}
}

func (v IntegerID) TypeOfID() TypeOfID {
	return IsIntID
}

func (v *IntegerID) SetIntID(id int64) {
	v.ID = id
}

func (v IntegerID) IntID() int64 {
	return v.ID
}

