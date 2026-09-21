package dal

import (
	"context"
	"fmt"
	"sort"

	"github.com/dal-go/dalgo/recordset"
)

// ExecuteQueryToRecordsetReader gives columnar consumers the same generic
// aggregation fallback as RecordsReader. Native aggregate queries still pass
// straight through to the provider.
func (db validatedDB) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	if q, ok := query.(StructuredQuery); ok && HasSubquery(q) {
		return executeRecursiveRecordset(ctx, db.Backend, q, options...)
	}
	if hasJoin(query) {
		provider, _ := db.Backend.(NativeJoinProvider)
		return executeJoinRecordset(ctx, db.Backend, query, queryCapabilitiesOf(db.Backend), provider, options...)
	}
	return executeAggregationRecordset(ctx, db.Backend, query, queryCapabilitiesOf(db.Backend), options...)
}

func (tx *validatedReadTx) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	if q, ok := query.(StructuredQuery); ok && HasSubquery(q) {
		return executeRecursiveRecordset(ctx, tx.ReadTransaction, q, options...)
	}
	if hasJoin(query) {
		return executeJoinRecordset(ctx, tx.ReadTransaction, query, tx.capabilities, tx.joinProvider, options...)
	}
	return executeAggregationRecordset(ctx, tx.ReadTransaction, query, tx.capabilities, options...)
}

func (tx *validatedTx) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	if q, ok := query.(StructuredQuery); ok && HasSubquery(q) {
		return executeRecursiveRecordset(ctx, tx.ReadTransaction, q, options...)
	}
	if hasJoin(query) {
		return executeJoinRecordset(ctx, tx.ReadTransaction, query, tx.capabilities, tx.joinProvider, options...)
	}
	return executeAggregationRecordset(ctx, tx.ReadTransaction, query, tx.capabilities, options...)
}

func executeRecursiveRecordset(ctx context.Context, executor QueryExecutor, query StructuredQuery, options ...recordset.Option) (RecordsetReader, error) {
	reader, err := executeGenericRecursive(ctx, executor, query, nil)
	if err != nil {
		return nil, err
	}
	return recursiveReaderToRecordset(ctx, reader, query, options...)
}

