package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// countingSession is a wrapped session that counts every read that reaches it.
// Every read returns no rows; writes are accepted and not counted.
type countingSession struct{ reads int }

func (s *countingSession) Exists(context.Context, *record.Key) (bool, error) {
	s.reads++
	return false, nil
}
func (s *countingSession) Get(context.Context, record.Record) error {
	s.reads++
	return nil
}
func (s *countingSession) GetMulti(context.Context, []record.Record) error {
	s.reads++
	return nil
}
func (s *countingSession) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	s.reads++
	return dal.EmptyReader{}, nil
}
func (s *countingSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	s.reads++
	return emptyRecordsetReader{}, nil
}
func (*countingSession) Set(context.Context, record.Record) error        { return nil }
func (*countingSession) SetMulti(context.Context, []record.Record) error { return nil }
func (*countingSession) Insert(context.Context, record.Record, ...dal.InsertOption) error {
	return nil
}
func (*countingSession) InsertMulti(context.Context, []record.Record, ...dal.InsertOption) error {
	return nil
}
func (*countingSession) Update(context.Context, *record.Key, []update.Update, ...dal.Precondition) error {
	return nil
}
func (*countingSession) UpdateRecord(context.Context, record.Record, []update.Update, ...dal.Precondition) error {
	return nil
}
func (*countingSession) UpdateMulti(context.Context, []*record.Key, []update.Update, ...dal.Precondition) error {
	return nil
}
func (*countingSession) Delete(context.Context, *record.Key) error        { return nil }
func (*countingSession) DeleteMulti(context.Context, []*record.Key) error { return nil }

type emptyRecordsetReader struct{}

func (emptyRecordsetReader) Cursor() (string, error)        { return "", nil }
func (emptyRecordsetReader) Close() error                   { return nil }
func (emptyRecordsetReader) Recordset() recordset.Recordset { return nil }
func (emptyRecordsetReader) Next() (recordset.Row, recordset.Recordset, error) {
	return nil, nil, dal.ErrNoMoreRecords
}

// countingTx hands a countingSession to a transaction worker.
type countingTx struct{ *countingSession }

func (countingTx) ID() string                      { return "tx" }
func (countingTx) Options() dal.TransactionOptions { return dal.NewTransactionOptions() }

// countingDB is a database whose sessions, transactions and direct reads all
// share one countingSession and run no query planning of their own, so the
// count is exactly what the secured wrapper let through.
type countingDB struct {
	dal.DB
	session *countingSession
}

func newCountingDB(session *countingSession) countingDB {
	backend := &fakeDB{fakeSession: &fakeSession{}, adapter: dal.NewAdapter("counting", "1"), schema: dal.NewSchema(nil, nil)}
	return countingDB{DB: dal.NewDB(backend), session: session}
}

func (db countingDB) Exists(ctx context.Context, key *record.Key) (bool, error) {
	return db.session.Exists(ctx, key)
}
func (db countingDB) Get(ctx context.Context, rec record.Record) error {
	return db.session.Get(ctx, rec)
}
func (db countingDB) GetMulti(ctx context.Context, records []record.Record) error {
	return db.session.GetMulti(ctx, records)
}
func (db countingDB) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return db.session.ExecuteQueryToRecordsReader(ctx, query)
}
func (db countingDB) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return db.session.ExecuteQueryToRecordsetReader(ctx, query, options...)
}
func (db countingDB) Select(ctx context.Context, query dal.Query) (dal.Reader, error) {
	return db.session.ExecuteQueryToRecordsReader(ctx, query)
}
func (db countingDB) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return worker(ctx, countingTx{db.session})
}
func (db countingDB) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return worker(ctx, countingTx{db.session})
}

// selectReader is the legacy read surface every secured database and
// transaction carries next to the QueryExecutor methods.
type selectReader interface {
	Select(context.Context, dal.Query) (dal.Reader, error)
}

// sourcePath runs one query through one secured read path over a wrapped
// session that counts the reads reaching it.
type sourcePath func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error

func secured(wrapped *countingSession, policy Policy) dal.DB {
	return MustSecureDB(newCountingDB(wrapped), WithDatabasePolicies(policy))
}

// countingWriteDB is a countingDB that is also a write session, so SecureDB
// wraps it as a database that writes.
type countingWriteDB struct{ countingDB }

