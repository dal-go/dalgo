package dtql

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

func TestSerializeAndBack(t *testing.T) {
	out, err := Serialize(fullQuery())
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	// The document is plain YAML parseable by a standard library.
	var generic any
	if err := yaml.Unmarshal(out, &generic); err != nil {
		t.Fatalf("not plain YAML: %v", err)
	}
	q, err := Deserialize(out)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	if q == nil {
		t.Fatal("Deserialize returned a nil query")
	}
	if got := q.From().Base().(dal.CollectionRef).Name(); got != "users" {
		t.Errorf("From name = %q, want users", got)
	}
	if len(q.Columns()) != 2 {
		t.Errorf("Columns = %d, want 2", len(q.Columns()))
	}
	if q.Limit() != 10 || q.Offset() != 20 {
		t.Errorf("Limit/Offset = %d/%d, want 10/20", q.Limit(), q.Offset())
	}
}

func TestQualifiedSourceRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		yaml   string
	}{
		{
			name:   "SQLite main",
			schema: "main",
			yaml:   "from:\n  schema: main\n  name: Customer\nlimit: 50\n",
		},
		{
			name:   "SQL Server dbo",
			schema: "dbo",
			yaml:   "from:\n  schema: dbo\n  name: Customer\n  alias: c\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Deserialize([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			ref := q.From().Base().(dal.CollectionRef)
			if ref.Schema() != tt.schema || ref.Name() != "Customer" {
				t.Fatalf("source = schema %q, name %q; want schema %q, name Customer", ref.Schema(), ref.Name(), tt.schema)
			}
			got, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if string(got) != tt.yaml {
				t.Fatalf("canonical round-trip mismatch\n--- want ---\n%s--- got ---\n%s", tt.yaml, got)
			}
		})
	}
}

func TestWildcardExclusionRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantSource string
		wantNames  []string
	}{
		{
			name:      "unqualified",
			yaml:      "from:\n  name: customers\ncolumns:\n  - wildcard:\n      exclude:\n        - email\n        - password_hash\n",
			wantNames: []string{"email", "password_hash"},
		},
		{
			name:       "qualified with missing and duplicate exclusions",
			yaml:       "from:\n  name: customers\n  alias: c\ncolumns:\n  - wildcard:\n      source: c\n      exclude:\n        - email\n        - missing\n        - email\n",
			wantSource: "c",
			wantNames:  []string{"email", "missing", "email"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Deserialize([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			wildcard := q.Columns()[0].Wildcard
			if wildcard == nil {
				t.Fatal("Wildcard = nil")
			}
			if wildcard.Source != tt.wantSource || !reflect.DeepEqual(wildcard.Exclude, tt.wantNames) {
				t.Fatalf("Wildcard = %#v, want source %q exclusions %#v", wildcard, tt.wantSource, tt.wantNames)
			}
			got, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if string(got) != tt.yaml {
				t.Fatalf("canonical round-trip mismatch\n--- want ---\n%s--- got ---\n%s", tt.yaml, got)
			}
		})
	}
}

func TestFromMergeAndAliasCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantSchema string
		wantName   string
	}{
		{
			name:     "unqualified merge",
			yaml:     "from:\n  <<: {name: users}\n",
			wantName: "users",
		},
		{
			name:     "unqualified merge sequence",
			yaml:     "from:\n  <<: [{name: users}]\n",
			wantName: "users",
		},
		{
			name:     "shared merge alias",
			yaml:     "from:\n  <<: [&source {name: users}, *source]\n",
			wantName: "users",
		},
		{
			name:       "qualified merge",
			yaml:       "from:\n  <<: {schema: main, name: Customer}\n",
			wantSchema: "main",
			wantName:   "Customer",
		},
		{
			name:       "schema string alias",
			yaml:       "from:\n  name: &identifier Customer\n  schema: *identifier\n",
			wantSchema: "Customer",
			wantName:   "Customer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Deserialize([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			ref := q.From().Base().(dal.CollectionRef)
			if ref.Schema() != tt.wantSchema || ref.Name() != tt.wantName {
				t.Fatalf("source = schema %q, name %q; want schema %q, name %q", ref.Schema(), ref.Name(), tt.wantSchema, tt.wantName)
			}
		})
	}
}

func TestNullConstantRoundTripAndDirectDecode(t *testing.T) {
	serialized, err := Serialize(fakeQuery{from: rootFrom(), columns: []dal.Column{{Expression: dal.Constant{Value: nil}}}})
	if err != nil {
		t.Fatalf("serialize null: %v", err)
	}
	query, err := Deserialize(serialized)
	if err != nil {
		t.Fatalf("deserialize serialized null: %v\n%s", err, serialized)
	}
	if got := query.Columns()[0].Expression.(dal.Constant).Value; got != nil {
		t.Fatalf("round-trip null = %#v", got)
	}

	direct := []byte("from: {name: users}\nwhere:\n  op: ==\n  left: {field: deletedAt}\n  right: {value: null}\n")
	query, err = Deserialize(direct)
	if err != nil {
		t.Fatalf("direct null: %v", err)
	}
	comparison := query.Where().(dal.Comparison)
	if got := comparison.Right.(dal.Constant).Value; got != nil {
		t.Fatalf("direct null = %#v", got)
	}
}

