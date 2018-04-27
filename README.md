# Go package `strongo/db`
Database abstraction layer (DAL) in Go language

[![Build Status](https://travis-ci.org/strongo/db.svg?branch=master)](https://travis-ci.org/strongo/db)
[![Go Report Card](https://goreportcard.com/badge/github.com/strongo/db)](https://goreportcard.com/report/github.com/strongo/db)
[![GoDoc](https://godoc.org/github.com/strongo/db?status.svg)](https://godoc.org/github.com/strongo/db)

There are 4 main purposes for the package:

1. To abstract work with data storage so underlying storage engine can be changed.

2. To allow write less code that is more readable.

3. To allow easialy add logging & hooks for `tx/insert/get/query/update/delete` operations across app. _Think about preventing updates made outside of transaction or logging automatically what properties have changed._

4. Allow to write unit tests without dependency on specific implementation. (_do no compile AppEngine packages for examples_)

The main abstraction is done though interface `EntityHolder`:

	type EntityHolder interface {
		TypeOfID() TypeOfID            // Either `string`, `int`, or `complex`
		Kind() string                  // Defines `table` name of the entity
		NewEntity() interface{}        // Used for `get` operations to create emtity to fill with values.
		Entity() interface{}           // Entity with fields to be stored to DB (without ID)
		SetEntity(entity interface{})  //
		IntID() int64                  // Returns ID for entities identified by integer value
		StrID() string                 // Returns ID for entities identified by string value
		SetIntID(id int64)             // Sets integer ID for entities identified by integer value
		SetStrID(id string)            // Sets string ID for entities identified by string value
	}

All methods are working with the `EntityHolder`.

The `Database` interface defines an interface to a storage that should be implemented by a specific driver.
This repo contains implementation for Google AppEngine Datastore. Contributions for other engines are welcome.
If the db driver does not support some of the operations it must return `ErrNotSupported`.

	type Database interface {
		TransactionCoordinator
		Inserter
		Getter
		Updater
		MultiGetter
		MultiUpdater
	}

where for example the  `Getter` & `MultiGetter` interfaces are defined as:


	type Getter interface {
		Get(c context.Context, entityHolder EntityHolder) error
	}

	type MultiGetter interface {
		GetMulti(c context.Context, entityHolders []EntityHolder) error
	}

Note that getters are populating entities in place through calling `SetEntity(entity interface{})` method.

Originally developed to support work with Google AppEngine Datastore it takes into account its specifics. This should work well for other key-value storages as well.

## Used by
This package is used in production by:
* https://debtstracker.io/ - an app and [Telegram bot](https://t.me/DebtsTrackerBot) to track your personal debts

## Frameworks that utilise this `strongo/db` package
* <a href="https://github.com/strongo/bots-framework">`strongo/bots-framework`</a> - framework to build chat bots in Go language.
