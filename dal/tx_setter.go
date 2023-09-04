package dal

import "context"

// Setter defines a function to store a single record into database by key
type Setter interface {

	// Set stores a single record into database by key
	Set(ctx context.Context, record Record) error
}

// MultiSetter defines a function to store multiple records into database by keys
type MultiSetter interface {

	// SetMulti stores multiples records into database by keys
	SetMulti(ctx context.Context, records []Record) error
}
