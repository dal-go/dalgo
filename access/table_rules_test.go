package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

var (
	plainCustomer     = dal.NewRootCollectionRef("Customer", "")
	salesCustomer     = dal.NewQualifiedRootCollectionRef("sales", "Customer", "")
	publicCustomer    = dal.NewQualifiedRootCollectionRef("public", "Customer", "")
	appCustomer       = dal.NewDatabaseCollectionRef("app", "", "Customer", "")
	appSalesCustomer  = dal.NewDatabaseCollectionRef("app", "sales", "Customer", "")
	salesOrders       = dal.NewQualifiedRootCollectionRef("sales", "Orders", "")
	plainOrders       = dal.NewRootCollectionRef("Orders", "")
	tableSalesNamed   = TableName{Schema: "sales", Name: "Customer"}
	tablePublicSecret = TableName{Schema: "public", Name: "Secret"}
)

func tableRead(source dal.RecordsetSource) dal.StructuredQuery {
	return dal.From(source).NewQuery().SelectKeysOnly(reflect.String)
}

// decideRead is the decision of a policy on a query that reads one source, with
// the resources the secured session builds for it.
func decideRead(policy Policy, source dal.RecordsetSource) Decision {
	query := tableRead(source)
	return policy.Decide(context.Background(), Request{Operation: Query, Resources: resourcesForQuery(query), Query: query})
}

func requireDenied(t *testing.T, decision Decision, code ReasonCode) {
	t.Helper()
	require.False(t, decision.Allowed, "decision = %+v", decision)
	require.Equal(t, code, decision.Code, "explanation: %s", decision.Explanation)
}

// pathResourceOf is the resource of a source that writes no schema, with the
// identity a session that resolves names would give it.
func pathResourceOf(collection string, table TableName) Resource {
	resource := CollectionResourceFor(nil, collection)
	resource.table = &table
	return resource
}

func TestResourceTableIdentityOfEverySourceSpelling(t *testing.T) {
	type expectation struct {
		kind  ResourceKind
		text  string
		table *TableName
	}
	byPrefix := []struct {
		prefix string
		want   expectation
	}{
		{"database and schema", expectation{OpaqueQueryResource, "opaque-query:sales.Customer", &TableName{Database: "app", Schema: "sales", Name: "Customer"}}},
		{"database", expectation{PathResource, "/Customer", &TableName{Database: "app", Name: "Customer"}}},
		{"schema", expectation{OpaqueQueryResource, "opaque-query:sales.Customer", &TableName{Schema: "sales", Name: "Customer"}}},
		{"unqualified", expectation{PathResource, "/Customer", nil}},
	}
	for _, source := range matrixSources() {
		var want expectation
		for _, candidate := range byPrefix {
			if strings.HasPrefix(source.name, candidate.prefix) {
				want = candidate.want
				break
			}
		}
		for _, position := range matrixPositions() {
			t.Run(source.name+" @ "+position.name, func(t *testing.T) {
				found := 0
				for _, resource := range resourcesForQuery(position.build(source.source)) {
					table, hasTable := resource.Table()
					if !strings.Contains(resource.String(), "Customer") {
						require.False(t, hasTable, "%s must carry no identity", resource)
						continue
					}
					found++
					require.Equal(t, want.kind, resource.Kind())
					require.Equal(t, want.text, resource.String())
					require.Equal(t, want.table != nil, hasTable)
					if hasTable {
						require.Equal(t, *want.table, table)
					} else {
						require.Equal(t, TableName{}, table)
					}
				}
				require.NotZero(t, found, "the source is one of the resources of the query")
			})
		}
	}
}

func TestResourceWithoutIdentityIsTheEarlierValue(t *testing.T) {
	require.Equal(t, CollectionResourceFor(nil, "Customer"), resourceForRecordsetSource(plainCustomer))
	require.Equal(t, CollectionResourceFor(nil, "Customer"), resourceForRecordsetSource(&plainCustomer))

	// A source that writes a schema is the same opaque resource as before, with the identity beside it.
	for _, source := range []dal.RecordsetSource{salesCustomer, &salesCustomer} {
		resource := resourceForRecordsetSource(source)
		table, hasTable := resource.Table()
		require.True(t, hasTable)
		require.Equal(t, tableSalesNamed, table)
		resource.table = nil
		require.Equal(t, OpaqueQuery("sales.Customer"), resource)
	}

	// Keys, collection groups, unreadable parts and text queries carry no identity.
	for _, resource := range []Resource{
		RecordResourceForKey(record.NewKeyWithID("Customer", "1")),
		RecordResourceForKey(record.NewKeyWithParentAndID(record.NewKeyWithID("Order", "7"), "Customer", "1")),
		RecordResourceForKey(nil),
		CollectionResourceFor(nil, "Customer"),
		CollectionGroup("Customer"),
		OpaqueQuery("select 1"),
		resourceForRecordsetSource(dal.NewCollectionGroupRef("Customer", "")),
		resourceForRecordsetSource(nil),
		resourcesForQuery(opaqueQ{})[0],
	} {
		table, hasTable := resource.Table()
		require.False(t, hasTable, resource.String())
		require.Equal(t, TableName{}, table)
	}
}

