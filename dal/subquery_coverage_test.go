package dal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type unnamedRecursiveSource struct{}

func (unnamedRecursiveSource) Name() string     { return "" }
func (unnamedRecursiveSource) Alias() string    { return "" }
func (unnamedRecursiveSource) recordsetSource() {}

type malformedRecursiveExpression struct{}

func (malformedRecursiveExpression) String() string { return "malformed expression" }

type malformedRecursiveCondition struct{}

func (malformedRecursiveCondition) String() string { return "malformed condition" }

type scriptedRecursiveBackend struct {
	reader RecordsReader
	err    error
}

type scriptedRecursiveSchemaBackend struct{ scriptedRecursiveBackend }

func (scriptedRecursiveSchemaBackend) JoinFields(context.Context, RecordsetSource) ([]string, error) {
	return []string{"id"}, nil
}

func (b scriptedRecursiveBackend) ExecuteQueryToRecordsReader(context.Context, Query) (RecordsReader, error) {
	return b.reader, b.err
}
func (scriptedRecursiveBackend) ExecuteQueryToRecordsetReader(context.Context, Query, ...recordset.Option) (RecordsetReader, error) {
	return nil, errors.New("recordset unsupported")
}

type closeErrorRecursiveReader struct{ RecordsReader }

func (closeErrorRecursiveReader) Close() error { return errors.New("reader close failed") }

type readErrorRecursiveReader struct{ EmptyReader }

func (readErrorRecursiveReader) Next() (record.Record, error) {
	return nil, errors.New("reader read failed")
}

type cancelOnCloseRecursiveReader struct {
	RecordsReader
	cancel context.CancelFunc
}

func (r cancelOnCloseRecursiveReader) Close() error {
	r.cancel()
	return r.RecordsReader.Close()
}

func TestRecursiveTreeInspectionAtEachNestedPosition(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	root := NewRootCollectionRef("Customer", "c")
	if inspectQueryTree(nil, func(StructuredQuery) bool { return true }) {
		t.Fatal("nil query reported a nested query")
	}
	if inspectQueryTree(&subqueryTestQuery{}, func(StructuredQuery) bool { return true }) {
		t.Fatal("nil FROM reported a nested query")
	}
	loop := &subqueryTestQuery{}
	loop.from = From(NewQuerySource(loop, "loop"))
	if inspectQueryTree(loop, func(StructuredQuery) bool { return false }) {
		t.Fatal("cyclic query reported a predicate match")
	}
	matchLeaf := func(candidate StructuredQuery) bool { return candidate != nil && candidate.String() == leaf.String() }
	cases := map[string]StructuredQuery{
		"aggregate argument": From(root).NewQuery().SelectColumns(Column{Expression: NewAggregate(COUNT, false, NewQueryExpression(leaf, "v"))}),
		"group by":           From(root).NewQuery().GroupBy(NewQueryExpression(leaf, "v")).SelectIntoRecord(nil),
		"order by":           From(root).NewQuery().OrderBy(Ascending(NewQueryExpression(leaf, "v"))).SelectIntoRecord(nil),
		"join ON": From(root).Join(NewJoinedSource(NewRootCollectionRef("Invoice", "i"), JoinInner,
			NewExistsCondition(leaf))).NewQuery().SelectIntoRecord(nil),
		"nested relation": From(root).Join(NewNestedJoinedSource(
			From(NewQuerySource(leaf, "d")), JoinInner)).NewQuery().SelectIntoRecord(nil),
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			if !inspectQueryTree(query, matchLeaf) {
				t.Fatal("nested query was not visited")
			}
		})
	}
}

