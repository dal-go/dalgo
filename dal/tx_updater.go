package dal

import (
	"context"
	"github.com/dal-go/dalgo/update"
)

// Updater defines a function to update a single record in database by key
type Updater interface {

	// Update updates a single record in a database by key
	Update(ctx context.Context, key *Key, updates []update.Update, preconditions ...Precondition) error

	// UpdateRecord updates a single record in a database.
	// For example, this is useful in case if we want to put the record.Data to memcache.
	// See https://github.com/dal-go/dalgo-memcache-appengine
	// A regular DB adapter should call update(record.Key()) inside this method.
	UpdateRecord(ctx context.Context, record Record, updates []update.Update, preconditions ...Precondition) error
}

// MultiUpdater defines a function to update multiples records in database by keys
type MultiUpdater interface {

	// UpdateMulti updates multiples records in database by keys
	UpdateMulti(ctx context.Context, keys []*Key, updates []update.Update, preconditions ...Precondition) error
}
