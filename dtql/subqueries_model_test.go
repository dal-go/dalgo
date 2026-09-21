package dtql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// TestSubqueryFixturesDeserializeAndRoundTrip proves that the shared recursive
// wire contract is understood by the Go model before an executor is involved.
func TestSubqueryFixturesDeserializeAndRoundTrip(t *testing.T) {
	const directory = "testdata/subqueries"
	data, err := os.ReadFile(filepath.Join(directory, "suite.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite subqueryFixtureSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range suite.Cases {
		if fixture.Input == "" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(directory, fixture.Input))
			if err != nil {
				t.Fatal(err)
			}
			query, err := Deserialize(input)
			if strings.HasPrefix(fixture.Name, "scope-") && fixture.Name != "scope-ambiguous" && fixture.Error != "" {
				if err == nil {
					t.Fatal("Deserialize succeeded for an expected scope error fixture")
				}
				return
			}
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			encoded, err := Serialize(query)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			roundTripped, err := Deserialize(encoded)
			if err != nil {
				t.Fatalf("Deserialize serialized query: %v\n%s", err, encoded)
			}
			if !Equal(query, roundTripped) {
				t.Fatalf("recursive query changed after round trip\n%s", encoded)
			}
		})
	}
}

func TestPointerDerivedSourceRoundTripsAndNilSourceIsRejected(t *testing.T) {
	inner := dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(dal.Column{Expression: dal.NewFieldRef("i", "id")})
	source := dal.NewQuerySource(inner, "d")
	query := dal.From(&source).NewQuery().SelectColumns(dal.Column{Expression: dal.NewFieldRef("d", "id")})
	encoded, err := Serialize(query)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped, err := Deserialize(encoded)
	if err != nil || !Equal(query, roundTripped) {
		t.Fatalf("pointer derived round trip query=%v equal=%v err=%v", roundTripped, Equal(query, roundTripped), err)
	}
	var nilSource *dal.QuerySource
	invalid := dal.From(nilSource).NewQuery().SelectIntoRecord(nil)
	if _, err := Serialize(invalid); err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("nil derived source error = %v", err)
	}
}

func TestRecursiveWireHelpersRejectMalformedNestedForms(t *testing.T) {
	inner := document{From: fromYAML{Name: "Invoice"}}
	for name, run := range map[string]func() error{
		"source needs a name":              func() error { _, err := fromFromYAML(fromYAML{}, "from.joins[0].from"); return err },
		"source cannot mix query and name": func() error { _, err := fromFromYAML(fromYAML{Name: "Invoice", Query: &inner}, "from"); return err },
		"query source needs as":            func() error { _, err := fromFromYAML(fromYAML{Query: &inner}, "from"); return err },
		"query source cannot set alias": func() error {
			_, err := fromFromYAML(fromYAML{Query: &document{From: fromYAML{Name: "Invoice"}, As: "d"}, Alias: "bad"}, "from")
			return err
		},
		"invalid column expression":   func() error { _, err := columnsFromYAML([]columnYAML{{}}, fromYAML{Name: "Invoice"}); return err },
		"invalid ordering expression": func() error { _, err := orderFromYAML([]orderYAML{{}}); return err },
		"invalid direct expression":   func() error { _, err := exprFromYAML(exprYAML{}); return err },
		"invalid comparison":          func() error { _, err := comparisonFromYAML(condYAML{Op: "=="}); return err },
		"empty group":                 func() error { _, err := groupFromYAML(dal.And, nil); return err },
		"exists query required":       func() error { _, err := condFromYAML(condYAML{Exists: &existsYAML{}}); return err },
		"not exists query required":   func() error { _, err := condFromYAML(condYAML{NotExists: &existsYAML{}}); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("malformed form was accepted")
			}
		})
	}
	orders, err := orderFromYAML([]orderYAML{{exprYAML: exprYAML{Field: "id"}}, {exprYAML: exprYAML{Field: "created"}, Desc: true}})
	if err != nil || len(orders) != 2 || orders[0].Descending() || !orders[1].Descending() {
		t.Fatalf("orders = %#v, %v", orders, err)
	}
}

func TestRecursiveSerializationHelpersRejectMissingNestedQueries(t *testing.T) {
	inner := dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	for name, run := range map[string]func() error{
		"derived source query": func() error {
			_, err := fromToYAML(dal.From(dal.NewQuerySource(nil, "d")))
			return err
		},
		"derived source alias": func() error {
			_, err := fromToYAML(dal.From(dal.NewQuerySource(inner, "")))
			return err
		},
		"scalar query": func() error {
			_, err := exprToYAML(dal.NewQueryExpression(nil, "value"))
			return err
		},
		"exists query": func() error {
			_, err := condToYAML(dal.NewExistsCondition(nil))
			return err
		},
		"not exists query": func() error {
			_, err := condToYAML(dal.NewNotExistsCondition(nil))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("missing nested query was accepted")
			}
		})
	}
	var nilSource *dal.QuerySource
	if source, ok := derivedQuerySource(nilSource); !ok || source.Query() != nil {
		t.Fatalf("nil pointer derived source = %#v, %v", source, ok)
	}
	if _, ok := derivedQuerySource(dal.NewRootCollectionRef("Invoice", "i")); ok {
		t.Fatal("root collection reported as derived source")
	}
}

