package dal

import "context"

// Getter defines a method to get a single record by key or check its existence
type Getter interface {

	// Get gets a single record from a database by key
	Get(ctx context.Context, record Record) error

	// Exists returns true if a record with the given key exists
	Exists(ctx context.Context, key *Key) (bool, error)
}

// MultiGetter defines method to get multiple records from a database by keys
type MultiGetter interface {

	// GetMulti gets multiple records from a database by keys
	GetMulti(ctx context.Context, records []Record) error
}
