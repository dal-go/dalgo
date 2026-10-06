package dbschema

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// SourceDefinition preserves provider-native schema facts for an export.
// Dialect identifies the SQL dialect of CreateSQL and index statements.
// These statements are provenance only: consumers must not execute them
// without their own trust and dialect checks. Providers may omit this field
// when they cannot inspect native schema.
type SourceDefinition struct {
	Dialect   string            `json:"dialect"`
	CreateSQL string            `json:"createSql,omitempty"`
	Columns   []SourceColumnDef `json:"columns,omitempty"`
	Indexes   []SourceIndexDef  `json:"indexes,omitempty"`
}

// SourceColumnDef is one source column in declared order. NotNull records
// the source catalog flag, which may differ from effective primary-key null
// semantics. A nil DefaultSQL means no DEFAULT clause; a pointer to "NULL"
// means an explicit DEFAULT NULL.
type SourceColumnDef struct {
	Name               string  `json:"name"`
	DeclaredType       string  `json:"declaredType"`
	NotNull            bool    `json:"notNull"`
	DefaultSQL         *string `json:"defaultSql,omitempty"`
	PrimaryKeyPosition int     `json:"primaryKeyPosition,omitempty"`
}

// SourceIndexDef retains the catalog shape of an index, including expression
// and partial indexes that portable IndexDef cannot fully describe. CreateSQL
// is empty for engine-generated indexes such as SQLite autoindexes.
type SourceIndexDef struct {
	Name      string                 `json:"name"`
	Unique    bool                   `json:"unique"`
	Origin    string                 `json:"origin,omitempty"`
	Partial   bool                   `json:"partial,omitempty"`
	CreateSQL string                 `json:"createSql,omitempty"`
	Columns   []SourceIndexColumnDef `json:"columns,omitempty"`
}

// SourceIndexColumnDef is one index term in storage order. Name is nil for
// expression terms and rowid. Position records the provider's term ordinal;
// Key distinguishes key terms from included or auxiliary terms.
type SourceIndexColumnDef struct {
	Position   int     `json:"position"`
	Name       *string `json:"name,omitempty"`
	Key        bool    `json:"key"`
	Descending bool    `json:"descending,omitempty"`
	Collation  string  `json:"collation,omitempty"`
}

// SourceViewDef retains a provider's view definition as metadata only. A
// view is not a collection and need not have materialized rows in an export.
type SourceViewDef struct {
	Name      string   `json:"name"`
	Columns   []string `json:"columns,omitempty"`
	CreateSQL string   `json:"createSql,omitempty"`
}

// SourceViewReader is an optional schema-introspection capability. It is
// separate from SchemaReader so existing drivers need no API changes.
type SourceViewReader interface {
	ListSourceViews(ctx context.Context) ([]SourceViewDef, error)
}

// SourceRow is one source record with its original storage classes. Values
// may use a lossless transport representation (for example, a canonical
// decimal string for a SQLite REAL stored in a declared NUMERIC column),
// while StorageClasses retains the original source storage class per field.
type SourceRow struct {
	Values         map[string]any    `json:"values"`
	StorageClasses map[string]string `json:"storageClasses,omitempty"`
}

// SourceRowCursor streams source rows without buffering the collection.
// Next returns io.EOF after the last row. The caller must Close the cursor.
type SourceRowCursor interface {
	Next() (SourceRow, error)
	Close() error
}

// SourceRowsReader is an optional export capability for drivers that can
// preserve source storage classes more faithfully than a general query.
// Generic exporters may fall back to dal.QueryExecutor when it is absent.
type SourceRowsReader interface {
	OpenSourceRows(ctx context.Context, ref *dal.CollectionRef) (SourceRowCursor, error)
}
