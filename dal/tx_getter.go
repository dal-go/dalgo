package dal

import "context"

// Getter defines method to get a single record by key
type Getter interface {

	// Get gets a single record from database by key
	Get(ctx context.Context, record Record) error
}

// MultiGetter defines method to get multiples records from database by keys
type MultiGetter interface {

	// GetMulti gets multiples records from database by keys
	GetMulti(ctx context.Context, records []Record) error
}
