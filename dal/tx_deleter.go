package dal

import "context"

// Deleter defines a function to delete a single record from database by key
type Deleter interface {

	// Delete deletes a single record from database by key
	Delete(ctx context.Context, key *Key) error
}

// MultiDeleter defines a function to delete multiple records from database by keys
type MultiDeleter interface {

	// DeleteMulti deletes multiple records from database by keys
	DeleteMulti(ctx context.Context, keys []*Key) error
}
