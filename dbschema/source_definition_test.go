package dbschema

import (
	"encoding/json"
	"testing"
)

func TestSourceDefinitionJSONPreservesAbsentAndExplicitNullDefaults(t *testing.T) {
	explicitNull := "NULL"
	definition := CollectionDef{SourceDefinition: &SourceDefinition{
		Dialect:   "sqlite",
		CreateSQL: `CREATE TABLE item (id TEXT PRIMARY KEY, note TEXT DEFAULT NULL)`,
		Columns: []SourceColumnDef{
			{Name: "id", DeclaredType: "TEXT", PrimaryKeyPosition: 1},
			{Name: "note", DeclaredType: "TEXT", DefaultSQL: &explicitNull},
		},
		Indexes: []SourceIndexDef{{Name: "item_expr", CreateSQL: `CREATE INDEX item_expr ON item(lower(id))`,
			Columns: []SourceIndexColumnDef{{Position: 0, Key: true, Collation: "BINARY"}}}},
	}}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CollectionDef
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SourceDefinition == nil || decoded.SourceDefinition.Columns[0].DefaultSQL != nil ||
		decoded.SourceDefinition.Columns[1].DefaultSQL == nil || *decoded.SourceDefinition.Columns[1].DefaultSQL != "NULL" ||
		decoded.SourceDefinition.Indexes[0].Columns[0].Name != nil {
		t.Fatalf("source metadata round trip: %+v", decoded.SourceDefinition)
	}
}
