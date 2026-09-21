package dal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dal-go/record"
)

const (
	defaultMaxAggregationGroups = 100_000
	defaultMaxDistinctValues    = 100_000
	defaultMaxAggregateStates   = 1_000_000
	defaultMaxTotalDistinct     = 1_000_000
	defaultMaxAggregationBytes  = 64 << 20

	// These are deliberately conservative estimates rather than Go runtime
	// layout promises. The 64 MiB guard bounds retained group/state structures
	// and materialized output, including maps, entries, and record wrappers.
	aggregationGroupOverheadBytes          = 256
	aggregationAggregateStateOverheadBytes = 192
	aggregationOutputMapOverheadBytes      = 128
	aggregationMapEntryOverheadBytes       = 96
	aggregationRecordOverheadBytes         = 128
)

// ExecuteQueryToRecordsReader intercepts aggregate structured queries and
// delegates non-aggregate/native queries directly to the backend.
func (db validatedDB) ExecuteQueryToRecordsReader(ctx context.Context, query Query) (RecordsReader, error) {
	return executeAggregationRecords(ctx, db.Backend, query, queryCapabilitiesOf(db.Backend))
}

func executeAggregationRecords(ctx context.Context, executor QueryExecutor, query Query, capabilities QueryCapabilities) (RecordsReader, error) {
	q, ok := query.(StructuredQuery)
	if !ok || !HasAggregation(q) {
		return executor.ExecuteQueryToRecordsReader(ctx, query)
	}
	plan, err := PlanAggregation(q, capabilities)
	if err != nil {
		return nil, fmt.Errorf("dalgo aggregation: %w", err)
	}
	if plan.Strategy == AggregationNative {
		return executor.ExecuteQueryToRecordsReader(ctx, query)
	}
	rawQuery := newAggregationSourceQuery(q, plan.Strategy == AggregationStreaming)
	raw, err := executor.ExecuteQueryToRecordsReader(ctx, rawQuery)
	if err != nil {
		return nil, err
	}
	return newLocalAggregationReader(ctx, q, raw, plan), nil
}

func queryCapabilitiesOf(value any) QueryCapabilities {
	if provider, ok := value.(QueryCapabilitiesProvider); ok {
		return provider.QueryCapabilities()
	}
	return QueryCapabilities{}
}

// aggregationSourceQuery removes result-stage operations from the provider
// scan. In particular OFFSET/LIMIT are never pushed below local aggregation.
type aggregationSourceQuery struct {
	StructuredQuery
	columns []Column
	orders  []OrderExpression
}

func newAggregationSourceQuery(q StructuredQuery, ordered bool) StructuredQuery {
	fields := collectSourceFields(q)
	columns := make([]Column, len(fields))
	for i, field := range fields {
		columns[i] = Column{Expression: field}
	}
	var orders []OrderExpression
	if ordered {
		for _, expression := range q.GroupBy() {
			orders = append(orders, Ascending(expression))
		}
	}
	return aggregationSourceQuery{StructuredQuery: q, columns: columns, orders: orders}
}

func (q aggregationSourceQuery) GroupBy() []Expression      { return nil }
func (q aggregationSourceQuery) Having() Condition          { return nil }
func (q aggregationSourceQuery) OrderBy() []OrderExpression { return q.orders }
func (q aggregationSourceQuery) Columns() []Column          { return q.columns }
func (q aggregationSourceQuery) Offset() int                { return 0 }
func (q aggregationSourceQuery) Limit() int                 { return 0 }
func (q aggregationSourceQuery) StartFrom() Cursor          { return "" }
func (q aggregationSourceQuery) StartAfter() Cursor         { return "" }

func collectSourceFields(q StructuredQuery) []FieldRef {
	seen := map[string]bool{}
	var result []FieldRef
	var walk func(Expression)
	walk = func(expression Expression) {
		switch e := expression.(type) {
		case FieldRef:
			key := e.Source() + "\x00" + e.Name()
			if !seen[key] {
				seen[key] = true
				result = append(result, e)
			}
		case AggregateFunc:
			for _, arg := range e.FuncArgs() {
				walk(arg)
			}
		case BinaryExpression:
			walk(e.Left)
			walk(e.Right)
		}
	}
	var walkCondition func(Condition)
	walkCondition = func(condition Condition) {
		switch c := condition.(type) {
		case Comparison:
			walk(c.Left)
			walk(c.Right)
		case GroupCondition:
			for _, child := range c.Conditions() {
				walkCondition(child)
			}
		}
	}
	for _, e := range q.GroupBy() {
		walk(e)
	}
	for _, c := range q.Columns() {
		walk(c.Expression)
	}
	for _, o := range q.OrderBy() {
		walk(o.Expression())
	}
	walkCondition(q.Having())
	return result
}

