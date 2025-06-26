package dal

import "fmt"

var _ RecordsetSource = (*CollectionGroupRef)(nil)

type CollectionGroupRef struct {
	name  string
	alias string
}

func (CollectionGroupRef) recordsetSource() {
}

func (v CollectionGroupRef) Name() string {
	return v.name
}
func (v CollectionGroupRef) Alias() string {
	return v.alias
}

func NewCollectionGroupRef(name, alias string) CollectionGroupRef {
	if name == "" {
		panic("name of collection group reference cannot be empty")
	}
	return CollectionGroupRef{
		name:  name,
		alias: alias,
	}
}

func (v CollectionGroupRef) String() string {
	if v.alias == "" {
		return fmt.Sprintf(`%T{name="%s"}`, v, v.name)
	}
	return fmt.Sprintf(`%T{name="%s",alias="%s"}`, v, v.name, v.alias)
}
