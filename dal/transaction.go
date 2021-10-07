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
	// from updating or inserting rows into the dataset until the transaction is complete.
	TxSerializable

	// TxSnapshot - Reduces blocking by storing a version of data that one application can read
	// while another is modifying the same data.
	// Indicates that from one transaction you cannot see changes made in other transactions,
	// even if you requery.
	TxSnapshot
)

// NewContextWithTransaction stores transaction and original context into a transactional context
func NewContextWithTransaction(nonTransactionalContext context.Context, tx Transaction) context.Context {
	nonTransactionalContext = context.WithValue(nonTransactionalContext, &nonTransactionalContextKey, nonTransactionalContext)
	return context.WithValue(nonTransactionalContext, &transactionContextKey, tx)
}

// GetTransaction returns original transaction object
func GetTransaction(ctx context.Context) Transaction {
	return ctx.Value(&transactionContextKey).(Transaction)
}

// GetNonTransactionalContext returns non transaction context (e.g. parent of transactional context)
// TODO: This is can be dangerous if child context creates a new context with a deadline for example
func GetNonTransactionalContext(ctx context.Context) context.Context {
	return ctx.Value(&nonTransactionalContextKey).(context.Context)
}

// TransactionOptions holds transaction settings
type TransactionOptions interface {

	// IsolationLevel indicates requested isolation level
	IsolationLevel() TxIsolationLevel

	// IsReadonly indicates if a readonly transaction required
	IsReadonly() bool

	// IsCrossGroup indicates if a cross-group transaction required
	IsCrossGroup() bool

	// Password() string - TODO: document why it was added
}

type txOption func(options *txOptions)

type txOptions struct {
	isolationLevel TxIsolationLevel
	isReadonly     bool
	isCrossGroup   bool
	password       string
}

var _ TransactionOptions = (*txOptions)(nil)

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

// Password // TODO: why we need it?
func (v txOptions) Password() string {
	return v.password
}

// NewTransactionOptions creates instance of TransactionOptions
func NewTransactionOptions(opts ...txOption) TransactionOptions {
	options := txOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// TxWithIsolationLevel requests transaction with required isolation level
func TxWithIsolationLevel(isolationLevel TxIsolationLevel) txOption {
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

// TxWithReadonly requests a readonly transaction
func TxWithReadonly() txOption {
	return func(options *txOptions) {
		options.isReadonly = true
	}
}

// TxWithCrossGroup requires transaction that spans multiple entity groups
func TxWithCrossGroup() txOption {
	return func(options *txOptions) {
		options.isCrossGroup = true
	}
}

//func WithPassword(password string) txOption {
//	return func(options *txOptions) {
//		options.password = password
//	}
//}
