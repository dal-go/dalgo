# Go package `strongo/db`
Database abstraction layer (DAL) in Go language

There are 2 purposes of the package:

1. To abstract work with data storage so underlying storage engine can be changed.
2. To allow write less code that is more readable

The main abstraction is done though interface `EntityHolder`:

	type EntityHolder interface {
		TypeOfID() TypeOfID
		Kind() string
		Entity() interface{}
		SetEntity(entity interface{})
		IntID() int64
		StrID() string
		SetIntID(id int64)
		SetStrID(id string)
	}

Almost all other interfaces/methods are working with the `EntityHolder`.

The `Database` interface defines an interface to a storage that should be implemented by a specific driver.
This repo contains implementation for Google AppEngine Datastore. Contributions for other engines are welcome.

	type Database interface {
		TransactionCoordinator
		Inserter
		Getter
		Updater
		MultiGetter
		MultiUpdater
	}

where for example the  `Getter` interface is defined as:


	type Getter interface {
		Get(c context.Context, entityHolder EntityHolder) error
	}

	type MultiGetter interface {
		GetMulti(c context.Context, entityHolders []EntityHolder) error
	}

Note that getters are populating entities in place through calling `SetEntity(entity interface{})` method.

Originally developed to support work with Google AppEngine Datastore it takes into account its specifics. This should work well for other key-value storages as well.

This package is used in production by:
* https://debtstracker.io/ - an app and [Telegram bot](https://t.me/DebtsTrackerBot) to track your personal debts