type aggregateState struct {
	expression AggregateFunc
	count      int64
	sum        float64
	value      any
	hasValue   bool
	distinct   map[string]struct{}
	valueBytes int
}

type localGroup struct {
	key       string
	keyValues map[string]any
	states    map[string]*aggregateState
	out       map[string]any
	bytes     int
}

type localAggregationReader struct {
	ctx           context.Context
	query         StructuredQuery
	raw           RecordsReader
	plan          AggregationPlan
	aggregates    []AggregateFunc
	rows          []record.Record
	rowIndex      int
	loaded        bool
	current       *localGroup
	inputDone     bool
	emitted       int
	skipped       int
	closed        bool
	totalDistinct int
	retainedBytes int
}

func newLocalAggregationReader(ctx context.Context, q StructuredQuery, raw RecordsReader, plan AggregationPlan) *localAggregationReader {
	aggregates := uniqueAggregates(q)
	return &localAggregationReader{ctx: ctx, query: q, raw: raw, plan: plan, aggregates: aggregates}
}

func (r *localAggregationReader) Cursor() (string, error) { return "", nil }

func (r *localAggregationReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.raw.Close()
}

func (r *localAggregationReader) Next() (record.Record, error) {
	if err := r.ctx.Err(); err != nil {
		_ = r.Close()
		return nil, err
	}
	if r.plan.Strategy == AggregationStreaming && len(r.query.OrderBy()) == 0 {
		return r.nextStreaming()
	}
	if !r.loaded {
		if err := r.loadMaterialized(); err != nil {
			return nil, err
		}
	}
	if r.rowIndex >= len(r.rows) {
		return nil, ErrNoMoreRecords
	}
	result := r.rows[r.rowIndex]
	r.rowIndex++
	return result, nil
}

func (r *localAggregationReader) nextStreaming() (result record.Record, resultErr error) {
	defer func() {
		if resultErr != nil && resultErr != ErrNoMoreRecords {
			_ = r.Close()
		}
	}()
	for {
		if limit := r.query.Limit(); limit > 0 && r.emitted >= limit {
			_ = r.Close()
			return nil, ErrNoMoreRecords
		}
		if r.inputDone {
			return nil, ErrNoMoreRecords
		}
		rec, err := r.raw.Next()
		if err == ErrNoMoreRecords {
			r.inputDone = true
			_ = r.Close()
			if r.current == nil {
				return nil, ErrNoMoreRecords
			}
			finished := r.current
			r.current = nil
			row, emit, finalErr := r.finalizeForStreaming(finished)
			r.releaseGroup(finished)
			if finalErr != nil {
				return nil, finalErr
			}
			if !emit {
				continue
			}
			return row, nil
		}
		if err != nil {
			return nil, err
		}
		row, err := normalizedRecordMap(rec)
		if err != nil {
			return nil, err
		}
		key, values, err := r.groupKey(row)
		if err != nil {
			return nil, err
		}
		if r.current == nil {
			r.current, err = r.newGroup(key, values)
			if err != nil {
				return nil, err
			}
			if err := r.updateGroup(r.current, row); err != nil {
				return nil, err
			}
			continue
		}
		if key == r.current.key {
			if err := r.updateGroup(r.current, row); err != nil {
				return nil, err
			}
			continue
		}
		finished := r.current
		r.current, err = r.newGroup(key, values)
		if err != nil {
			return nil, err
		}
		if err := r.updateGroup(r.current, row); err != nil {
			return nil, err
		}
		result, emit, err := r.finalizeForStreaming(finished)
		r.releaseGroup(finished)
		if err != nil {
			return nil, err
		}
		if !emit {
			continue
		}
		return result, nil
	}
}

func (r *localAggregationReader) finalizeForStreaming(group *localGroup) (record.Record, bool, error) {
	out, keep, err := r.finalizeGroup(group)
	if err != nil || !keep {
		return nil, false, err
	}
	if r.skipped < r.query.Offset() {
		r.skipped++
		return nil, false, nil
	}
	rec := record.NewRecordWithData(record.NewKeyWithID(r.query.From().Base().Name(), strconv.Itoa(r.emitted)), out).SetError(nil)
	r.emitted++
	return rec, true, nil
}

