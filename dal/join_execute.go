package dal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/dal-go/record"
)

const (
	joinSourcesKey = "\x00dalgo_join_sources"
	joinBaseKey    = "\x00dalgo_join_base"
	maxJoinRows    = 10000
	maxJoinBytes   = 16 << 20
)

type joinRow struct {
	key     *record.Key
	base    string
	sources map[string]map[string]any
	outer   *joinRow
}

type scannedJoinRow struct {
	key  *record.Key
	data map[string]any
}

type joinExecution struct {
	ctx        context.Context
	q          StructuredQuery
	executor   QueryExecutor
	scans      map[string][]scannedJoinRow
	indexes    map[string]map[string][]scannedJoinRow
	fields     map[string][]string
	keyRefs    map[string][]joinKeyReference
	aliases    []string
	bytes      int
	fetched    int
	candidates int
	outer      *joinRow
	recursive  bool
	budget     *recursiveBudget
	memo       map[string][]memoizedQuery
}

type recursiveBudget struct {
	fetched, output, candidates, bytes int
	memo                               map[string][]memoizedQuery
}

type memoizedQuery struct {
	query         StructuredQuery
	records       []record.Record
	candidateWork int
}

// queryTruth retains SQL's third truth value until a WHERE, ON, or HAVING
// boundary decides that only TRUE retains a row.
type queryTruth uint8

const (
	queryUnknown queryTruth = iota
	queryFalse
	queryTrue
)

type joinKeyReference struct{ field, path string }

// executePlannedRecords is shared by DB and transaction entrypoints.
func executePlannedRecords(ctx context.Context, executor QueryExecutor, query Query, capabilities QueryCapabilities, provider NativeJoinProvider) (RecordsReader, error) {
	if q, ok := query.(StructuredQuery); ok && HasSubquery(q) {
		return executeGenericRecursive(ctx, executor, q, nil)
	}
	if !hasJoin(query) {
		return executeAggregationRecords(ctx, executor, query, capabilities)
	}
	q := query.(StructuredQuery)
	plan, err := PlanJoin(ctx, q, provider)
	if err != nil {
		return nil, err
	}
	if plan.Strategy == JoinNative {
		return executor.ExecuteQueryToRecordsReader(ctx, query)
	}
	return executeGenericJoin(ctx, executor, q)
}

// executeGenericRecursive materializes a recursive DTQL query through ordinary
// QueryExecutor leaf reads. It deliberately has no native capability route:
// adapters must opt into a complete recursive implementation in a later
// capability contract.
func executeGenericRecursive(ctx context.Context, executor QueryExecutor, q StructuredQuery, outer *joinRow) (RecordsReader, error) {
	if _, err := PlanRecursiveQuery(q); err != nil {
		return nil, err
	}
	return executeGenericRecursiveBudget(ctx, executor, q, outer, &recursiveBudget{})
}

func executeGenericRecursiveBudget(ctx context.Context, executor QueryExecutor, q StructuredQuery, outer *joinRow, budget *recursiveBudget) (RecordsReader, error) {
	if budget.memo == nil {
		budget.memo = map[string][]memoizedQuery{}
	}
	e := &joinExecution{ctx: ctx, q: q, executor: executor, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}, fields: map[string][]string{}, keyRefs: map[string][]joinKeyReference{}, outer: outer, recursive: true, budget: budget, memo: budget.memo}
	return e.execute()
}

// ExecuteRecursiveQuery evaluates a recursive DTQL query through executor's
// ordinary leaf-read surface. Callers that enforce access policy should pass
// their secured executor so every nested source is authorized independently.
func ExecuteRecursiveQuery(ctx context.Context, executor QueryExecutor, q StructuredQuery) (RecordsReader, error) {
	return executeGenericRecursive(ctx, executor, q, nil)
}

func executeGenericJoin(ctx context.Context, executor QueryExecutor, q StructuredQuery) (RecordsReader, error) {
	e := &joinExecution{ctx: ctx, q: q, executor: executor, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}, fields: map[string][]string{}, keyRefs: map[string][]joinKeyReference{}}
	return e.execute()
}

func (e *joinExecution) execute() (RecordsReader, error) {
	if e.q.StartFrom() != "" || e.q.StartAfter() != "" {
		return nil, joinError("join_plan", "from", "generic JOIN does not support provider cursors")
	}
	e.collectKeyRefs(e.q.From(), "from")
	if err := e.scanTree(e.q.From(), "from"); err != nil {
		return nil, err
	}
	if err := e.validateQueryFields(); err != nil {
		return nil, err
	}
	rows, err := e.build(e.q.From(), "from", nil, nil)
	if err != nil {
		return nil, err
	}
	filtered := make([]joinRow, 0, len(rows))
	for _, row := range rows {
		ok, err := e.conditionAt(e.q.Where(), row, "where")
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, row)
		}
	}
	if HasAggregation(e.q) {
		records := make([]record.Record, len(filtered))
		for i, row := range filtered {
			data := flattenJoinRow(row, e.aliases, true)
			if err := e.chargeOutput(data); err != nil {
				return nil, err
			}
			records[i] = record.NewRecordWithData(row.key, data)
		}
		plan, err := PlanAggregation(e.q, QueryCapabilities{StableRowOrder: true})
		if err != nil {
			return nil, err
		}
		aggregated := newLocalAggregationReader(e.ctx, e.q, NewRecordsReader(records), plan)
		materialized, err := ReadAllToRecords(e.ctx, aggregated)
		if err != nil {
			return nil, err
		}
		if materialized == nil {
			materialized = []record.Record{}
		}
		return NewRecordsReader(materialized), nil
	}
	if len(e.q.OrderBy()) > 0 {
		var orderErr error
		sort.SliceStable(filtered, func(i, j int) bool {
			if orderErr != nil {
				return false
			}
			for orderIndex, order := range e.q.OrderBy() {
				path := fmt.Sprintf("orderBy[%d]", orderIndex)
				left, err := e.expressionAt(order.Expression(), filtered[i], path)
				if err != nil {
					orderErr = err
					return false
				}
				right, err := e.expressionAt(order.Expression(), filtered[j], path)
				if err != nil {
					orderErr = err
					return false
				}
				cmp := compareAggregationValues(left, right)
				if order.Descending() {
					cmp = -cmp
				}
				if cmp != 0 {
					return cmp < 0
				}
			}
			return false
		})
		if orderErr != nil {
			return nil, orderErr
		}
	}
	start := e.q.Offset()
	if start < 0 {
		start = 0
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := len(filtered)
	if e.q.Limit() > 0 && start+e.q.Limit() < end {
		end = start + e.q.Limit()
	}
	filtered = filtered[start:end]
	results := make([]record.Record, len(filtered))
	for i, row := range filtered {
		var data map[string]any
		if len(e.q.Columns()) == 0 {
			data = flattenJoinRow(row, e.aliases, false)
		} else {
			data, err = e.projection(e.q.Columns(), row)
			if err != nil {
				return nil, err
			}
		}
		if err := e.chargeOutput(data); err != nil {
			return nil, err
		}
		results[i] = record.NewRecordWithData(row.key, data)
	}
	return NewRecordsReader(results), nil
}