func TestResourceTableReturnsACopy(t *testing.T) {
	resource := resourceForRecordsetSource(salesCustomer)
	table, _ := resource.Table()
	table.Name = "changed"
	again, _ := resource.Table()
	require.Equal(t, "Customer", again.Name)
}

func TestTableScopeDeclarations(t *testing.T) {
	allow := Allow(Query, "read")
	valid := map[string]Rule{
		"schema":            TableScope(TableName{Schema: "s", Name: "n"}, allow),
		"database":          TableScope(TableName{Database: "d", Name: "n"}, allow),
		"database, schema":  TableScope(TableName{Database: "d", Schema: "s", Name: "n"}, allow),
		"where, check":      TableScope(tableSalesNamed, Allow(Write, "w").Where(dal.WhereField("country", dal.Equal, "GB")).Check(dal.WhereField("country", dal.Equal, "GB"))),
		"fields":            TableScope(tableSalesNamed, Allow(Query, "f").Fields("id", "name")),
		"field mask":        TableScope(tableSalesNamed, Allow(Query, "m").WithFieldMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"secret"}}}})),
		"deny":              TableScope(tableSalesNamed, Deny(Query, "no")),
		"dotted name pairs": TableScope(TableName{Schema: "a.b", Name: "c"}, allow),
		"under the root":    Root(TableScope(tableSalesNamed, allow)),
	}
	for name, rule := range valid {
		t.Run("valid "+name, func(t *testing.T) {
			_, err := NewPolicy("p", rule)
			require.NoError(t, err)
		})
	}
	// Two tables whose parts join to the same text are two tables with two rule names.
	_, err := NewPolicy("p",
		TableScope(TableName{Schema: "a.b", Name: "c"}, Allow(Query)),
		TableScope(TableName{Schema: "a", Name: "b.c"}, Allow(Query)),
	)
	require.NoError(t, err)
	_, err = NewPolicy("p",
		TableScope(TableName{Database: "a", Name: "b"}, Allow(Query)),
		TableScope(TableName{Schema: "a", Name: "b"}, Allow(Query)),
	)
	require.NoError(t, err)

	invalid := map[string]struct {
		rule Rule
		want string
	}{
		"no name":        {TableScope(TableName{Schema: "s"}, allow), "table name is required"},
		"blank name":     {TableScope(TableName{Schema: "s", Name: "  "}, allow), "table name is required"},
		"unqualified":    {TableScope(TableName{Name: "n"}, allow), "schema, a database, or both"},
		"blank schema":   {TableScope(TableName{Schema: " ", Name: "n"}, allow), "schema must not be blank"},
		"blank database": {TableScope(TableName{Database: "\t", Schema: "s", Name: "n"}, allow), "database must not be blank"},
		"nested in a path": {
			Collection("x", TableScope(tableSalesNamed, allow)), "table rules must be top-level",
		},
		"nested in a table": {
			TableScope(tableSalesNamed, TableScope(TableName{Schema: "s", Name: "t"}, allow)), "table rules must be top-level",
		},
		"path scope inside": {
			TableScope(tableSalesNamed, Collection("x", allow)), "path scopes cannot be nested inside table",
		},
		"opaque scope inside": {
			TableScope(tableSalesNamed, OpaqueQueryScope(allow)), "opaque-query rules must be top-level",
		},
		"captured path value": {
			TableScope(tableSalesNamed, Allow(Query, "c").Where(dal.WhereField("id", dal.Equal, dal.NewParam("path.id")))), "captures no such segment",
		},
		"field mask with fields": {
			TableScope(tableSalesNamed, Allow(Query, "m").Fields("id").WithFieldMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}}})), "fieldMask requires a path or table allow rule without fields",
		},
		"fields on the scope": {
			TableScope(tableSalesNamed, allow).Fields("id"), "only valid on allow rules, not on scopes",
		},
		"audit effect in an access policy": {
			TableScope(tableSalesNamed, Audit(Query, "a")), "not valid in this policy",
		},
	}
	for name, tc := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			_, err := NewPolicy("p", tc.rule)
			require.ErrorContains(t, err, tc.want)
			require.Panics(t, func() { MustPolicy("p", tc.rule) })
		})
	}
	_, err = NewPolicy("p", TableScope(tableSalesNamed, allow), TableScope(tableSalesNamed, allow))
	require.ErrorContains(t, err, "duplicate rule name")

	// An audit policy takes table scopes with audit effects.
	_, err = NewAuditPolicy("a", TableScope(tableSalesNamed, Audit(Write, "audit-sales")))
	require.NoError(t, err)
}

