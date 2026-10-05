package access

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// The compatibility golden holds, for a corpus of policies that carry no table
// rule, the decision of every policy over every way a query or a key can name a
// source. It was produced by running this matrix on the code that had no table
// rules (see testdata/compat_golden/regenerate.sh) and it must not change: a
// policy written before table rules existed is decided exactly as it was.
//
// This file uses no name that table rules added, so it runs on that older code.

const compatGoldenPath = "testdata/compat_golden/decisions.golden"

// matrixSource is one way a query names a source.
type matrixSource struct {
	name   string
	source dal.RecordsetSource
}

func orderOrderSource() dal.CollectionRef { return dal.NewRootCollectionRef("Order", "o") }

// matrixSources are the spellings of a source named Customer: with no schema, with
// a schema, with a database, and with both, each as a value and as a pointer,
// with an alias, and bounded by a scan.
func matrixSources() []matrixSource {
	plain := dal.NewRootCollectionRef("Customer", "")
	aliased := dal.NewRootCollectionRef("Customer", "c")
	qualified := dal.NewQualifiedRootCollectionRef("sales", "Customer", "")
	qualifiedAliased := dal.NewQualifiedRootCollectionRef("sales", "Customer", "c")
	database := dal.NewDatabaseCollectionRef("app", "", "Customer", "")
	databaseAliased := dal.NewDatabaseCollectionRef("app", "", "Customer", "c")
	both := dal.NewDatabaseCollectionRef("app", "sales", "Customer", "")
	order := dal.AscendingField("id")
	return []matrixSource{
		{"unqualified", plain},
		{"unqualified pointer", &plain},
		{"unqualified aliased", aliased},
		{"unqualified scan-bounded", plain.WithScan(10, order)},
		{"schema", qualified},
		{"schema pointer", &qualified},
		{"schema aliased", qualifiedAliased},
		{"schema scan-bounded", qualified.WithScan(10, order)},
		{"database", database},
		{"database pointer", &database},
		{"database aliased", databaseAliased},
		{"database and schema", both},
		{"database and schema pointer", &both},
	}
}

// matrixPosition is one place in a query a source can stand.
type matrixPosition struct {
	name  string
	build func(source dal.RecordsetSource) dal.StructuredQuery
}

func matrixInner(source dal.RecordsetSource) dal.StructuredQuery {
	return dal.From(source).NewQuery().SelectKeysOnly(reflect.String)
}

func matrixOuter() dal.IQueryBuilder { return dal.From(orderOrderSource()).NewQuery() }

func matrixSubquery(source dal.RecordsetSource) dal.QueryExpression {
	return dal.NewQueryExpression(matrixInner(source), "x")
}

