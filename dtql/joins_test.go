package dtql

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

const chinookNestedDTQLSHA256 = "2eeef93ffb02a4272f06f6ab909b2db4cbbb9a2a1df280ae4c47851534f583b3"
const chinookNestedRowsSHA256 = "9f5b32d506541afd8f651d5474f87b44d2e8646d1410e3a03d03c70fccf4a757"

func TestJoinNegativeFixtures_DiagnosticParity(t *testing.T) {
	for _, name := range []string{"forward-alias", "unknown-alias"} {
		t.Run(name, func(t *testing.T) {
			document, err := os.ReadFile("testdata/joins/" + name + ".dtql.yaml")
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile("testdata/joins/" + name + ".error.json")
			if err != nil {
				t.Fatal(err)
			}
			var want struct{ Category, Path string }
			if err := json.Unmarshal(expected, &want); err != nil {
				t.Fatal(err)
			}
			_, err = Deserialize(document)
			var diagnostic *dal.JoinValidationError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("want join diagnostic: %v", err)
			}
			if diagnostic.Category != want.Category || diagnostic.Path != want.Path {
				t.Fatalf("want %#v, got %#v", want, diagnostic)
			}
		})
	}
}

func TestJoinFixtureManifestDigests(t *testing.T) {
	data, err := os.ReadFile("testdata/joins/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion int               `json:"schemaVersion"`
		Files         map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("invalid fixture manifest: %#v", manifest)
	}
	entries, err := os.ReadDir("testdata/joins")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(manifest.Files)+1 {
		t.Fatalf("fixture manifest has %d files; directory has %d entries", len(manifest.Files), len(entries)-1)
	}
	for _, entry := range entries {
		if entry.Name() == "manifest.json" {
			continue
		}
		fixture, err := os.ReadFile("testdata/joins/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(fixture))
		if want := manifest.Files[entry.Name()]; got != want {
			t.Fatalf("fixture %s digest = %s, want %s", entry.Name(), got, want)
		}
	}
}

func TestChinookJoinedWildcardFixture_RoundTrips(t *testing.T) {
	data, err := os.ReadFile("testdata/joins/chinook-wildcard.dtql.yaml")
	if err != nil {
		t.Fatal(err)
	}
	q, err := Deserialize(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Columns()) != 3 || q.Columns()[1].Wildcard == nil || q.Columns()[1].Wildcard.Source != "c" {
		t.Fatalf("joined wildcard was lost: %#v", q.Columns())
	}
	encoded, err := Serialize(q)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Deserialize(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(q, got) {
		t.Fatalf("joined wildcard roundtrip changed query:\n%s", encoded)
	}
}

func TestJoinClauseFieldsRequireKnownQualifierWithoutSchema(t *testing.T) {
	const base = "from:\n  name: A\n  alias: a\n  joins:\n    - from: {name: B, alias: b}\n      on: [{left: {field: id, source: a}, op: '==', right: {field: aid, source: b}}]\n"
	tests := []struct{ name, suffix, category, path string }{
		{"unqualified projection", "columns: [{field: id}]\n", "join_field", "columns[0].source"},
		{"unknown projection alias", "columns: [{field: id, source: missing}]\n", "join_scope", "columns[0].source"},
		{"unqualified WHERE", "where: {op: '==', left: {field: id}, right: {value: 1}}\n", "join_field", "where.left.source"},
		{"unqualified GROUP", "groupBy: [{field: id}]\n", "join_field", "groupBy[0].source"},
		{"unqualified HAVING", "having: {op: '==', left: {field: id}, right: {value: 1}}\n", "join_field", "having.left.source"},
		{"unqualified ORDER", "orderBy: [{field: id}]\n", "join_field", "orderBy[0].source"},
		{"unqualified wildcard", "columns: [{wildcard: {exclude: [id]}}]\n", "join_field", "columns[0].source"},
		{"binary left", "columns: [{as: total, binary: {op: '+', left: {field: id}, right: {value: 1}}}]\n", "join_field", "columns[0].left.source"},
		{"binary right", "columns: [{as: total, binary: {op: '+', left: {value: 1}, right: {field: id}}}]\n", "join_field", "columns[0].right.source"},
		{"aggregate argument", "columns: [{as: total, aggregate: {function: sum, args: [{field: id}]}}]\n", "join_field", "columns[0].args[0].source"},
		{"group condition", "where: {and: [{op: '==', left: {field: id}, right: {value: 1}}]}\n", "join_field", "where.conditions[0].left.source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Deserialize([]byte(base + tt.suffix))
			var diagnostic *dal.JoinValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Category != tt.category || diagnostic.Path != tt.path {
				t.Fatalf("want %s at %s, got %v", tt.category, tt.path, err)
			}
		})
	}
	root := dal.From(dal.NewRootCollectionRef("A", "a")).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("B", "b"), dal.JoinInner, dal.NewComparison(dal.NewFieldRef("a", "id"), dal.Equal, dal.NewFieldRef("b", "aid"))))
	if _, err := Serialize(root.NewQuery().SelectColumns(dal.Column{Expression: dal.Field("id")})); err == nil || !strings.Contains(err.Error(), "join_field at columns[0].source") {
		t.Fatalf("Serialize accepted unqualified JOIN field: %v", err)
	}
	if _, err := Serialize(dal.From(dal.NewRootCollectionRef("A", "")).NewQuery().SelectColumns(dal.Column{Expression: dal.Field("id")})); err != nil {
		t.Fatalf("single-source compatibility: %v", err)
	}
}

