package dal

import "fmt"

var _ RecordsetSource = (*CollectionGroupRef)(nil)

type CollectionGroupRef struct {
	name string
}

func (CollectionGroupRef) recordsetSource() {
}

func (v CollectionGroupRef) Name() string {
	return v.name
}
func (v CollectionGroupRef) Alias() string {
	return v.name
}

func NewCollectionGroupRef(name string) CollectionGroupRef {
	if name == "" {
		panic("name of recordsetSource group reference cannot be empty")
	}
	return CollectionGroupRef{
		name: name,
	}
}

func (v CollectionGroupRef) String() string {
	return fmt.Sprintf("%T{name=%s}", v, v.name)
}
