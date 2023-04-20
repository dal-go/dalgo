package dal

import "fmt"

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	Name   string
	Alias  string
	Parent *Key
}

//func (v CollectionRef) Name() string {
//	return v.Name
//}
//
//func (v CollectionRef) Alias() string {
//	return v.Alias
//}
//
//func (v CollectionRef) Parent() *Key {
//	return v.Parent
//}

func (v CollectionRef) String() string {
	if v.Name != "" {
		if v.Parent == nil {
			if v.Alias == "" {
				return v.Name
			} else {
				return fmt.Sprintf("%s AS %s", v.Name, v.Alias)
			}
		}
	}
	path := v.Path()
	if v.Alias == "" {
		return path
	}
	return fmt.Sprintf("%s AS %s", path, v.Alias)
}

func (v CollectionRef) Path() string {
	if v.Parent == nil {
		return v.Name
	}
	return v.Parent.String() + "/" + v.Name
}

func NewCollectionRef(name string, alias string, parent *Key) CollectionRef {
	if name == "" {
		panic("Name is required parameter for NewCollectionRef()")
	}
	if alias == name {
		alias = ""
	}
	return CollectionRef{
		Name:   name,
		Alias:  alias,
		Parent: parent,
	}
}