func matrixPositions() []matrixPosition {
	on := func(left, right string) dal.Condition {
		return dal.NewComparison(dal.NewFieldRef(left, "id"), dal.Equal, dal.NewFieldRef(right, "ref"))
	}
	return []matrixPosition{
		{"base", func(source dal.RecordsetSource) dal.StructuredQuery {
			return dal.From(source).NewQuery().SelectKeysOnly(reflect.String)
		}},
		{"join direct", func(source dal.RecordsetSource) dal.StructuredQuery {
			return dal.From(orderOrderSource()).Join(dal.NewJoinedSource(source, dal.JoinInner, on("o", "c"))).NewQuery().SelectKeysOnly(reflect.String)
		}},
		{"join tree", func(source dal.RecordsetSource) dal.StructuredQuery {
			return dal.From(orderOrderSource()).Join(dal.NewJoinedFrom(dal.From(source), dal.JoinInner, on("o", "c"))).NewQuery().SelectKeysOnly(reflect.String)
		}},
		{"derived source", func(source dal.RecordsetSource) dal.StructuredQuery {
			return dal.From(dal.NewQuerySource(matrixInner(source), "d")).NewQuery().SelectKeysOnly(reflect.String)
		}},
		{"subquery in WHERE", func(source dal.RecordsetSource) dal.StructuredQuery {
			return matrixOuter().Where(dal.NewExistsCondition(matrixInner(source))).SelectKeysOnly(reflect.String)
		}},
		{"subquery in HAVING", func(source dal.RecordsetSource) dal.StructuredQuery {
			return matrixOuter().GroupBy(dal.Field("a")).
				Having(dal.NewComparison(dal.Count().Expression, dal.Equal, matrixSubquery(source))).SelectKeysOnly(reflect.String)
		}},
		{"subquery in ON", func(source dal.RecordsetSource) dal.StructuredQuery {
			return dal.From(orderOrderSource()).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("Line", "l"), dal.JoinInner,
				dal.NewComparison(dal.NewFieldRef("l", "ref"), dal.Equal, matrixSubquery(source)),
			)).NewQuery().SelectKeysOnly(reflect.String)
		}},
		{"subquery in a column", func(source dal.RecordsetSource) dal.StructuredQuery {
			return matrixOuter().SelectColumns(dal.Column{Alias: "x", Expression: matrixSubquery(source)})
		}},
		{"subquery in ORDER BY", func(source dal.RecordsetSource) dal.StructuredQuery {
			return matrixOuter().OrderBy(dal.Ascending(matrixSubquery(source))).SelectKeysOnly(reflect.String)
		}},
	}
}

// matrixCells lists the (source, position) pairs of the golden: every spelling at
// the base and as a direct join, and the four basic spellings everywhere else.
func matrixCells() (cells []matrixCell) {
	positions := matrixPositions()
	for _, source := range matrixSources() {
		cells = append(cells, matrixCell{source, positions[0]}, matrixCell{source, positions[1]})
		if !strings.Contains(source.name, "pointer") && !strings.Contains(source.name, "aliased") && !strings.Contains(source.name, "scan") {
			for _, position := range positions[2:] {
				cells = append(cells, matrixCell{source, position})
			}
		}
	}
	return cells
}

type matrixCell struct {
	source   matrixSource
	position matrixPosition
}

func (c matrixCell) String() string { return c.source.name + " @ " + c.position.name }

// recordingPolicy is a policy of a caller's own type. It records the kind and
// the text of every resource it is asked about, denies the opaque-query kind and
// allows every other.
type recordingPolicy struct{ seen *[]string }

func (recordingPolicy) Name() string { return "recording" }

func (p recordingPolicy) Decide(_ context.Context, request Request) Decision {
	decision := Decision{Allowed: true, Operation: request.Operation, Policy: "recording", Effect: "allow"}
	for _, resource := range request.Resources {
		*p.seen = append(*p.seen, string(resource.Kind())+"|"+resource.String())
		if resource.Kind() == OpaqueQueryResource {
			decision = Decision{Operation: request.Operation, Resource: resource, Policy: "recording", Effect: "deny", Code: CodeRuleDenied, Explanation: "opaque queries are not allowed"}
		}
	}
	return decision
}

func (p recordingPolicy) Authorize(ctx context.Context, request Request) error {
	if decision := p.Decide(ctx, request); !decision.Allowed {
		return &DeniedError{Decision: decision}
	}
	return nil
}

// passThrough is an access policy wrapped in a type of the caller's own.
type passThrough struct{ Policy }

type matrixPolicy struct {
	name string
	// build returns the policy and the list the recording policy fills, when it is one.
	build func() (Policy, *[]string)
	ctx   func() context.Context
}

func matrixPrincipalContext() context.Context {
	return WithPrincipal(context.Background(), Principal{ID: "u1", Roles: []string{"reader"}})
}

func matrixPlain(policy Policy) func() (Policy, *[]string) {
	return func() (Policy, *[]string) { return policy, nil }
}

func matrixMasked(rules ...Rule) func() (Policy, *[]string) {
	return func() (Policy, *[]string) {
		policy := MustPolicy("masked", rules...)
		mask, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"Customer", "Order", "Line"}}}}, NameMask)
		if err != nil {
			panic(err)
		}
		policy.collectionMask = mask
		return policy, nil
	}
}

