package dal

import "fmt"

var _ RecordsetSource = (*CollectionRef)(nil)

//type ICollectionRef interface {
//	fmt.Stringer
//	Name() string
//	Alias() string
//	Parent() *Key
//}
//
//var _ ICollectionRef = (*CollectionRef)(nil)

func NewCollectionRef(name, alias string, parent *Key) (collectionRef CollectionRef) {
	return newCollectionRef(name, alias, parent)
}

func NewRootCollectionRef(name, alias string) CollectionRef {
	return newCollectionRef(name, alias, nil)
}

func newCollectionRef(name, alias string, parent *Key) CollectionRef {
	if name == "" {
		panic("name is required parameter for NewCollectionRef()")
	}
	if alias == name {
		alias = ""
	}
	return CollectionRef{
		name:   name,
		alias:  alias,
		parent: parent,
	}
}

// CollectionRef points to a recordsetSource (e.g. table) in a database
type CollectionRef struct {
	name   string
	alias  string
	parent *Key
}

func (v CollectionRef) Equal(other CollectionRef, ignoreAlias bool) bool {
	return v.name == other.name && v.parent == other.parent && (ignoreAlias || v.alias == other.alias)
}

func (CollectionRef) recordsetSource() {
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
