package dal

import (
	"fmt"

	"github.com/dal-go/record"
)

var _ RecordsetSource = (*CollectionRef)(nil)

//type ICollectionRef interface {
//	fmt.Stringer
//	Name() string
//	Alias() string
//	Parent() *record.Key
//}
//
//var _ ICollectionRef = (*CollectionRef)(nil)

func NewCollectionRef(name, alias string, parent *record.Key) (collectionRef CollectionRef) {
	return newCollectionRef(name, alias, parent)
}

func NewRootCollectionRef(name, alias string) CollectionRef {
	return newCollectionRef(name, alias, nil)
}

// NewQualifiedRootCollectionRef creates a root collection reference in a database schema or namespace.
// The schema and collection name remain separate so adapters can handle each identifier segment correctly.
func NewQualifiedRootCollectionRef(schema, name, alias string) CollectionRef {
	if schema == "" {
		panic("schema is required parameter for NewQualifiedRootCollectionRef()")
	}
	collectionRef := newCollectionRef(name, alias, nil)
	collectionRef.schema = schema
	return collectionRef
}

func newCollectionRef(name, alias string, parent *record.Key) CollectionRef {
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
	schema string
	name   string
	alias  string
	parent *record.Key
}

func (v CollectionRef) Equal(other CollectionRef, ignoreAlias bool) bool {
	return v.schema == other.schema && v.name == other.name && v.parent == other.parent && (ignoreAlias || v.alias == other.alias)
}

func (CollectionRef) recordsetSource() {
	_ = 0 // marker statement
}

func (v CollectionRef) Name() string {
	return v.name
}

// Schema returns the database schema or namespace for a qualified root collection, or an empty string otherwise.
func (v CollectionRef) Schema() string {
	return v.schema
}

func (v CollectionRef) Alias() string {
	return v.alias
}

func (v CollectionRef) Parent() *record.Key {
	return v.parent
}

func (v CollectionRef) String() string {
	if v.name != "" {
		if v.parent == nil && v.schema == "" {
			if v.alias == "" {
				return v.name
			}
			return fmt.Sprintf("%s AS %s", v.name, v.alias)
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
		if v.schema != "" {
			return v.schema + "." + v.name
		}
		return v.name
	}
	return v.parent.String() + "/" + v.name
}
