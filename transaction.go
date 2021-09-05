package db

type TransactionOptions interface {
	IsReadonly() bool
	Password() string
}

type TransactionOption func(options *transactionOptions)

type transactionOptions struct {
	isReadonly bool
	password   string
}

var _ TransactionOptions = (*transactionOptions)(nil)

func (v transactionOptions) IsReadonly() bool {
	return v.isReadonly
}

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

func WithPassword(password string) TransactionOption {
	return func(options *transactionOptions) {
		options.password = password
	}
}
