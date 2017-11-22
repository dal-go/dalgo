package db

type NoStrID struct {}

func (NoStrID) StrID() string {
	return ""
}

func (*NoStrID) SetStrID(id string) {
	panic("String ID is not supported")
}

type NoIntID struct {}

func (_ NoIntID) IntID() int64 {
	return 0
}

func (_ *NoIntID) SetIntID(id int64) {
	panic("Integer ID is not supported")
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
