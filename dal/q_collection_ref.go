package dal

import "fmt"

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	name   string
	alias  string
	parent *Key
}

func (v CollectionRef) Name() string {
	return v.name
}

func (v CollectionRef) Alias() string {
	return v.alias
}

func (v CollectionRef) Parent() *Key {
	return v.parent
}

func (v CollectionRef) String() string {
	if v.name != "" {
		if v.parent == nil {
			if v.alias == "" {
				return v.name
			} else {
				return fmt.Sprintf("%s AS %s", v.name, v.alias)
			}
		}
	}
	path := v.Path()
	if v.alias == "" {
		return path
	}
	return fmt.Sprintf("%s AS %s", path, v.alias)
}

func (v CollectionRef) Path() string {
	if v.parent == nil {
		return v.name
	}
	return v.parent.String() + "/" + v.name
}

func newCollectionRef(name, alias string) CollectionRef {
	if name == "" {
		panic("Field is required parameter for NewCollectionRef()")
	}
	if alias == name {
		alias = ""
	}
	return CollectionRef{
		name:  name,
		alias: alias,
	}
}

func NewCollectionRef(name, alias string, parent *Key) (collectionRef CollectionRef) {
	collectionRef = newCollectionRef(name, alias)
	collectionRef.parent = parent
	return
}

func NewRootCollectionRef(name, alias string) CollectionRef {
	return newCollectionRef(name, alias)
}
