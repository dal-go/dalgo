package dal

import (
	"context"
	"fmt"
)

var (
	transactionContextKey      = "transactionContextKey"
	nonTransactionalContextKey = "nonTransactionalContextKey"
)

// TxIsolationLevel defines an isolation level for a transaction
type TxIsolationLevel int

const (
	// TxUnspecified indicates transaction level is not specified
	TxUnspecified TxIsolationLevel = iota

	// TxChaos - The pending changes from more highly isolated transactions cannot be overwritten.
	TxChaos

	// TxReadCommitted - Shared locks are held while the data is being read to avoid dirty reads,
	// but the data can be changed before the end of the transaction,
	// resulting in non-repeatable reads or phantom data.
	TxReadCommitted

	// TxReadUncommitted - A dirty read is possible, meaning that no shared locks are issued
	// and no exclusive locks are honored.
	TxReadUncommitted

	// TxRepeatableRead - Locks are placed on all data that is used in a query,
	// preventing other users from updating the data.
	// Prevents non-repeatable reads but phantom rows are still possible.
	TxRepeatableRead

	// TxSerializable - A range lock is placed on the DataSet, preventing other users
	// from updating or inserting rows intoRecord the dataset until the transaction is complete.
	TxSerializable

	// TxSnapshot - Reduces blocking by storing a version of data that one application can read
	// while another is modifying the same data.
	// Indicates that from one transaction you cannot see changes made in other transactions,
	// even if you requery.
	TxSnapshot
)

// ROTxWorker defines a callback to be called to do work within a readonly transaction
type ROTxWorker = func(ctx context.Context, tx ReadTransaction) error

// RWTxWorker defines a callback to be called to do work within a readwrite transaction
type RWTxWorker = func(ctx context.Context, tx ReadwriteTransaction) error

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {

	// ReadTransactionCoordinator can start a readonly transaction
	ReadTransactionCoordinator

	// ReadwriteTransactionCoordinator can start a readwrite transaction
	ReadwriteTransactionCoordinator
}

// ReadTransactionCoordinator creates a readonly transaction
type ReadTransactionCoordinator interface {

	// RunReadonlyTransaction starts readonly transaction
	RunReadonlyTransaction(ctx context.Context, f ROTxWorker, options ...TransactionOption) error
}

// ReadwriteTransactionCoordinator creates a read-write transaction
type ReadwriteTransactionCoordinator interface {

	// RunReadwriteTransaction starts read-write transaction
	RunReadwriteTransaction(ctx context.Context, f RWTxWorker, options ...TransactionOption) error
}

// Transaction defines an instance of DALgo transaction
type Transaction interface {
	// Options indicates parameters that were requested at time of transaction creation.
	Options() TransactionOptions
}

// ReadTransaction defines an interface for a readonly transaction
type ReadTransaction interface {
	Transaction
	ReadSession
}

// ReadwriteTransaction defines an interface for a readwrite transaction
type ReadwriteTransaction interface {

	// ID returns a unique ID of a transaction if it is supported by the underlying DB client
	ID() string

	Transaction
	ReadwriteSession
}

// NewContextWithTransaction stores transaction and original context intoRecord a transactional context
func NewContextWithTransaction(nonTransactionalContext context.Context, tx Transaction) context.Context {
	nonTransactionalContext = context.WithValue(nonTransactionalContext, &nonTransactionalContextKey, nonTransactionalContext)
	return context.WithValue(nonTransactionalContext, &transactionContextKey, tx)
}

// GetTransaction returns original transaction object
func GetTransaction(ctx context.Context) Transaction {
	tx := ctx.Value(&transactionContextKey)
	if tx == nil {
		return nil
	}
	return tx.(Transaction)
}

// GetNonTransactionalContext returns non transaction context (e.g. Parent of transactional context)
// TODO: This is can be dangerous if child context creates a new context with a deadline for example
func GetNonTransactionalContext(ctx context.Context) context.Context {
	return ctx.Value(&nonTransactionalContextKey).(context.Context)
}

// TransactionOptions holds transaction settings
type TransactionOptions interface {

	// Name describes what will be done in transaction.
	// This is useful for mocking transaction in tests
	Name() string

	// IsolationLevel indicates requested isolation level
	IsolationLevel() TxIsolationLevel

	// IsReadonly indicates a readonly transaction
	IsReadonly() bool

	// IsCrossGroup indicates a cross-group transaction. Makes sense for Google App Engine.
	IsCrossGroup() bool

	// Attempts returns number of attempts to execute a transaction. This is used in Google Datastore for example.
	Attempts() int

	// Password() string - TODO: document why it was added
}

// TransactionOption defines contact for transaction option
type TransactionOption func(options *txOptions)

type txOptions struct {
	name           string
	isolationLevel TxIsolationLevel
	isReadonly     bool
	isCrossGroup   bool
	attempts       int
	password       string
}

var _ TransactionOptions = (*txOptions)(nil)

func (v txOptions) Name() string {
	return v.name
}

// IsReadonly indicates a readonly transaction was requested
func (v txOptions) IsReadonly() bool {
	return v.isReadonly
}

// IsolationLevel indicates what isolation level was requested for a transaction
func (v txOptions) IsolationLevel() TxIsolationLevel {
	return v.isolationLevel
}

// IsCrossGroup indicates a cross-group transaction was requested
func (v txOptions) IsCrossGroup() bool {
	return v.isCrossGroup
}

func (v txOptions) Attempts() int {
	return v.attempts
}

// Password // TODO: why we need it?
func (v txOptions) Password() string {
	return v.password
}

// NewTransactionOptions creates instance of TransactionOptions
func NewTransactionOptions(opts ...TransactionOption) TransactionOptions {
	options := txOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// TxWithIsolationLevel requests transaction with required isolation level
func TxWithIsolationLevel(isolationLevel TxIsolationLevel) TransactionOption {
	if isolationLevel == TxUnspecified {
		panic("isolationLevel == TxUnspecified")
	}
	return func(options *txOptions) {
		if options.isolationLevel != TxUnspecified {
			if options.isolationLevel == isolationLevel {
				panic(fmt.Sprintf("an attempt to set same isolation level twice: %v", isolationLevel))
			}
			panic("an attempt to request more then 1 isolation level")
		}
		options.isolationLevel = isolationLevel
	}
}

// TxWithName specifies number of attempts to execute a transaction
func TxWithName(name string) TransactionOption {
	return func(options *txOptions) {
		options.name = name
	}
}

// TxWithAttempts specifies number of attempts to execute a transaction
func TxWithAttempts(attempts int) TransactionOption {
	return func(options *txOptions) {
		options.attempts = attempts
	}
}

// TxWithReadonly requests a readonly transaction
func TxWithReadonly() TransactionOption {
	return func(options *txOptions) {
		options.isReadonly = true
	}
}

// TxWithCrossGroup requires transaction that spans multiple entity groups
func TxWithCrossGroup() TransactionOption {
	return func(options *txOptions) {
		options.isCrossGroup = true
	}
}

//func WithPassword(password string) TransactionOption {
//	return func(options *txOptions) {
//		options.password = password
//	}
//}