func TestJoinNestedShapeAndSourceSerializationErrors(t *testing.T) {
	inputs := []struct{ text, want string }{
		{"from: {name: A, joins: [{from: {name: ''}, on: [{left: {field: id, source: A}, op: '==', right: {field: aid, source: B}}]}]}\n", "join_shape at from.joins[0].from.name"},
		{"from: {name: A, joins: [{from: {name: B}, on: [{left: {field: id, source: A}, op: 'bogus', right: {field: aid, source: B}}]}]}\n", "join_shape at from.joins[0].on[0]"},
	}
	for _, input := range inputs {
		_, err := Deserialize([]byte(input.text))
		if err == nil || !strings.Contains(err.Error(), input.want) {
			t.Fatalf("want %q, got %v", input.want, err)
		}
	}
	root := dal.From(dal.NewRootCollectionRef("A", "a"))
	root.Join(dal.NewJoinedSource(dal.NewCollectionGroupRef("B", "b"), dal.JoinInner, dal.NewComparison(dal.NewFieldRef("a", "id"), dal.Equal, dal.NewFieldRef("b", "aid"))))
	if _, err := Serialize(root.NewQuery().SelectIntoRecord(nil)); err == nil || !strings.Contains(err.Error(), "unsupported From source") {
		t.Fatalf("collection group serialized: %v", err)
	}
	legacy := []byte("from: {name: A, alias: a}\ncolumns: [{wildcard: {source: A, exclude: [id]}}]\n")
	if _, err := Deserialize(legacy); err != nil {
		t.Fatalf("legacy root-name wildcard rejected: %v", err)
	}
}

func TestChinookNestedFixture_RoundTripsCanonically(t *testing.T) {
	data, err := os.ReadFile("testdata/joins/chinook-nested.dtql.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != chinookNestedDTQLSHA256 {
		t.Fatalf("fixture sha256 = %s, want %s", got, chinookNestedDTQLSHA256)
	}
	rows, err := os.ReadFile("testdata/joins/chinook-nested.rows.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(rows)); got != chinookNestedRowsSHA256 {
		t.Fatalf("rows sha256 = %s, want %s", got, chinookNestedRowsSHA256)
	}
	if err := validateDTQL(compileSchema(t), data); err != nil {
		t.Fatalf("fixture does not satisfy schema: %v", err)
	}
	query, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	encoded, err := Serialize(query)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	reparsed, err := Deserialize(encoded)
	if err != nil || !Equal(query, reparsed) {
		t.Fatalf("canonical output did not preserve the fixture: %v\n%s", err, encoded)
	}
}

func TestDeserialize_JoinInputAliasesNormalize(t *testing.T) {
	query, err := Deserialize([]byte(`from:
  name: Invoice
  as: i
  joins:
    - from:
        name: Customer
        as: c
      on:
        - left: {field: CustomerId, source: i}
          op: eq
          right: {field: CustomerId, source: c}
`))
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	encoded, err := Serialize(query)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	got := string(encoded)
	if strings.Contains(got, "\n  as:") || strings.Contains(got, "op: eq") || !strings.Contains(got, "alias: i") || !strings.Contains(got, "op: ==") {
		t.Fatalf("non-canonical output:\n%s", got)
	}
}

func TestDeserialize_JoinDiagnostics(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"missing from", "from:\n  name: A\n  joins:\n    - on: []\n", "join_shape at from.joins[0].from"},
		{"empty on", "from:\n  name: A\n  joins:\n    - from: {name: B, alias: b}\n      on: []\n", "join_shape at from.joins[0].on"},
		{"bad type", "from:\n  name: A\n  alias: a\n  joins:\n    - type: right\n      from: {name: B, alias: b}\n      on: [{left: {field: id, source: a}, op: '==', right: {field: aId, source: b}}]\n", "join_type at from.joins[0].type"},
		{"forward alias", "from:\n  name: A\n  alias: a\n  joins:\n    - from: {name: B, alias: b}\n      on: [{left: {field: id, source: a}, op: '==', right: {field: cId, source: c}}]\n", "join_scope at from.joins[0].on[0].right.source"},
		{"both aliases", "from: {name: A, alias: a, as: old}\n", "cannot contain both alias and as"},
		{"unknown join key", "from: {name: A, joins: [{from: {name: B}, on: [], nope: true}]}\n", "not found in join"},
		{"bad join value", "from: {name: A, joins: [wrong]}\n", "join must be a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Deserialize([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Deserialize error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSerialize_RecursiveJoin(t *testing.T) {
	child := dal.From(dal.NewRootCollectionRef("Customer", "c"))
	query := dal.From(dal.NewRootCollectionRef("Invoice", "i")).Join(dal.NewNestedJoinedSource(child, dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("i", "CustomerId"), dal.Equal, dal.NewFieldRef("c", "CustomerId")),
	)).NewQuery().SelectIntoRecordset()
	if _, err := Serialize(query); err != nil {
		t.Fatalf("Serialize: %v", err)
	}
}