func matrixPrincipalSet(masked bool) func() (Policy, *[]string) {
	return func() (Policy, *[]string) {
		set := MustPrincipalPolicySet("principals", map[string][]Rule{
			"base":      {Root(Allow(Read, "read-all"))},
			"customers": {Collection("Customer", Allow(Write, "write-customers"))},
		}, Bindings{Roles: map[string][]string{"reader": {"base", "customers"}}})
		if masked {
			mask, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"Customer", "Order", "Line"}}}}, NameMask)
			if err != nil {
				panic(err)
			}
			set.collectionMask = mask
		}
		return set, nil
	}
}

func matrixPolicies() []matrixPolicy {
	background := context.Background
	return []matrixPolicy{
		{"root allow", matrixPlain(MustPolicy("p", Root(Allow(ReadWrite, "all")))), background},
		{"collection allow", matrixPlain(MustPolicy("p", Collection("Customer", Allow(ReadWrite, "customers")), Collection("Order", Allow(ReadWrite, "orders")), Collection("Line", Allow(ReadWrite, "lines")))), background},
		{"field list", matrixPlain(MustPolicy("p", Root(Allow(Read, "all").Fields("id", "name")))), background},
		{"row condition", matrixPlain(MustPolicy("p", Collection("Customer", Allow(Read, "gb").Where(dal.WhereField("country", dal.Equal, "GB"))), Collection("Order", Allow(Read, "orders")), Collection("Line", Allow(Read, "lines")))), background},
		{"root allow, collection denied", matrixPlain(MustPolicy("p", Root(Allow(ReadWrite, "all")), Collection("Customer", Deny(ReadWrite, "no-customers")))), background},
		{"opaque allow", matrixPlain(MustPolicy("p", OpaqueQueryScope(Allow(Query, "opaque")))), background},
		{"opaque and collection allow", matrixPlain(MustPolicy("p", OpaqueQueryScope(Allow(Query, "opaque")), Root(Allow(ReadWrite, "all")))), background},
		{"record scope", matrixPlain(MustPolicy("p", Root(Allow(Query, "query")), Scope("Customer", AnyID, Allow(Read|Write, "records")))), background},
		{"unrelated rule", matrixPlain(MustPolicy("p", Collection("Other", Allow(ReadWrite, "other")))), background},
		{"collection mask", matrixMasked(Root(Allow(ReadWrite, "all"))), background},
		{"principal set", matrixPrincipalSet(false), matrixPrincipalContext},
		{"principal set with a collection mask", matrixPrincipalSet(true), matrixPrincipalContext},
		{"recording policy of the caller", func() (Policy, *[]string) {
			seen := &[]string{}
			return recordingPolicy{seen: seen}, seen
		}, background},
		{"access policy wrapped by the caller", func() (Policy, *[]string) {
			return passThrough{MustPolicy("p", Root(Allow(ReadWrite, "all")), Collection("Customer", Deny(Write, "no-customer-writes")))}, nil
		}, background},
	}
}

func summarizeDecision(decision Decision) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("allowed=%t", decision.Allowed))
	parts = append(parts, "code="+string(decision.Code), "scope="+string(decision.Scope), "slot="+string(decision.Slot))
	parts = append(parts, fmt.Sprintf("rule=%q effect=%s explanation=%q condition=%q", decision.Rule, decision.Effect, decision.Explanation, decision.Condition))
	parts = append(parts, "resource="+string(decision.Resource.Kind())+"|"+decision.Resource.String())
	for i, residual := range decision.Residuals {
		if residual != nil {
			parts = append(parts, fmt.Sprintf("residual[%d]=%s", i, residual))
		}
	}
	for i, write := range decision.Writes {
		if write == nil {
			continue
		}
		for _, alternative := range write.Alternatives {
			parts = append(parts, fmt.Sprintf("write[%d].alt=%q where=%q check=%q fields=%v", i, alternative.Rule, alternative.WhereText, alternative.CheckText, alternative.Fields))
		}
		if write.Terminal != nil {
			parts = append(parts, fmt.Sprintf("write[%d].terminal=%q where=%q check=%q fields=%v", i, write.Terminal.Rule, write.Terminal.WhereText, write.Terminal.CheckText, write.Terminal.Fields))
		}
	}
	return strings.Join(parts, " ")
}

