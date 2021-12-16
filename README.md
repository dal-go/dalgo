# DALgo - Database Abstraction Layer (DAL) in Go

**To use**: `go get `[`github.com/strongo/dalgo`](https://github.com/strongo/dalgo)

## Status

[![Vet, Test, Build](https://github.com/strongo/dalgo/actions/workflows/ci.yml/badge.svg)](https://github.com/strongo/dalgo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/strongo/dalgo)](https://goreportcard.com/report/github.com/strongo/dalgo)
[![GoDoc](https://godoc.org/github.com/strongo/dalgo?status.svg)](https://godoc.org/github.com/strongo/dalgo)
[![Sourcegraph](https://sourcegraph.com/github.com/strongo/dalgo/-/badge.svg)](https://sourcegraph.com/github.com/strongo/dalgo?badge)
[![Coverage Status](https://coveralls.io/repos/github/strongo/dalgo/badge.svg?branch=main)](https://coveralls.io/github/strongo/dalgo?branch=main)

## Summary

Using this module allows you:

1. To abstract work with data storage so underlying API can be swapped.

2. Write less code. Write more readable code.

3. Easily add logging & hooks for all DB operations.

4. Write unit tests for your business logic without dependency on specific API.

## Packages

- [`dal`](dal) - Database Abstraction Layer
- [`orm`](orm) - Object–relational mapping
- [`query`](query) - SQL like interface with support of WHERE conditions, GROUP BY, etc.

## DAL implementations for specific APIs

DALgo defines abstract interfaces and helpers methods to work with databases in abstract manner.

Here is modules that bridge DALgo to specific APIs:

- ### [**dalgo2sql**](https://github.com/strongo/dalgo2sql)
  for [database/sql](https://pkg.go.dev/database/sql) - a generic interface around SQL (or SQL-like) databases.

- ### [**dalgo2firestore**](https://github.com/strongo/dalgo2firestore)
  for [Firestore](https://pkg.go.dev/cloud.google.com/go/firestore) - a NoSQL document database that lets you easily
  store, sync, and query data for your mobile and web apps - at global scale.

- ### [**dalgo2buntdb**](https://github.com/strongo/dalgo2buntdb)
  for [BuntDB](https://github.com/tidwall/buntdb) - an embeddable, in-memory key/value database for Go with custom
  indexing and geospatial support.

- ### [**dalgo2badger**](https://github.com/strongo/dalgo2badger)
  for [BadgerDB](https://github.com/strongo/dalgo) - an embeddable, persistent and fast key-value (KV) database written
  in pure Go.

## Test coverage

The CI process for this package and for officially supported bridges runs unit tests
and [end-to-end](https://github.com/strongo/dalgo-end2end-tests) integration tests.

## DALgo interfaces

**Package**: `github.com/strongo/dalgo/dal`

The main abstraction is though `dalgo.Record` interface :

	type Record interface {
      Key() *Key          // defines `table` name of the entity
      Data() interface{}  // value to be stored/retrieved (without ID)
      Error() error       // holds error for the record
      SetError(err error) // sets error relevant to specific record
      Exists() bool		// indicates if the record exists in DB
	}

All methods are working with the `Record` and use `context.Context`.

The [`Database`](./dal/database.go) interface defines an interface to a storage that should be implemented by a specific
driver. This repo contains implementation for Google AppEngine Datastore. Contributions for other engines are welcome.
If the db driver does not support some operations it must return `dalgo.ErrNotSupported`.

	type Database interface {
		TransactionCoordinator
		ReadonlySession
	}

    // TransactionCoordinator provides methods to work with transactions
    type TransactionCoordinator interface {
    
        // RunReadonlyTransaction starts readonly transaction
        RunReadonlyTransaction(ctx context.Context, f ROTxWorker, options ...TransactionOption) error
    
        // RunReadwriteTransaction starts read-write transaction
        RunReadwriteTransaction(ctx context.Context, f RWTxWorker, options ...TransactionOption) error
    }

    // ReadonlySession defines methods that do not modify database
    type ReadonlySession interface {
    
        // Get gets a single record from database by key
        Get(ctx context.Context, record Record) error
    
        // GetMulti gets multiples records from database by keys
        GetMulti(ctx context.Context, records []Record) error
    
        // Select executes a data retrieval query
        Select(ctx context.Context, query Select) (Reader, error)
    }

    // ReadwriteSession defines methods that can modify database
    type ReadwriteSession interface {
        ReadonlySession
        writeOnlySession
    }
    
    type writeOnlySession interface {
    
        // Insert inserts a single record in database
        Insert(c context.Context, record Record, opts ...InsertOption) error
    
        // Set sets a single record in database by key
        Set(ctx context.Context, record Record) error
    
        // SetMulti sets multiples records in database by keys
        SetMulti(ctx context.Context, records []Record) error
    
        // Update updates a single record in database by key
        Update(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error
    
        // UpdateMulti updates multiples records in database by keys
        UpdateMulti(c context.Context, keys []*Key, updates []Update, preconditions ...Precondition) error
    
        // Delete deletes a single record from database by key
        Delete(ctx context.Context, key *Key) error
    
        // DeleteMulti deletes multiple records from database by keys
        DeleteMulti(ctx context.Context, keys []*Key) error
    }

Note that getters are populating records in place using target instance obtained via `Record.GetData()`.

Originally developed to support work with Google AppEngine Datastore and Firebase Firestore it takes into account its
specifics. This works well with other key-value storages as well. Also `dalgo` supports SQL databases.

## Frameworks that utilise DALgo

* <a href="https://github.com/strongo/bots-framework">`strongo/bots-framework`</a>

- framework to build chat-bots in Go language.
