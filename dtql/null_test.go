package dtql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"gopkg.in/yaml.v3"
)

// nullDocs are canonical documents (byte-identical after a round trip) that use
// the explicit null tests in every position a condition may appear.
var nullDocs = map[string]string{
	"where isNull": `from:
  name: Chat
where:
  isNull:
    field: Company
`,
	"where isNotNull": `from:
  name: Chat
where:
  isNotNull:
    field: Company
`,
	"group": `from:
  name: Chat
  alias: c
where:
  and:
    - isNull:
        field: Company
        source: c
    - or:
        - isNotNull:
            field: Region
            source: c
        - op: ==
          left:
            field: Tier
            source: c
          right:
            value: gold
`,
	"having": `from:
  name: Sale
groupBy:
  - field: category
having:
  isNotNull:
    aggregate:
      function: max
      args:
        - field: amount
columns:
  - field: category
  - aggregate:
      function: max
      args:
        - field: amount
    as: top
`,
	"expression operand": `from:
  name: Sale
where:
  isNull:
    binary:
      op: +
      left:
        field: a
      right:
        field: b
`,
	"joined where": `from:
  name: Chat
  alias: c
  joins:
    - type: left
      from:
        name: Invoice
        alias: i
      "on":
        - op: ==
          left:
            field: id
            source: c
          right:
            field: chat
            source: i
where:
  isNull:
    field: chat
    source: i
columns:
  - field: id
    source: c
`,
}

func TestNullTestCanonicalRoundTrip(t *testing.T) {
	for name, doc := range nullDocs {
		t.Run(name, func(t *testing.T) {
			q, err := Deserialize([]byte(doc))
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			got, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if string(got) != doc {
				t.Fatalf("not byte-identical.\n--- want ---\n%s\n--- got ---\n%s", doc, got)
			}
			again, err := Deserialize(got)
			if err != nil || !Equal(q, again) {
				t.Fatalf("round trip not structurally equal: %v", err)
			}
			// Same document as JSON: DTQL-JSON is plain JSON over the YAML shapes.
			var tree any
			if err := yaml.Unmarshal([]byte(doc), &tree); err != nil {
				t.Fatal(err)
			}
			asJSON, err := json.Marshal(tree)
			if err != nil {
				t.Fatal(err)
			}
			fromJSON, err := Deserialize(asJSON)
			if err != nil {
				t.Fatalf("Deserialize JSON %s: %v", asJSON, err)
			}
			if !Equal(q, fromJSON) {
				t.Fatalf("JSON form differs from YAML form: %s", asJSON)
			}
			if err := validateDTQL(compileSchema(t), []byte(doc)); err != nil {
				t.Fatalf("schema rejects %s: %v", name, err)
			}
		})
	}
}

func TestNullTestModelMapping(t *testing.T) {
	q, err := Deserialize([]byte(nullDocs["where isNotNull"]))
	if err != nil {
		t.Fatal(err)
	}
	cond, ok := q.Where().(dal.IsNullCondition)
	if !ok || !cond.Negated() || cond.Operand().String() != "Company" {
		t.Fatalf("where = %#v", q.Where())
	}
	q, err = Deserialize([]byte(nullDocs["where isNull"]))
	if err != nil {
		t.Fatal(err)
	}
	if cond, ok := q.Where().(dal.IsNullCondition); !ok || cond.Negated() {
		t.Fatalf("where = %#v", q.Where())
	}
	// The existing equality against null stays a comparison; it is not rewritten.
	cmp, err := Deserialize([]byte("from: {name: Chat}\nwhere:\n  op: ==\n  left: {field: Company}\n  right: {value: null}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cmp.Where().(dal.Comparison); !ok {
		t.Fatalf("== null = %#v", cmp.Where())
	}
}

func TestNullTestDeserializeErrors(t *testing.T) {
	cases := map[string]string{
		"empty operand":             "from: {name: Chat}\nwhere:\n  isNull: {}\n",
		"two operand forms":         "from: {name: Chat}\nwhere:\n  isNull: {field: a, value: 1}\n",
		"isNull and isNotNull":      "from: {name: Chat}\nwhere:\n  isNull: {field: a}\n  isNotNull: {field: a}\n",
		"isNull with comparison":    "from: {name: Chat}\nwhere:\n  isNull: {field: a}\n  op: ==\n  left: {field: a}\n  right: {value: 1}\n",
		"isNull with group":         "from: {name: Chat}\nwhere:\n  isNull: {field: a}\n  and: [{isNull: {field: b}}]\n",
		"isNotNull with exists":     "from: {name: Chat}\nwhere:\n  isNotNull: {field: a}\n  exists: {query: {from: {name: X}}}\n",
		"unknown key inside":        "from: {name: Chat}\nwhere:\n  isNull: {field: a, bogus: 1}\n",
		"scalar instead of mapping": "from: {name: Chat}\nwhere:\n  isNull: a\n",
		"unknown alias":             "from: {name: Chat, alias: c, joins: [{from: {name: Invoice, alias: i}, on: [{left: {field: id, source: c}, op: ==, right: {field: chat, source: i}}]}]}\nwhere:\n  isNull: {field: x, source: zzz}\n",
		"isNull in join on":         "from: {name: Chat, alias: c, joins: [{from: {name: Invoice, alias: i}, on: [{isNull: {field: chat, source: i}}]}]}\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			q, err := Deserialize([]byte(doc))
			if err == nil || q != nil {
				t.Fatalf("expected an error and no query, got %v / %v", q, err)
			}
		})
	}
	// An explicit YAML null is "absent", so a lone isNull: null names no condition.
	if _, err := Deserialize([]byte("from: {name: Chat}\nwhere:\n  isNull:\n")); err == nil || !strings.Contains(err.Error(), "isNull") {
		t.Fatalf("lone empty isNull error = %v", err)
	}
	if _, err := condFromYAML(condYAML{IsNull: &exprYAML{}}); err == nil {
		t.Fatal("empty operand accepted")
	}
	if _, err := condFromYAML(condYAML{IsNotNull: &exprYAML{}}); err == nil {
		t.Fatal("empty isNotNull operand accepted")
	}
}

