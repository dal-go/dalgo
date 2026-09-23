package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// DatabaseResolver supplies the authorized executor for one named database.
// The caller owns its lifetime and must authorize each database independently.
type DatabaseResolver func(ctx context.Context, database string) (QueryExecutor, error)

// FederatedProgress reports cumulative source rows read and joined rows
// processed. Total is omitted because most providers cannot count cheaply.
type FederatedProgress struct {
	Phase    string
	Database string
	Rows     int64
}

type FederatedQueryOptions struct {
	OnProgress func(FederatedProgress)
	Money      *MoneyConfig
}

// ExecuteFederatedQuery evaluates a DTQL query across named databases. Joined
// rows and expressions are evaluated by DALgo; only single-source reads reach
// the resolved executors. A missing database name fails closed.
func ExecuteFederatedQuery(ctx context.Context, query StructuredQuery, resolve DatabaseResolver) (RecordsReader, error) {
	return ExecuteFederatedQueryWithOptions(ctx, query, resolve, FederatedQueryOptions{})
}

// ExecuteFederatedQueryWithOptions uses a streaming hash-aggregate for a flat
// fact-to-dimension join. The fact relation can exceed generic JOIN limits;
// the indexed dimension and distinct aggregate groups remain bounded.
func ExecuteFederatedQueryWithOptions(ctx context.Context, query StructuredQuery, resolve DatabaseResolver, options FederatedQueryOptions) (RecordsReader, error) {
	if query == nil || query.From() == nil || resolve == nil {
		return nil, fmt.Errorf("federated query requires a query and database resolver")
	}
	if options.Money == nil {
		if declarative, ok := query.(interface{ Money() *MoneyConfig }); ok {
			options.Money = declarative.Money()
		}
	}
	if err := validateMoney(options.Money); err != nil {
		return nil, err
	}
	if options.Money != nil && !canStreamFederatedAggregate(query) {
		return nil, fmt.Errorf("money requires the federated streaming aggregate plan")
	}
	routed := federatedQueryExecutor{resolve: resolve}
	if canStreamFederatedAggregate(query) {
		return executeStreamingFederatedAggregate(ctx, query, routed, options)
	}
	if canStreamFederatedRows(query) {
		return executeStreamingFederatedRows(ctx, query, routed, options)
	}
	if HasSubquery(query) || hasJoin(query) || HasAggregation(query) {
		routed.progress = options.OnProgress
		reader, err := executeGenericRecursive(ctx, routed, query, nil)
		if err != nil || options.OnProgress == nil {
			return reader, err
		}
		return &federatedProgressReader{RecordsReader: reader, phase: "process", report: options.OnProgress}, nil
	}
	routed.progress = options.OnProgress
	if ref, ok := query.From().Base().(CollectionRef); ok && (len(ref.ScanOrders()) != 0 || ref.ScanLimit() > 0) {
		if query.Where() != nil || len(query.Columns()) != 0 || len(query.OrderBy()) != 0 || query.Offset() != 0 {
			return executeGenericRecursive(ctx, routed, query, nil)
		}
		leaf := From(ref).NewQuery()
		if orders := ref.ScanOrders(); len(orders) != 0 {
			leaf.OrderBy(orders...)
		}
		limit := ref.ScanLimit()
		if outer := query.Limit(); outer > 0 && (limit == 0 || outer < limit) {
			limit = outer
		}
		if limit > 0 {
			leaf.Limit(limit)
		}
		return routed.ExecuteQueryToRecordsReader(ctx, leaf.SelectIntoRecord(nil))
	}
	return routed.ExecuteQueryToRecordsReader(ctx, query)
}

type federatedQueryExecutor struct {
	resolve  DatabaseResolver
	progress func(FederatedProgress)
}

func (e federatedQueryExecutor) executor(ctx context.Context, query Query) (QueryExecutor, error) {
	q, ok := query.(StructuredQuery)
	if !ok || q.From() == nil || len(q.From().Joins()) != 0 {
		return nil, fmt.Errorf("federated leaf must be a single-source structured query")
	}
	return e.sourceExecutor(ctx, q.From().Base())
}

func (e federatedQueryExecutor) sourceExecutor(ctx context.Context, source RecordsetSource) (QueryExecutor, error) {
	ref, ok := source.(CollectionRef)
	if !ok {
		if pointer, isPointer := source.(*CollectionRef); isPointer && pointer != nil {
			ref, ok = *pointer, true
		}
	}
	if !ok || ref.Database() == "" {
		return nil, fmt.Errorf("federated source %q has no database", source.Name())
	}
	return e.resolve(ctx, ref.Database())
}

func (e federatedQueryExecutor) ExecuteQueryToRecordsReader(ctx context.Context, query Query) (RecordsReader, error) {
	executor, err := e.executor(ctx, query)
	if err != nil {
		return nil, err
	}
	reader, err := executor.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil || e.progress == nil {
		return reader, err
	}
	ref, _ := query.(StructuredQuery).From().Base().(CollectionRef)
	return &federatedProgressReader{RecordsReader: reader, phase: "download", database: ref.Database(), report: e.progress}, nil
}

type federatedProgressReader struct {
	RecordsReader
	phase, database string
	count           int64
	report          func(FederatedProgress)
}

func (r *federatedProgressReader) Next() (record.Record, error) {
	row, err := r.RecordsReader.Next()
	switch err {
	case nil:
		r.count++
		if r.count%1024 == 0 {
			r.report(FederatedProgress{Phase: r.phase, Database: r.database, Rows: r.count})
		}
	case ErrNoMoreRecords:
		r.report(FederatedProgress{Phase: r.phase, Database: r.database, Rows: r.count})
	}
	return row, err
}

func (e federatedQueryExecutor) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	executor, err := e.executor(ctx, query)
	if err != nil {
		return nil, err
	}
	return executor.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func (e federatedQueryExecutor) JoinFields(ctx context.Context, source RecordsetSource) ([]string, error) {
	executor, err := e.sourceExecutor(ctx, source)
	if err != nil {
		return nil, err
	}
	if fields, ok := executor.(JoinFieldsProvider); ok {
		return fields.JoinFields(ctx, source)
	}
	return nil, nil
}
