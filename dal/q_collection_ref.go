package dal

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	Name   string
	Parent *Key
}

func (v CollectionRef) Path() string {
	if v.Parent == nil {
		return v.Name
	}
	return v.Parent.String() + "/" + v.Name
}