func TestNullTestSerializeErrors(t *testing.T) {
	for name, cond := range map[string]dal.Condition{
		"nil operand":         dal.NewIsNullCondition(nil),
		"nil not-null":        dal.NewIsNotNullCondition(nil),
		"unsupported operand": dal.NewIsNullCondition(unsupportedExpr{}),
		"unsupported nested":  dal.NewIsNotNullCondition(unsupportedExpr{}),
	} {
		t.Run(name, func(t *testing.T) {
			if data, err := Serialize(fakeQuery{from: rootFrom(), where: cond}); err == nil || data != nil {
				t.Fatalf("expected an error, got %s / %v", data, err)
			}
		})
	}
}

func TestNullTestEquality(t *testing.T) {
	isNull := dal.NewIsNullCondition(dal.Field("a"))
	cases := []struct {
		name string
		a, b dal.Condition
		want bool
	}{
		{"same", isNull, dal.NewIsNullCondition(dal.Field("a")), true},
		{"negation differs", isNull, dal.NewIsNotNullCondition(dal.Field("a")), false},
		{"operand differs", isNull, dal.NewIsNullCondition(dal.Field("b")), false},
		{"other kind", isNull, dal.NewComparison(dal.Field("a"), dal.Equal, dal.Constant{Value: nil}), false},
		{"nil", isNull, nil, false},
	}
	for _, c := range cases {
		if got := condEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: condEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

// chatRows serves Chat and Invoice rows by collection name, ignoring the query's
// filter: the federated executor must apply WHERE itself over the joined rows.
type chatRows struct{ rows map[string][]record.Record }

func (db chatRows) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q := query.(dal.StructuredQuery)
	name := q.From().Base().Name()
	rows, ok := db.rows[name]
	if !ok {
		return nil, fmt.Errorf("unknown collection %q", name)
	}
	return dal.NewRecordsReader(append([]record.Record(nil), rows...)), nil
}

func (chatRows) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, fmt.Errorf("recordsets unsupported")
}