func (db countingWriteDB) Set(ctx context.Context, rec record.Record) error {
	return db.session.Set(ctx, rec)
}
func (db countingWriteDB) SetMulti(ctx context.Context, records []record.Record) error {
	return db.session.SetMulti(ctx, records)
}
func (db countingWriteDB) Insert(ctx context.Context, rec record.Record, options ...dal.InsertOption) error {
	return db.session.Insert(ctx, rec, options...)
}
func (db countingWriteDB) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	return db.session.InsertMulti(ctx, records, options...)
}
func (db countingWriteDB) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.session.Update(ctx, key, updates, preconditions...)
}
func (db countingWriteDB) UpdateRecord(ctx context.Context, rec record.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.session.UpdateRecord(ctx, rec, updates, preconditions...)
}
func (db countingWriteDB) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.session.UpdateMulti(ctx, keys, updates, preconditions...)
}
func (db countingWriteDB) Delete(ctx context.Context, key *record.Key) error {
	return db.session.Delete(ctx, key)
}
func (db countingWriteDB) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	return db.session.DeleteMulti(ctx, keys)
}

// securedWriting secures a database that is also a write session. It is a
// *securedWriteDB, a different type from the one secured returns.
func securedWriting(wrapped *countingSession, policy Policy) dal.DB {
	db := MustSecureDB(countingWriteDB{newCountingDB(wrapped)}, WithDatabasePolicies(policy))
	if _, ok := db.(*securedWriteDB); !ok {
		panic("a database that writes is not secured as one")
	}
	return db
}

var sourcePaths = map[string]sourcePath{
	"session records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"session recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := SecureReadSession(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, query)
		return err
	},
	"database records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := secured(wrapped, policy).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"database recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := secured(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, query)
		return err
	},
	"database select": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := secured(wrapped, policy).(selectReader).Select(ctx, query)
		return err
	},
	"read transaction records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			_, err := tx.ExecuteQueryToRecordsReader(ctx, query)
			return err
		})
	},
	"read transaction recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			_, err := tx.ExecuteQueryToRecordsetReader(ctx, query)
			return err
		})
	},
	"read transaction select": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			_, err := tx.(selectReader).Select(ctx, query)
			return err
		})
	},
	"read-write session records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := SecureReadwriteSession(wrapped, policy).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"read-write session recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := SecureReadwriteSession(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, query)
		return err
	},
	"bound database records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := BindDB(secured(wrapped, policy), ctx).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"bound database recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := BindDB(secured(wrapped, policy), ctx).ExecuteQueryToRecordsetReader(ctx, query)
		return err
	},
	"bound database select": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := BindDB(secured(wrapped, policy), ctx).(selectReader).Select(ctx, query)
		return err
	},
	"writing database records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := securedWriting(wrapped, policy).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"writing database recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := securedWriting(wrapped, policy).ExecuteQueryToRecordsetReader(ctx, query)
		return err
	},
	"writing database select": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := securedWriting(wrapped, policy).(selectReader).Select(ctx, query)
		return err
	},
	"bound writing database records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		_, err := BindDB(securedWriting(wrapped, policy), ctx).ExecuteQueryToRecordsReader(ctx, query)
		return err
	},
	"writing database read transaction records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return securedWriting(wrapped, policy).RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			_, err := tx.ExecuteQueryToRecordsReader(ctx, query)
			return err
		})
	},
	"read-write transaction records reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			_, err := tx.ExecuteQueryToRecordsReader(ctx, query)
			return err
		})
	},
	"read-write transaction recordset reader": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			_, err := tx.ExecuteQueryToRecordsetReader(ctx, query)
			return err
		})
	},
	"read-write transaction select": func(ctx context.Context, wrapped *countingSession, policy Policy, query dal.Query) error {
		return secured(wrapped, policy).RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			_, err := tx.(selectReader).Select(ctx, query)
			return err
		})
	},
}

const hiddenCollection = "Secret"

var (
	customers = dal.NewRootCollectionRef("Customer", "c")
	orders    = dal.NewRootCollectionRef("Order", "o")
	lines     = dal.NewRootCollectionRef("Line", "l")
	hidden    = dal.NewRootCollectionRef(hiddenCollection, "s")
)

// hiddenOnly allows every ordinary collection except Secret.
func hiddenOnly() Policy {
	return MustPolicy("secret-denied",
		Root(Allow(Query, "ordinary collections")),
		Collection(hiddenCollection, Deny(Query, "secret is denied")),
	)
}

