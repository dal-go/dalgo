package db

import (
	"context"
	"math/rand"
	"time"
)

// TypeOfID represents type of ID: IsComplexID, IsStringID, IsIntID
type TypeOfID int

const (
	// IsComplexID is not implemented yet
	IsComplexID = iota

	// IsStringID for strings IDs
	IsStringID

	// IsIntID for integer IDs
	IsIntID
)

// EntityHolder is an interface a struct should satisfy to comply with "strongo/db" library
type EntityHolder interface {
	Kind() string
	TypeOfID() TypeOfID
	IntOrStrIdentifier
	Entity() interface{}
	NewEntity() interface{}
	SetEntity(entity interface{})
	SetIntID(id int64)
	SetStrID(id string)
}

// MultiUpdater is an interface that describe DB provider that can update multiple entities at once (batch mode)
type MultiUpdater interface {
	UpdateMulti(c context.Context, entityHolders []EntityHolder) error
}

// MultiGetter is an interface that describe DB provider that can get multiple entities at once (batch mode)
type MultiGetter interface {
	GetMulti(c context.Context, entityHolders []EntityHolder) error
}

// Getter is an interface that describe DB provider that can get a single entity by key
type Getter interface {
	Get(c context.Context, entityHolder EntityHolder) error
}

// Inserter is an interface that describe DB provider that can insert a single entity with a specific or random ID
type Inserter interface {
	InsertWithRandomIntID(c context.Context, entityHolder EntityHolder) error
	InsertWithRandomStrID(c context.Context, entityHolder EntityHolder, idLength uint8, attempts int, prefix string) error
}

// Updater is an interface that describe DB provider that can update a single entity by key
type Updater interface {
	Update(c context.Context, entityHolder EntityHolder) error
}

// Deleter is an interface that describe DB provider that can delete a single entity by key
type Deleter interface {
	Delete(c context.Context, entityHolder EntityHolder) error
}

// RunOptions hold arbitrary parameters to be passed throw DAL
type RunOptions map[string]interface{}

// TransactionCoordinator provides methods to work with transactions
type TransactionCoordinator interface {
	RunInTransaction(c context.Context, f func(c context.Context) error, options RunOptions) (err error)
	IsInTransaction(c context.Context) bool
	NonTransactionalContext(tc context.Context) (c context.Context)
}

// Database is an interface that define a DB provider
type Database interface {
	TransactionCoordinator
	Inserter
	Getter
	Updater
	MultiGetter
	MultiUpdater
	Deleter
}

// IntIdentifier is satisfied by entities with integer ID
type IntIdentifier interface {
	IntID() int64
}

// StrIdentifier is satisfied by entities with string ID
type StrIdentifier interface {
	StrID() string
}

// IntOrStrIdentifier is satisfied by entities with both integer and string ID TODO: why we need this?
type IntOrStrIdentifier interface {
	IntIdentifier
	StrIdentifier
}

var (
	// CrossGroupTransaction is an options that tells DB that multiple entity groups are affected, see Google Datastore
	CrossGroupTransaction = RunOptions{"XG": true}

	// SingleGroupTransaction specifies that only single entity group is affected, see Google Datastore
	SingleGroupTransaction = RunOptions{}
)

const idChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // Removed 1, I and 0, O as can be messed with l/1 and 0.

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

// RandomStringID creates a random string ID of requested length
var RandomStringID = func(n uint8) string {
	b := make([]byte, n)
	lettersCount := len(idChars)
	for i := range b {
		b[i] = idChars[random.Intn(lettersCount)]
	}
	return string(b)
}
