package dtql

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

const chinookNestedDTQLSHA256 = "2eeef93ffb02a4272f06f6ab909b2db4cbbb9a2a1df280ae4c47851534f583b3"
const chinookNestedRowsSHA256 = "9f5b32d506541afd8f651d5474f87b44d2e8646d1410e3a03d03c70fccf4a757"

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
		{"forward alias", "from:\n  name: A\n  alias: a\n  joins:\n    - from: {name: B, alias: b}\n      on: [{left: {field: id, source: a}, op: '==', right: {field: cId, source: c}}]\n", "join_scope at from.joins[0].on[0]"},
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