func TestRecursiveJoinEqualityDetectsDifferences(t *testing.T) {
	makeQuery := func(kind dal.JoinType, on dal.Condition, nested bool) dal.StructuredQuery {
		child := dal.From(dal.NewRootCollectionRef("B", "b"))
		if nested {
			child.Join(dal.NewJoinedSource(dal.NewRootCollectionRef("C", "c"), dal.JoinInner, dal.NewComparison(dal.NewFieldRef("b", "id"), dal.Equal, dal.NewFieldRef("c", "bId"))))
		}
		return dal.From(dal.NewRootCollectionRef("A", "a")).Join(dal.NewNestedJoinedSource(child, kind, on)).NewQuery().SelectIntoRecordset()
	}
	base := makeQuery(dal.JoinInner, dal.NewComparison(dal.NewFieldRef("a", "id"), dal.Equal, dal.NewFieldRef("b", "aId")), false)
	for _, other := range []dal.StructuredQuery{
		makeQuery(dal.JoinLeft, dal.NewComparison(dal.NewFieldRef("a", "id"), dal.Equal, dal.NewFieldRef("b", "aId")), false),
		makeQuery(dal.JoinInner, dal.NewComparison(dal.NewFieldRef("a", "other"), dal.Equal, dal.NewFieldRef("b", "aId")), false),
		makeQuery(dal.JoinInner, dal.NewComparison(dal.NewFieldRef("a", "id"), dal.Equal, dal.NewFieldRef("b", "aId")), true),
	} {
		if Equal(base, other) {
			t.Fatal("different relation trees compared equal")
		}
	}
}

func TestJoinHelperDiagnosticsAndEquality(t *testing.T) {
	base := dal.From(dal.NewRootCollectionRef("A", ""))
	on := dal.NewComparison(dal.NewFieldRef("A", "id"), dal.Equal, dal.NewFieldRef("B", "aId"))
	base.Join(dal.NewJoinedSource(dal.NewRootCollectionRef("B", ""), dal.JoinInner, on))
	q := base.NewQuery().SelectColumns(dal.Column{Wildcard: &dal.WildcardProjection{Source: "missing", Exclude: []string{"id"}}})
	if err := validateJoinClauseFields(q); err == nil || !strings.Contains(err.Error(), "join_scope at columns[0].source") {
		t.Fatalf("unknown wildcard source: %v", err)
	}
	other := dal.From(dal.NewRootCollectionRef("A", ""))
	other.Join(dal.NewJoinedSource(dal.NewRootCollectionRef("B", ""), dal.JoinInner, on,
		dal.NewComparison(dal.NewFieldRef("A", "other"), dal.Equal, dal.NewFieldRef("B", "other"))))
	if fromEqual(base, other) || fromEqual(other, base) {
		t.Fatal("different ON condition counts compared equal")
	}
	third := dal.From(dal.NewRootCollectionRef("A", ""))
	third.Join(dal.NewJoinedSource(dal.NewRootCollectionRef("C", ""), dal.JoinInner, on))
	if fromEqual(base, third) {
		t.Fatal("different child relation compared equal")
	}
}

func TestJoinYAMLNodeMalformedShapes(t *testing.T) {
	for _, input := range []string{
		"from: {name: A, joins: {bad: true}}\n",
		"from: {name: A, joins: [{from: []}]}\n",
		"from: {name: A, joins: [{from: {joins: {bad: true}}}]}\n",
		"from: {name: A, joins: [{from: {name: B}, on: [{left: 1, op: '==', right: {value: 1}}]}]}\n",
	} {
		if _, err := Deserialize([]byte(input)); err == nil {
			t.Fatalf("malformed JOIN accepted: %s", input)
		}
	}
	var join joinYAML
	if err := join.UnmarshalYAML(&yaml.Node{Kind: yaml.ScalarNode, Value: "bad"}); err == nil {
		t.Fatal("scalar JOIN accepted")
	}
	if err := join.UnmarshalYAML(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "on"}, {Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "bad"}}}}}); err == nil {
		t.Fatal("invalid JOIN ON shape accepted")
	}
}
