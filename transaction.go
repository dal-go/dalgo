package dalgo

import "context"

var (
	transactionContextKey      = "transactionContextKey"
	nonTransactionalContextKey = "nonTransactionalContextKey"
)

// NewContextWithTransaction stores transaction and original context into a transactional context
func NewContextWithTransaction(nonTransactionalContext context.Context, tx Transaction) context.Context {
	nonTransactionalContext = context.WithValue(nonTransactionalContext, &nonTransactionalContextKey, nonTransactionalContext)
	return context.WithValue(nonTransactionalContext, &transactionContextKey, tx)
}

// GetTransaction returns original transaction object
func GetTransaction(ctx context.Context) Transaction {
	return ctx.Value(&transactionContextKey)
}

// GetNonTransactionalContext returns non transaction context (e.g. parent of transactional context)
// TODO: This is can be dangerous if child context creates a new context with a deadline for example
func GetNonTransactionalContext(ctx context.Context) context.Context {
	return ctx.Value(&nonTransactionalContextKey).(context.Context)
}

// TransactionOptions holds transaction settings
type TransactionOptions interface {
	// IsReadonly indicates if a readonly transaction required
	IsReadonly() bool

	// IsCrossGroup indicates if a cross-group transaction required
	IsCrossGroup() bool

	// Password() string - TODO: document why it was added
}

type txOption func(options *txOptions)

type txOptions struct {
	isReadonly   bool
	isCrossGroup bool
	password     string
}

var _ TransactionOptions = (*txOptions)(nil)

func (v txOptions) IsReadonly() bool {
	return v.isReadonly
}

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

func WithReadonly() txOption {
	return func(options *txOptions) {
		options.isReadonly = true
	}
}

func WithCrossGroup() txOption {
	return func(options *txOptions) {
		options.isCrossGroup = true
	}
}

//func WithPassword(password string) txOption {
//	return func(options *txOptions) {
//		options.password = password
//	}
//}