func summarizeResources(resources []Resource) string {
	texts := make([]string, len(resources))
	for i, resource := range resources {
		texts[i] = string(resource.Kind()) + "|" + resource.String()
	}
	return strings.Join(texts, " ; ")
}

func compatGolden() []string {
	var lines []string
	add := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	for _, policy := range matrixPolicies() {
		for _, cell := range matrixCells() {
			ctx := policy.ctx()
			query := cell.position.build(cell.source.source)
			resources := resourcesForQuery(query)

			decider, seen := policy.build()
			decision := decider.Decide(ctx, Request{Operation: Query, Resources: resources, Query: query})
			recorded := ""
			if seen != nil {
				recorded = " seen=[" + strings.Join(*seen, " ; ") + "]"
			}
			add("%s | %s | resources: %s | decide: %s%s", policy.name, cell, summarizeResources(resources), summarizeDecision(decision), recorded)

			sessionPolicy, seen := policy.build()
			wrapped := &countingSession{}
			_, err := SecureReadSession(wrapped, sessionPolicy).ExecuteQueryToRecordsReader(ctx, query)
			outcome := "allowed"
			switch {
			case errors.Is(err, ErrAccessDenied):
				outcome = "denied: " + err.Error()
			case err != nil:
				outcome = "error"
			}
			recorded = ""
			if seen != nil {
				recorded = " seen=[" + strings.Join(*seen, " ; ") + "]"
			}
			add("%s | %s | session: %s reads=%d%s", policy.name, cell, outcome, wrapped.reads, recorded)
		}
		for _, key := range []struct {
			name string
			key  *record.Key
		}{
			{"key", record.NewKeyWithID("Customer", "1")},
			{"key with a parent", record.NewKeyWithParentAndID(record.NewKeyWithID("Order", "7"), "Customer", "1")},
			{"key of another collection", record.NewKeyWithID("Order", "1")},
		} {
			for _, operation := range []Operations{Get, Exists, Insert, Set, Update, Delete} {
				decider, seen := policy.build()
				decision := decider.Decide(policy.ctx(), Request{Operation: operation, Resources: []Resource{RecordResourceForKey(key.key)}})
				recorded := ""
				if seen != nil {
					recorded = " seen=[" + strings.Join(*seen, " ; ") + "]"
				}
				add("%s | %s | %s | decide: %s%s", policy.name, key.name, operation, summarizeDecision(decision), recorded)
			}
		}
	}
	return lines
}

func TestCompatGolden(t *testing.T) {
	got := compatGolden()
	if parent := os.Getenv("DALGO_COMPAT_GOLDEN_PARENT"); parent != "" {
		header := "# Decisions of the policies of the matrix over every source spelling, written by\n" +
			"# testdata/compat_golden/regenerate.sh from the code of commit " + parent + ",\n" +
			"# which has no table rules. Do not edit by hand.\n"
		if err := os.WriteFile(compatGoldenPath, []byte(header+strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d lines to %s", len(got), compatGoldenPath)
		return
	}
	data, err := os.ReadFile(compatGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if !strings.HasPrefix(line, "#") {
			want = append(want, line)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the matrix has %d lines, the golden has %d", len(got), len(want))
	}
	differing := 0
	for i := 0; i < len(got) && i < len(want); i++ {
		if got[i] != want[i] {
			if differing++; differing <= 5 {
				t.Errorf("line %d differs from the golden\n  got:  %s\n  want: %s", i+1, got[i], want[i])
			}
		}
	}
	if differing > 5 {
		t.Errorf("%d lines differ from the golden in all", differing)
	}
}