// workedExamplePolicy has a path rule for the table the unqualified name Customer is
// read from, and a table rule for sales.Customer with its own fields and row condition.
func workedExamplePolicy() *AccessPolicy {
	return MustPolicy("worked-example",
		Collection("Customer", Allow(Query, "customers-basic").Fields("CustomerId", "FirstName", "Country")),
		TableScope(tableSalesNamed,
			Allow(Query, "sales-customers-gb").Fields("id", "company", "country").
				Where(dal.WhereField("country", dal.Equal, "GB"))),
	)
}

func TestTableRulesOnASessionThatResolvesNoName(t *testing.T) {
	policy := workedExamplePolicy()

	// A source that writes the schema is decided by the table rule alone: its
	// fields and its row condition.
	allowed := decideRead(policy, salesCustomer)
	require.True(t, allowed.Allowed, allowed.Explanation)
	require.Equal(t, "sales-customers-gb", allowed.Rule)
	require.Equal(t, `country = 'GB'`, allowed.Condition)
	require.Len(t, allowed.Residuals, 1)
	require.Equal(t, []string{"id", "company", "country"}, allowed.Writes[0].Alternatives[0].Fields)
	require.Equal(t, "opaque-query:sales.Customer", allowed.Resource.String())

	// Another table of the schema, another schema, and another spelling of the
	// table are not named by a table rule.
	for name, source := range map[string]dal.RecordsetSource{
		"another table":       salesOrders,
		"another schema":      publicCustomer,
		"another case":        dal.NewQualifiedRootCollectionRef("sales", "customer", ""),
		"another schema case": dal.NewQualifiedRootCollectionRef("Sales", "Customer", ""),
	} {
		t.Run(name, func(t *testing.T) {
			decision := decideRead(policy, source)
			requireDenied(t, decision, CodeNoMatch)
			require.Equal(t, "no table rule names this table", decision.Explanation)
		})
	}

	// The name the table rule has, with no schema, could be that table: denied,
	// whatever the path rule for the name allows.
	for name, source := range map[string]dal.RecordsetSource{
		"unqualified":            plainCustomer,
		"unqualified pointer":    &plainCustomer,
		"another case":           dal.NewRootCollectionRef("CUSTOMER", ""),
		"database, no schema":    appCustomer,
		"database, no schema, p": &appCustomer,
	} {
		t.Run(name, func(t *testing.T) {
			decision := decideRead(policy, source)
			requireDenied(t, decision, CodeEnforcementUnsupported)
			require.Contains(t, decision.Explanation, "does not say which schema")
		})
	}

	// A name that no table rule has is decided as before.
	requireDenied(t, decideRead(policy, plainOrders), CodeNoMatch)
	allowAll := MustPolicy("all", Root(Allow(Query, "all")), TableScope(tableSalesNamed, Allow(Query, "t")))
	require.True(t, decideRead(allowAll, plainOrders).Allowed)
	requireDenied(t, decideRead(allowAll, plainCustomer), CodeEnforcementUnsupported)
}

func TestTableRuleNeverMatchesASourceThatNamesADatabaseOnASessionThatResolvesNoName(t *testing.T) {
	const cannotCheck = "a table rule has this schema and name, but this session cannot check the database a source or a rule names"
	for name, table := range map[string]TableName{
		"schema only":         {Schema: "sales", Name: "Customer"},
		"database and schema": {Database: "app", Schema: "sales", Name: "Customer"},
	} {
		t.Run(name, func(t *testing.T) {
			policy := MustPolicy("p", TableScope(table, Allow(Query, "t")))
			// The source writes a database, so the session cannot say that it is the one
			// the rule means, even when the rule names exactly the three parts written.
			decision := decideRead(policy, appSalesCustomer)
			requireDenied(t, decision, CodeNoMatch)
			require.Equal(t, cannotCheck, decision.Explanation)
		})
	}
	// A rule that names a database cannot be checked against a source that names none.
	policy := MustPolicy("p", TableScope(TableName{Database: "app", Schema: "sales", Name: "Customer"}, Allow(Query, "t")))
	decision := decideRead(policy, salesCustomer)
	requireDenied(t, decision, CodeNoMatch)
	require.Equal(t, cannotCheck, decision.Explanation)
	// ... unless the rule is for another operation, or the source is another table.
	other := MustPolicy("p", TableScope(TableName{Database: "app", Schema: "sales", Name: "Customer"}, Allow(Get, "t")))
	decision = decideRead(other, salesCustomer)
	requireDenied(t, decision, CodeNoMatch)
	require.Equal(t, "no table rule names this table", decision.Explanation)
	decision = decideRead(policy, salesOrders)
	requireDenied(t, decision, CodeNoMatch)
	require.Equal(t, "no table rule names this table", decision.Explanation)
	decision = decideRead(policy, dal.NewDatabaseCollectionRef("app", "sales", "Orders", ""))
	requireDenied(t, decision, CodeNoMatch)
	require.Equal(t, "no table rule names this table", decision.Explanation)

	// A rule that names a database and no schema never matches either; the name is still
	// ambiguous, so a source that writes no schema is refused as such.
	policy = MustPolicy("p", TableScope(TableName{Database: "app", Name: "Customer"}, Allow(Query, "t")))
	requireDenied(t, decideRead(policy, appCustomer), CodeEnforcementUnsupported)
	requireDenied(t, decideRead(policy, plainCustomer), CodeEnforcementUnsupported)
	requireDenied(t, decideRead(policy, salesCustomer), CodeNoMatch)
}

