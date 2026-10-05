package dal

import (
	"context"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

const (
	placementInWhere     = "an aggregate with an order cannot stand in where"
	placementInScanOrder = "an aggregate with an order cannot stand in a scan order"
)

// placementOrdered is an aggregate with an order, on the fields of the Invoice source.
func placementOrdered() AggregateFunc {
	return NewOrderedAggregate(LAST, orderedBy(Ascending(NewFieldRef("i", "InvoiceDate"))), NewFieldRef("i", "Total"))
}

// placementProvider is a provider for the placement tests: one that runs no join, or one that
// accepts every join, over the tables of the ordered aggregate tests.
type placementProvider struct {
	name  string
	db    DB
	stub  *orderedStub
	joins *orderedJoinStub // nil for a provider that runs no join
}

func placementProviders(caps QueryCapabilities) []placementProvider {
	newStub := func() *orderedStub {
		return &orderedStub{caps: caps, rows: map[string][]record.Record{
			"Invoice":  instantRecords(timestampDatasets[0].rows, false),
			"Customer": customerRecords(),
		}}
	}
	plain := newStub()
	accepting := &orderedJoinStub{orderedStub: newStub()}
	return []placementProvider{
		{"a provider that runs no join", NewDB(plain), plain, nil},
		{"a provider that accepts every join", NewDB(accepting), accepting.orderedStub, accepting},
	}
}

// joinsAsked is the number of joins the provider was asked to accept.
func (p placementProvider) joinsAsked() int {
	if p.joins == nil {
		return 0
	}
	return p.joins.accepted
}

func (b *orderedStub) readsReached() int { return b.native + b.plain + b.nativeSets }

// An aggregate with an order is refused in where before any route is chosen: whether the
// provider runs joins or not, whether the query holds a subquery, and through the optional
// Select of the database and of its transactions, which an adapter serves itself for a plain
// query. Nothing reaches the provider.
func TestOrderedAggregateInWhereIsRefusedBeforeAnyRouteIsChosen(t *testing.T) {
	ctx := context.Background()
	invoice, customer := NewRootCollectionRef("Invoice", "i"), NewRootCollectionRef("Customer", "c")
	onCustomer := joinOn("i", "CustomerId", "c", "Id")
	inWhere := NewComparison(placementOrdered(), GreaterThen, NewConstant(1))
	columns := []Column{{Expression: NewFieldRef("i", "Total")}}
	exists := NewExistsCondition(From(customer).NewQuery().Where(NewComparison(NewFieldRef("c", "Id"), Equal, NewFieldRef("i", "CustomerId"))).
		SelectColumns(Column{Expression: NewFieldRef("c", "Id")}))
	// The nested query holds the aggregate, and the query it is nested in does not.
	nested := NewExistsCondition(From(customer).NewQuery().Where(inWhere).SelectColumns(Column{Expression: NewFieldRef("c", "Id")}))
	derived := NewQuerySource(From(customer).NewQuery().Where(NewComparison(NewOrderedAggregate(LAST, orderedBy(AscendingField("Id")), Field("Id")), GreaterThen, NewConstant(1))).
		SelectColumns(Column{Expression: NewFieldRef("c", "Id")}), "d")

	for name, tc := range map[string]struct {
		q StructuredQuery
		// the sources read before the refusal; a nested query is refused when it runs, after the sources of the
		// query that holds it are read, and before any of its own
		reads []string
	}{
		"a join the provider does not run":                  {From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().Where(inWhere).SelectColumns(columns...), nil},
		"a join the provider does not run, held by pointer": {From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().Where(&inWhere).SelectColumns(columns...), nil},
		"one source": {From(invoice).NewQuery().Where(inWhere).SelectColumns(columns...), nil},
		"a subquery in where, beside the aggregate":    {From(invoice).NewQuery().Where(NewGroupCondition(And, inWhere, exists)).SelectColumns(columns...), nil},
		"a subquery in where that holds the aggregate": {From(invoice).NewQuery().Where(nested).SelectColumns(columns...), []string{"Invoice"}},
		"a derived source that holds the aggregate":    {From(derived).NewQuery().SelectColumns(Column{Expression: NewFieldRef("d", "Id")}), nil},
	} {
		for label, caps := range map[string]QueryCapabilities{"no capabilities": {}, "capabilities": orderedCapabilities} {
			for _, provider := range placementProviders(caps) {
				t.Run(name+"/"+label+"/"+provider.name, func(t *testing.T) {
					stub := provider.stub
					if _, err := provider.db.ExecuteQueryToRecordsReader(ctx, tc.q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
						t.Fatalf("records reader: error = %v", err)
					}
					if _, err := provider.db.ExecuteQueryToRecordsetReader(ctx, tc.q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
						t.Fatalf("recordset reader: error = %v", err)
					}
					var read []string
					for _, q := range stub.plainReads {
						read = append(read, q.From().Base().Name())
					}
					// Each reader reads the sources of the query that holds a nested one, and the nested one's own sources never.
					var want []string
					for i := 0; i < 2 && len(tc.reads) > 0; i++ {
						want = append(want, tc.reads...)
					}
					if strings.Join(read, ",") != strings.Join(want, ",") || stub.native+stub.nativeSets+provider.joinsAsked() != 0 {
						t.Fatalf("sources read = %v, want %v; native = %d, recordset reads = %d, joins asked = %d", read, want, stub.native, stub.nativeSets, provider.joinsAsked())
					}
				})
			}
		}
	}

	// The optional Select an adapter serves itself for a plain query is not reached.
	t.Run("Select of the database", func(t *testing.T) {
		base := &ignoringJoinBackend{data: map[string][]record.Record{"Invoice": nil}, reads: map[string]int{}}
		selector := NewDB(base).(interface {
			Select(context.Context, Query) (Reader, error)
		})
		q := From(invoice).NewQuery().Where(inWhere).SelectColumns(columns...)
		if _, err := selector.Select(ctx, q); err == nil || !strings.Contains(err.Error(), placementInWhere) || base.selected || len(base.reads) != 0 {
			t.Fatalf("error = %v, the adapter's Select was reached = %v, reads = %v", err, base.selected, base.reads)
		}
		scanned := From(invoice.WithScan(5, Ascending(placementOrdered()))).NewQuery().SelectColumns(columns...)
		if _, err := selector.Select(ctx, scanned); err == nil || !strings.Contains(err.Error(), placementInScanOrder) || base.selected || len(base.reads) != 0 {
			t.Fatalf("scan order: error = %v, the adapter's Select was reached = %v, reads = %v", err, base.selected, base.reads)
		}
		// The same query without an ordered aggregate is handed to the adapter as before.
		plain := From(invoice).NewQuery().Where(NewComparison(NewFieldRef("i", "Total"), GreaterThen, NewConstant(1))).SelectColumns(columns...)
		if _, err := selector.Select(ctx, plain); err == nil || !base.selected {
			t.Fatalf("error = %v, the adapter's Select was reached = %v", err, base.selected)
		}
	})
	t.Run("Select of a transaction", func(t *testing.T) {
		base := &ignoringJoinBackend{data: map[string][]record.Record{"Invoice": nil}, reads: map[string]int{}}
		backend := &joinTransactionBackend{ignoringJoinBackend: base, read: &joinTestReadTx{backend: base}, write: &joinTestWriteTx{backend: base}}
		db := NewDB(backend)
		q := From(invoice).NewQuery().Where(inWhere).SelectColumns(columns...)
		scanned := From(invoice.WithScan(5, Ascending(placementOrdered()))).NewQuery().SelectColumns(columns...)
		plain := From(invoice).NewQuery().Where(NewComparison(NewFieldRef("i", "Total"), GreaterThen, NewConstant(1))).SelectColumns(columns...)
		const bypass = "raw transaction Select bypassed planner"
		selectIn := func(tx any, query StructuredQuery) error {
			_, err := tx.(interface {
				Select(context.Context, Query) (Reader, error)
			}).Select(ctx, query)
			return err
		}
		if err := db.RunReadonlyTransaction(ctx, func(_ context.Context, tx ReadTransaction) error {
			if err := selectIn(tx, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
				t.Errorf("read transaction: error = %v", err)
			}
			if err := selectIn(tx, scanned); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
				t.Errorf("read transaction, a scan order: error = %v", err)
			}
			if err := selectIn(tx, plain); err == nil || !strings.Contains(err.Error(), bypass) {
				t.Errorf("read transaction, a query with no ordered aggregate: error = %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.RunReadwriteTransaction(ctx, func(_ context.Context, tx ReadwriteTransaction) error {
			if err := selectIn(tx, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
				t.Errorf("read-write transaction: error = %v", err)
			}
			if err := selectIn(tx, scanned); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
				t.Errorf("read-write transaction, a scan order: error = %v", err)
			}
			if err := selectIn(tx, plain); err == nil || !strings.Contains(err.Error(), bypass) {
				t.Errorf("read-write transaction, a query with no ordered aggregate: error = %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if len(base.reads) != 0 {
			t.Fatalf("reads = %v, want none", base.reads)
		}
	})
	// The readers of a transaction hold the query to the rule as the readers of the database do.
	t.Run("the readers of a transaction", func(t *testing.T) {
		base := &ignoringJoinBackend{data: map[string][]record.Record{"Invoice": nil, "Customer": nil}, reads: map[string]int{}}
		backend := &joinTransactionBackend{ignoringJoinBackend: base, read: &joinTestReadTx{backend: base}, write: &joinTestWriteTx{backend: base}}
		db := NewDB(backend)
		for name, q := range map[string]StructuredQuery{
			"one source": From(invoice).NewQuery().Where(inWhere).SelectColumns(columns...),
			"a join":     From(invoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().Where(inWhere).SelectColumns(columns...),
		} {
			readersIn := func(tx QueryExecutor) {
				if _, err := tx.ExecuteQueryToRecordsReader(ctx, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
					t.Errorf("%s, records reader: error = %v", name, err)
				}
				if _, err := tx.ExecuteQueryToRecordsetReader(ctx, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
					t.Errorf("%s, recordset reader: error = %v", name, err)
				}
			}
			if err := db.RunReadonlyTransaction(ctx, func(_ context.Context, tx ReadTransaction) error { readersIn(tx); return nil }); err != nil {
				t.Fatal(err)
			}
			if err := db.RunReadwriteTransaction(ctx, func(_ context.Context, tx ReadwriteTransaction) error { readersIn(tx); return nil }); err != nil {
				t.Fatal(err)
			}
		}
		if len(base.reads) != 0 || backend.read.probes+backend.write.probes != 0 {
			t.Fatalf("reads = %v, joins asked = %d, want none", base.reads, backend.read.probes+backend.write.probes)
		}
	})
	// The recursive executor, which runs a nested query, holds the query to it before any source is read.
	t.Run("the recursive executor", func(t *testing.T) {
		provider := placementProviders(orderedCapabilities)[0]
		stub := provider.stub
		q := From(invoice).NewQuery().Where(inWhere).SelectColumns(columns...)
		if _, err := ExecuteRecursiveQuery(ctx, stub, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
			t.Fatalf("ExecuteRecursiveQuery error = %v", err)
		}
		if _, err := ExecuteRecursiveRecordset(ctx, stub, q); err == nil || !strings.Contains(err.Error(), placementInWhere) {
			t.Fatalf("ExecuteRecursiveRecordset error = %v", err)
		}
		if stub.readsReached() != 0 {
			t.Fatalf("reads = %d, want none", stub.readsReached())
		}
	})
}

// An aggregate with an order is refused in the scan order of a source as well: a scan order is
// carried to the provider as the ORDER BY of a plain read. Nothing is read, on one source, on a join
// DALgo runs, on a join the provider accepts, and through ExecuteFederatedQuery. A scan order that
// holds no aggregate with an order is read as before.
func TestOrderedAggregateInAScanOrderIsRefusedBeforeAnythingIsRead(t *testing.T) {
	ctx := context.Background()
	scan := func(ref CollectionRef) CollectionRef { return ref.WithScan(5, Ascending(placementOrdered())) }
	invoice, customer := NewRootCollectionRef("Invoice", "i"), NewRootCollectionRef("Customer", "c")
	onCustomer := joinOn("i", "CustomerId", "c", "Id")
	columns := []Column{{Expression: NewFieldRef("c", "Id")}}
	scannedInvoice, scannedCustomer := scan(invoice), scan(customer)
	otherJoin := NewJoinedFrom(From(customer), JoinInner, onCustomer)
	otherJoin.RecordsetSource = scannedCustomer

	queries := map[string]StructuredQuery{
		"one source":                       From(scannedInvoice).NewQuery().SelectColumns(columns...),
		"one source, held by pointer":      From(&scannedInvoice).NewQuery().SelectColumns(columns...),
		"the base of a join":               From(scannedInvoice).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().SelectColumns(columns...),
		"a joined source":                  From(invoice).Join(NewJoinedSource(scannedCustomer, JoinInner, onCustomer)).NewQuery().SelectColumns(columns...),
		"a source a join names for itself": From(invoice).Join(otherJoin).NewQuery().SelectColumns(columns...),
		"a source of a nested join": From(invoice).Join(NewJoinedFrom(From(customer).Join(NewJoinedSource(scannedCustomer, JoinInner, joinOn("c", "Id", "c", "Id"))), JoinInner, onCustomer)).
			NewQuery().SelectColumns(columns...),
		"inside arithmetic": From(invoice.WithScan(5, Ascending(Binary(placementOrdered(), Add, NewConstant(1))))).NewQuery().SelectColumns(columns...),
	}
	for name, q := range queries {
		for label, caps := range map[string]QueryCapabilities{"no capabilities": {}, "capabilities": orderedCapabilities} {
			for _, provider := range placementProviders(caps) {
				t.Run(name+"/"+label+"/"+provider.name, func(t *testing.T) {
					if err := ValidateAggregation(q); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
						t.Fatalf("ValidateAggregation error = %v", err)
					}
					if _, err := provider.db.ExecuteQueryToRecordsReader(ctx, q); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
						t.Fatalf("records reader: error = %v", err)
					}
					if _, err := provider.db.ExecuteQueryToRecordsetReader(ctx, q); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
						t.Fatalf("recordset reader: error = %v", err)
					}
					if provider.stub.readsReached() != 0 || provider.joinsAsked() != 0 {
						t.Fatalf("reads = %d, joins asked = %d, want none", provider.stub.readsReached(), provider.joinsAsked())
					}
				})
			}
		}
	}

	t.Run("ExecuteFederatedQuery", func(t *testing.T) {
		federatedInvoice := NewDatabaseCollectionRef("orders", "", "Invoice", "i")
		federatedCustomer := NewDatabaseCollectionRef("customers", "", "Customer", "c")
		reads, resolved := 0, 0
		resolve := func(context.Context, string) (QueryExecutor, error) {
			resolved++
			return &readCountingExecutor{reads: &reads}, nil
		}
		for name, q := range map[string]StructuredQuery{
			"one source": From(scan(federatedInvoice)).NewQuery().SelectColumns(columns...),
			"a join":     From(federatedInvoice).Join(NewJoinedSource(scan(federatedCustomer), JoinInner, onCustomer)).NewQuery().SelectColumns(columns...),
		} {
			if _, err := ExecuteFederatedQuery(ctx, q, resolve); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
				t.Fatalf("%s: error = %v", name, err)
			}
		}
		if reads != 0 || resolved != 0 {
			t.Fatalf("reads = %d, databases resolved = %d, want none", reads, resolved)
		}
		// A scan order that holds no aggregate with an order is read as before, with an aggregate that has none too.
		plain := From(federatedInvoice.WithScan(5, AscendingField("Total"), Ascending(NewAggregate(LAST, false, NewFieldRef("i", "Total"))))).NewQuery().
			SelectColumns(Column{Expression: NewFieldRef("i", "Total")})
		resolvePlain := func(context.Context, string) (QueryExecutor, error) {
			resolved++
			return federatedStub{rows: instantRecords(timestampDatasets[0].rows, false)}, nil
		}
		reader, err := ExecuteFederatedQuery(ctx, plain, resolvePlain)
		if err != nil || resolved == 0 {
			t.Fatalf("error = %v, databases resolved = %d", err, resolved)
		}
		if rows := readOrderedRows(t, reader); len(rows) != len(timestampDatasets[0].rows) {
			t.Fatalf("rows = %v", rows)
		}
	})

	// A nested query is held to the rule for its scan orders as it runs: a subquery of one source, a subquery
	// of a join, and a derived source. None reads its own sources.
	t.Run("a nested query", func(t *testing.T) {
		nestedScan := scannedCustomer
		onId := NewComparison(NewFieldRef("c", "Id"), Equal, NewFieldRef("i", "CustomerId"))
		simple := NewExistsCondition(From(nestedScan).NewQuery().Where(onId).SelectColumns(columns...))
		joinedSubquery := NewExistsCondition(From(nestedScan).Join(NewJoinedSource(NewRootCollectionRef("Customer", "c2"), JoinInner, joinOn("c", "Id", "c2", "Id"))).NewQuery().
			Where(onId).SelectColumns(columns...))
		derived := NewQuerySource(From(nestedScan).NewQuery().SelectColumns(columns...), "d")
		for name, q := range map[string]StructuredQuery{
			"a subquery of one source": From(invoice).NewQuery().Where(simple).SelectColumns(Column{Expression: NewFieldRef("i", "Total")}),
			"a subquery of a join":     From(invoice).NewQuery().Where(joinedSubquery).SelectColumns(Column{Expression: NewFieldRef("i", "Total")}),
			"a derived source":         From(derived).NewQuery().SelectColumns(Column{Expression: NewFieldRef("d", "Id")}),
		} {
			for _, provider := range placementProviders(orderedCapabilities) {
				t.Run(name+"/"+provider.name, func(t *testing.T) {
					if _, err := provider.db.ExecuteQueryToRecordsReader(ctx, q); err == nil || !strings.Contains(err.Error(), placementInScanOrder) {
						t.Fatalf("records reader: error = %v", err)
					}
					for _, read := range provider.stub.plainReads {
						if read.From().Base().Name() == "Customer" {
							t.Fatalf("the nested query read its own source %q", read.From().Base().Name())
						}
					}
				})
			}
		}
	})

	// A scan order that holds no aggregate with an order is read as before: the join is read by the provider that
	// accepts it, and source by source by DALgo when the provider runs none.
	t.Run("a scan order that holds no aggregate with an order is read as before", func(t *testing.T) {
		joined := From(invoice.WithScan(5, AscendingField("InvoiceDate"))).Join(NewJoinedSource(customer, JoinInner, onCustomer)).NewQuery().SelectColumns(columns...)
		single := From(invoice.WithScan(5, AscendingField("InvoiceDate"))).NewQuery().SelectColumns(columns...)
		for _, provider := range placementProviders(orderedCapabilities) {
			reader, err := provider.db.ExecuteQueryToRecordsReader(ctx, joined)
			if err != nil {
				t.Fatal(err)
			}
			readOrderedRows(t, reader)
			want := 2
			if provider.joins != nil {
				want = 1
			}
			if provider.stub.plain != want {
				t.Fatalf("%s: plain reads of the join = %d, want %d", provider.name, provider.stub.plain, want)
			}
			if _, err := provider.db.ExecuteQueryToRecordsReader(ctx, single); err != nil || provider.stub.plain != want+1 {
				t.Fatalf("%s: error = %v, plain reads = %d, want %d", provider.name, err, provider.stub.plain, want+1)
			}
		}
	})
}
