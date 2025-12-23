package dal

import "context"

// Setter defines a function to store a single record intoRecord database by key
type Setter interface {

	// Set stores a single record intoRecord database by key
	Set(ctx context.Context, record Record) error
}

// MultiSetter defines a function to store multiple records intoRecord database by keys
type MultiSetter interface {

	// SetMulti stores multiples records intoRecord database by keys
	SetMulti(ctx context.Context, records []Record) error
}
