package db

// NoStrID is base struct intended to be used by entities to specify they have no string ID
type NoStrID struct{}

// StrID returns string ID of an entity
func (NoStrID) StrID() string {
	return ""
}

// SetStrID sets string ID of an entity
func (*NoStrID) SetStrID(id string) {
	panic("String ID is not supported")
}

// IntegerID is base struct intended to be used by entities to specify they have integer ID
type IntegerID struct {
	ID int64
	NoStrID
}

// NewIntID creates new integer ID
func NewIntID(id int64) IntegerID {
	return IntegerID{ID: id}
}

// TypeOfID returns IsIntID
func (v IntegerID) TypeOfID() TypeOfID {
	return IsIntID
}

// SetIntID sets integer ID
func (v *IntegerID) SetIntID(id int64) {
	v.ID = id
}

// IntID returns integer ID of an entity
func (v IntegerID) IntID() int64 {
	return v.ID
}