func (r *localAggregationReader) loadMaterialized() (resultErr error) {
	defer func() { _ = r.Close() }()
	r.loaded = true
	groups := map[string]*localGroup{}
	order := []string{}
	for {
		if err := r.ctx.Err(); err != nil {
			_ = r.Close()
			return err
		}
		rec, err := r.raw.Next()
		if err == ErrNoMoreRecords {
			break
		}
		if err != nil {
			return err
		}
		row, err := normalizedRecordMap(rec)
		if err != nil {
			return err
		}
		key, values, err := r.groupKey(row)
		if err != nil {
			return err
		}
		group := groups[key]
		if group == nil {
			if len(groups) >= defaultMaxAggregationGroups {
				return fmt.Errorf("dalgo aggregation: group limit %d exceeded", defaultMaxAggregationGroups)
			}
			if len(r.aggregates) > 0 && (len(groups)+1) > defaultMaxAggregateStates/len(r.aggregates) {
				return fmt.Errorf("dalgo aggregation: aggregate-state limit %d exceeded", defaultMaxAggregateStates)
			}
			group, err = r.newGroup(key, values)
			if err != nil {
				return err
			}
			groups[key] = group
			order = append(order, key)
		}
		if err := r.updateGroup(group, row); err != nil {
			return err
		}
	}
	if len(r.query.GroupBy()) == 0 && len(groups) == 0 {
		group, err := r.newGroup("implicit", map[string]any{})
		if err != nil {
			return err
		}
		groups[group.key] = group
		order = append(order, group.key)
	}
	var output []map[string]any
	for _, key := range order {
		group := groups[key]
		out, keep, err := r.finalizeGroup(group)
		if err != nil {
			return err
		}
		if keep {
			for i, requested := range r.query.OrderBy() {
				value, err := r.resolveGroupExpression(requested.Expression(), group, out)
				if err != nil {
					return fmt.Errorf("ORDER BY expression #%d: %w", i, err)
				}
				out[aggregationOrderKey(i)] = value
			}
			if err := r.reserveMaterializedOutput(out); err != nil {
				return err
			}
			output = append(output, out)
		}
	}
	if orders := r.query.OrderBy(); len(orders) > 0 {
		sort.SliceStable(output, func(i, j int) bool {
			for orderIndex, order := range orders {
				a := output[i][aggregationOrderKey(orderIndex)]
				b := output[j][aggregationOrderKey(orderIndex)]
				comparison := compareAggregationValues(a, b)
				if order.Descending() {
					comparison = -comparison
				}
				if comparison != 0 {
					return comparison < 0
				}
			}
			return false
		})
	}
	start := r.query.Offset()
	if start > len(output) {
		start = len(output)
	}
	end := len(output)
	if limit := r.query.Limit(); limit > 0 && start+limit < end {
		end = start + limit
	}
	output = output[start:end]
	r.rows = make([]record.Record, len(output))
	for i, row := range output {
		for orderIndex := range r.query.OrderBy() {
			delete(row, aggregationOrderKey(orderIndex))
		}
		r.rows[i] = record.NewRecordWithData(record.NewKeyWithID(r.query.From().Base().Name(), strconv.Itoa(i)), row).SetError(nil)
	}
	return nil
}

func (r *localAggregationReader) newGroup(key string, values map[string]any) (*localGroup, error) {
	// The encoded key and decoded values retain equivalent scalar payloads.
	// Charge both plus conservative group/state and map overhead, rather than
	// depending on Go runtime object-layout details.
	bytes := len(key)*2 + aggregationGroupOverheadBytes
	bytes += len(r.aggregates) * aggregationAggregateStateOverheadBytes
	bytes += (len(r.aggregates) + 1) * aggregationMapEntryOverheadBytes
	if err := r.reserveAggregationBytes(bytes); err != nil {
		return nil, err
	}
	states := make(map[string]*aggregateState, len(r.aggregates))
	for _, aggregate := range r.aggregates {
		state := &aggregateState{expression: aggregate}
		if aggregateDistinct(aggregate) {
			state.distinct = map[string]struct{}{}
		}
		states[aggregate.String()] = state
	}
	return &localGroup{key: key, keyValues: values, states: states, bytes: bytes}, nil
}

func (r *localAggregationReader) reserveAggregationBytes(bytes int) error {
	if bytes < 0 || r.retainedBytes > defaultMaxAggregationBytes-bytes {
		return fmt.Errorf("dalgo aggregation: retained aggregation byte limit %d exceeded", defaultMaxAggregationBytes)
	}
	r.retainedBytes += bytes
	return nil
}