// A rule for opaque queries is never consulted for a source that names a table, so
// a spelling the table rule does not match is denied and never gets every field.
func TestRuleForOpaqueQueriesIsNotConsultedForATableSource(t *testing.T) {
	policy := MustPolicy("opaque-and-table",
		OpaqueQueryScope(Allow(Query, "any-opaque")),
		TableScope(TableName{Schema: "aux", Name: "Customer"}, Allow(Query, "aux-customers").Fields("id")),
	)
	allowed := decideRead(policy, dal.NewQualifiedRootCollectionRef("aux", "Customer", ""))
	require.True(t, allowed.Allowed)
	require.Equal(t, "aux-customers", allowed.Rule)
	require.Equal(t, []string{"id"}, allowed.Writes[0].Terminal.Fields)

	for _, source := range []dal.RecordsetSource{
		dal.NewQualifiedRootCollectionRef("aux", "customer", ""),
		dal.NewQualifiedRootCollectionRef("aux", "Other", ""),
		dal.NewQualifiedRootCollectionRef("other", "Customer", ""),
	} {
		requireDenied(t, decideRead(policy, source), CodeNoMatch)
	}

	// Through a secured session nothing is read.
	stub := &stubReadSession{}
	session := SecureReadSession(stub, policy)
	_, err := session.ExecuteQueryToRecordsReader(context.Background(), tableRead(dal.NewQualifiedRootCollectionRef("aux", "customer", "")))
	require.ErrorIs(t, err, ErrAccessDenied)
	require.Empty(t, stub.queries)

	// A text query and a source the walk cannot read keep the rule for opaque queries.
	_, err = session.ExecuteQueryToRecordsReader(context.Background(), opaqueQ{})
	require.NoError(t, err)
	require.Len(t, stub.queries, 1)
	// The same policy without a table rule decides a qualified source by the rule for opaque queries.
	plainOpaque := MustPolicy("opaque", OpaqueQueryScope(Allow(Query, "any-opaque")))
	require.True(t, decideRead(plainOpaque, dal.NewQualifiedRootCollectionRef("aux", "customer", "")).Allowed)
}

func TestTableRuleThroughASecuredSessionAppliesItsFieldsAndRowCondition(t *testing.T) {
	stub := &stubReadSession{}
	session := SecureReadSession(stub, workedExamplePolicy())
	query := dal.From(salesCustomer).NewQuery().SelectKeysOnly(reflect.String)
	_, err := session.ExecuteQueryToRecordsReader(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, stub.queries, 1)
	sent := stub.queries[0].(dal.StructuredQuery)
	require.Contains(t, sent.Where().String(), "country = 'GB'")
	names := make([]string, len(sent.Columns()))
	for i, column := range sent.Columns() {
		names[i] = column.Expression.(dal.FieldRef).Name()
	}
	require.ElementsMatch(t, []string{"id", "company", "country"}, names)

	// The same session refuses the other tables and reads nothing for them.
	stub.queries = nil
	for _, source := range []dal.RecordsetSource{salesOrders, plainCustomer} {
		_, err = session.ExecuteQueryToRecordsReader(context.Background(), tableRead(source))
		require.ErrorIs(t, err, ErrAccessDenied)
	}
	require.Empty(t, stub.queries)
}

func TestKeysOnASessionThatResolvesNoName(t *testing.T) {
	policy := MustPolicy("p", Root(Allow(ReadWrite, "all")), TableScope(tableSalesNamed, Allow(Query, "t")))
	decide := func(operation Operations, key *record.Key) Decision {
		return policy.Decide(context.Background(), Request{Operation: operation, Resources: []Resource{RecordResourceForKey(key)}})
	}
	for _, operation := range []Operations{Get, Exists, Insert, Set, Update, Delete} {
		// A key names a collection, and a table rule has a table of that name.
		requireDenied(t, decide(operation, record.NewKeyWithID("Customer", "1")), CodeEnforcementUnsupported)
		requireDenied(t, decide(operation, record.NewKeyWithID("customer", "1")), CodeEnforcementUnsupported)
		// A key of another collection, and a key with a parent, are decided as before.
		require.True(t, decide(operation, record.NewKeyWithID("Order", "1")).Allowed)
		require.True(t, decide(operation, record.NewKeyWithParentAndID(record.NewKeyWithID("Customer", "7"), "Line", "1")).Allowed)
		require.True(t, decide(operation, nil).Allowed)
	}
	// The collection itself, as a resource, is ambiguous the same way.
	requireDenied(t, policy.Decide(context.Background(), Request{Operation: Query, Resources: []Resource{CollectionResourceFor(nil, "Customer")}}), CodeEnforcementUnsupported)
}

