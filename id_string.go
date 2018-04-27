package db

// NoIntID is base struct intended to be used by entities to specify they have no integer ID
type NoIntID struct{}

// IntID returns integer ID of an entity
func (NoIntID) IntID() int64 {
	return 0
}

// SetIntID sets integer ID of an entity
func (*NoIntID) SetIntID(id int64) {
	panic("Integer ID is not supported")
}

// NewStrID creates new string ID
func NewStrID(id string) StringID {
	return StringID{ID: id}
}

// StringID is base struct intended to be used by entities to specify they have string ID
type StringID struct {
	NoIntID
	ID string
}

// SetStrID sets string ID
func (v *StringID) SetStrID(id string) {
	v.ID = id
}

// StrID returns string ID
func (v StringID) StrID() string {
	return v.ID
}

// TypeOfID returns IsStringID
func (v StringID) TypeOfID() TypeOfID {
	return IsStringID
}