func (r *localAggregationReader) reserveMaterializedOutput(row map[string]any) error {
	bytes := aggregationOutputMapOverheadBytes + aggregationRecordOverheadBytes
	for name, value := range row {
		valueBytes := aggregationValueBytes(value)
		if valueBytes > defaultMaxAggregationBytes ||
			len(name) > defaultMaxAggregationBytes-aggregationMapEntryOverheadBytes ||
			bytes > defaultMaxAggregationBytes-aggregationMapEntryOverheadBytes-len(name)-valueBytes {
			return r.reserveAggregationBytes(defaultMaxAggregationBytes + 1)
		}
		entryBytes := aggregationMapEntryOverheadBytes + len(name) + valueBytes
		bytes += entryBytes
	}
	return r.reserveAggregationBytes(bytes)
}

func (r *localAggregationReader) releaseGroup(group *localGroup) {
	if group == nil {
		return
	}
	r.retainedBytes -= group.bytes
	group.bytes = 0
}

func (r *localAggregationReader) groupKey(row map[string]any) (string, map[string]any, error) {
	if len(r.query.GroupBy()) == 0 {
		return "implicit", map[string]any{}, nil
	}
	parts := make([]string, len(r.query.GroupBy()))
	values := make(map[string]any, len(parts))
	for i, expression := range r.query.GroupBy() {
		value, err := evalScalar(expression, row)
		if err != nil {
			return "", nil, err
		}
		encoded, err := encodeTypedValue(value)
		if err != nil {
			return "", nil, fmt.Errorf("GROUP BY %q: %w", expression.String(), err)
		}
		parts[i] = encoded
		values[expression.String()] = value
	}
	return strings.Join(parts, "|"), values, nil
}

