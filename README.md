# DALgo - Database Abstraction Layer (DAL) in Go language

**Import**: [`github.com/strongo/dalgo`](https://github.com/strongo/dalgo)

[![Go Report Card](https://goreportcard.com/badge/github.com/strongo/dalgo)](https://goreportcard.com/report/github.com/strongo/dalgo)
[![GoDoc](https://godoc.org/github.com/strongo/dalgo?status.svg)](https://godoc.org/github.com/strongo/dalgo)

Using this module allows you:

1. To abstract work with data storage so underlying API can be swapped.

2. Write less code. Write more readable code.

3. Easily add logging & hooks for all DB operations.

4. Write unit tests for your business logic without dependency on specific API.

## Implementations for specific APIs

DALgo defines abstract interfaces and helpers methods to work with databases in abstract manner.

Here is modules that bridge DALgo to specific API:

- [`github.com/strongo/dalgo2firestore`](https://github.com/strongo/dalgo2firestore)
  for [Firestore](https://pkg.go.dev/cloud.google.com/go/firestore) - a NoSQL document database that lets you easily
  store, sync, and query data for your mobile and web apps - at global scale.

- [`github.com/strongo/dalgo2buntdb`](https://github.com/strongo/dalgo2buntdb)
  for [BuntDB](https://github.com/tidwall/buntdb) - an embeddable, in-memory key/value database for Go with custom
  indexing and geospatial support.

- [`github.com/strongo/dalgo2badger`](https://github.com/strongo/dalgo2badger)
  for [BadgerDB](https://github.com/dgraph-io/badger) - an embeddable, persistent and fast key-value (KV) database
  written in pure Go.

## Test coverage

The CI process for this package and for officially supported bridges runs unit tests
and [end-to-end](https://github.com/strongo/dalgo-end2end-tests) integration tests.

## DALgo interfaces

The main abstraction is though `dalgo.Record` interface :

	type Record interface {
      Key() *Key          // defines `table` name of the entity
      Data() interface{}  // value to be stored/retrieved (without ID)
      Validate() error    // validate record
      Error() error       // holds error for the record
      SetError(err error) // sets error relevant to specific record
      IsReceived() bool   // indicates if an attempt to retrieve a record has been peformed
      Exists() bool		// indicates if the record exists in DB
	}

All methods are working with the `Record` and use `context.Context`.

The `Database` interface defines an interface to a storage that should be implemented by a specific driver. This repo
contains implementation for Google AppEngine Datastore. Contributions for other engines are welcome. If the db driver
does not support some of the operations it must return `ErrNotSupported`.

	type Database interface {
		TransactionCoordinator
		Inserter
		Getter
		Updater
		MultiGetter
		MultiUpdater
	}

where for example the  `Getter` & `MultiGetter` interfaces defined as:

	type Getter interface {
		Get(c context.Context, record Record) error
	}

	type MultiGetter interface {
		GetMulti(c context.Context, records []Record) error
	}

Note that getters are populating records in place using target instance obtained via `Record.GetData()`.

Originally developed to support work with Google AppEngine Datastore and Firebase Firestore it takes into account its
specifics. This works well with other key-value storages as well. Also `dalgo` supports SQL databases.

## Used by

Next applications are using `dalgo` in production:

* https://sneat.app/
* https://debtstracker.io/ - an app and [Telegram bot](https://t.me/DebtsTrackerBot) to track your personal debts

## Frameworks that utilise this `strongo/db` package

* <a href="https://github.com/strongo/bots-framework">`strongo/bots-framework`</a> - framework to build chat bots in Go
  language.