func TestDeserialize_invalidInputRejected(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "wildcard without exclusions",
			yaml:    "from: {name: users}\ncolumns:\n  - wildcard: {}\n",
			wantErr: "must contain at least one",
		},
		{
			name:    "wildcard with unknown source",
			yaml:    "from: {name: users, alias: u}\ncolumns:\n  - wildcard: {source: x, exclude: [email]}\n",
			wantErr: "does not match from name or alias",
		},
		{
			name:    "wildcard mixed with field",
			yaml:    "from: {name: users}\ncolumns:\n  - field: id\n    wildcard: {exclude: [email]}\n",
			wantErr: "mixes wildcard and expression",
		},
		{
			name:    "wildcard with alias",
			yaml:    "from: {name: users}\ncolumns:\n  - wildcard: {exclude: [email]}\n    as: rest\n",
			wantErr: "cannot have an alias",
		},
		{
			name:    "wildcard with explicitly empty alias",
			yaml:    "from: {name: users}\ncolumns:\n  - wildcard: {exclude: [email]}\n    as: ''\n",
			wantErr: "cannot have an alias",
		},
		{
			name:    "wildcard with explicitly empty source",
			yaml:    "from: {name: users}\ncolumns:\n  - wildcard: {source: '', exclude: [email]}\n",
			wantErr: "source must be a non-empty string",
		},
		{
			name:    "wildcard mixed with empty field",
			yaml:    "from: {name: users}\ncolumns:\n  - field: ''\n    wildcard: {exclude: [email]}\n",
			wantErr: "mixes wildcard and expression",
		},
		{
			name:    "wildcard with unknown key",
			yaml:    "from: {name: users}\ncolumns:\n  - wildcard: {except: [email]}\n",
			wantErr: "not found in column wildcard",
		},
		{
			name:    "unknown key",
			yaml:    "from:\n  name: users\nbogus: 1\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "unknown from key",
			yaml:    "from:\n  namespace: main\n  name: users\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "quoted merge token is an unknown key",
			yaml:    "from: {\"<<\": {name: ignored}, name: users}\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "unknown merged from key",
			yaml:    "from:\n  <<: {name: users, namespace: main}\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "unknown key in merge sequence",
			yaml:    "from:\n  <<: [{name: users}, {namespace: main}]\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "non-string merged schema",
			yaml:    "from:\n  <<: {schema: 123, name: users}\n",
			wantErr: "from.schema must be a string",
		},
		{
			name:    "invalid merge value",
			yaml:    "from:\n  <<: users\n",
			wantErr: "from merge value must be a mapping",
		},
		{
			name:    "recursive merge alias",
			yaml:    "from: &source\n  <<: *source\n  name: users\n",
			wantErr: "from merge contains a recursive alias",
		},
		{
			name:    "recursive merge sequence alias",
			yaml:    "from:\n  <<: &sources\n    - <<: *sources\n      name: users\n",
			wantErr: "from merge contains a recursive alias",
		},
		{
			name:    "missing from name",
			yaml:    "limit: 5\n",
			wantErr: "from.name is required",
		},
		{
			name:    "from is not a mapping",
			yaml:    "from: users\n",
			wantErr: "from must be a mapping",
		},
		{
			name:    "from name has wrong type",
			yaml:    "from:\n  name: [users]\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "empty schema",
			yaml:    "from:\n  schema: ''\n  name: users\n",
			wantErr: "from.schema must not be empty",
		},
		{
			name:    "null schema",
			yaml:    "from:\n  schema: null\n  name: users\n",
			wantErr: "from.schema must be a string",
		},
		{
			name:    "numeric schema",
			yaml:    "from:\n  schema: 123\n  name: users\n",
			wantErr: "from.schema must be a string",
		},
		{
			name:    "boolean schema",
			yaml:    "from:\n  schema: true\n  name: users\n",
			wantErr: "from.schema must be a string",
		},
		{
			name:    "wrong value type for limit",
			yaml:    "from:\n  name: users\nlimit: notanint\n",
			wantErr: "invalid DTQL-YAML",
		},
		{
			name:    "unknown operator",
			yaml:    "from:\n  name: users\nwhere:\n  op: \"!=\"\n  left:\n    field: a\n  right:\n    value: 1\n",
			wantErr: "unknown comparison operator",
		},
		{
			name:    "comparison missing right",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n",
			wantErr: "requires both left and right",
		},
		{
			name:    "mixed comparison and group",
			yaml:    "from:\n  name: users\nwhere:\n  op: ==\n  left:\n    field: a\n  right:\n    value: 1\n  and:\n    - op: ==\n      left:\n        field: b\n      right:\n        value: 2\n",
			wantErr: "mixes comparison and group",
		},
		{
			name:    "expression with no field/value/values",
			yaml:    "from:\n  name: users\ncolumns:\n  - as: x\n",
			wantErr: "exactly one of field, value, values or param",
		},
		{name: "negative limit", yaml: "from: {name: users}\nlimit: -1\n", wantErr: "non-negative"},
		{name: "negative offset", yaml: "from: {name: users}\noffset: -1\n", wantErr: "non-negative"},
		{name: "map value", yaml: "from: {name: users}\ncolumns: [{value: {secret: x}}]\n", wantErr: "value must be a scalar"},
		{name: "scalar values", yaml: "from: {name: users}\ncolumns: [{values: x}]\n", wantErr: "values must be an array"},
		{name: "nested values", yaml: "from: {name: users}\ncolumns: [{values: [[x]]}]\n", wantErr: "values must be an array of scalars"},
		{name: "trailing document", yaml: "from: {name: users}\n---\nfrom: {name: orders}\n", wantErr: "multiple documents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Deserialize([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if q != nil {
				t.Errorf("expected nil query on error, got %#v", q)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