// recursiveReaderToRecordset materializes the output of the generic executor.
// ReadAllToRecords owns and closes the reader, including on read failure.
func recursiveReaderToRecordset(ctx context.Context, reader RecordsReader, query StructuredQuery, options ...recordset.Option) (RecordsetReader, error) {
	records, err := ReadAllToRecords(ctx, reader)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, rec := range records {
		data, ok := rec.Data().(map[string]any)
		if !ok {
			return nil, queryError("shape", "recordset", "recursive query returned a non-object row")
		}
		for name := range data {
			names[name] = true
		}
	}
	// A materialized empty result still has the schema declared by its select
	// list. Keep that shape for recordset consumers instead of inferring no
	// columns from the absence of rows.
	if len(names) == 0 {
		for _, column := range query.Columns() {
			if column.Wildcard != nil {
				continue // a wildcard requires source metadata, which an empty scan lacks.
			}
			columnName := column.Alias
			if columnName == "" {
				if subquery, ok := column.Expression.(QueryExpression); ok && subquery.As() != "" {
					columnName = subquery.As()
				} else if column.Expression != nil {
					columnName = aggregationColumnName(column)
				}
			}
			names[columnName] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	definitions := make([]recordset.Column[any], len(ordered))
	for i, name := range ordered {
		definitions[i] = recordset.NewTypedColumn[any](name, nil)
	}
	name := query.From().Base().Name()
	if configured := recordset.NewOptions(options...).Name(); configured != "" {
		name = configured
	}
	rs := recordset.NewColumnarRecordset(name, definitions...)
	for _, rec := range records {
		row := rs.NewRow()
		data := rec.Data().(map[string]any)
		for _, column := range ordered {
			_ = row.SetValueByName(column, data[column], rs)
		}
	}
	return &aggregationRecordsetReader{recordset: rs}, nil
}

// ExecuteRecursiveRecordset is the recordset counterpart to
// ExecuteRecursiveQuery. The executor is used for every authorized leaf scan.
func ExecuteRecursiveRecordset(ctx context.Context, executor QueryExecutor, query StructuredQuery, options ...recordset.Option) (RecordsetReader, error) {
	return executeRecursiveRecordset(ctx, executor, query, options...)
}

func executeJoinRecordset(ctx context.Context, executor QueryExecutor, query Query, capabilities QueryCapabilities, provider NativeJoinProvider, options ...recordset.Option) (RecordsetReader, error) {
	q := query.(StructuredQuery)
	plan, err := PlanJoin(ctx, q, provider)
	if err != nil {
		return nil, err
	}
	if plan.Strategy == JoinNative {
		return executor.ExecuteQueryToRecordsetReader(ctx, query, options...)
	}
	reader, err := executeGenericJoin(ctx, executor, q)
	if err != nil {
		return nil, err
	}
	records, err := ReadAllToRecords(ctx, reader)
	if err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]bool{}
	sources := map[string]RecordsetSource{}
	if len(q.Columns()) > 0 {
		var collect func(FromSource)
		collect = func(node FromSource) {
			sources[joinAlias(node.Base())] = node.Base()
			for _, child := range node.Joins() {
				collect(joinedFrom(child))
			}
		}
		collect(q.From())
	}
	for _, column := range q.Columns() {
		if column.Wildcard != nil {
			fieldProvider := executor.(JoinFieldsProvider) // executeGenericJoin already required schema metadata.
			fields, err := fieldProvider.JoinFields(ctx, sources[column.Wildcard.Source])
			if err != nil {
				return nil, joinError("join_plan", "columns", fmt.Sprintf("cannot load wildcard fields: %v", err))
			}
			for _, name := range fields {
				if !column.Wildcard.Excludes(name) && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
			continue
		}
		name := aggregationColumnName(column)
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(q.Columns()) == 0 {
		for _, rec := range records {
			data := rec.Data().(map[string]any)
			for name := range data {
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
		sort.Strings(names)
	}
	definitions := make([]recordset.Column[any], len(names))
	for i, name := range names {
		definitions[i] = recordset.NewTypedColumn[any](name, nil)
	}
	name := q.From().Base().Name()
	if configured := recordset.NewOptions(options...).Name(); configured != "" {
		name = configured
	}
	rs := recordset.NewColumnarRecordset(name, definitions...)
	for _, rec := range records {
		data := rec.Data().(map[string]any)
		row := rs.NewRow()
		for _, name := range names {
			_ = row.SetValueByName(name, data[name], rs)
		}
	}
	return &aggregationRecordsetReader{recordset: rs}, nil
}

func executeAggregationRecordset(ctx context.Context, executor QueryExecutor, query Query, capabilities QueryCapabilities, options ...recordset.Option) (RecordsetReader, error) {
	q, ok := query.(StructuredQuery)
	if !ok || !HasAggregation(q) {
		return executor.ExecuteQueryToRecordsetReader(ctx, query, options...)
	}
	plan, err := PlanAggregation(q, capabilities)
	if err != nil {
		return nil, fmt.Errorf("dalgo aggregation: %w", err)
	}
	if plan.Strategy == AggregationNative {
		return executor.ExecuteQueryToRecordsetReader(ctx, query, options...)
	}
	reader, err := executeAggregationRecords(ctx, executor, query, capabilities)
	if err != nil {
		return nil, err
	}
	records, err := ReadAllToRecords(ctx, reader)
	if err != nil {
		return nil, err
	}
	columns := EffectiveAggregationColumns(q)
	names := make([]string, len(columns))
	definitions := make([]recordset.Column[any], len(columns))
	for i, column := range columns {
		names[i] = aggregationColumnName(column)
		definitions[i] = recordset.NewTypedColumn[any](names[i], nil)
	}
	name := q.From().Base().Name()
	if configured := recordset.NewOptions(options...).Name(); configured != "" {
		name = configured
	}
	rs := recordset.NewColumnarRecordset(name, definitions...)
	for _, rec := range records {
		data := rec.Data().(map[string]any)
		row := rs.NewRow()
		for _, column := range names {
			_ = row.SetValueByName(column, data[column], rs)
		}
	}
	return &aggregationRecordsetReader{recordset: rs}, nil
}

type aggregationRecordsetReader struct {
	recordset *recordset.ColumnarRecordset
	position  int
}

func (r *aggregationRecordsetReader) Recordset() recordset.Recordset { return r.recordset }
func (r *aggregationRecordsetReader) Cursor() (string, error)        { return "", nil }
func (r *aggregationRecordsetReader) Close() error                   { return nil }
func (r *aggregationRecordsetReader) Next() (recordset.Row, recordset.Recordset, error) {
	if r.position >= r.recordset.RowsCount() {
		return nil, r.recordset, ErrNoMoreRecords
	}
	row := r.recordset.GetRow(r.position)
	r.position++
	return row, r.recordset, nil
}
