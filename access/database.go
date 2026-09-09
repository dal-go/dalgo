package access

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type secureDBOptions struct {
	databasePolicies []Policy
	policyProvider   PolicyProvider
	requireContext   bool
	coordinator      *EnforcementCoordinator
}

// WithEnforcementCoordinator enables the bounded protected write profile.
func WithEnforcementCoordinator(coordinator *EnforcementCoordinator) DBOption {
	return func(options *secureDBOptions) error {
		if coordinator == nil {
			return fmt.Errorf("access: enforcement coordinator is required")
		}
		if options.coordinator != nil {
			return fmt.Errorf("access: enforcement coordinator already configured")
		}
		options.coordinator = coordinator
		return nil
	}
}

// PolicyProvider returns one immutable owner policy snapshot for an operation.
type PolicyProvider func(context.Context) ([]Policy, error)

// WithDatabasePolicyProvider configures a required dynamic owner snapshot.
func WithDatabasePolicyProvider(provider PolicyProvider) DBOption {
	return func(options *secureDBOptions) error {
		if provider == nil {
			return fmt.Errorf("access: database policy provider is required")
		}
		if options.policyProvider != nil {
			return fmt.Errorf("access: database policy provider is already configured")
		}
		options.policyProvider = provider
		return nil
	}
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
	secured := &securedDB{
		DB:          db,
		coordinator: settings.coordinator,
		guard: guard{
			databasePolicies: append([]Policy(nil), settings.databasePolicies...),
			policyProvider:   settings.policyProvider,
			requireContext:   settings.requireContext,
		},
	}
	if writer, ok := db.(dal.WriteSession); ok || settings.coordinator != nil {
		return &securedWriteDB{securedDB: secured, writer: writer}, nil
	}
	return secured, nil
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
	if secured, ok := db.(*securedWriteDB); ok {
		bound := *secured.securedDB
		bound.guard = secured.guard.bind(ctx)
		return &securedWriteDB{securedDB: &bound, writer: secured.writer}
	}
	if secured, ok := db.(*securedDB); ok {
		bound := *secured
		bound.guard = secured.guard.bind(ctx)
		return &bound
	}
	return &securedDB{DB: db, guard: guard{}.bind(ctx)}
}

// securedDB decorates a DB with access-policy enforcement. It embeds dal.DB
// rather than holding it in a named field so that it satisfies the sealed
// dal.DB interface: the unexported marker method is promoted along with every
// method securedDB does not override.
type securedDB struct {
	dal.DB
	guard       guard
	coordinator *EnforcementCoordinator
}

func (db *securedDB) ID() string { return db.DB.ID() }

func (db *securedDB) Adapter() dal.Adapter { return db.DB.Adapter() }

func (db *securedDB) Schema() dal.Schema { return db.DB.Schema() }

func (db *securedDB) SupportsConcurrentConnections() bool {
	return db.DB.SupportsConcurrentConnections()
}

func (db *securedDB) Exists(ctx context.Context, key *record.Key) (bool, error) {
	return securedReadSession{session: db.DB, guard: db.guard}.Exists(ctx, key)
}

func (db *securedDB) Get(ctx context.Context, record record.Record) error {
	return securedReadSession{session: db.DB, guard: db.guard}.Get(ctx, record)
}

func (db *securedDB) GetMulti(ctx context.Context, records []record.Record) error {
	return securedReadSession{session: db.DB, guard: db.guard}.GetMulti(ctx, records)
}

func (db *securedDB) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return securedReadSession{session: db.DB, guard: db.guard}.ExecuteQueryToRecordsReader(ctx, query)
}

func (db *securedDB) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return securedReadSession{session: db.DB, guard: db.guard}.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func (db *securedDB) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	if err := db.guard.checkContext(ctx); err != nil {
		return err
	}
	captured := db.guard.bind(ctx)
	var err error
	captured, err = captured.pinDatabasePolicies(ctx)
	if err != nil {
		return err
	}
	return db.DB.RunReadonlyTransaction(ctx, func(workerCtx context.Context, tx dal.ReadTransaction) error {
		securedTx := &securedReadTransaction{
			securedReadSession: securedReadSession{session: tx, guard: captured},
			tx:                 tx,
		}
		workerCtx = dal.NewContextWithTransaction(workerCtx, securedTx)
		return worker(workerCtx, securedTx)
	}, options...)
}

func (db *securedDB) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, options ...dal.TransactionOption) error {
	if db.coordinator != nil {
		return enforcementUnsupported(Update, "dynamic read-write transactions are unavailable under the protected profile")
	}
	if err := db.guard.checkContext(ctx); err != nil {
		return err
	}
	captured := db.guard.bind(ctx)
	var err error
	captured, err = captured.pinDatabasePolicies(ctx)
	if err != nil {
		return err
	}
	return db.DB.RunReadwriteTransaction(ctx, func(workerCtx context.Context, tx dal.ReadwriteTransaction) error {
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

type securedWriteDB struct {
	*securedDB
	writer dal.WriteSession
}

func (db *securedWriteDB) writeSession() securedWriteSession {
	return securedWriteSession{session: db.writer, guard: db.guard, coordinator: db.coordinator}
}
func (db *securedWriteDB) Set(ctx context.Context, rec record.Record) error {
	return db.writeSession().Set(ctx, rec)
}
func (db *securedWriteDB) SetMulti(ctx context.Context, records []record.Record) error {
	return db.writeSession().SetMulti(ctx, records)
}
func (db *securedWriteDB) Insert(ctx context.Context, rec record.Record, options ...dal.InsertOption) error {
	return db.writeSession().Insert(ctx, rec, options...)
}
func (db *securedWriteDB) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	return db.writeSession().InsertMulti(ctx, records, options...)
}
func (db *securedWriteDB) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.writeSession().Update(ctx, key, updates, preconditions...)
}
func (db *securedWriteDB) UpdateRecord(ctx context.Context, rec record.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.writeSession().UpdateRecord(ctx, rec, updates, preconditions...)
}
func (db *securedWriteDB) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return db.writeSession().UpdateMulti(ctx, keys, updates, preconditions...)
}
func (db *securedWriteDB) Delete(ctx context.Context, key *record.Key) error {
	return db.writeSession().Delete(ctx, key)
}
func (db *securedWriteDB) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	return db.writeSession().DeleteMulti(ctx, keys)
}

func enforcementUnsupported(operation Operations, explanation string) error {
	return &DeniedError{Decision: Decision{Operation: operation, Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeOperation, Explanation: explanation}}
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
