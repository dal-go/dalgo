package dal

import "context"

// Updater defines a function to update a single record in database by key
type Updater interface {

	// Update updates a single record in database by key
	Update(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error
}

// MultiUpdater defines a function to update multiples records in database by keys
type MultiUpdater interface {

	// UpdateMulti updates multiples records in database by keys
	UpdateMulti(c context.Context, keys []*Key, updates []Update, preconditions ...Precondition) error
}