func TestRecursiveScopeRejectsInvalidClauseAndSourcePositions(t *testing.T) {
	root := NewRootCollectionRef("Customer", "c")
	bad := NewFieldRef("missing", "id")
	cases := map[string]StructuredQuery{
		"HAVING":            From(root).NewQuery().Having(NewComparison(bad, Equal, Constant{Value: 1})).SelectIntoRecord(nil),
		"GROUP BY":          From(root).NewQuery().GroupBy(bad).SelectIntoRecord(nil),
		"ORDER BY":          From(root).NewQuery().OrderBy(Ascending(bad)).SelectIntoRecord(nil),
		"wildcard":          From(root).NewQuery().SelectColumns(Column{Wildcard: &WildcardProjection{Source: "missing"}}),
		"derived query":     From(NewQuerySource(nil, "d")).NewQuery().SelectIntoRecord(nil),
		"empty source name": From(unnamedRecursiveSource{}).NewQuery().SelectIntoRecord(nil),
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateQueryScope(query); err == nil {
				t.Fatal("invalid recursive scope was accepted")
			}
		})
	}
	var nilSource *QuerySource
	if err := ValidateQueryScope(From(nilSource).NewQuery().SelectIntoRecord(nil)); err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("nil derived source error = %v", err)
	}
	if source, ok := asQuerySource(nilSource); !ok || source.Query() != nil {
		t.Fatalf("nil derived source inspection = %#v, %v", source, ok)
	}
	if queryPointerID(nil) != 0 {
		t.Fatal("nil query has a pointer identity")
	}
	if err := validateJoinFrom(From(nilSource), "from", nil, map[FromSource]bool{}); err == nil {
		t.Fatal("JOIN validation accepted nil derived source")
	}
	if _, err := relationAliases(From(nilSource), "from", map[FromSource]bool{}); err == nil {
		t.Fatal("JOIN alias enumeration accepted nil derived source")
	}
}

func TestRecursiveEvaluatorHandlesMalformedAndThreeValuedOperands(t *testing.T) {
	execution := &joinExecution{ctx: context.Background(), recursive: true}
	row := joinRow{base: "c", sources: map[string]map[string]any{"c": {"id": 1}}}
	if value, err := execution.evalExpressionAt(NewFieldRef("", "id"), row, "field"); err != nil || value != 1 {
		t.Fatalf("base field = %v, %v", value, err)
	}
	for name, expression := range map[string]Expression{
		"unsupported":  malformedRecursiveExpression{},
		"binary left":  Binary(malformedRecursiveExpression{}, Add, Constant{Value: 1}),
		"binary right": Binary(Constant{Value: 1}, Add, malformedRecursiveExpression{}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := execution.evalExpressionAt(expression, row, "expression"); err == nil {
				t.Fatal("malformed expression was accepted")
			}
		})
	}
	if _, err := execution.evalTruthAt(malformedRecursiveCondition{}, row, "where"); err == nil {
		t.Fatal("malformed condition was accepted")
	}
	unknown := NewComparison(Constant{Value: nil}, Equal, Constant{Value: 1})
	truth, err := execution.evalTruthAt(NewGroupCondition(And, unknown), row, "where")
	if err != nil || truth != queryUnknown {
		t.Fatalf("AND unknown = %v, %v", truth, err)
	}
	for name, comparison := range map[string]Comparison{
		"greater":          NewComparison(Constant{Value: 1}, GreaterThen, Constant{Value: 2}),
		"greater or equal": NewComparison(Constant{Value: 1}, GreaterOrEqual, Constant{Value: 2}),
		"less":             NewComparison(Constant{Value: 2}, LessThen, Constant{Value: 1}),
		"less or equal":    NewComparison(Constant{Value: 2}, LessOrEqual, Constant{Value: 1}),
	} {
		t.Run(name, func(t *testing.T) {
			truth, err := execution.evalTruthAt(comparison, row, "where")
			if err != nil || truth != queryFalse {
				t.Fatalf("false comparison = %v, %v", truth, err)
			}
		})
	}
	for name, comparison := range map[string]Comparison{
		"left":        NewComparison(malformedRecursiveExpression{}, Equal, Constant{Value: 1}),
		"right":       NewComparison(Constant{Value: 1}, Equal, malformedRecursiveExpression{}),
		"IN right":    NewComparison(Constant{Value: 1}, In, malformedRecursiveExpression{}),
		"IN left":     NewComparison(malformedRecursiveExpression{}, In, NewArray([]int{1})),
		"binary left": NewComparison(Binary(malformedRecursiveExpression{}, Add, Constant{Value: 1}), Equal, Constant{Value: 1}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := execution.evalTruthAt(comparison, row, "where"); err == nil {
				t.Fatal("malformed comparison was accepted")
			}
		})
	}
}