// A resource that carries the identity of a table read through a session that
// says which schema a name is read from. The rows below are the precedence between
// path rules and table rules.
func TestTableRulesAndPathRulesOnAResourceWithAnIdentity(t *testing.T) {
	secret := pathResourceOf("Secret", tablePublicSecret)
	decide := func(policy Policy, operation Operations, resource Resource) Decision {
		return policy.Decide(context.Background(), Request{Operation: operation, Resources: []Resource{resource}})
	}
	tableAllow := TableScope(tablePublicSecret, Allow(Query, "table-allow").Fields("a"))

	t.Run("a table rule decides over a path allow", func(t *testing.T) {
		policy := MustPolicy("p", Collection("Secret", Allow(Query, "path-allow").Fields("b")), tableAllow)
		decision := decide(policy, Query, secret)
		require.True(t, decision.Allowed)
		require.Equal(t, "table-allow", decision.Rule)
		require.Equal(t, []string{"a"}, decision.Writes[0].Terminal.Fields)
	})
	t.Run("a table rule alone decides", func(t *testing.T) {
		require.True(t, decide(MustPolicy("p", tableAllow), Query, secret).Allowed)
	})
	t.Run("a deny for the bare name beats a table allow", func(t *testing.T) {
		policy := MustPolicy("p", Collection("Secret", Deny(Query, "no-secret")), tableAllow)
		decision := decide(policy, Query, secret)
		requireDenied(t, decision, CodeRuleDenied)
		require.Equal(t, "no-secret", decision.Rule)
	})
	t.Run("a deny for the bare name beats a table allow on a key", func(t *testing.T) {
		policy := MustPolicy("p", Collection("Secret", Deny(Get, "no-secret")), TableScope(tablePublicSecret, Allow(Get, "table-allow")))
		key := RecordResourceForKey(record.NewKeyWithID("Secret", "1"))
		key.table = &tablePublicSecret
		requireDenied(t, decide(policy, Get, key), CodeRuleDenied)
	})
	t.Run("a deny for the bare name beats a table allow in another rule set of a principal set", func(t *testing.T) {
		set := MustPrincipalPolicySet("set", map[string][]Rule{
			"deny":  {Collection("Secret", Deny(Query, "no-secret"))},
			"table": {tableAllow},
		}, Bindings{Everyone: []string{"deny", "table"}})
		requireDenied(t, decide(set, Query, secret), CodeRuleDenied)
		// With only the set that has the table rule bound, the table rule allows.
		only := MustPrincipalPolicySet("set", map[string][]Rule{
			"deny":  {Collection("Secret", Deny(Query, "no-secret"))},
			"table": {tableAllow},
		}, Bindings{Everyone: []string{"table"}})
		require.True(t, decide(only, Query, secret).Allowed)
	})
	t.Run("a deny for the bare name stands behind a conditional allow of a deeper scope", func(t *testing.T) {
		// The path rules grant the rows the condition selects and deny the rest; a table
		// allow does not widen that.
		policy := MustPolicy("p",
			Scope("Secret", AnyID, Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, "alice"))),
			Collection("Secret", Deny(Get, "no-secret")),
			TableScope(tablePublicSecret, Allow(Get, "table-allow")),
		)
		key := RecordResourceForKey(record.NewKeyWithID("Secret", "1"))
		key.table = &tablePublicSecret
		decision := decide(policy, Get, key)
		require.True(t, decision.Allowed, decision.Explanation)
		require.Contains(t, decision.Rule, "own")
		require.NotContains(t, decision.Rule, "table-allow")
		require.Len(t, decision.Residuals, 1)
		require.Nil(t, decision.Writes[0].Terminal)
		// Without the deny, the table rule is the unconditional allow.
		open := MustPolicy("p",
			Scope("Secret", AnyID, Allow(Get, "own").Where(dal.WhereField("owner", dal.Equal, "alice"))),
			TableScope(tablePublicSecret, Allow(Get, "table-allow")),
		)
		decision = decide(open, Get, key)
		require.True(t, decision.Allowed, decision.Explanation)
		require.Equal(t, "table-allow", decision.Rule)
		require.Empty(t, decision.Residuals)
	})
	t.Run("a deny at the root does not name the collection", func(t *testing.T) {
		policy := MustPolicy("p", Root(Deny(Query, "deny-all")), tableAllow)
		require.True(t, decide(policy, Query, secret).Allowed)
	})
	t.Run("a table rule that does not match leaves the path rules to decide", func(t *testing.T) {
		policy := MustPolicy("p", Collection("Secret", Allow(Query, "path-allow")), TableScope(TableName{Schema: "other", Name: "Secret"}, Allow(Query, "other")))
		decision := decide(policy, Query, secret)
		require.True(t, decision.Allowed)
		require.Equal(t, "path-allow", decision.Rule)
		// ... also when the table rule is for another operation.
		policy = MustPolicy("p", Collection("Secret", Allow(Query, "path-allow")), TableScope(tablePublicSecret, Allow(Get, "get-only")))
		require.Equal(t, "path-allow", decide(policy, Query, secret).Rule)
		// ... and when nothing allows, the denial is the path rules' own.
		policy = MustPolicy("p", TableScope(TableName{Schema: "other", Name: "Secret"}, Allow(Query, "other")))
		decision = decide(policy, Query, secret)
		requireDenied(t, decision, CodeNoMatch)
		require.Equal(t, "no matching allow rule", decision.Explanation)
	})
	t.Run("the rule that names more parts comes first", func(t *testing.T) {
		policy := MustPolicy("p",
			TableScope(TableName{Schema: "public", Name: "Secret"}, Deny(Query, "deny-two-parts")),
			TableScope(TableName{Schema: "public", Name: "Secret"}, Allow(Get, "other-operation")),
		)
		requireDenied(t, decide(policy, Query, secret), CodeRuleDenied)
		// Between a deny and an allow of the same specificity the deny wins, as for path rules.
		policy = MustPolicy("p",
			TableScope(tablePublicSecret, Allow(Query, "allow")),
			TableScope(tablePublicSecret, Deny(Query, "deny")),
		)
		requireDenied(t, decide(policy, Query, secret), CodeRuleDenied)
	})
	t.Run("the collection mask is checked first", func(t *testing.T) {
		policy := MustPolicy("p", tableAllow)
		mask, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"Secret"}}}}, NameMask)
		require.NoError(t, err)
		policy.collectionMask = mask
		requireDenied(t, decide(policy, Query, secret), CodeCollectionDenied)
		requireDenied(t, decideRead(policy, dal.NewQualifiedRootCollectionRef("public", "Secret", "")), CodeCollectionDenied)
		set := MustPrincipalPolicySet("set", map[string][]Rule{"r": {tableAllow}}, Bindings{Everyone: []string{"r"}})
		set.collectionMask = mask
		requireDenied(t, decide(set, Query, secret), CodeCollectionDenied)
		requireDenied(t, decideRead(set, dal.NewQualifiedRootCollectionRef("public", "Secret", "")), CodeCollectionDenied)
		// A mask that admits the name leaves the decision to the rules.
		admit, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"Secret"}}}}, NameMask)
		require.NoError(t, err)
		policy.collectionMask = admit
		require.True(t, decide(policy, Query, secret).Allowed)
		// A schema-qualified source is an opaque resource, which a mask has always denied.
		requireDenied(t, decideRead(policy, dal.NewQualifiedRootCollectionRef("public", "Secret", "")), CodeCollectionDenied)
	})
	t.Run("a policy with no table rule ignores the identity", func(t *testing.T) {
		policy := MustPolicy("p", Collection("Secret", Allow(Query, "path-allow").Fields("b")))
		with, without := decide(policy, Query, secret), decide(policy, Query, CollectionResourceFor(nil, "Secret"))
		require.Equal(t, summarizeDecision(without), summarizeDecision(with))
		require.Equal(t, "path-allow", with.Rule)
	})
	t.Run("a field mask on a table rule", func(t *testing.T) {
		policy := MustPolicy("p", TableScope(tablePublicSecret,
			Allow(Query, "masked").WithFieldMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}, {Exclude: []string{"hidden"}}}})))
		decision := decide(policy, Query, secret)
		require.True(t, decision.Allowed)
		require.Equal(t, []string{"scoped field mask"}, decision.Writes[0].Terminal.Fields)
	})
}