func TestRecursiveNestedErrorPathsAndJoinFieldNoop(t *testing.T) {
	badQuery := document{From: fromYAML{Query: &document{From: fromYAML{}}}}
	badQuery.As = "d"
	if _, err := fromFromYAML(fromYAML{Query: &badQuery}, "from"); err == nil {
		t.Fatal("invalid derived source was accepted")
	}
	if _, err := exprFromYAMLAt(exprYAML{Query: &badQuery}, "columns[0]"); err == nil {
		t.Fatal("invalid scalar query was accepted")
	}
	if _, err := condFromYAMLAt(condYAML{Exists: &existsYAML{Query: &badQuery}}, "where"); err == nil {
		t.Fatal("invalid EXISTS query was accepted")
	}
	if _, err := condFromYAMLAt(condYAML{NotExists: &existsYAML{Query: &badQuery}}, "where"); err == nil {
		t.Fatal("invalid NOT EXISTS query was accepted")
	}
	if pathPrefix("columns[0]") != "columns[0]." {
		t.Fatal("nested path prefix changed")
	}
	if err := validateJoinClauseFields(fakeQuery{}); err != nil {
		t.Fatalf("join field validation without a FROM: %v", err)
	}
	derived := dal.NewQuerySource(dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil), "d")
	if fromEqual(dal.From(derived), dal.From(dal.NewRootCollectionRef("Invoice", "i"))) {
		t.Fatal("derived and root sources compared equal")
	}
}

func TestRecursiveYAMLNodeValidationRejectsNestedInvalidShapes(t *testing.T) {
	mapping := func(values ...*yaml.Node) *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Content: values} }
	scalar := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value} }
	queryScalar := mapping(scalar("query"), scalar("bad"))
	if err := validateFromYAMLNode(queryScalar); err == nil {
		t.Fatal("scalar query source accepted")
	}
	badFrom := mapping(scalar("from"), scalar("bad"))
	document := mapping(scalar("query"), mapping(scalar("from"), badFrom))
	if err := validateFromYAMLNode(document); err == nil {
		t.Fatal("nested invalid from accepted")
	}
	// A nested document also visits mapping-valued content while checking
	// aliases. Its nested FROM must retain the same shape rules.
	nestedItem := mapping(scalar("from"), scalar("bad"))
	nestedQuery := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{nestedItem}}
	if err := validateFromYAMLNode(mapping(scalar("query"), nestedQuery)); err == nil {
		t.Fatal("nested alias-walk from accepted")
	}
	if err := validateDocumentYAMLNode(scalar("bad"), map[*yaml.Node]bool{}, map[*yaml.Node]bool{}); err == nil {
		t.Fatal("scalar document accepted")
	}
	valid := mapping(scalar("from"), mapping(scalar("name"), scalar("Invoice")))
	validated := map[*yaml.Node]bool{valid: true}
	if err := validateDocumentYAMLNode(valid, map[*yaml.Node]bool{}, validated); err != nil {
		t.Fatal(err)
	}
	if err := validateDocumentYAMLNode(valid, map[*yaml.Node]bool{valid: true}, map[*yaml.Node]bool{}); err == nil {
		t.Fatal("recursive document alias accepted")
	}
}

func TestDocumentToQueryWrapsNestedScopeError(t *testing.T) {
	doc := document{
		From: fromYAML{Name: "Customer", Alias: "c"},
		Where: &condYAML{Exists: &existsYAML{Query: &document{
			From: fromYAML{Name: "Invoice", Alias: "i"},
			Where: &condYAML{
				Op:    "==",
				Left:  &exprYAML{Field: "customer", Source: "missing"},
				Right: &exprYAML{Field: "id", Source: "i"},
			},
		}}},
	}
	if _, err := documentToQuery(doc); err == nil || !strings.Contains(err.Error(), "invalid DTQL: query_scope") {
		t.Fatalf("nested scope error = %v", err)
	}
}

func TestRecursiveSerializationPropagatesNestedQueryErrors(t *testing.T) {
	invalid := fakeQuery{}
	for name, run := range map[string]func() error{
		"derived source": func() error {
			_, err := fromToYAML(dal.From(dal.NewQuerySource(invalid, "d")))
			return err
		},
		"scalar expression": func() error {
			_, err := exprToYAML(dal.NewQueryExpression(invalid, "value"))
			return err
		},
		"exists condition": func() error {
			_, err := condToYAML(dal.NewExistsCondition(invalid))
			return err
		},
		"not exists condition": func() error {
			_, err := condToYAML(dal.NewNotExistsCondition(invalid))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("invalid nested query serialized")
			}
		})
	}
}

func TestDerivedSourceSerializationPreservesJoinedChild(t *testing.T) {
	inner := dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	from := dal.From(dal.NewQuerySource(inner, "d")).Join(dal.NewJoinedSource(
		dal.NewRootCollectionRef("Payment", "p"), dal.JoinLeft,
		dal.NewComparison(dal.NewFieldRef("d", "id"), dal.Equal, dal.NewFieldRef("p", "invoice")),
	))
	encoded, err := fromToYAML(from)
	if err != nil || len(encoded.Joins) != 1 || encoded.Joins[0].From == nil || encoded.Joins[0].Type != "left" {
		t.Fatalf("derived joined source = %#v, %v", encoded, err)
	}
}

func TestDerivedJoinSerializationPropagatesChildAndPredicateErrors(t *testing.T) {
	inner := dal.From(dal.NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	derived := dal.NewQuerySource(inner, "d")
	badChild := dal.From(derived).Join(dal.NewJoinedSource(dal.NewCollectionGroupRef("Payment", "p"), dal.JoinInner))
	if _, err := fromToYAML(badChild); err == nil {
		t.Fatal("unsupported derived join child serialized")
	}
	badCondition := dal.From(derived).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("Payment", "p"), dal.JoinInner, unsupportedCond{}))
	if _, err := fromToYAML(badCondition); err == nil {
		t.Fatal("unsupported derived join predicate serialized")
	}
}