func TestRecursiveJoinInspectionSkipsNonKeyPredicates(t *testing.T) {
	root := NewRootCollectionRef("Customer", "c")
	child := NewRootCollectionRef("Invoice", "i")
	joined := NewJoinedSource(child, JoinInner,
		NewExistsCondition(nil),
		NewComparison(Constant{Value: 1}, Equal, NewFieldRef("i", "id")),
		NewComparison(NewFieldRef("c", "id"), Equal, Constant{Value: 1}),
	)
	from := From(root).Join(joined)
	execution := &joinExecution{keyRefs: map[string][]joinKeyReference{}}
	execution.collectKeyRefs(from, "from")
	if len(execution.keyRefs["i"]) != 1 || len(execution.keyRefs["c"]) != 1 {
		t.Fatalf("collected key refs = %#v", execution.keyRefs)
	}
	parent := joinRow{sources: map[string]map[string]any{"c": {"id": 1}}}
	if _, _, applicable := directJoinHashKey(joined, "i", parent); applicable {
		t.Fatal("non-key predicates were treated as a direct hash key")
	}
}

func TestRecursiveOuterReferenceInspectionAtGroupAndOrder(t *testing.T) {
	root := NewRootCollectionRef("Invoice", "i")
	for name, query := range map[string]StructuredQuery{
		"nil FROM": &subqueryTestQuery{},
		"group":    From(root).NewQuery().GroupBy(NewFieldRef("outer", "id")).SelectIntoRecord(nil),
		"order":    From(root).NewQuery().OrderBy(Ascending(NewFieldRef("outer", "id"))).SelectIntoRecord(nil),
		"binary":   From(root).NewQuery().SelectColumns(Column{Expression: Binary(Constant{Value: 1}, Add, NewFieldRef("outer", "id"))}),
	} {
		t.Run(name, func(t *testing.T) {
			found := queryHasOuterReference(query)
			if name != "nil FROM" && !found {
				t.Fatal("outer reference was missed")
			}
		})
	}
}

func TestRecursiveFieldMetadataAndOutputLimits(t *testing.T) {
	root := NewRootCollectionRef("Customer", "c")
	for name, tc := range map[string]struct {
		query     StructuredQuery
		fields    map[string][]string
		wantError bool
	}{
		"metadata unavailable uses base":         {query: From(root).NewQuery().SelectColumns(Column{Expression: NewFieldRef("", "id")}), fields: map[string][]string{}},
		"complete metadata rejects absent field": {query: From(root).NewQuery().SelectColumns(Column{Expression: NewFieldRef("", "missing")}), fields: map[string][]string{"c": {"id"}}, wantError: true},
		"aggregate output name":                  {query: From(root).NewQuery().SelectColumns(Column{Expression: NewAggregate(COUNT, false, Star())}), fields: map[string][]string{"c": {"id"}}},
	} {
		t.Run(name, func(t *testing.T) {
			execution := &joinExecution{q: tc.query, recursive: true, aliases: []string{"c"}, fields: tc.fields, keyRefs: map[string][]joinKeyReference{}}
			if err := execution.validateQueryFields(); (err != nil) != tc.wantError {
				t.Fatalf("field validation = %v, want error %t", err, tc.wantError)
			}
		})
	}
	execution := &joinExecution{budget: &recursiveBudget{bytes: maxJoinBytes}}
	if err := execution.chargeOutput(map[string]any{"id": 1}); err == nil || !strings.Contains(err.Error(), "retained_bytes") {
		t.Fatalf("retained byte bound = %v", err)
	}
	execution = &joinExecution{ctx: context.Background(), fields: map[string][]string{}}
	if err := execution.scanTree(From(NewQuerySource(nil, "d")), "from"); err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("nil derived scan = %v", err)
	}
}