// A principal policy set with a collection mask decides an unqualified source as it
// did, whether or not the source carries an identity.
func TestPrincipalSetWithMaskDecidesAnUnqualifiedSourceAsBefore(t *testing.T) {
	golden := compatGoldenLines(t)
	var policy matrixPolicy
	for _, candidate := range matrixPolicies() {
		if candidate.name == "principal set with a collection mask" {
			policy = candidate
		}
	}
	require.NotNil(t, policy.build)
	for _, source := range []matrixSource{{"unqualified", plainCustomer}, {"unqualified pointer", &plainCustomer}} {
		decider, _ := policy.build()
		query := tableRead(source.source)
		decision := decider.Decide(policy.ctx(), Request{Operation: Query, Resources: resourcesForQuery(query), Query: query})
		prefix := policy.name + " | " + source.name + " @ base | resources: "
		var line string
		for _, candidate := range golden {
			if strings.HasPrefix(candidate, prefix) {
				line = candidate
			}
		}
		require.NotEmpty(t, line, "the golden has no line for %s", source.name)
		require.True(t, strings.HasSuffix(line, " | decide: "+summarizeDecision(decision)), "decision %q, golden line %q", summarizeDecision(decision), line)
	}
	// With the identity a session that resolves names would give it, the set decides the same.
	decider, _ := policy.build()
	for _, identity := range []TableName{{Schema: "public", Name: "Customer"}, {Database: "app", Schema: "public", Name: "Customer"}} {
		with := decider.Decide(policy.ctx(), Request{Operation: Query, Resources: []Resource{pathResourceOf("Customer", identity)}})
		without := decider.Decide(policy.ctx(), Request{Operation: Query, Resources: []Resource{CollectionResourceFor(nil, "Customer")}})
		require.Equal(t, summarizeDecision(without), summarizeDecision(with))
		require.True(t, with.Allowed)
	}
}

