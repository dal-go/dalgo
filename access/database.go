package access

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

type secureDBOptions struct {
	databasePolicies []Policy
	requireContext   bool
}

// DBOption configures SecureDB.
type DBOption func(*secureDBOptions) error

// WithDatabasePolicies adds policies that apply to every operation through the
// secured DB and cannot be widened by a context policy.
func WithDatabasePolicies(policies ...Policy) DBOption {
	return func(options *secureDBOptions) error {
		for i, policy := range policies {
			if policy == nil {
				return fmt.Errorf("access: nil database policy at index %d", i)
			}
		}
		options.databasePolicies = append(options.databasePolicies, policies...)
		return nil
	}
}

// RequireContextPolicy makes missing context-bound authority fail closed.
func RequireContextPolicy() DBOption {
	return func(options *secureDBOptions) error {
		options.requireContext = true
		return nil
	}
}

// SecureDB wraps db with adapter-independent access-policy enforcement.
func SecureDB(db dal.DB, options ...DBOption) (dal.DB, error) {
	if db == nil {
		return nil, fmt.Errorf("access: db is required")
	}
	var settings secureDBOptions
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("access: nil DB option")
		}
		if err := option(&settings); err != nil {
			return nil, err
		}
	}
	return &securedDB{
		db: db,
		guard: guard{
			databasePolicies: append([]Policy(nil), settings.databasePolicies...),
			requireContext:   settings.requireContext,
		},
	}, nil
}

// MustSecureDB wraps db and panics when configuration is invalid.
func MustSecureDB(db dal.DB, options ...DBOption) dal.DB {
	secured, err := SecureDB(db, options...)
	if err != nil {
		panic(err)
	}
	return secured
}

// BindDB captures context policies on the returned DB handle. Passing a later
// operation context cannot remove them, while additional policies still narrow
// the capability.
func BindDB(db dal.DB, ctx context.Context) dal.DB {
	if secured, ok := db.(*securedDB); ok {
		bound := *secured
		bound.guard = secured.guard.bind(ctx)
		return &bound
	}
	return &securedDB{db: db, guard: guard{}.bind(ctx)}
}

type securedDB struct {
	db    dal.DB
	guard guard
}

func (db *securedDB) ID() string { return db.db.ID() }

func (db *securedDB) Adapter() dal.Adapter { return db.db.Adapter() }

func (db *securedDB) Schema() dal.Schema { return db.db.Schema() }

func (db *securedDB) SupportsConcurrentConnections() bool {
	return db.db.SupportsConcurrentConnections()
}

func (db *securedDB) Exists(ctx context.Context, key *record.Key) (bool, error) {
	return securedReadSession{session: db.db, guard: db.guard}.Exists(ctx, key)
}

func (db *securedDB) Get(ctx context.Context, record record.Record) error {
	return securedReadSession{session: db.db, guard: db.guard}.Get(ctx, record)
}

func (db *securedDB) GetMulti(ctx context.Context, records []record.Record) error {
	return securedReadSession{session: db.db, guard: db.guard}.GetMulti(ctx, records)
}

func (db *securedDB) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return securedReadSession{session: db.db, guard: db.guard}.ExecuteQueryToRecordsReader(ctx, query)
}

func (db *securedDB) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return securedReadSession{session: db.db, guard: db.guard}.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func (db *securedDB) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	if err := db.guard.checkContext(ctx); err != nil {
		return err
	}
	captured := db.guard.bind(ctx)
	return db.db.RunReadonlyTransaction(ctx, func(workerCtx context.Context, tx dal.ReadTransaction) error {
		securedTx := &securedReadTransaction{
			securedReadSession: securedReadSession{session: tx, guard: captured},
			tx:                 tx,
		}
		workerCtx = dal.NewContextWithTransaction(workerCtx, securedTx)
		return worker(workerCtx, securedTx)
	}, options...)
}

func (db *securedDB) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, options ...dal.TransactionOption) error {
	if err := db.guard.checkContext(ctx); err != nil {
		return err
	}
	captured := db.guard.bind(ctx)
	return db.db.RunReadwriteTransaction(ctx, func(workerCtx context.Context, tx dal.ReadwriteTransaction) error {
		securedTx := &securedReadwriteTransaction{
			securedReadwriteSession: securedReadwriteSession{
				securedReadSession:  securedReadSession{session: tx, guard: captured},
				securedWriteSession: securedWriteSession{session: tx, guard: captured},
			},
			tx: tx,
		}
		workerCtx = dal.NewContextWithTransaction(workerCtx, securedTx)
		return worker(workerCtx, securedTx)
	}, options...)
}

type securedReadTransaction struct {
	securedReadSession
	tx dal.ReadTransaction
}

func (tx *securedReadTransaction) Options() dal.TransactionOptions { return tx.tx.Options() }

type securedReadwriteTransaction struct {
	securedReadwriteSession
	tx dal.ReadwriteTransaction
}

func (tx *securedReadwriteTransaction) ID() string { return tx.tx.ID() }

func (tx *securedReadwriteTransaction) Options() dal.TransactionOptions { return tx.tx.Options() }

var (
	_ dal.DB                   = (*securedDB)(nil)
	_ dal.ReadTransaction      = (*securedReadTransaction)(nil)
	_ dal.ReadwriteTransaction = (*securedReadwriteTransaction)(nil)
)