func TestRecursiveCappedLeafSurfacesReadAndResourceErrors(t *testing.T) {
	base := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	good := joinTestRecord("Invoice", "1", map[string]any{"id": 1})
	bad := record.NewRecordWithData(good.Key(), 17)
	cases := map[string]struct {
		backend scriptedRecursiveBackend
		query   StructuredQuery
		budget  recursiveBudget
	}{
		"backend read":     {backend: scriptedRecursiveBackend{err: errors.New("read failed")}, query: base},
		"reader close":     {backend: scriptedRecursiveBackend{reader: closeErrorRecursiveReader{NewRecordsReader([]record.Record{good})}}, query: base},
		"invalid raw row":  {backend: scriptedRecursiveBackend{reader: NewRecordsReader([]record.Record{bad})}, query: base},
		"retained bytes":   {backend: scriptedRecursiveBackend{reader: NewRecordsReader([]record.Record{good})}, query: base, budget: recursiveBudget{bytes: maxJoinBytes}},
		"where error":      {backend: scriptedRecursiveBackend{reader: NewRecordsReader([]record.Record{good})}, query: From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(malformedRecursiveExpression{}, Equal, Constant{Value: 1})).SelectIntoRecord(nil)},
		"projection error": {backend: scriptedRecursiveBackend{reader: NewRecordsReader([]record.Record{good})}, query: From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Alias: "bad", Expression: malformedRecursiveExpression{}})},
		"result rows":      {backend: scriptedRecursiveBackend{reader: NewRecordsReader([]record.Record{good})}, query: base, budget: recursiveBudget{output: maxJoinRows}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			execution := &joinExecution{ctx: context.Background(), executor: tc.backend, budget: &tc.budget, memo: map[string][]memoizedQuery{}}
			if _, err := execution.executeSimpleCapped(tc.query, nil, 2, true); err == nil {
				t.Fatal("failed capped leaf read was accepted")
			}
		})
	}
}

func TestRecursiveRemainingScopeAndTreeBranches(t *testing.T) {
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	root := NewRootCollectionRef("Customer", "c")
	grouped := From(root).NewQuery().Where(NewGroupCondition(And, NewExistsCondition(leaf))).SelectIntoRecord(nil)
	if !inspectQueryTree(grouped, func(q StructuredQuery) bool { return q != nil && q.String() == leaf.String() }) {
		t.Fatal("nested query in grouped condition was missed")
	}
	duplicate := From(root).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c"), JoinInner)).NewQuery().SelectIntoRecord(nil)
	if err := ValidateQueryScope(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Fatalf("duplicate alias = %v", err)
	}
	badLeaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().Where(NewComparison(NewFieldRef("missing", "id"), Equal, Constant{Value: 1})).SelectIntoRecord(nil)
	badDerived := From(NewQuerySource(badLeaf, "derived")).NewQuery().SelectIntoRecord(nil)
	if err := ValidateQueryScope(badDerived); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("invalid nested scope = %v", err)
	}
	if err := validateConditionScope(malformedRecursiveCondition{}, nil, "where", map[uintptr]bool{}); err != nil {
		t.Fatalf("unsupported condition scope should be deferred to execution: %v", err)
	}
	if err := validateFromScope(From(root), nil, map[string]bool{"c": true}, "from", map[uintptr]bool{}); err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Fatalf("duplicate local source = %v", err)
	}
	if err := validateFromScope(From(NewQuerySource(badLeaf, "derived")), nil, map[string]bool{}, "from", map[uintptr]bool{}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("invalid derived source = %v", err)
	}
	badChild := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Receipt", "i"), JoinInner))
	if err := validateFromScope(From(root).Join(NewNestedJoinedSource(badChild, JoinInner)), nil, map[string]bool{}, "from", map[uintptr]bool{}); err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Fatalf("invalid JOIN child scope = %v", err)
	}
}

