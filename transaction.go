package db

import "context"

var (
	transactionContextKey      = "transactionContextKey"
	nonTransactionalContextKey = "nonTransactionalContextKey"
)

// NewContextWithTransaction stores transaction and original context into a transactional context
func NewContextWithTransaction(ctx context.Context, tx interface{}) context.Context {
	ctx = context.WithValue(ctx, &nonTransactionalContextKey, ctx)
	return context.WithValue(ctx, &transactionContextKey, tx)
}

// GetTransaction returns original transaction object
func GetTransaction(ctx context.Context) interface{} {
	return ctx.Value(&transactionContextKey)
}

// GetNonTransactionalContext returns non transaction context (e.g. parent of transactional context)
// TODO: This is can be dangerous if child context creates a new context with a deadline for example
func GetNonTransactionalContext(ctx context.Context) context.Context {
	return ctx.Value(&nonTransactionalContextKey).(context.Context)
}

type TransactionOptions interface {
	IsReadonly() bool
	IsCrossGroup() bool
	Password() string
}

type TransactionOption func(options *transactionOptions)

type transactionOptions struct {
	isReadonly   bool
	isCrossGroup bool
	password     string
}

var _ TransactionOptions = (*transactionOptions)(nil)

func (v transactionOptions) IsReadonly() bool {
	return v.isReadonly
}

func (v transactionOptions) IsCrossGroup() bool {
	return v.isCrossGroup
}

// Password // TODO: why we need it?
func (v transactionOptions) Password() string {
	return v.password
}

func NewTransactionOptions(opts ...TransactionOption) TransactionOptions {
	options := transactionOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

func WithReadonly() TransactionOption {
	return func(options *transactionOptions) {
		options.isReadonly = true
	}
}

func WithCrossGroup() TransactionOption {
	return func(options *transactionOptions) {
		options.isCrossGroup = true
	}
}

func WithPassword(password string) TransactionOption {
	return func(options *transactionOptions) {
		options.password = password
	}
}