// allowEverything allows every ordinary collection, Secret included.
func allowEverything() Policy {
	return MustPolicy("all-allowed", Root(Allow(Query, "ordinary collections")))
}

func joinOn(left, right string) dal.Condition {
	return dal.NewComparison(dal.NewFieldRef(left, "id"), dal.Equal, dal.NewFieldRef(right, "ref"))
}

// secretRows is a query over the hidden collection, nested as a subquery.
func secretRows() dal.StructuredQuery {
	return dal.From(hidden).NewQuery().SelectKeysOnly(reflect.String)
}

func scalarSecret() dal.QueryExpression { return dal.NewQueryExpression(secretRows(), "x") }

func customerQuery() dal.IQueryBuilder { return dal.From(customers).NewQuery() }

// sourceShape builds a query that reads the hidden collection somewhere. reads
// is the number of reads an allowed query makes on the wrapped session over
// sources that hold no rows: one for a query handed to the wrapped session
// whole, and one per source scan for a query with nested queries, which the
// access layer executes a source at a time so that each scan is authorised and
// narrowed on its own. invalid marks a shape DALgo's own query validation
// rejects once the engine has started (a subquery as a HAVING operand, a GROUP
// BY key or an aggregate argument): the allowed query ends in a validation
// error that is not an access denial.
type sourceShape struct {
	name    string
	build   func() dal.StructuredQuery
	reads   int
	invalid bool
}

func sourceShapes() []sourceShape {
	return []sourceShape{
		{"join at depth two", func() dal.StructuredQuery {
			return dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedSource(hidden, dal.JoinInner, joinOn("o", "s"))),
				dal.JoinInner, joinOn("c", "o"),
			)).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
		{"join at depth three", func() dal.StructuredQuery {
			return dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedFrom(
					dal.From(lines).Join(dal.NewJoinedSource(hidden, dal.JoinInner, joinOn("l", "s"))),
					dal.JoinInner, joinOn("o", "l"),
				)),
				dal.JoinInner, joinOn("c", "o"),
			)).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
		{"derived source", func() dal.StructuredQuery {
			return dal.From(dal.NewQuerySource(secretRows(), "d")).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
		{"derived source inside a join", func() dal.StructuredQuery {
			return dal.From(customers).Join(
				dal.NewJoinedSource(dal.NewQuerySource(secretRows(), "d"), dal.JoinInner, joinOn("c", "d")),
			).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
		{"EXISTS", func() dal.StructuredQuery {
			return customerQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
		}, 1, false},
		{"NOT EXISTS", func() dal.StructuredQuery {
			return customerQuery().Where(dal.NewNotExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
		}, 1, false},
		{"scalar subquery in the select list", func() dal.StructuredQuery {
			return customerQuery().SelectColumns(dal.Column{Alias: "x", Expression: scalarSecret()})
		}, 1, false},
		{"scalar subquery in WHERE", func() dal.StructuredQuery {
			return customerQuery().Where(dal.NewComparison(dal.Field("a"), dal.Equal, scalarSecret())).SelectKeysOnly(reflect.String)
		}, 1, false},
		{"scalar subquery in HAVING", func() dal.StructuredQuery {
			return customerQuery().GroupBy(dal.Field("a")).
				Having(dal.NewComparison(dal.Count().Expression, dal.Equal, scalarSecret())).SelectKeysOnly(reflect.String)
		}, 1, true},
		{"scalar subquery in ORDER BY", func() dal.StructuredQuery {
			return customerQuery().OrderBy(dal.Ascending(scalarSecret())).SelectKeysOnly(reflect.String)
		}, 1, false},
		{"scalar subquery in a join ON", func() dal.StructuredQuery {
			return dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				dal.NewComparison(dal.NewFieldRef("o", "ref"), dal.Equal, scalarSecret()),
			)).NewQuery().SelectKeysOnly(reflect.String)
		}, 2, false},
		{"scalar subquery in GROUP BY", func() dal.StructuredQuery {
			return customerQuery().GroupBy(scalarSecret()).SelectKeysOnly(reflect.String)
		}, 1, true},
		{"scalar subquery as an aggregate argument", func() dal.StructuredQuery {
			return customerQuery().SelectColumns(dal.SumAs(scalarSecret(), "total"))
		}, 1, true},
		{"scalar subquery as an arithmetic operand", func() dal.StructuredQuery {
			return customerQuery().SelectColumns(dal.Column{Alias: "sum", Expression: dal.Binary(dal.Field("a"), dal.Add, scalarSecret())})
		}, 1, false},
		{"scalar subquery in a null test", func() dal.StructuredQuery {
			return customerQuery().Where(dal.NewIsNullCondition(scalarSecret())).SelectKeysOnly(reflect.String)
		}, 1, false},
		{"subquery in a scan order", func() dal.StructuredQuery {
			return dal.From(customers.WithScan(10, dal.Ascending(scalarSecret()))).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
		{"subquery inside a derived source", func() dal.StructuredQuery {
			inner := customerQuery().Where(dal.NewExistsCondition(secretRows())).SelectKeysOnly(reflect.String)
			return dal.From(dal.NewQuerySource(inner, "d")).NewQuery().SelectKeysOnly(reflect.String)
		}, 1, false},
	}
}

func checkSourceShapes(t *testing.T, run sourcePath) {
	t.Helper()
	for _, shape := range sourceShapes() {
		t.Run(shape.name, func(t *testing.T) {
			ctx := context.Background()
			denied := &countingSession{}
			if err := run(ctx, denied, hiddenOnly(), shape.build()); !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("denied collection: error = %v, want ErrAccessDenied", err)
			}
			if denied.reads != 0 {
				t.Fatalf("denied collection: %d reads reached the wrapped session, want 0", denied.reads)
			}
			allowed := &countingSession{}
			err := run(ctx, allowed, allowEverything(), shape.build())
			if shape.invalid != (err != nil) || errors.Is(err, ErrAccessDenied) {
				t.Fatalf("allowed collection: error = %v, want no access denial (invalid shape: %v)", err, shape.invalid)
			}
			if allowed.reads != shape.reads {
				t.Fatalf("allowed collection: %d reads reached the wrapped session, want %d", allowed.reads, shape.reads)
			}
		})
	}
}

func TestEverySourceAuthorisedOnSessionRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["session records reader"])
}

func TestEverySourceAuthorisedOnSessionRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["session recordset reader"])
}

func TestEverySourceAuthorisedOnDatabaseRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["database records reader"])
}

func TestEverySourceAuthorisedOnDatabaseRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["database recordset reader"])
}

func TestEverySourceAuthorisedOnDatabaseSelect(t *testing.T) {
	checkSourceShapes(t, sourcePaths["database select"])
}

func TestEverySourceAuthorisedOnReadTransactionRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read transaction records reader"])
}

func TestEverySourceAuthorisedOnReadTransactionRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read transaction recordset reader"])
}

func TestEverySourceAuthorisedOnReadTransactionSelect(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read transaction select"])
}

func TestEverySourceAuthorisedOnReadwriteTransactionRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read-write transaction records reader"])
}

func TestEverySourceAuthorisedOnReadwriteTransactionRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read-write transaction recordset reader"])
}

func TestEverySourceAuthorisedOnReadwriteTransactionSelect(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read-write transaction select"])
}

func TestEverySourceAuthorisedOnReadwriteSessionRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read-write session records reader"])
}

func TestEverySourceAuthorisedOnReadwriteSessionRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["read-write session recordset reader"])
}

func TestEverySourceAuthorisedOnBoundDatabaseRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["bound database records reader"])
}

func TestEverySourceAuthorisedOnBoundDatabaseRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["bound database recordset reader"])
}

func TestEverySourceAuthorisedOnBoundDatabaseSelect(t *testing.T) {
	checkSourceShapes(t, sourcePaths["bound database select"])
}

func TestEverySourceAuthorisedOnWritingDatabaseRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["writing database records reader"])
}

func TestEverySourceAuthorisedOnWritingDatabaseRecordsetReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["writing database recordset reader"])
}

func TestEverySourceAuthorisedOnWritingDatabaseSelect(t *testing.T) {
	checkSourceShapes(t, sourcePaths["writing database select"])
}

func TestEverySourceAuthorisedOnBoundWritingDatabaseRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["bound writing database records reader"])
}

func TestEverySourceAuthorisedOnWritingDatabaseReadTransactionRecordsReader(t *testing.T) {
	checkSourceShapes(t, sourcePaths["writing database read transaction records reader"])
}