func TestRecursiveRemainingEvaluatorBranches(t *testing.T) {
	ctx := context.Background()
	row := joinRow{base: "c", sources: map[string]map[string]any{"c": {"id": 1}}}
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Alias: "id", Expression: NewFieldRef("i", "id")})
	execution := &joinExecution{ctx: ctx, executor: scriptedRecursiveBackend{reader: &EmptyReader{}}, budget: &recursiveBudget{}, memo: map[string][]memoizedQuery{}, recursive: true}
	if _, err := execution.evalExpressionAt(NewQueryExpression(nil, "v"), row, "scalar"); err == nil {
		t.Fatal("nil scalar query was accepted")
	}
	if value, err := execution.evalExpressionAt(NewQueryExpression(leaf, "v"), row, "scalar"); err != nil || value != nil {
		t.Fatalf("empty scalar query = %v, %v", value, err)
	}
	if truth, err := execution.evalTruthAt(NewNotExistsCondition(leaf), row, "where"); err != nil || truth != queryTrue {
		t.Fatalf("NOT EXISTS empty query = %v, %v", truth, err)
	}
	if _, err := execution.evalTruthAt(NewComparison(Constant{Value: 1}, In, NewQueryExpression(nil, "set")), row, "where"); err == nil {
		t.Fatal("nil membership query was accepted")
	}
	for name, comparison := range map[string]Comparison{
		"greater or equal": NewComparison(Constant{Value: 2}, GreaterOrEqual, Constant{Value: 1}),
		"less or equal":    NewComparison(Constant{Value: 1}, LessOrEqual, Constant{Value: 2}),
	} {
		truth, err := execution.evalTruthAt(comparison, row, "where")
		if err != nil || truth != queryTrue {
			t.Fatalf("%s = %v, %v", name, truth, err)
		}
	}
	bad := record.NewRecordWithData(joinTestRecord("Invoice", "bad", map[string]any{}).Key(), 17)
	key := fmt.Sprintf("%s\x00%d\x00%t", leaf.String(), 2, true)
	execution.memo[key] = []memoizedQuery{{query: leaf, records: []record.Record{bad}}}
	if _, err := execution.evalExpressionAt(NewQueryExpression(leaf, "v"), row, "scalar"); err == nil || !strings.Contains(err.Error(), "non-object") {
		t.Fatalf("non-object scalar = %v", err)
	}
	wide := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Alias: "id", Expression: NewFieldRef("i", "id")}, Column{Alias: "total", Expression: NewFieldRef("i", "total")})
	wideKey := fmt.Sprintf("%s\x00%d\x00%t", wide.String(), 0, true)
	execution.memo[wideKey] = []memoizedQuery{{query: wide, records: []record.Record{record.NewRecordWithData(bad.Key(), map[string]any{"id": 1, "total": 2})}}}
	if _, err := execution.evalTruthAt(NewComparison(Constant{Value: 1}, In, NewQueryExpression(wide, "set")), row, "where"); err == nil || !strings.Contains(err.Error(), "exactly one column") {
		t.Fatalf("wide membership result = %v", err)
	}
	if ok, err := evalJoinCondition(NewGroupCondition(And, NewComparison(Constant{Value: 1}, Equal, Constant{Value: 2})), row); err != nil || ok {
		t.Fatalf("false JOIN condition = %t, %v", ok, err)
	}
}

func TestRecursiveJoinPropagatesDerivedAndPredicateErrors(t *testing.T) {
	base := joinRow{base: "c", sources: map[string]map[string]any{"c": {"id": 1}}}
	derived := NewJoinedSource(NewQuerySource(nil, "d"), JoinInner)
	execution := &joinExecution{ctx: context.Background(), recursive: true, budget: &recursiveBudget{}, memo: map[string][]memoizedQuery{}, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}}
	if _, err := execution.applyJoin([]joinRow{base}, derived, "from.joins[0]"); err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("derived query error = %v", err)
	}
	leaf := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Alias: "id", Expression: NewFieldRef("i", "id")})
	bad := record.NewRecordWithData(joinTestRecord("Invoice", "bad", map[string]any{}).Key(), 17)
	key := fmt.Sprintf("%s\x00%d\x00%t", leaf.String(), 0, true)
	execution.memo[key] = []memoizedQuery{{query: leaf, records: []record.Record{bad}}}
	if _, err := execution.applyJoin([]joinRow{base}, NewJoinedSource(NewQuerySource(leaf, "d"), JoinInner), "from.joins[0]"); err == nil {
		t.Fatalf("derived row shape = %v", err)
	}
	execution.scans["i"] = []scannedJoinRow{{data: map[string]any{"id": 1}}}
	broken := NewJoinedSource(NewRootCollectionRef("Invoice", "i"), JoinInner, malformedRecursiveCondition{})
	if _, err := execution.applyJoin([]joinRow{base}, broken, "from.joins[0]"); err == nil || !strings.Contains(err.Error(), "unsupported condition") {
		t.Fatalf("JOIN predicate error = %v", err)
	}
}