func TestAuditPolicyTableRules(t *testing.T) {
	audit := MustAuditPolicy("audit",
		Collection("Customer", Audit(Write, "customer-writes")),
		TableScope(tableSalesNamed, Audit(Write, "sales-customer-writes")),
		TableScope(TableName{Schema: "sales", Name: "Log"}, IgnoreAudit(Write, "ignore-log")),
		Collection("Secret", IgnoreAudit(Write, "ignore-secret")),
		TableScope(tablePublicSecret, Audit(Write, "audit-public-secret")),
	)
	classify := func(resource Resource) AuditDecision {
		return audit.Classify(context.Background(), Request{Operation: Insert, Resources: []Resource{resource}})
	}
	sourceResource := func(source dal.RecordsetSource) Resource { return resourceForRecordsetSource(source) }

	// A table rule audits the table it names; another table is not audited.
	decision := classify(sourceResource(salesCustomer))
	require.True(t, decision.Audit)
	require.Equal(t, "sales-customer-writes", decision.Rule)
	require.False(t, classify(sourceResource(publicCustomer)).Audit)
	require.False(t, classify(sourceResource(dal.NewQualifiedRootCollectionRef("sales", "Log", ""))).Audit)

	// An audit policy never refuses, so a name that could be the table a table rule has is
	// classified by the path rules, as before.
	decision = classify(CollectionResourceFor(nil, "Customer"))
	require.True(t, decision.Audit)
	require.Equal(t, "customer-writes", decision.Rule)

	// An ignore rule for the bare name beats a table rule that audits.
	require.False(t, classify(pathResourceOf("Secret", tablePublicSecret)).Audit)
	// A table rule decides a resource with an identity when no path rule names the collection.
	require.True(t, classify(pathResourceOf("Customer", tableSalesNamed)).Audit)
	require.Equal(t, "sales-customer-writes", classify(pathResourceOf("Customer", tableSalesNamed)).Rule)

	// An audit policy with no table rule classifies as before.
	plain := MustAuditPolicy("plain", Collection("Customer", Audit(Write, "customer-writes")))
	require.True(t, plain.Classify(context.Background(), Request{Operation: Insert, Resources: []Resource{pathResourceOf("Customer", tableSalesNamed)}}).Audit)
}

// A source that writes a schema and that no table rule names is classified by the
// audit policy's rule for opaque queries, as it was before the table rule was added
// to the policy. A table rule that names the source decides it, an ignore-audit
// rule included.
func TestAuditPolicyKeepsItsRuleForOpaqueQueriesBesideTableRules(t *testing.T) {
	opaque := OpaqueQueryScope(Audit(Query, "all-opaque"))
	withTables := MustAuditPolicy("audit", opaque,
		TableScope(TableName{Schema: "hr", Name: "Salary"}, Audit(Query, "salary")),
		TableScope(TableName{Schema: "sales", Name: "Log"}, IgnoreAudit(Query, "ignore-log")),
		TableScope(TableName{Database: "app", Schema: "sales", Name: "Audited"}, IgnoreAudit(Query, "ignore-audited")),
	)
	opaqueOnly := MustAuditPolicy("audit", opaque)
	classify := func(policy *AuditPolicy, source dal.RecordsetSource) AuditDecision {
		return policy.Classify(context.Background(), Request{Operation: Query, Resources: []Resource{resourceForRecordsetSource(source)}})
	}

	for name, source := range map[string]dal.RecordsetSource{
		"a schema and a table no table rule names":                dal.NewQualifiedRootCollectionRef("sales", "Customer", ""),
		"a table of another schema":                               dal.NewQualifiedRootCollectionRef("other", "Salary", ""),
		"a database and a schema":                                 appSalesCustomer,
		"a database and a schema, a rule that names the database": dal.NewDatabaseCollectionRef("app", "sales", "Audited", ""),
	} {
		t.Run(name, func(t *testing.T) {
			got := classify(withTables, source)
			require.True(t, got.Audit, "decision = %+v", got)
			require.Equal(t, "all-opaque", got.Rule)
			// The same as before the table rules were added to the policy.
			require.Equal(t, classify(opaqueOnly, source).Rule, got.Rule)
		})
	}

	// A table rule that names the source decides it.
	salary := classify(withTables, dal.NewQualifiedRootCollectionRef("hr", "Salary", ""))
	require.True(t, salary.Audit)
	require.Equal(t, "salary", salary.Rule)
	ignored := classify(withTables, dal.NewQualifiedRootCollectionRef("sales", "Log", ""))
	require.False(t, ignored.Audit)
	require.Equal(t, "ignore-log", ignored.Rule)
	require.Equal(t, effectIgnoreAudit.String(), ignored.Effect)

	// Custom SQL text and a part of a query that cannot be read keep the rule.
	require.True(t, withTables.Classify(context.Background(), Request{Operation: Query, Resources: []Resource{OpaqueQuery("select 1")}}).Audit)

	// A policy with table rules and no rule for opaque queries audits what its table
	// rules say, and nothing else.
	tablesOnly := MustAuditPolicy("audit", TableScope(TableName{Schema: "hr", Name: "Salary"}, Audit(Query, "salary")))
	require.True(t, classify(tablesOnly, dal.NewQualifiedRootCollectionRef("hr", "Salary", "")).Audit)
	none := classify(tablesOnly, dal.NewQualifiedRootCollectionRef("sales", "Customer", ""))
	require.False(t, none.Audit)
	require.Equal(t, "no matching audit rule", none.Explanation)
}