func (e *joinExecution) validateQueryFields() error {
	knownAlias := map[string]bool{}
	for _, alias := range e.aliases {
		knownAlias[alias] = true
	}
	for _, alias := range e.aliases {
		refs := e.keyRefs[alias]
		if names := e.fields[alias]; names != nil {
			for _, ref := range refs {
				found := false
				for _, name := range names {
					if name == ref.field {
						found = true
						break
					}
				}
				if !found {
					return joinError("join_field", ref.path+".field", fmt.Sprintf("field %q is unavailable in %q", ref.field, alias))
				}
			}
		}
	}
	check := func(field FieldRef, path string) error {
		alias := field.Source()
		if alias == "" {
			// Legacy JOIN parsing keeps its own no-schema rule. Recursive
			// evaluation can bind an unqualified reference when every scanned
			// source supplied field metadata, and can then report ambiguity.
			if e.recursive {
				matches := make([]string, 0, len(e.aliases))
				metadataComplete := true
				for _, candidate := range e.aliases {
					names := e.fields[candidate]
					if names == nil {
						metadataComplete = false
						continue
					}
					for _, name := range names {
						if name == field.Name() {
							matches = append(matches, candidate)
							break
						}
					}
				}
				if len(matches) > 1 {
					return queryError("scope", path, fmt.Sprintf("ambiguous unqualified field %s", field.Name()))
				}
				if len(matches) == 1 {
					alias = matches[0]
				} else if metadataComplete {
					return queryError("shape", path, fmt.Sprintf("field %q is unavailable", field.Name()))
				}
			}
			if alias == "" {
				alias = joinAlias(e.q.From().Base())
			}
		}
		for outer := e.outer; !knownAlias[alias] && outer != nil; outer = outer.outer {
			if _, ok := outer.sources[alias]; ok {
				return nil
			}
		}
		if !knownAlias[alias] {
			return joinError("join_scope", path+".source", fmt.Sprintf("unknown alias %q", alias))
		}
		if names := e.fields[alias]; names != nil {
			found := false
			for _, name := range names {
				if name == field.Name() {
					found = true
					break
				}
			}
			if !found {
				return joinError("join_field", path+".field", fmt.Sprintf("field %q is unavailable in %q", field.Name(), alias))
			}
		}
		return nil
	}
	var expression func(Expression, string) error
	expression = func(value Expression, path string) error {
		switch v := value.(type) {
		case FieldRef:
			return check(v, path)
		case BinaryExpression:
			if err := expression(v.Left, path+".left"); err != nil {
				return err
			}
			return expression(v.Right, path+".right")
		case AggregateFunc:
			for i, arg := range v.FuncArgs() {
				if err := expression(arg, fmt.Sprintf("%s.args[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	var condition func(Condition, string) error
	condition = func(value Condition, path string) error {
		switch v := value.(type) {
		case Comparison:
			if err := expression(v.Left, path+".left"); err != nil {
				return err
			}
			return expression(v.Right, path+".right")
		case GroupCondition:
			for i, child := range v.Conditions() {
				if err := condition(child, fmt.Sprintf("%s.conditions[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	outputNames := map[string]bool{}
	for i, column := range e.q.Columns() {
		path := fmt.Sprintf("columns[%d]", i)
		if column.Wildcard != nil {
			if column.Wildcard.Source == "" {
				return joinError("join_field", path, "JOIN wildcard requires source")
			}
			if !knownAlias[column.Wildcard.Source] {
				return joinError("join_scope", path+".source", "unknown wildcard source")
			}
			fields := e.fields[column.Wildcard.Source]
			if fields == nil {
				return joinError("join_plan", path, "wildcard expansion requires ordered schema metadata")
			}
			for _, name := range fields {
				if column.Wildcard.Excludes(name) {
					continue
				}
				if outputNames[name] {
					return joinError("join_field", path, "duplicate output name "+name)
				}
				outputNames[name] = true
			}
			continue
		}
		name := column.Alias
		if name == "" {
			field, ok := column.Expression.(FieldRef)
			if !ok {
				if query, ok := column.Expression.(QueryExpression); ok && query.As() != "" {
					name = query.As()
				} else if _, aggregate := column.Expression.(AggregateFunc); aggregate {
					name = aggregationColumnName(column)
				} else {
					return joinError("join_shape", path+".as", "non-field JOIN column requires alias")
				}
			} else {
				name = field.Name()
			}
		}
		if outputNames[name] {
			return joinError("join_field", path, "duplicate output name "+name)
		}
		outputNames[name] = true
		if err := expression(column.Expression, path); err != nil {
			return err
		}
	}
	if err := condition(e.q.Where(), "where"); err != nil {
		return err
	}
	for i, group := range e.q.GroupBy() {
		if err := expression(group, fmt.Sprintf("groupBy[%d]", i)); err != nil {
			return err
		}
	}
	if err := condition(e.q.Having(), "having"); err != nil {
		return err
	}
	for i, order := range e.q.OrderBy() {
		if err := expression(order.Expression(), fmt.Sprintf("orderBy[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func (e *joinExecution) chargeOutput(data map[string]any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return joinError("join_plan", "columns", fmt.Sprintf("output is not JSON serializable: %v", err))
	}
	e.bytes += 128 + len(encoded)
	if e.budget != nil {
		e.budget.output++
		e.budget.bytes += 128 + len(encoded)
		if e.budget.output > maxJoinRows {
			return queryError("query_limit", "columns", "result_rows")
		}
		if e.budget.bytes > maxJoinBytes {
			return queryError("query_limit", "columns", "retained_bytes")
		}
	}
	if e.bytes > maxJoinBytes {
		return joinError("join_plan", "columns", "joined byte bound exceeded")
	}
	return nil
}

func joinAlias(source RecordsetSource) string {
	if source.Alias() != "" {
		return source.Alias()
	}
	return source.Name()
}
func joinedFrom(j JoinedSource) FromSource {
	if j.From() != nil {
		return j.From()
	}
	return From(j.RecordsetSource)
}

func (e *joinExecution) scanTree(node FromSource, path string) (resultErr error) {
	alias := joinAlias(node.Base())
	e.aliases = append(e.aliases, alias)
	if provider, ok := e.executor.(JoinFieldsProvider); ok {
		fields, err := provider.JoinFields(e.ctx, node.Base())
		if err != nil {
			return joinError("join_plan", path, fmt.Sprintf("cannot load fields for %s: %v", alias, err))
		}
		if fields != nil {
			e.fields[alias] = append(make([]string, 0, len(fields)), fields...)
		}
	}
	var reader RecordsReader
	var err error
	if source, ok := asQuerySource(node.Base()); ok {
		if source.Query() == nil {
			return joinError("query_shape", path+".query", "query is required")
		}
		// A derived source on a JOIN edge is evaluated by applyJoin with that
		// edge's left row as its lexical outer scope. Scanning it here would
		// discard the correlation before a left row exists.
		if e.recursive && strings.Contains(path, ".joins[") {
			return nil
		}
		reader, err = executeGenericRecursiveBudget(e.ctx, e.executor, source.Query(), e.outer, e.budget)
	} else {
		query := From(node.Base()).NewQuery().SelectIntoRecord(nil)
		reader, err = e.executor.ExecuteQueryToRecordsReader(e.ctx, query)
	}
	if err != nil {
		return joinError("join_plan", path, fmt.Sprintf("cannot scan %s: %v", alias, err))
	}
	defer func() {
		if err := reader.Close(); resultErr == nil && err != nil {
			resultErr = joinError("join_plan", path, fmt.Sprintf("close scan %s: %v", alias, err))
		}
	}()
	for {
		if err := e.ctx.Err(); err != nil {
			return err
		}
		rec, err := reader.Next()
		if errors.Is(err, ErrNoMoreRecords) {
			break
		}
		if err != nil {
			return joinError("join_plan", path, fmt.Sprintf("scan %s: %v", alias, err))
		}
		// Check raw values before JSON normalization so bytes, dates, NaN, and
		// unsafe integers cannot hide in rows later removed by WHERE or ON.
		for _, ref := range e.keyRefs[alias] {
			value := rawJoinField(rec.Data(), ref.field)
			if _, err := joinValueKey(value, ref.path); err != nil {
				return err
			}
		}
		data, err := normalizedJoinRecordMap(rec)
		if err != nil {
			return joinError("join_plan", path, err.Error())
		}
		encoded, _ := json.Marshal(data) // normalizedJoinRecordMap produced JSON data.
		e.bytes += 128 + len(encoded)
		e.fetched++
		if e.budget != nil {
			e.budget.fetched++
			e.budget.bytes += 128 + len(encoded)
			if e.budget.fetched > maxJoinRows {
				return queryError("query_limit", path, "fetched_rows")
			}
			if e.budget.bytes > maxJoinBytes {
				return queryError("query_limit", path, "retained_bytes")
			}
		}
		if e.fetched > maxJoinRows || e.bytes > maxJoinBytes {
			return joinError("join_plan", path, "relation scan exceeds row or byte bound")
		}
		e.scans[alias] = append(e.scans[alias], scannedJoinRow{key: rec.Key(), data: data})
	}
	for i, join := range node.Joins() {
		if err := e.scanTree(joinedFrom(join), fmt.Sprintf("%s.joins[%d].from", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func (e *joinExecution) collectKeyRefs(node FromSource, path string) {
	for i, join := range node.Joins() {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		for j, condition := range join.On() {
			cmp, ok := condition.(Comparison)
			if !ok {
				continue
			}
			for _, operand := range []struct {
				side       string
				expression Expression
			}{{"left", cmp.Left}, {"right", cmp.Right}} {
				field, ok := operand.expression.(FieldRef)
				if !ok {
					continue
				}
				e.keyRefs[field.Source()] = append(e.keyRefs[field.Source()], joinKeyReference{field.Name(), fmt.Sprintf("%s.on[%d].%s", joinPath, j, operand.side)})
			}
		}
		e.collectKeyRefs(joinedFrom(join), joinPath+".from")
	}
}

func rawJoinField(data any, path string) any {
	current := reflect.ValueOf(data)
	for _, part := range strings.Split(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				return nil
			}
			current = current.Elem()
		}
		if !current.IsValid() {
			return nil
		}
		switch current.Kind() {
		case reflect.Map:
			if current.Type().Key().Kind() != reflect.String {
				return nil
			}
			current = current.MapIndex(reflect.ValueOf(part))
		case reflect.Struct:
			found := false
			for i := 0; i < current.NumField(); i++ {
				field := current.Type().Field(i)
				if field.PkgPath != "" {
					continue
				}
				name := strings.Split(field.Tag.Get("json"), ",")[0]
				if name == "" {
					name = field.Name
				}
				if name == part {
					current = current.Field(i)
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		default:
			return nil
		}
	}
	if !current.IsValid() || !current.CanInterface() {
		return nil
	}
	return current.Interface()
}

func normalizedJoinRecordMap(rec record.Record) (map[string]any, error) {
	if rec.Data() == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(rec.Data())
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	if data == nil {
		return map[string]any{}, nil
	}
	return normalizeJoinNumbers(data).(map[string]any), nil
}

func normalizeJoinNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(string(v), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return v
		}
		if strings.ContainsAny(string(v), ".eE") {
			return number
		}
		// JSON integers beyond the JS safe-integer domain cannot be compared
		// portably to floating values. Keep them tagged for key rejection.
		if number > 9007199254740991 || number < -9007199254740991 {
			return v
		}
		return number
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeJoinNumbers(item)
		}
	case []any:
		for i, item := range v {
			v[i] = normalizeJoinNumbers(item)
		}
	}
	return value
}

func (e *joinExecution) build(node FromSource, path string, outer []joinRow, candidates []scannedJoinRow) ([]joinRow, error) {
	alias := joinAlias(node.Base())
	if candidates == nil {
		candidates = e.scans[alias]
	}
	if outer == nil {
		outer = []joinRow{{sources: map[string]map[string]any{}}}
	}
	rows := make([]joinRow, 0)
	for _, parent := range outer {
		for _, source := range candidates {
			values := make(map[string]map[string]any, len(parent.sources)+1)
			for name, data := range parent.sources {
				values[name] = data
			}
			values[alias] = source.data
			key := parent.key
			if key == nil {
				key = source.key
			}
			base := parent.base
			if base == "" {
				base = alias
			}
			rows = append(rows, joinRow{key: key, base: base, sources: values, outer: e.outer})
			if len(rows) > maxJoinRows {
				return nil, joinError("join_plan", path, "joined row bound exceeded")
			}
		}
	}
	var err error
	for i, join := range node.Joins() {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		rows, err = e.applyJoin(rows, join, joinPath)
		if err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (e *joinExecution) applyJoin(left []joinRow, join JoinedSource, path string) ([]joinRow, error) {
	child := joinedFrom(join)
	alias := joinAlias(child.Base())
	var out []joinRow
	for _, parent := range left {
		candidates := e.scans[alias]
		if source, ok := asQuerySource(child.Base()); ok && e.recursive {
			records, err := e.queryRecordsAt(source.Query(), &parent, path+".from.query")
			if err != nil {
				return nil, err
			}
			candidates = make([]scannedJoinRow, 0, len(records))
			for _, rec := range records {
				data, err := normalizedJoinRecordMap(rec)
				if err != nil {
					return nil, err
				}
				candidates = append(candidates, scannedJoinRow{key: rec.Key(), data: data})
			}
		}
		// A selected nested loop deliberately probes every right base row.
		// A selected hash narrows the scan on a direct cross-side equality.
		right, other, hashApplicable := directJoinHashKey(join, alias, parent)
		if _, derived := asQuerySource(child.Base()); derived && e.recursive {
			hashApplicable = false
		}
		if selectGenericJoinAlgorithm(join.Algorithms(), hashApplicable) == genericJoinHash {
			// Every referenced key was validated on its raw fetched row.
			key, _ := joinValueKey(fieldInJoinRow(parent, other), path+".on")
			if key == "" {
				candidates = nil
			} else {
				indexName := alias + "\x00" + right.Name()
				index := e.indexes[indexName]
				if index == nil {
					index = map[string][]scannedJoinRow{}
					for _, candidate := range e.scans[alias] {
						value, _ := lookupAggregationField(candidate.data, right.Name())
						candidateKey, _ := joinValueKey(value, path+".on")
						if candidateKey != "" {
							index[candidateKey] = append(index[candidateKey], candidate)
						}
					}
					e.indexes[indexName] = index
				}
				candidates = index[key]
			}
		}
		if candidates == nil {
			candidates = []scannedJoinRow{}
		}
		rightRows, err := e.build(child, path+".from", []joinRow{parent}, candidates)
		if err != nil {
			return nil, err
		}
		matched := false
		for _, candidate := range rightRows {
			e.candidates++
			if e.budget != nil {
				e.budget.candidates++
				if e.budget.candidates > maxJoinRows*10 {
					return nil, queryError("query_limit", path, "candidate_evaluations")
				}
			}
			if e.candidates > maxJoinRows*10 {
				return nil, joinError("join_plan", path, "candidate evaluation bound exceeded")
			}
			valid := true
			for onIndex, condition := range join.On() {
				ok, err := e.conditionAt(condition, candidate, fmt.Sprintf("%s.on[%d]", path, onIndex))
				if err != nil {
					return nil, err
				}
				if !ok {
					valid = false
					break
				}
			}
			if valid {
				matched = true
				out = append(out, candidate)
			}
			if len(out) > maxJoinRows {
				return nil, joinError("join_plan", path, "joined row bound exceeded")
			}
		}
		if !matched && join.JoinType() == JoinLeft {
			values := make(map[string]map[string]any, len(parent.sources))
			for name, data := range parent.sources {
				values[name] = data
			}
			for _, name := range childAliases(child) {
				values[name] = nil
			}
			out = append(out, joinRow{key: parent.key, base: parent.base, sources: values})
		}
	}
	return out, nil
}

type genericJoinAlgorithm string

const (
	genericJoinHash       genericJoinAlgorithm = "hash"
	genericJoinNestedLoop genericJoinAlgorithm = "nestedLoop"
)

// selectGenericJoinAlgorithm decides one edge independently. An unavailable
// preference is skipped; no applicable preference uses the ordinary strategy.
func selectGenericJoinAlgorithm(preferences []JoinAlgorithm, hashApplicable bool) genericJoinAlgorithm {
	for _, preference := range preferences {
		switch preference {
		case JoinAlgorithmHash:
			if hashApplicable {
				return genericJoinHash
			}
		case JoinAlgorithmNestedLoop:
			return genericJoinNestedLoop
		}
	}
	if hashApplicable {
		return genericJoinHash
	}
	return genericJoinNestedLoop
}

func directJoinHashKey(join JoinedSource, alias string, parent joinRow) (right, other FieldRef, applicable bool) {
	for _, condition := range join.On() {
		cmp, ok := condition.(Comparison)
		if !ok {
			continue
		}
		left, leftOK := cmp.Left.(FieldRef)
		rightOperand, rightOK := cmp.Right.(FieldRef)
		if !leftOK || !rightOK {
			continue
		}
		if _, ok := parent.sources[rightOperand.Source()]; left.Source() == alias && ok {
			return left, rightOperand, true
		}
		if _, ok := parent.sources[left.Source()]; rightOperand.Source() == alias && ok {
			return rightOperand, left, true
		}
	}
	return FieldRef{}, FieldRef{}, false
}

func childAliases(node FromSource) []string {
	result := []string{joinAlias(node.Base())}
	for _, join := range node.Joins() {
		result = append(result, childAliases(joinedFrom(join))...)
	}
	return result
}

func fieldInJoinRow(row joinRow, field FieldRef) any {
	source := field.Source()
	if source == "" {
		source = row.base
	}
	data := row.sources[source]
	value, _ := lookupAggregationField(data, field.Name())
	return value
}

func joinValueKey(value any, path string) (string, error) {
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		return "s:" + strconv.Itoa(len(v)) + ":" + v, nil
	case bool:
		return "b:" + strconv.FormatBool(v), nil
	default:
		raw := reflect.ValueOf(value)
		switch raw.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if raw.Int() > 9007199254740991 || raw.Int() < -9007199254740991 {
				return "", joinError("join_key_type", path, "integer exceeds portable safe range")
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if raw.Uint() > 9007199254740991 {
				return "", joinError("join_key_type", path, "integer exceeds portable safe range")
			}
		}
		if number, ok := aggregationNumber(value); ok {
			if math.IsNaN(number) || math.IsInf(number, 0) {
				return "", joinError("join_key_type", path, "non-finite number")
			}
			if math.Trunc(number) == number && (number > 9007199254740991 || number < -9007199254740991) {
				return "", joinError("join_key_type", path, "integer exceeds portable safe range")
			}
			if number == 0 {
				number = 0
			}
			return "n:" + strconv.FormatFloat(number, 'g', -1, 64), nil
		}
		return "", joinError("join_key_type", path, fmt.Sprintf("unsupported key type %T", value))
	}
}

func flattenJoinRow(row joinRow, aliases []string, includeSources bool) map[string]any {
	data := map[string]any{}
	for _, alias := range aliases {
		for name, value := range row.sources[alias] {
			data[name] = value
		}
	}
	if includeSources {
		sources := map[string]any{}
		for alias, source := range row.sources {
			sources[alias] = source
		}
		data[joinSourcesKey] = sources
		data[joinBaseKey] = row.base
	}
	return data
}

func evalJoinExpression(expr Expression, row joinRow) (any, error) {
	switch v := expr.(type) {
	case FieldRef:
		return fieldInJoinRow(row, v), nil
	case Constant:
		return v.Value, nil
	case Array:
		return v.Value, nil
	case BinaryExpression:
		l, err := evalJoinExpression(v.Left, row)
		if err != nil {
			return nil, err
		}
		r, err := evalJoinExpression(v.Right, row)
		if err != nil {
			return nil, err
		}
		return evalArithmeticValues(v.Operator, l, r)
	default:
		return nil, joinError("join_plan", "columns", fmt.Sprintf("unsupported expression %T", expr))
	}
}

func evalJoinCondition(condition Condition, row joinRow) (bool, error) {
	if condition == nil {
		return true, nil
	}
	switch c := condition.(type) {
	case Comparison:
		left, err := evalJoinExpression(c.Left, row)
		if err != nil {
			return false, err
		}
		right, err := evalJoinExpression(c.Right, row)
		if err != nil {
			return false, err
		}
		if c.Operator == In || c.Operator == NotIn {
			items := reflect.ValueOf(right)
			if !items.IsValid() || (items.Kind() != reflect.Slice && items.Kind() != reflect.Array) {
				return false, joinError("join_plan", "where", "IN or NOT IN requires an array")
			}
			hasNull := false
			for i := 0; i < items.Len(); i++ {
				item := items.Index(i).Interface()
				if item == nil {
					hasNull = true
				} else if left != nil && valuesEqual(left, item) {
					return c.Operator == In, nil
				}
			}
			if c.Operator == NotIn {
				return items.Len() == 0 || (left != nil && !hasNull), nil
			}
			return false, nil
		}
		if left == nil || right == nil {
			return false, nil
		}
		cmp := compareAggregationValues(left, right)
		switch c.Operator {
		case Equal:
			return valuesEqual(left, right), nil
		case GreaterThen:
			return cmp > 0, nil
		case GreaterOrEqual:
			return cmp >= 0, nil
		case LessThen:
			return cmp < 0, nil
		case LessOrEqual:
			return cmp <= 0, nil
		default:
			return false, joinError("join_plan", "where", fmt.Sprintf("unsupported operator %s", c.Operator))
		}
	case GroupCondition:
		if c.Operator() == Or {
			for _, child := range c.Conditions() {
				ok, err := evalJoinCondition(child, row)
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		for _, child := range c.Conditions() {
			ok, err := evalJoinCondition(child, row)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	default:
		return false, joinError("join_plan", "where", fmt.Sprintf("unsupported condition %T", condition))
	}
}

func (e *joinExecution) field(row joinRow, field FieldRef) any {
	alias := field.Source()
	if alias == "" && e.recursive {
		matches := make([]string, 0, len(e.aliases))
		for _, candidate := range e.aliases {
			for _, name := range e.fields[candidate] {
				if name == field.Name() {
					matches = append(matches, candidate)
					break
				}
			}
		}
		if len(matches) == 1 {
			alias = matches[0]
		}
	}
	if alias == "" {
		alias = row.base
	}
	if data, ok := row.sources[alias]; ok {
		value, _ := lookupAggregationField(data, field.Name())
		return value
	}
	for outer := e.outer; outer != nil; outer = outer.outer {
		if data, ok := outer.sources[alias]; ok {
			value, _ := lookupAggregationField(data, field.Name())
			return value
		}
	}
	return nil
}

func (e *joinExecution) expression(expr Expression, row joinRow) (any, error) {
	return e.expressionAt(expr, row, "expression")
}

func (e *joinExecution) expressionAt(expr Expression, row joinRow, path string) (any, error) {
	if !e.recursive {
		return evalJoinExpression(expr, row)
	}
	return e.evalExpressionAt(expr, row, path)
}

func (e *joinExecution) condition(condition Condition, row joinRow) (bool, error) {
	return e.conditionAt(condition, row, "where")
}

func (e *joinExecution) conditionAt(condition Condition, row joinRow, path string) (bool, error) {
	if !e.recursive {
		return evalJoinCondition(condition, row)
	}
	return e.evalConditionAt(condition, row, path)
}

func (e *joinExecution) projection(columns []Column, row joinRow) (map[string]any, error) {
	if !e.recursive {
		return projectJoinRow(columns, row, e.fields)
	}
	return e.project(columns, row)
}

func (e *joinExecution) evalExpression(expr Expression, row joinRow) (any, error) {
	return e.evalExpressionAt(expr, row, "expression")
}

func (e *joinExecution) evalExpressionAt(expr Expression, row joinRow, path string) (any, error) {
	switch value := expr.(type) {
	case FieldRef:
		return e.field(row, value), nil
	case QueryExpression:
		records, err := e.queryRecordsCappedAt(value.Query(), &row, 2, true, path+".query")
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			return nil, nil
		}
		if len(records) > 1 {
			return nil, queryError("cardinality", path+".query", "scalar query returned more than one row")
		}
		data, ok := records[0].Data().(map[string]any)
		if !ok {
			return nil, queryError("shape", path+".query", "scalar query returned non-object row")
		}
		if len(data) != 1 {
			return nil, queryError("shape", path+".query.columns", "scalar query requires exactly one column")
		}
		for _, result := range data {
			return result, nil
		}
	case BinaryExpression:
		left, err := e.evalExpressionAt(value.Left, row, path+".left")
		if err != nil {
			return nil, err
		}
		right, err := e.evalExpressionAt(value.Right, row, path+".right")
		if err != nil {
			return nil, err
		}
		return evalArithmeticValues(value.Operator, left, right)
	case Constant:
		return value.Value, nil
	case Array:
		return value.Value, nil
	}
	return nil, queryError("query_shape", "expression", fmt.Sprintf("unsupported expression %T", expr))
}

func (e *joinExecution) queryRecords(query StructuredQuery, outer *joinRow) ([]record.Record, error) {
	return e.queryRecordsCapped(query, outer, 0, true)
}

func (e *joinExecution) queryRecordsAt(query StructuredQuery, outer *joinRow, path string) ([]record.Record, error) {
	return e.queryRecordsCappedAt(query, outer, 0, true, path)
}

func (e *joinExecution) queryRecordsCapped(query StructuredQuery, outer *joinRow, cap int, project bool) ([]record.Record, error) {
	return e.queryRecordsCappedAt(query, outer, cap, project, "query")
}

func (e *joinExecution) queryRecordsCappedAt(query StructuredQuery, outer *joinRow, cap int, project bool, path string) ([]record.Record, error) {
	if query == nil {
		return nil, queryError("query_shape", path, "query is required")
	}
	key := fmt.Sprintf("%s\x00%d\x00%t", query.String(), cap, project)
	if !queryHasOuterReference(query) {
		for _, cached := range e.memo[key] {
			if reflect.DeepEqual(cached.query, query) {
				if e.budget != nil {
					work := cached.candidateWork
					if work == 0 {
						work = 1
					}
					e.budget.candidates += work
					if e.budget.candidates > maxJoinRows*10 {
						return nil, queryError("query_limit", path, "candidate_evaluations")
					}
				}
				return cached.records, nil
			}
		}
	}
	if cap > 0 && simpleRecursiveQuery(query) {
		before := e.budget.candidates
		records, err := e.executeSimpleCapped(query, outer, cap, project)
		if err == nil && !queryHasOuterReference(query) {
			e.memo[key] = append(e.memo[key], memoizedQuery{query: query, records: records, candidateWork: e.budget.candidates - before})
		}
		return records, nestedLimitError(err, path)
	}
	before := e.budget.candidates
	reader, err := executeGenericRecursiveBudget(e.ctx, e.executor, query, outer, e.budget)
	if err != nil {
		return nil, nestedLimitError(err, path)
	}
	records, err := ReadAllToRecords(e.ctx, reader)
	if err != nil {
		return nil, nestedLimitError(err, path)
	}
	if !queryHasOuterReference(query) {
		e.memo[key] = append(e.memo[key], memoizedQuery{query: query, records: records, candidateWork: e.budget.candidates - before})
	}
	return records, nil
}

func nestedLimitError(err error, path string) error {
	var diagnostic *QueryValidationError
	if path == "query" || !errors.As(err, &diagnostic) || diagnostic.Category != "query_limit" {
		return err
	}
	relative := strings.TrimPrefix(diagnostic.Path, "query.")
	if relative == "query" || relative == "" {
		return queryError(diagnostic.Category, path, diagnostic.Message)
	}
	return queryError(diagnostic.Category, path+"."+relative, diagnostic.Message)
}

func simpleRecursiveQuery(q StructuredQuery) bool {
	if q.From() == nil || q.From().Base() == nil || q.StartFrom() != "" || q.StartAfter() != "" {
		return false
	}
	switch q.From().Base().(type) {
	case QuerySource, *QuerySource:
		return false
	}
	return len(q.From().Joins()) == 0 && len(q.GroupBy()) == 0 && q.Having() == nil && len(q.OrderBy()) == 0 && q.Offset() == 0 && !HasAggregation(q)
}

func (e *joinExecution) executeSimpleCapped(q StructuredQuery, outer *joinRow, cap int, project bool) (records []record.Record, resultErr error) {
	if limit := q.Limit(); limit > 0 && limit < cap {
		cap = limit
	}
	reader, err := e.executor.ExecuteQueryToRecordsReader(e.ctx, From(q.From().Base()).NewQuery().SelectIntoRecord(nil))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := reader.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	child := &joinExecution{ctx: e.ctx, q: q, executor: e.executor, outer: outer, recursive: true, budget: e.budget, aliases: []string{joinAlias(q.From().Base())}, memo: e.memo}
	for len(records) < cap {
		if err := e.ctx.Err(); err != nil {
			return nil, err
		}
		rec, err := reader.Next()
		if errors.Is(err, ErrNoMoreRecords) {
			break
		}
		if err != nil {
			return nil, err
		}
		data, err := normalizedJoinRecordMap(rec)
		if err != nil {
			return nil, err
		}
		if child.budget != nil {
			encoded, _ := json.Marshal(data)
			child.budget.fetched++
			child.budget.bytes += 128 + len(encoded)
			if child.budget.fetched > maxJoinRows {
				return nil, queryError("query_limit", "query.from", "fetched_rows")
			}
			if child.budget.bytes > maxJoinBytes {
				return nil, queryError("query_limit", "query.from", "retained_bytes")
			}
		}
		row := joinRow{key: rec.Key(), base: child.aliases[0], sources: map[string]map[string]any{child.aliases[0]: data}, outer: outer}
		ok, err := child.evalConditionAt(q.Where(), row, "where")
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		output := data
		if project && len(q.Columns()) > 0 {
			output, err = child.project(q.Columns(), row)
			if err != nil {
				return nil, err
			}
		}
		records = append(records, record.NewRecordWithData(rec.Key(), output))
		if child.budget != nil {
			if err := child.chargeOutput(output); err != nil {
				return nil, err
			}
		}
	}
	return records, nil
}

func (e *joinExecution) evalCondition(condition Condition, row joinRow) (bool, error) {
	return e.evalConditionAt(condition, row, "where")
}

func (e *joinExecution) evalConditionAt(condition Condition, row joinRow, path string) (bool, error) {
	truth, err := e.evalTruthAt(condition, row, path)
	return truth == queryTrue, err
}

func (e *joinExecution) evalTruth(condition Condition, row joinRow) (queryTruth, error) {
	return e.evalTruthAt(condition, row, "where")
}

func (e *joinExecution) evalTruthAt(condition Condition, row joinRow, path string) (queryTruth, error) {
	if condition == nil {
		return queryTrue, nil
	}
	switch value := condition.(type) {
	case ExistsCondition:
		records, err := e.queryRecordsCappedAt(value.Query(), &row, 1, false, path+".query")
		if err != nil {
			return queryUnknown, err
		}
		present := queryFalse
		if len(records) > 0 {
			present = queryTrue
		}
		if value.Negated() {
			if present == queryTrue {
				present = queryFalse
			} else {
				present = queryTrue
			}
		}
		return present, nil
	case GroupCondition:
		if value.Operator() == Or {
			unknown := false
			for i, child := range value.Conditions() {
				truth, err := e.evalTruthAt(child, row, fmt.Sprintf("%s.conditions[%d]", path, i))
				if err != nil || truth == queryTrue {
					return truth, err
				}
				if truth == queryUnknown {
					unknown = true
				}
			}
			if unknown {
				return queryUnknown, nil
			}
			return queryFalse, nil
		}
		unknown := false
		for i, child := range value.Conditions() {
			truth, err := e.evalTruthAt(child, row, fmt.Sprintf("%s.conditions[%d]", path, i))
			if err != nil || truth == queryFalse {
				return truth, err
			}
			if truth == queryUnknown {
				unknown = true
			}
		}
		if unknown {
			return queryUnknown, nil
		}
		return queryTrue, nil
	case Comparison:
		left, err := e.evalExpressionAt(value.Left, row, path+".left")
		if err != nil {
			return queryUnknown, err
		}
		if value.Operator == In || value.Operator == NotIn {
			var values []any
			if subquery, ok := value.Right.(QueryExpression); ok {
				records, err := e.queryRecordsAt(subquery.Query(), &row, path+".right.query")
				if err != nil {
					return queryUnknown, err
				}
				for _, rec := range records {
					data, _ := rec.Data().(map[string]any)
					if len(data) != 1 {
						return queryUnknown, queryError("query_shape", path+".right.query", "membership query must return exactly one column")
					}
					for _, item := range data {
						values = append(values, item)
					}
				}
			} else {
				right, err := e.evalExpressionAt(value.Right, row, path+".right")
				if err != nil {
					return queryUnknown, err
				}
				rv := reflect.ValueOf(right)
				if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
					return queryUnknown, queryError("query_shape", path+".right", "IN requires an array or query")
				}
				for i := 0; i < rv.Len(); i++ {
					values = append(values, rv.Index(i).Interface())
				}
			}
			matched, hasNull := false, false
			for _, item := range values {
				if item == nil {
					hasNull = true
				}
				if left != nil && item != nil && valuesEqual(left, item) {
					matched = true
				}
			}
			truth := queryFalse
			if len(values) == 0 {
				truth = queryFalse
			} else if matched {
				truth = queryTrue
			} else if left == nil || hasNull {
				truth = queryUnknown
			}
			if value.Operator == NotIn {
				switch truth {
				case queryTrue:
					truth = queryFalse
				case queryFalse:
					truth = queryTrue
				}
			}
			return truth, nil
		}
		right, err := e.evalExpressionAt(value.Right, row, path+".right")
		if err != nil {
			return queryUnknown, err
		}
		if left == nil || right == nil {
			return queryUnknown, nil
		}
		cmp := compareAggregationValues(left, right)
		switch value.Operator {
		case Equal:
			if valuesEqual(left, right) {
				return queryTrue, nil
			}
			return queryFalse, nil
		case GreaterThen:
			if cmp > 0 {
				return queryTrue, nil
			}
			return queryFalse, nil
		case GreaterOrEqual:
			if cmp >= 0 {
				return queryTrue, nil
			}
			return queryFalse, nil
		case LessThen:
			if cmp < 0 {
				return queryTrue, nil
			}
			return queryFalse, nil
		case LessOrEqual:
			if cmp <= 0 {
				return queryTrue, nil
			}
			return queryFalse, nil
		}
	}
	return queryUnknown, queryError("query_shape", "where", fmt.Sprintf("unsupported condition %T", condition))
}

func (e *joinExecution) project(columns []Column, row joinRow) (map[string]any, error) {
	result := map[string]any{}
	for i, column := range columns {
		if column.Wildcard != nil {
			for name, value := range row.sources[column.Wildcard.Source] {
				if !column.Wildcard.Excludes(name) {
					result[name] = value
				}
			}
			continue
		}
		name := column.Alias
		if name == "" {
			if field, ok := column.Expression.(FieldRef); ok {
				name = field.Name()
			} else if query, ok := column.Expression.(QueryExpression); ok && query.As() != "" {
				name = query.As()
			} else {
				name = fmt.Sprintf("column_%d", i)
			}
		}
		value, err := e.evalExpressionAt(column.Expression, row, fmt.Sprintf("columns[%d]", i))
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}

func queryError(category, path, message string) error {
	return &QueryValidationError{Category: category, Path: path, Message: message}
}

// queryHasOuterReference finds free lexical bindings across the whole subtree.
// A nested query may capture this query's aliases without making this query
// dependent on its own caller.
func queryHasOuterReference(q StructuredQuery) bool {
	return len(queryFreeReferences(q, map[uintptr]bool{})) != 0
}

func queryFreeReferences(q StructuredQuery, visiting map[uintptr]bool) map[string]bool {
	free := map[string]bool{}
	if q == nil {
		free[""] = true
		return free
	}
	if id := queryPointerID(q); id != 0 {
		if visiting[id] {
			free[""] = true
			return free
		}
		visiting[id] = true
		defer delete(visiting, id)
	}
	mergeChild := func(child StructuredQuery, visible map[string]bool) {
		for alias := range queryFreeReferences(child, visiting) {
			if !visible[alias] {
				free[alias] = true
			}
		}
	}
	var expression func(Expression, map[string]bool)
	expression = func(expr Expression, visible map[string]bool) {
		switch value := expr.(type) {
		case FieldRef:
			if value.Source() == "" || !visible[value.Source()] {
				free[value.Source()] = true
			}
		case BinaryExpression:
			expression(value.Left, visible)
			expression(value.Right, visible)
		case QueryExpression:
			mergeChild(value.Query(), visible)
		case AggregateFunc:
			for _, arg := range value.FuncArgs() {
				expression(arg, visible)
			}
		}
	}
	var condition func(Condition, map[string]bool)
	condition = func(value Condition, visible map[string]bool) {
		switch item := value.(type) {
		case ExistsCondition:
			mergeChild(item.Query(), visible)
		case Comparison:
			expression(item.Left, visible)
			expression(item.Right, visible)
		case GroupCondition:
			for _, child := range item.Conditions() {
				condition(child, visible)
			}
		}
	}
	var inspectFrom func(FromSource, map[string]bool) map[string]bool
	inspectFrom = func(from FromSource, visible map[string]bool) map[string]bool {
		if from == nil || from.Base() == nil {
			return visible
		}
		if source, ok := asQuerySource(from.Base()); ok {
			mergeChild(source.Query(), visible)
		}
		visible = cloneQueryAliases(visible)
		visible[joinAlias(from.Base())] = true
		for _, join := range from.Joins() {
			childVisible := inspectFrom(joinedFrom(join), visible)
			for _, on := range join.On() {
				condition(on, childVisible)
			}
			visible = childVisible
		}
		return visible
	}
	local := inspectFrom(q.From(), map[string]bool{})
	condition(q.Where(), local)
	condition(q.Having(), local)
	for _, column := range q.Columns() {
		expression(column.Expression, local)
	}
	for _, value := range q.GroupBy() {
		expression(value, local)
	}
	for _, value := range q.OrderBy() {
		expression(value.Expression(), local)
	}
	return free
}

func projectJoinRow(columns []Column, row joinRow, fields map[string][]string) (map[string]any, error) {
	result := map[string]any{}
	for _, column := range columns {
		if column.Wildcard != nil {
			alias := column.Wildcard.Source
			names := fields[alias]
			for _, name := range names {
				if column.Wildcard.Excludes(name) {
					continue
				}
				result[name], _ = lookupAggregationField(row.sources[alias], name)
			}
			continue
		}
		name := column.Alias
		if name == "" {
			name = column.Expression.(FieldRef).Name()
		}
		value, err := evalJoinExpression(column.Expression, row)
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}