func (r *localAggregationReader) updateGroup(group *localGroup, row map[string]any) error {
	for _, state := range group.states {
		aggregate := state.expression
		arg := aggregate.FuncArgs()[0]
		_, isStar := arg.(StarExpression)
		var value any
		var err error
		if !isStar {
			value, err = evalScalar(arg, row)
		}
		if err != nil {
			return err
		}
		if state.distinct != nil && value != nil {
			key, err := encodeTypedValue(value)
			if err != nil {
				return err
			}
			if _, exists := state.distinct[key]; exists {
				continue
			}
			if len(state.distinct) >= defaultMaxDistinctValues {
				return fmt.Errorf("dalgo aggregation: distinct-value limit %d exceeded for %s", defaultMaxDistinctValues, aggregate)
			}
			if r.totalDistinct >= defaultMaxTotalDistinct {
				return fmt.Errorf("dalgo aggregation: total distinct-value limit %d exceeded", defaultMaxTotalDistinct)
			}
			distinctBytes := len(key) + aggregationMapEntryOverheadBytes
			if err := r.reserveAggregationBytes(distinctBytes); err != nil {
				return err
			}
			state.distinct[key] = struct{}{}
			group.bytes += distinctBytes
			r.totalDistinct++
		}
		switch strings.ToUpper(aggregate.FuncName()) {
		case COUNT:
			if isStar || value != nil {
				if state.count == math.MaxInt64 {
					return fmt.Errorf("COUNT overflow")
				}
				state.count++
			}
		case SUM, AVERAGE:
			if value == nil {
				continue
			}
			number, ok := aggregationNumber(value)
			if !ok {
				// Match the portable SQL path: non-numeric dynamic values are
				// outside the aggregate's numeric domain and are ignored.
				continue
			}
			if math.IsInf(number, 0) || math.IsNaN(number) {
				return fmt.Errorf("%s produced a non-finite value", aggregate.FuncName())
			}
			state.sum += number
			if math.IsInf(state.sum, 0) {
				return fmt.Errorf("%s numeric overflow", aggregate.FuncName())
			}
			if state.count == math.MaxInt64 {
				return fmt.Errorf("AVG input count overflow")
			}
			state.count++
		case MIN, MAX:
			if value == nil {
				continue
			}
			if !state.hasValue || strings.EqualFold(aggregate.FuncName(), MIN) && compareAggregationValues(value, state.value) < 0 || strings.EqualFold(aggregate.FuncName(), MAX) && compareAggregationValues(value, state.value) > 0 {
				if err := r.setAggregateStateValue(group, state, value); err != nil {
					return err
				}
			}
		case FIRST:
			if !state.hasValue {
				if err := r.setAggregateStateValue(group, state, value); err != nil {
					return err
				}
			}
		case LAST:
			if err := r.setAggregateStateValue(group, state, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *localAggregationReader) setAggregateStateValue(group *localGroup, state *aggregateState, value any) error {
	bytes := aggregationValueBytes(value)
	delta := bytes - state.valueBytes
	if delta > 0 {
		if err := r.reserveAggregationBytes(delta); err != nil {
			return err
		}
	} else {
		r.retainedBytes += delta
	}
	group.bytes += delta
	state.value, state.hasValue, state.valueBytes = value, true, bytes
	return nil
}

func aggregationValueBytes(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		// Non-JSON/cyclic values are outside the portable result domain. Force
		// the caller through the explicit resource error rather than retaining
		// an unmeasurable value.
		return defaultMaxAggregationBytes + 1
	}
	return len(encoded)
}

func (r *localAggregationReader) finalizeGroup(group *localGroup) (map[string]any, bool, error) {
	out := map[string]any{}
	for _, column := range effectiveAggregationColumns(r.query) {
		value, err := r.resolveGroupExpression(column.Expression, group, out)
		if err != nil {
			return nil, false, err
		}
		out[aggregationColumnName(column)] = value
	}
	group.out = out
	if r.query.Having() != nil {
		keep, err := r.evalHaving(r.query.Having(), group)
		if err != nil {
			return nil, false, err
		}
		if !keep {
			return nil, false, nil
		}
	}
	return out, true, nil
}

func (r *localAggregationReader) resolveGroupExpression(expression Expression, group *localGroup, output map[string]any) (any, error) {
	if field, ok := expression.(FieldRef); ok && field.Source() == "" {
		if value, exists := output[field.Name()]; exists {
			return value, nil
		}
	}
	if aggregate, ok := expression.(AggregateFunc); ok {
		state := group.states[aggregate.String()]
		if state == nil {
			return nil, fmt.Errorf("aggregate state missing for %s", aggregate)
		}
		switch strings.ToUpper(aggregate.FuncName()) {
		case COUNT:
			return state.count, nil
		case SUM:
			if state.count == 0 {
				return nil, nil
			}
			return state.sum, nil
		case AVERAGE:
			if state.count == 0 {
				return nil, nil
			}
			return state.sum / float64(state.count), nil
		case MIN, MAX, FIRST, LAST:
			if !state.hasValue {
				return nil, nil
			}
			return state.value, nil
		}
	}
	if binary, ok := expression.(BinaryExpression); ok {
		left, err := r.resolveGroupExpression(binary.Left, group, output)
		if err != nil {
			return nil, err
		}
		right, err := r.resolveGroupExpression(binary.Right, group, output)
		if err != nil {
			return nil, err
		}
		return evalArithmeticValues(binary.Operator, left, right)
	}
	if value, ok := group.keyValues[expression.String()]; ok {
		return value, nil
	}
	if constant, ok := expression.(Constant); ok {
		return normalizeAggregationValue(constant.Value), nil
	}
	return nil, fmt.Errorf("cannot resolve grouped expression %q", expression.String())
}

func (r *localAggregationReader) evalHaving(condition Condition, group *localGroup) (bool, error) {
	switch c := condition.(type) {
	case Comparison:
		left, err := r.resolveGroupExpression(c.Left, group, group.out)
		if err != nil {
			return false, err
		}
		right, err := r.resolveGroupExpression(c.Right, group, group.out)
		if err != nil {
			return false, err
		}
		cmp := compareAggregationValues(left, right)
		switch c.Operator {
		case Equal:
			return valuesEqual(left, right), nil
		case GreaterThen:
			return left != nil && right != nil && cmp > 0, nil
		case GreaterOrEqual:
			return left != nil && right != nil && cmp >= 0, nil
		case LessThen:
			return left != nil && right != nil && cmp < 0, nil
		case LessOrEqual:
			return left != nil && right != nil && cmp <= 0, nil
		default:
			return false, fmt.Errorf("unsupported HAVING operator %q", c.Operator)
		}
	case GroupCondition:
		if c.Operator() == Or {
			for _, child := range c.Conditions() {
				ok, err := r.evalHaving(child, group)
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		for _, child := range c.Conditions() {
			ok, err := r.evalHaving(child, group)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("unsupported HAVING condition %T", condition)
	}
}

func aggregationOrderKey(index int) string { return "\x00dalgo_order_" + strconv.Itoa(index) }

func uniqueAggregates(q StructuredQuery) []AggregateFunc {
	seen := map[string]bool{}
	var result []AggregateFunc
	var add func(Expression)
	add = func(expression Expression) {
		switch e := expression.(type) {
		case AggregateFunc:
			if !seen[e.String()] {
				seen[e.String()] = true
				result = append(result, e)
			}
		case BinaryExpression:
			add(e.Left)
			add(e.Right)
		}
	}
	var addCondition func(Condition)
	addCondition = func(condition Condition) {
		switch c := condition.(type) {
		case Comparison:
			add(c.Left)
			add(c.Right)
		case GroupCondition:
			for _, child := range c.Conditions() {
				addCondition(child)
			}
		}
	}
	for _, column := range q.Columns() {
		add(column.Expression)
	}
	for _, order := range q.OrderBy() {
		add(order.Expression())
	}
	addCondition(q.Having())
	return result
}

func evalScalar(expression Expression, row map[string]any) (any, error) {
	switch e := expression.(type) {
	case FieldRef:
		value, _ := lookupAggregationField(row, e.Name())
		return value, nil
	case Constant:
		return normalizeAggregationValue(e.Value), nil
	case BinaryExpression:
		left, err := evalScalar(e.Left, row)
		if err != nil {
			return nil, err
		}
		right, err := evalScalar(e.Right, row)
		if err != nil {
			return nil, err
		}
		return evalArithmeticValues(e.Operator, left, right)
	default:
		return nil, fmt.Errorf("unsupported scalar expression %T", expression)
	}
}

func evalArithmeticValues(operator ArithmeticOperator, left, right any) (any, error) {
	if left == nil || right == nil {
		return nil, nil
	}
	a, okA := aggregationNumber(left)
	b, okB := aggregationNumber(right)
	if !okA || !okB {
		return nil, nil
	}
	switch operator {
	case Add:
		return a + b, nil
	case Subtract:
		return a - b, nil
	case Multiply:
		return a * b, nil
	case Divide:
		if b == 0 {
			return nil, nil
		}
		return a / b, nil
	default:
		return nil, fmt.Errorf("unsupported arithmetic operator %q", operator)
	}
}

func normalizedRecordMap(rec record.Record) (map[string]any, error) {
	data := rec.Data()
	if data == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("dalgo aggregation: record is not JSON serializable: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("dalgo aggregation: record data is not an object: %w", err)
	}
	if result == nil {
		result = map[string]any{}
	}
	return result, nil
}

func lookupAggregationField(row map[string]any, path string) (any, bool) {
	current := any(row)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func aggregationColumnName(column Column) string {
	if column.Alias != "" {
		return column.Alias
	}
	if field, ok := column.Expression.(FieldRef); ok {
		return field.Name()
	}
	return column.Expression.String()
}

func encodeTypedValue(value any) (string, error) {
	value = normalizeAggregationValue(value)
	switch v := value.(type) {
	case nil:
		return "n", nil
	case bool:
		return "b:" + strconv.FormatBool(v), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("non-finite numeric group key")
		}
		if v == 0 {
			v = 0
		}
		return "f:" + strconv.FormatFloat(v, 'g', -1, 64), nil
	case string:
		return "s:" + strconv.Itoa(len(v)) + ":" + v, nil
	case []byte:
		return "x:" + base64.StdEncoding.EncodeToString(v), nil
	default:
		return "", fmt.Errorf("unsupported group/distinct key type %T", value)
	}
}

func normalizeAggregationValue(value any) any {
	if value == nil {
		return nil
	}
	if t, ok := value.(time.Time); ok {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if number, ok := aggregationNumber(value); ok {
		return number
	}
	return value
}

func aggregationNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func compareAggregationValues(a, b any) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}
	if an, ok := aggregationNumber(a); ok {
		if bn, ok := aggregationNumber(b); ok {
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
			return 0
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs)
	}
	ab, aok := a.(bool)
	bb, bok := b.(bool)
	if aok && bok {
		if ab == bb {
			return 0
		}
		if !ab {
			return -1
		}
		return 1
	}
	return strings.Compare(fmt.Sprintf("%T:%v", a, a), fmt.Sprintf("%T:%v", b, b))
}

func valuesEqual(a, b any) bool {
	keyA, errA := encodeTypedValue(a)
	keyB, errB := encodeTypedValue(b)
	return errA == nil && errB == nil && keyA == keyB
}
