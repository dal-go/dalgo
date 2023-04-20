package dal

import "fmt"

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	Name   string
	Alias  string
	Parent *Key
}

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