func TestRecursiveRecordsetEmptySchemaAndInvalidQuery(t *testing.T) {
	ctx := context.Background()
	backend := scriptedRecursiveSchemaBackend{scriptedRecursiveBackend{reader: &EmptyReader{}}}
	if _, err := ExecuteRecursiveRecordset(ctx, backend, From(NewQuerySource(nil, "d")).NewQuery().SelectIntoRecord(nil)); err == nil {
		t.Fatal("invalid derived query was accepted")
	}
	root := NewRootCollectionRef("Invoice", "i")
	for name, query := range map[string]StructuredQuery{
		"wildcard": From(root).NewQuery().SelectColumns(Column{Wildcard: &WildcardProjection{Source: "i"}}),
		"field":    From(root).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ExecuteRecursiveRecordset(ctx, backend, query); err != nil {
				t.Fatalf("empty recursive recordset = %v", err)
			}
		})
	}
}

func TestRecursiveRecordsetMaterializationRejectsInvalidReaderRows(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	if _, err := recursiveReaderToRecordset(context.Background(), readErrorRecursiveReader{}, query); err == nil || !strings.Contains(err.Error(), "reader read failed") {
		t.Fatalf("read failure = %v", err)
	}
	bad := record.NewRecordWithData(joinTestRecord("Invoice", "bad", map[string]any{}).Key(), 17)
	if _, err := recursiveReaderToRecordset(context.Background(), NewRecordsReader([]record.Record{bad}), query); err == nil || !strings.Contains(err.Error(), "non-object") {
		t.Fatalf("row shape = %v", err)
	}
}

func TestRecursiveEmptyGroupedResultProducesEmptyReader(t *testing.T) {
	query := From(NewRootCollectionRef("Invoice", "i")).NewQuery().GroupBy(NewFieldRef("i", "id")).SelectColumns(
		Column{Alias: "id", Expression: NewFieldRef("i", "id")},
		Column{Alias: "count", Expression: NewAggregate(COUNT, false, Star())},
	)
	reader, err := ExecuteRecursiveQuery(context.Background(), scriptedRecursiveBackend{reader: &EmptyReader{}}, query)
	if err != nil {
		t.Fatalf("grouped empty query = %v", err)
	}
	if _, err := reader.Next(); !errors.Is(err, ErrNoMoreRecords) {
		t.Fatalf("empty grouped result = %v", err)
	}
}

func TestRecursiveNestedAndJoinRecordsetHonorCancellationAfterLeafRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leaf := joinTestRecord("Invoice", "one", map[string]any{"id": 1})
	backend := scriptedRecursiveBackend{reader: cancelOnCloseRecursiveReader{RecordsReader: NewRecordsReader([]record.Record{leaf}), cancel: cancel}}
	child := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectColumns(Column{Expression: NewFieldRef("i", "id")})
	execution := &joinExecution{ctx: ctx, executor: backend, budget: &recursiveBudget{}, memo: map[string][]memoizedQuery{}, recursive: true}
	if _, err := execution.queryRecordsAt(child, nil, "nested.query"); !errors.Is(err, context.Canceled) {
		t.Fatalf("nested cancellation = %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	backend = scriptedRecursiveBackend{reader: cancelOnCloseRecursiveReader{RecordsReader: NewRecordsReader([]record.Record{leaf}), cancel: cancel}}
	joined := From(NewRootCollectionRef("Invoice", "i")).Join(NewJoinedSource(NewRootCollectionRef("Receipt", "r"), JoinLeft, NewComparison(NewFieldRef("i", "id"), Equal, NewFieldRef("r", "invoiceId")))).NewQuery().SelectColumns(Column{Alias: "id", Expression: NewFieldRef("i", "id")})
	if _, err := executeJoinRecordset(ctx, backend, joined, QueryCapabilities{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("JOIN recordset cancellation = %v", err)
	}
}
