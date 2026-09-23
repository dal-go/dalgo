package dal

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/dal-go/record"
)

// The streaming plan keeps only the dimension and aggregate groups in memory.
// Other query shapes use the existing bounded generic evaluator.
func canStreamFederatedAggregate(q StructuredQuery) bool {
	if !HasAggregation(q) || HasSubquery(q) || len(q.OrderBy()) != 0 || q.From() == nil || len(q.From().Joins()) != 1 {
		return false
	}
	child := joinedFrom(q.From().Joins()[0])
	if child == nil || len(child.Joins()) != 0 {
		return false
	}
	join := q.From().Joins()[0]
	if selectGenericJoinAlgorithm(join.Algorithms(), true) != genericJoinHash {
		return false
	}
	rootAlias, childAlias := joinAlias(q.From().Base()), joinAlias(child.Base())
	for _, on := range join.On() {
		cmp, ok := on.(Comparison)
		if !ok || cmp.Operator != Equal {
			continue
		}
		left, leftOK := cmp.Left.(FieldRef)
		right, rightOK := cmp.Right.(FieldRef)
		if leftOK && rightOK && (left.Source() == rootAlias && right.Source() == childAlias || left.Source() == childAlias && right.Source() == rootAlias) {
			return true
		}
	}
	return false
}

func executeStreamingFederatedAggregate(ctx context.Context, q StructuredQuery, routed federatedQueryExecutor, options FederatedQueryOptions) (RecordsReader, error) {
	root := q.From()
	child := joinedFrom(root.Joins()[0])
	e := &joinExecution{ctx: ctx, q: q, executor: routed, scans: map[string][]scannedJoinRow{}, indexes: map[string]map[string][]scannedJoinRow{}, fields: map[string][]string{}, keyRefs: map[string][]joinKeyReference{}}
	e.collectKeyRefs(root, "from")
	e.aliases = append(e.aliases, joinAlias(root.Base()))
	if err := e.scanTree(child, "from.joins[0].from"); err != nil {
		return nil, err
	}
	if options.OnProgress != nil {
		if ref, ok := child.Base().(CollectionRef); ok {
			options.OnProgress(FederatedProgress{Phase: "download", Database: ref.Database(), Rows: int64(len(e.scans[joinAlias(child.Base())]))})
		}
	}
	rootQuery := From(root.Base()).NewQuery()
	if ref, ok := root.Base().(CollectionRef); ok {
		if orders := ref.ScanOrders(); len(orders) != 0 {
			rootQuery.OrderBy(orders...)
		}
		if limit := ref.ScanLimit(); limit > 0 {
			rootQuery.Limit(limit)
		}
	}
	source, err := routed.ExecuteQueryToRecordsReader(ctx, rootQuery.SelectIntoRecord(nil))
	if err != nil {
		return nil, fmt.Errorf("scan federated fact source: %w", err)
	}
	stream := &federatedJoinStream{source: source, execution: e, root: root, progress: options.OnProgress}
	if ref, ok := root.Base().(CollectionRef); ok {
		stream.database = ref.Database()
	}
	plan, err := PlanAggregation(q, QueryCapabilities{StableRowOrder: true})
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	return newLocalAggregationReader(ctx, q, stream, plan), nil
}

type federatedJoinStream struct {
	source          RecordsReader
	execution       *joinExecution
	root            FromSource
	database        string
	progress        func(FederatedProgress)
	read, processed int64
	pending         []record.Record
	closed          bool
}

func (s *federatedJoinStream) Cursor() (string, error) { return "", nil }
func (s *federatedJoinStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.source.Close()
}

func (s *federatedJoinStream) Next() (record.Record, error) {
	for {
		if len(s.pending) != 0 {
			row := s.pending[0]
			s.pending = s.pending[1:]
			s.processed++
			if s.progress != nil && s.processed%1024 == 0 {
				s.progress(FederatedProgress{Phase: "process", Rows: s.processed})
			}
			return row, nil
		}
		rec, err := s.source.Next()
		if errors.Is(err, io.EOF) {
			if s.progress != nil {
				s.progress(FederatedProgress{Phase: "download", Database: s.database, Rows: s.read})
				s.progress(FederatedProgress{Phase: "process", Rows: s.processed})
			}
			return nil, ErrNoMoreRecords
		}
		if err != nil {
			return nil, err
		}
		s.read++
		if s.progress != nil && s.read%1024 == 0 {
			s.progress(FederatedProgress{Phase: "download", Database: s.database, Rows: s.read})
		}
		for _, ref := range s.execution.keyRefs[joinAlias(s.root.Base())] {
			if _, err := joinValueKey(rawJoinField(rec.Data(), ref.field), ref.path); err != nil {
				return nil, err
			}
		}
		data, err := normalizedJoinRecordMap(rec)
		if err != nil {
			return nil, err
		}
		s.execution.candidates = 0 // the work bound applies to one streamed fact row
		rows, err := s.execution.build(s.root, "from", nil, []scannedJoinRow{{key: rec.Key(), data: data}})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			keep, err := s.execution.conditionAt(s.execution.q.Where(), row, "where")
			if err != nil {
				return nil, err
			}
			if keep {
				s.pending = append(s.pending, record.NewRecordWithData(row.key, flattenJoinRow(row, s.execution.aliases, true)))
			}
		}
	}
}
