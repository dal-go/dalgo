package dal

import (
	"fmt"
	"reflect"

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

// NewCollectionRef returns a collection reference with the given name, alias, and parent.
// An empty name panics. An alias equal to the name is stored as empty.
func NewCollectionRef(name, alias string, parent *record.Key) (collectionRef CollectionRef) {
	return newCollectionRef(name, alias, parent)
}

// NewRootCollectionRef returns a collection reference with no parent.
// An empty name panics. An alias equal to the name is stored as empty.
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

// NewDatabaseCollectionRef identifies a collection in a named database.
// Database is a routing identifier; schema remains the provider's namespace.
func NewDatabaseCollectionRef(database, schema, name, alias string) CollectionRef {
	if database == "" {
		panic("database is required parameter for NewDatabaseCollectionRef()")
	}
	ref := newCollectionRef(name, alias, nil)
	ref.database = database
	ref.schema = schema
	return ref
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
	database string
	scan     *collectionScan
	schema   string
	name     string
	alias    string
	parent   *record.Key
}

type collectionScan struct {
	limit  int
	orders []OrderExpression
}

// WithScan orders and bounds a relation before it participates in a JOIN.
func (v CollectionRef) WithScan(limit int, orders ...OrderExpression) CollectionRef {
	v.scan = &collectionScan{limit: limit, orders: append([]OrderExpression(nil), orders...)}
	return v
}

// ScanLimit returns the scan limit set by WithScan, or 0 when no scan is set.
func (v CollectionRef) ScanLimit() int {
	if v.scan == nil {
		return 0
	}
	return v.scan.limit
}

// ScanOrders returns a copy of the scan orders set by WithScan, or nil when no scan is set.
func (v CollectionRef) ScanOrders() []OrderExpression {
	if v.scan == nil {
		return nil
	}
	return append([]OrderExpression(nil), v.scan.orders...)
}

// Equal reports whether v and other have the same database, schema, name, parent, and scan, and the same alias unless ignoreAlias is set.
func (v CollectionRef) Equal(other CollectionRef, ignoreAlias bool) bool {
	return v.database == other.database && v.schema == other.schema && v.name == other.name && v.parent == other.parent && reflect.DeepEqual(v.scan, other.scan) && (ignoreAlias || v.alias == other.alias)
}

func (CollectionRef) recordsetSource() {
	_ = 0 // marker statement
}

// Name returns the collection name.
func (v CollectionRef) Name() string {
	return v.name
}

// Schema returns the database schema or namespace for a qualified root collection, or an empty string otherwise.
func (v CollectionRef) Schema() string {
	return v.schema
}

// Database returns the optional named database used by a federated executor.
func (v CollectionRef) Database() string { return v.database }

// Alias returns the collection alias.
func (v CollectionRef) Alias() string {
	return v.alias
}

// Parent returns the parent key, or nil for a root collection.
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

// Path returns the collection path.
// With no parent it is the name, prefixed by the schema and a dot when a schema is set.
// With a parent it is the parent key's string, a slash, and the name.
func (v CollectionRef) Path() string {
	if v.parent == nil {
		if v.schema != "" {
			return v.schema + "." + v.name
		}
		return v.name
	}
	return v.parent.String() + "/" + v.name
}