func TestTableRuleNeverMatchesAPathResource(t *testing.T) {
	// A table rule never matches a path resource, whatever the path rules say.
	policy := MustPolicy("p", TableScope(tableSalesNamed, Allow(ReadWrite, "t")))
	for _, resource := range []Resource{CollectionResourceFor(nil, "Customer"), CollectionResourceFor(nil, "sales"), RecordResourceForKey(record.NewKeyWithID("sales", "Customer")), CollectionGroup("Customer"), OpaqueQuery("sales.Customer")} {
		decision := policy.Decide(context.Background(), Request{Operation: Get, Resources: []Resource{resource}})
		require.False(t, decision.Allowed, resource.String())
	}
	var denied *DeniedError
	err := policy.Authorize(context.Background(), Request{Operation: Get, Resources: []Resource{CollectionResourceFor(nil, "Customer")}})
	require.True(t, errors.As(err, &denied))
}

// A policy of the caller's own type is handed the same kind and text for a
// schema-qualified source as it always was, and may read the identity beside them.
func TestCallerPolicySeesTheKindAndTextOfAQualifiedSource(t *testing.T) {
	seen := &[]string{}
	var identities []TableName
	policy := identityPolicy{recordingPolicy{seen: seen}, &identities}
	stub := &stubReadSession{}
	source := dal.NewQualifiedRootCollectionRef("hr", "Salary", "")
	_, err := SecureReadSession(stub, policy).ExecuteQueryToRecordsReader(context.Background(), tableRead(source))
	require.ErrorIs(t, err, ErrAccessDenied)
	require.Empty(t, stub.queries)
	require.Equal(t, []string{"opaque-query|opaque-query:hr.Salary"}, *seen)
	require.Equal(t, []TableName{{Schema: "hr", Name: "Salary"}}, identities)
}

// identityPolicy is a recording policy that also reads the identity of each resource.
type identityPolicy struct {
	recordingPolicy
	identities *[]TableName
}

func (p identityPolicy) Decide(ctx context.Context, request Request) Decision {
	for _, resource := range request.Resources {
		if table, ok := resource.Table(); ok {
			*p.identities = append(*p.identities, table)
		}
	}
	return p.recordingPolicy.Decide(ctx, request)
}

func (p identityPolicy) Authorize(ctx context.Context, request Request) error {
	if decision := p.Decide(ctx, request); !decision.Allowed {
		return &DeniedError{Decision: decision}
	}
	return nil
}

func TestCompiledTableRulesRankByPartsNamed(t *testing.T) {
	compiled, err := compileRules([]Rule{
		TableScope(TableName{Schema: "s", Name: "n"}, Allow(Query)),
		TableScope(TableName{Database: "d", Schema: "s", Name: "n"}, Allow(Query, "three")),
		TableScope(TableName{Database: "d", Name: "n"}, Allow(Query, "database")),
	}, map[effect]bool{effectAllow: true})
	require.NoError(t, err)
	require.Len(t, compiled, 3)
	require.Equal(t, `allow query at table schema="s" name="n"`, compiled[0].name)
	require.Equal(t, []int{2, 3, 2}, []int{compiled[0].depth, compiled[1].depth, compiled[2].depth})
	require.Equal(t, []int{2, 3, 2}, []int{compiled[0].literals, compiled[1].literals, compiled[2].literals})
	for _, rule := range compiled {
		require.Equal(t, tableRuleResource, rule.kind)
	}
	ranked := sortRules(append([]compiledRule(nil), compiled...))
	require.Equal(t, "three", ranked[0].name)
	require.Equal(t, `table database="d" schema="s" name="n"`, resourceDescription(tableRuleResource, compiled[1].resource, PathPattern{}))
}