func runChatQuery(t *testing.T, document string) []int {
	t.Helper()
	query, err := Deserialize([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	row := func(collection string, id int, data map[string]any) record.Record {
		return record.NewRecordWithData(record.NewKeyWithID(collection, id), data)
	}
	db := chatRows{rows: map[string][]record.Record{
		"Chat": {
			row("Chat", 1, map[string]any{"id": 1, "Company": nil}),
			row("Chat", 2, map[string]any{"id": 2, "Company": "acme"}),
			row("Chat", 3, map[string]any{"id": 3}),
		},
		"Invoice": {
			row("Invoice", 1, map[string]any{"id": 1, "chat": 1}),
			row("Invoice", 2, map[string]any{"id": 2, "chat": 1}),
			row("Invoice", 3, map[string]any{"id": 3, "chat": 2}),
		},
	}}
	reader, err := dal.ExecuteFederatedQuery(context.Background(), query, func(context.Context, string) (dal.QueryExecutor, error) { return db, nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dal.ReadAllToRecords(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, rec := range rows {
		ids = append(ids, int(rec.Data().(map[string]any)["id"].(float64)))
	}
	sort.Ints(ids)
	return ids
}

// TestNullTestFixesJoinedNullFilter reproduces the reported regression: once
// the Chat result is joined to Invoice, `Company == null` matches nothing (the
// join evaluates comparisons in three-valued logic), so the filter has to be
// written isNull to survive the join.
func TestNullTestFixesJoinedNullFilter(t *testing.T) {
	const join = `from:
  database: db
  name: Chat
  alias: c
  joins:
    - from: {database: db, name: Invoice, alias: i}
      on: [{left: {field: id, source: c}, op: '==', right: {field: chat, source: i}}]
where:
  %s
columns:
  - {field: id, source: c, as: id}
`
	cases := []struct {
		name  string
		where string
		want  []int
	}{
		{"== null stays three-valued", "op: ==\n  left: {field: Company, source: c}\n  right: {value: null}", nil},
		{"isNull", "isNull: {field: Company, source: c}", []int{1, 1}},
		{"isNotNull", "isNotNull: {field: Company, source: c}", []int{2}},
		{"group", "and:\n    - isNull: {field: Company, source: c}\n    - isNotNull: {field: chat, source: i}", []int{1, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runChatQuery(t, fmt.Sprintf(join, tc.where))
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("ids = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestNullTestOperandGrammar(t *testing.T) {
	const flat = "from: {name: Sale}\n"
	cases := []struct {
		name    string
		doc     string
		wantErr string // empty: accepted
	}{
		{"field", flat + "where: {isNull: {field: a}}\n", ""},
		{"literal", flat + "where: {isNotNull: {value: 1}}\n", ""},
		{"arithmetic", flat + "where: {isNull: {binary: {op: +, left: {field: a}, right: {value: 1}}}}\n", ""},
		{"scalar subquery", flat + "where: {isNull: {query: {as: t, from: {name: Other}, columns: [{field: x}]}}}\n", ""},
		{"aggregate in having", flat + "groupBy: [{field: g}]\nhaving: {isNull: {aggregate: {function: max, args: [{field: a}]}}}\ncolumns: [{field: g}]\n", ""},
		{"arithmetic over aggregates in having", flat + "groupBy: [{field: g}]\nhaving: {isNotNull: {binary: {op: /, left: {aggregate: {function: sum, args: [{field: a}]}}, right: {aggregate: {function: count, args: [{star: true}]}}}}}\ncolumns: [{field: g}]\n", ""},
		{"values list", flat + "where: {isNull: {values: [1, 2]}}\n", "query_shape at where.isNull: a values list"},
		{"star", flat + "where: {isNotNull: {star: true}}\n", "query_shape at where.isNotNull: star"},
		{"param", flat + "where: {isNull: {param: who}}\n", "query_shape at where.isNull: a param"},
		{"aggregate in where", flat + "where: {isNull: {aggregate: {function: max, args: [{field: a}]}}}\n", "query_shape at where.isNull: an aggregate has no value in where"},
		{"aggregate inside arithmetic in where", flat + "where: {isNull: {binary: {op: +, left: {value: 1}, right: {aggregate: {function: max, args: [{field: a}]}}}}}\n", "an aggregate has no value in where"},
		{"star inside arithmetic left", flat + "where: {isNull: {binary: {op: +, left: {star: true}, right: {value: 1}}}}\n", "star is not a value"},
		{"values inside arithmetic", flat + "where: {isNull: {binary: {op: +, left: {value: 1}, right: {values: [1]}}}}\n", "a values list"},
		{"deep in a group", flat + "where: {and: [{isNull: {field: a}}, {or: [{isNull: {field: b}}, {isNotNull: {param: p}}]}]}\n", "query_shape at where.and[1].or[1].isNotNull: a param"},
		{"star in having", flat + "groupBy: [{field: g}]\nhaving: {isNull: {star: true}}\ncolumns: [{field: g}]\n", "query_shape at having.isNull: star"},
		{"in a nested query", flat + "where: {exists: {query: {from: {name: X}, where: {isNull: {param: p}}}}}\n", "query_shape at where.exists.query.where.isNull: a param"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Deserialize([]byte(tc.doc))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("rejected: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v; want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestNullTestOperandProblemFallback(t *testing.T) {
	if got := nullOperandProblem(unsupportedExpr{}, false); !strings.Contains(got, "cannot be tested") {
		t.Fatalf("unknown operand problem = %q", got)
	}
	if err := validateNullTests(dal.NewIsNullCondition(unsupportedExpr{}), "where", false); err == nil {
		t.Fatal("unknown operand type accepted")
	}
	if err := validateNullTests(nil, "where", false); err != nil {
		t.Fatalf("nil condition = %v", err)
	}
}

func TestNullTestFormsMessageNamesEveryForm(t *testing.T) {
	for name, doc := range map[string]string{
		"isNull and isNotNull": "from: {name: Chat}\nwhere:\n  isNull: {field: a}\n  isNotNull: {field: a}\n",
		"isNull and and":       "from: {name: Chat}\nwhere:\n  isNull: {field: a}\n  and: [{isNull: {field: b}}]\n",
	} {
		_, err := Deserialize([]byte(doc))
		if err == nil || !strings.Contains(err.Error(), "mixes forms") || !strings.Contains(err.Error(), "isNull or isNotNull") || strings.Contains(err.Error(), "comparison and group forms") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}
