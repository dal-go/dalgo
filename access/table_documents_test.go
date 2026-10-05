package access

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

const tableDocumentHeader = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: tables
default: deny
`

// tableDocumentYAML says what workedExamplePolicy says in Go, and adds two table
// rules that name a database.
const tableDocumentYAML = tableDocumentHeader + `scopes:
  - path: /Customer
    rules:
      - {id: customers-basic, effect: allow, operations: [query], fields: [CustomerId, FirstName, Country]}
  - table: {schema: sales, name: Customer}
    rules:
      - id: sales-customers-gb
        effect: allow
        operations: [query]
        fields: [id, company, country]
        where: {op: "==", left: {field: country}, right: {value: GB}}
  - table: {database: app, schema: sales, name: Orders}
    rules:
      - {id: app-orders, effect: allow, operations: [query]}
  - table: {database: app, name: Invoices}
    rules:
      - {id: app-invoices, effect: deny, operations: [query]}
`

func TestTableScopesDecodeFromADocument(t *testing.T) {
	policy, err := UnmarshalAccessPolicyYAML([]byte(tableDocumentYAML))
	require.NoError(t, err)
	for _, source := range []dal.RecordsetSource{salesCustomer, publicCustomer, plainCustomer} {
		require.Equal(t, summarizeDecision(decideRead(workedExamplePolicy(), source)), summarizeDecision(decideRead(policy, source)))
	}
	// The document also has a table rule for a table named Orders, so that bare name is ambiguous too.
	requireDenied(t, decideRead(policy, plainOrders), CodeEnforcementUnsupported)
	allowed := decideRead(policy, salesCustomer)
	require.True(t, allowed.Allowed)
	require.Equal(t, "sales-customers-gb", allowed.Rule)
	require.Equal(t, []string{"id", "company", "country"}, allowed.Writes[0].Alternatives[0].Fields)
	// The document has a table rule for sales.Orders that names a database, which a session that
	// resolves no name cannot check; the worked example has none for that table.
	requireDenied(t, decideRead(workedExamplePolicy(), salesOrders), CodeNoMatch)
	require.Equal(t, "no table rule names this table", decideRead(workedExamplePolicy(), salesOrders).Explanation)
	unchecked := decideRead(policy, salesOrders)
	requireDenied(t, unchecked, CodeNoMatch)
	require.Contains(t, unchecked.Explanation, "cannot check the database")
	requireDenied(t, decideRead(policy, plainCustomer), CodeEnforcementUnsupported)
	// A table rule that names a database never matches on a session that resolves no name.
	requireDenied(t, decideRead(policy, appSalesCustomer), CodeNoMatch)
	requireDenied(t, decideRead(policy, dal.NewDatabaseCollectionRef("app", "sales", "Orders", "")), CodeNoMatch)
}

func tableDocumentPolicies(t *testing.T) (*AccessPolicy, *PrincipalPolicySet, *AuditPolicy) {
	t.Helper()
	access := MustPolicy("tables",
		Collection("Customer", Allow(Query, "customers-basic").Fields("CustomerId", "FirstName", "Country")),
		TableScope(tableSalesNamed,
			Allow(Query, "sales-customers-gb").Fields("id", "company", "country").Where(dal.WhereField("country", dal.Equal, "GB")),
			Deny(Write, "sales-no-writes")),
		TableScope(TableName{Database: "app", Schema: "sales", Name: "Orders"}, Allow(Query, "app-orders")),
		TableScope(TableName{Database: "app", Name: "Invoices"}, Deny(Query, "app-invoices")),
	)
	set := MustPrincipalPolicySet("tables", map[string][]Rule{
		"readers": {TableScope(tableSalesNamed, Allow(Query, "sales-read"))},
		"admins":  {TableScope(tableSalesNamed, Allow(Write, "sales-write"))},
	}, Bindings{Roles: map[string][]string{"reader": {"readers"}}, Everyone: []string{"admins"}})
	audit := MustAuditPolicy("tables-audit",
		TableScope(tableSalesNamed, Audit(Write, "audit-sales")),
		TableScope(TableName{Database: "app", Schema: "sales", Name: "Log"}, IgnoreAudit(Write, "ignore-log")),
	)
	return access, set, audit
}

func TestTableScopesRoundTripThroughYAMLAndJSON(t *testing.T) {
	policy, set, audit := tableDocumentPolicies(t)
	for name, codec := range map[string]Codec{"YAML": YAMLCodec{}, "JSON": JSONCodec{}} {
		t.Run(name, func(t *testing.T) {
			var encoded bytes.Buffer
			require.NoError(t, EncodeAccessPolicy(&encoded, codec, policy))
			require.Contains(t, encoded.String(), "table")
			decoded, err := DecodeAccessPolicy(bytes.NewReader(encoded.Bytes()), codec)
			require.NoError(t, err)
			var again bytes.Buffer
			require.NoError(t, EncodeAccessPolicy(&again, codec, decoded))
			require.Equal(t, encoded.String(), again.String())
			for _, source := range []dal.RecordsetSource{salesCustomer, salesOrders, publicCustomer, plainCustomer, plainOrders, appSalesCustomer} {
				require.Equal(t, summarizeDecision(decideRead(policy, source)), summarizeDecision(decideRead(decoded, source)))
			}

			var encodedSet bytes.Buffer
			require.NoError(t, EncodePrincipalPolicySet(&encodedSet, codec, set))
			decodedSet, err := DecodePrincipalPolicySet(bytes.NewReader(encodedSet.Bytes()), codec)
			require.NoError(t, err)
			var againSet bytes.Buffer
			require.NoError(t, EncodePrincipalPolicySet(&againSet, codec, decodedSet))
			require.Equal(t, encodedSet.String(), againSet.String())
			require.Equal(t, summarizeDecision(decideRead(set, salesCustomer)), summarizeDecision(decideRead(decodedSet, salesCustomer)))

			var encodedAudit bytes.Buffer
			require.NoError(t, EncodeAuditPolicy(&encodedAudit, codec, audit))
			decodedAudit, err := DecodeAuditPolicy(bytes.NewReader(encodedAudit.Bytes()), codec)
			require.NoError(t, err)
			var againAudit bytes.Buffer
			require.NoError(t, EncodeAuditPolicy(&againAudit, codec, decodedAudit))
			require.Equal(t, encodedAudit.String(), againAudit.String())
		})
	}
	// The byte helpers carry them too, and DecodePolicy picks the right type.
	data, err := MarshalAccessPolicyYAML(policy)
	require.NoError(t, err)
	require.Contains(t, string(data), "table:\n      schema: sales\n      name: Customer\n")
	data, err = MarshalAccessPolicyJSON(policy)
	require.NoError(t, err)
	_, err = UnmarshalAccessPolicyJSON(data)
	require.NoError(t, err)
	data, err = MarshalPrincipalPolicySetYAML(set)
	require.NoError(t, err)
	decoded, err := DecodePolicy(bytes.NewReader(data), YAMLCodec{})
	require.NoError(t, err)
	require.IsType(t, &PrincipalPolicySet{}, decoded)
}

func TestTableScopeThatCannotBeEncoded(t *testing.T) {
	// Every encoded directive needs an explicit rule name, a table rule's included.
	_, err := MarshalAccessPolicyYAML(MustPolicy("p", TableScope(tableSalesNamed, Allow(Query))))
	require.True(t, errors.Is(err, ErrNotSerializable), err)
	// A scoped field mask needs another serialization.
	policy := MustPolicy("p", TableScope(tableSalesNamed, Allow(Query, "masked").WithFieldMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}}})))
	_, err = MarshalAccessPolicyYAML(policy)
	require.True(t, errors.Is(err, ErrNotSerializable), err)
}

func TestTableScopeDocumentsThatAreRefused(t *testing.T) {
	rule := "rules: [{id: r, effect: allow, operations: [query]}]"
	tests := map[string]struct {
		scopes string
		want   string
	}{
		"unknown key in the table":    {"- table: {schema: sales, name: Customer, extra: 1}\n    " + rule, "field extra not found"},
		"table is not a mapping":      {"- table: sales.Customer\n    " + rule, "cannot unmarshal"},
		"two selectors, path":         {"- path: /Customer\n    table: {schema: sales, name: Customer}\n    " + rule, "exactly one of path, collectionGroup, opaqueQuery, or table"},
		"two selectors, group":        {"- collectionGroup: events\n    table: {schema: sales, name: Customer}\n    " + rule, "exactly one of path, collectionGroup, opaqueQuery, or table"},
		"two selectors, opaque":       {"- opaqueQuery: true\n    table: {schema: sales, name: Customer}\n    " + rule, "exactly one of path, collectionGroup, opaqueQuery, or table"},
		"no selector":                 {"- " + rule, "exactly one of path, collectionGroup, opaqueQuery, or table"},
		"no name":                     {"- table: {schema: sales}\n    " + rule, "table name is required"},
		"neither schema nor database": {"- table: {name: Customer}\n    " + rule, "schema, a database, or both"},
		"empty table":                 {"- table: {}\n    " + rule, "table name is required"},
		"table nested in a path":      {"- path: /Customer\n    scopes:\n      - table: {schema: sales, name: Customer}\n        " + rule, "table rules must be top-level"},
		"path nested in a table":      {"- table: {schema: sales, name: Customer}\n    scopes:\n      - path: /Customer\n        " + rule, "path scopes cannot be nested inside table"},
		"table nested in a table":     {"- table: {schema: sales, name: Customer}\n    scopes:\n      - table: {schema: sales, name: Orders}\n        " + rule, "table rules must be top-level"},
		"no rules":                    {"- table: {schema: sales, name: Customer}", "scope contains no rules"},
		"captured path value": {"- table: {schema: sales, name: Customer}\n    rules:\n      - {id: r, effect: allow, operations: [query], where: {op: \"==\", left: {field: id}, right: {param: path.id}}}",
			"captures no such segment"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			document := tableDocumentHeader + "scopes:\n  " + tc.scopes + "\n"
			_, err := UnmarshalAccessPolicyYAML([]byte(document))
			require.ErrorContains(t, err, tc.want)
			// Through the codec that is not told what the policy is for.
			_, err = DecodePolicy(bytes.NewReader([]byte(document)), YAMLCodec{})
			require.ErrorContains(t, err, tc.want)
			// The same refusal in a rule set of a principal policy set.
			set := tableDocumentHeader + "ruleSets:\n  r:\n    " + strings.ReplaceAll(tc.scopes, "\n", "\n  ") + "\nbindings: {everyone: [r]}\n"
			_, err = UnmarshalPrincipalPolicySetYAML([]byte(set))
			require.ErrorContains(t, err, tc.want)
		})
	}

	// JSON refuses an unknown key of the table, and a table that is not an object.
	for name, body := range map[string]string{
		"unknown key":   `{"apiVersion":"dalgo.io/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"table":{"schema":"s","name":"n","extra":1},"rules":[{"id":"x","effect":"allow","operations":["query"]}]}]}`,
		"not an object": `{"apiVersion":"dalgo.io/access/v1","kind":"AccessPolicy","metadata":{"name":"x"},"default":"deny","scopes":[{"table":"s.n","rules":[{"id":"x","effect":"allow","operations":["query"]}]}]}`,
	} {
		t.Run("JSON "+name, func(t *testing.T) {
			_, err := UnmarshalAccessPolicyJSON([]byte(body))
			require.Error(t, err)
		})
	}

	// An audit policy takes a table scope with audit effects only.
	_, err := UnmarshalAuditPolicyYAML([]byte(`apiVersion: dalgo.io/access/v1
kind: AuditPolicy
metadata:
  name: a
default: ignore-audit
scopes:
  - table: {schema: sales, name: Customer}
    rules: [{id: r, effect: allow, operations: [write]}]
`))
	require.ErrorContains(t, err, "not valid for policy kind")
}

// The portable file format keeps path scopes only: a table scope in a portable
// file is refused, as any key it does not know is.
func TestPortableFilePolicyRefusesATableScope(t *testing.T) {
	body := "scopes:\n  - table: {schema: sales, name: Customer}\n    rules: [{id: r, effect: allow, operations: [query]}]\n"
	_, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", body)))
	require.Error(t, err)
}
