# Database Adapters

This document explains how to choose and implement database adapters for DALgo.

## Table of Contents

- [Available Adapters](#available-adapters)
- [Choosing an Adapter](#choosing-an-adapter)
- [Using Adapters](#using-adapters)
- [Implementing an Adapter](#implementing-an-adapter)
- [Adapter Patterns](#adapter-patterns)
- [Testing Adapters](#testing-adapters)

---

## Available Adapters

DALgo provides official adapters for various databases:

| Adapter | Database | Package | Use Case |
|---------|----------|---------|----------|
| **dalgo2firestore** | Google Cloud Firestore | `github.com/dal-go/dalgo2firestore` | Serverless NoSQL, real-time sync |
| **dalgo2datastore** | Google Cloud Datastore | `github.com/dal-go/dalgo2datastore` | Legacy Google NoSQL |
| **dalgo2sql** | SQL databases | `github.com/dal-go/dalgo2sql` | PostgreSQL, MySQL, SQLite, MSSQL |
| **dalgo2fs** | File system | Built-in: `./dalgo2fs` | Local file storage |
| **dalgo2files** | File system | `github.com/dal-go/dalgo2files` | Simple file storage |
| **dalgo2git** | Git repository | `github.com/dal-go/dalgo2git` | Version-controlled storage |
| **dalgo2badger** | BadgerDB | `github.com/dal-go/dalgo2badger` | Embedded KV store |
| **dalgo2buntdb** | BuntDB | `github.com/dal-go/dalgo2buntdb` | In-memory KV with persistence |

---

## Choosing an Adapter

### Decision Matrix

```mermaid
graph TD
    Start[Choose Database] --> Cloud{Cloud or Local?}
    
    Cloud --> GCP{Google Cloud?}
    GCP -->|Yes| Firestore[dalgo2firestore]
    GCP -->|Legacy| Datastore[dalgo2datastore]
    
    Cloud --> SQL1{SQL Database?}
    SQL1 -->|Yes| SQLCloud[dalgo2sql]
    
    Cloud --> Other[Other Cloud DB]
    Other --> Implement[Implement Custom Adapter]
    
    Local --> SQLLocal{SQL Database?}
    SQLLocal -->|Yes| SQL2[dalgo2sql]
    
    Local --> Embed{Embedded DB?}
    Embed -->|KV Store| Badger[dalgo2badger/buntdb]
    
    Local --> Files{File-based?}
    Files -->|Simple| FileSystem[dalgo2fs/dalgo2files]
    Files -->|Versioned| Git[dalgo2git]
```

### By Use Case

#### Serverless/Cloud Applications

```go
// Google Cloud: Firestore
import "github.com/dal-go/dalgo2firestore"

// Advantages:
// - No server management
// - Real-time updates
// - Automatic scaling
// - Good for mobile/web apps

// Trade-offs:
// - Query limitations
// - Cost per read/write
// - Vendor lock-in
```

#### Traditional Web Applications

```go
// SQL database (PostgreSQL, MySQL, etc.)
import "github.com/dal-go/dalgo2sql"

// Advantages:
// - Flexible queries
// - ACID guarantees
// - Mature ecosystem
// - Wide tool support

// Trade-offs:
// - Server management
// - Schema migrations
// - Scaling complexity
```

#### Embedded Applications

```go
// BadgerDB or BuntDB
import "github.com/dal-go/dalgo2badger"

// Advantages:
// - No external dependencies
// - Fast local access
// - Single binary deployment

// Trade-offs:
// - Limited to single node
// - Manual backups
// - Less tooling
```

#### Testing and Development

```go
// File system adapter
import "github.com/dal-go/dalgo/dalgo2fs"

// Advantages:
// - No setup required
// - Human-readable data
// - Easy debugging
// - Git-friendly

// Trade-offs:
// - Not for production
// - Limited performance
// - No concurrent access
```

---

## Using Adapters

### Firestore Adapter

```go
package main

import (
    "context"
    "cloud.google.com/go/firestore"
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo2firestore"
)

func main() {
    ctx := context.Background()
    
    // Create Firestore client
    projectID := "my-project"
    client, err := firestore.NewClient(ctx, projectID)
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // Create DALgo database
    db := dalgo2firestore.NewDatabase(client, dal.NewAdapter("firestore", "1.0"))
    
    // Use db for operations
    key := dal.NewKeyWithID("users", "user123")
    user := &User{Name: "Alice"}
    record := dal.NewRecordWithData(key, user)
    
    err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Set(ctx, record)
    })
}
```

### SQL Adapter

```go
package main

import (
    "context"
    "database/sql"
    _ "github.com/lib/pq" // PostgreSQL driver
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo2sql"
)

func main() {
    ctx := context.Background()
    
    // Open SQL connection
    db, err := sql.Open("postgres", "postgres://user:password@localhost/mydb?sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // Create schema mapper
    schema := dal.NewSchema(
        // KeyToFields: inject key ID into data
        func(key *dal.Key, data any) ([]dal.ExtraField, error) {
            return []dal.ExtraField{
                {Name: "id", Value: key.ID},
            }, nil
        },
        // DataToKey: extract key ID from data
        func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
            user := data.(*User)
            return dal.NewKeyWithID(incompleteKey.Collection(), user.ID), nil
        },
    )
    
    // Create DALgo database
    dalDB := dalgo2sql.NewDatabase(db, schema, dal.NewAdapter("postgres", "1.0"))
    
    // Use dalDB for operations
}
```

### File System Adapter

```go
package main

import (
    "context"
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo/dalgo2fs"
)

func main() {
    ctx := context.Background()
    
    // Create file system database
    db := dalgo2fs.NewDatabase("./data", dal.NewAdapter("fs", "1.0"))
    
    // Use db for operations
    key := dal.NewKeyWithID("users", "user123")
    user := &User{Name: "Alice"}
    record := dal.NewRecordWithData(key, user)
    
    err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Set(ctx, record)
    })
    
    // Data is stored in: ./data/users/user123.json
}
```

### BadgerDB Adapter

```go
package main

import (
    "context"
    "github.com/dgraph-io/badger/v3"
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo2badger"
)

func main() {
    ctx := context.Background()
    
    // Open BadgerDB
    opts := badger.DefaultOptions("./badger")
    bdb, err := badger.Open(opts)
    if err != nil {
        panic(err)
    }
    defer bdb.Close()
    
    // Create DALgo database
    db := dalgo2badger.NewDatabase(bdb, dal.NewAdapter("badger", "1.0"))
    
    // Use db for operations
}
```

---

## Implementing an Adapter

To implement a custom adapter, implement the core interfaces.

### Minimum Interface Requirements

```go
// 1. Implement DB interface
type MyDatabase struct {
    client MyDBClient
    adapter dal.Adapter
    schema dal.Schema
}

func (db *MyDatabase) ID() string {
    return "my-db-instance"
}

func (db *MyDatabase) Adapter() dal.Adapter {
    return db.adapter
}

func (db *MyDatabase) Schema() dal.Schema {
    return db.schema
}

// 2. Implement TransactionCoordinator
func (db *MyDatabase) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, opts ...dal.TransactionOption) error {
    tx := &myReadTransaction{client: db.client}
    return f(ctx, tx)
}

func (db *MyDatabase) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, opts ...dal.TransactionOption) error {
    tx := &myReadwriteTransaction{client: db.client}
    
    // Begin transaction
    if err := tx.begin(ctx); err != nil {
        return err
    }
    
    // Execute worker
    err := f(ctx, tx)
    
    // Commit or rollback
    if err != nil {
        if rbErr := tx.rollback(ctx); rbErr != nil {
            return dal.NewRollbackError(rbErr, err)
        }
        return err
    }
    
    return tx.commit(ctx)
}

// 3. Implement ReadSession (for direct DB operations)
func (db *MyDatabase) Get(ctx context.Context, record dal.Record) error {
    // Implement get logic
    key := record.Key()
    data, err := db.client.Get(ctx, key)
    if err != nil {
        if isNotFound(err) {
            return record.SetError(dal.ErrRecordNotFound)
        }
        return err
    }
    
    // Unmarshal data into record
    if err := unmarshal(data, record.Data()); err != nil {
        return err
    }
    
    record.SetError(dal.ErrNoError) // Mark as loaded
    return nil
}

func (db *MyDatabase) GetMulti(ctx context.Context, records []dal.Record) error {
    // Implement batch get
    for _, record := range records {
        if err := db.Get(ctx, record); err != nil {
            record.SetError(err)
        }
    }
    return nil
}

func (db *MyDatabase) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
    // Implement query execution
    return &myRecordsReader{}, nil
}

func (db *MyDatabase) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, opts ...recordset.Option) (dal.RecordsetReader, error) {
    return &myRecordsetReader{}, nil
}
```

### Implementing Transactions

```go
type myReadTransaction struct {
    client MyDBClient
    tx     MyDBTransaction
}

// Implement ReadSession methods
func (tx *myReadTransaction) Get(ctx context.Context, record dal.Record) error {
    // Use transaction handle
    return tx.client.Get(ctx, record.Key(), tx.tx)
}

// ... implement other ReadSession methods

type myReadwriteTransaction struct {
    myReadTransaction // Embed read transaction
}

func (tx *myReadwriteTransaction) ID() string {
    return tx.tx.ID()
}

// Implement WriteSession methods
func (tx *myReadwriteTransaction) Set(ctx context.Context, record dal.Record) error {
    key := record.Key()
    data := record.Data()
    
    // Serialize data
    bytes, err := marshal(data)
    if err != nil {
        return err
    }
    
    // Write to database
    return tx.client.Set(ctx, key, bytes, tx.tx)
}

func (tx *myReadwriteTransaction) Insert(ctx context.Context, record dal.Record, opts ...dal.InsertOption) error {
    // Check if exists
    exists, err := tx.Exists(ctx, record.Key())
    if err != nil {
        return err
    }
    if exists {
        return errors.New("record already exists")
    }
    
    // Insert
    return tx.Set(ctx, record)
}

func (tx *myReadwriteTransaction) Update(ctx context.Context, key *dal.Key, updates []update.Update, preconds ...dal.Precondition) error {
    // Implement partial update
    for _, upd := range updates {
        field := upd.FieldName()
        value := upd.Value()
        // Update field
        if err := tx.client.UpdateField(ctx, key, field, value, tx.tx); err != nil {
            return err
        }
    }
    return nil
}

func (tx *myReadwriteTransaction) Delete(ctx context.Context, key *dal.Key) error {
    return tx.client.Delete(ctx, key, tx.tx)
}

// ... implement other WriteSession methods

func (tx *myReadwriteTransaction) begin(ctx context.Context) error {
    var err error
    tx.tx, err = tx.client.BeginTransaction(ctx)
    return err
}

func (tx *myReadwriteTransaction) commit(ctx context.Context) error {
    return tx.tx.Commit(ctx)
}

func (tx *myReadwriteTransaction) rollback(ctx context.Context) error {
    return tx.tx.Rollback(ctx)
}
```

### Implementing Query Reader

```go
type myRecordsReader struct {
    rows    []MyDBRow
    current int
}

func (r *myRecordsReader) Next() (dal.Record, error) {
    if r.current >= len(r.rows) {
        return nil, dal.ErrNoMoreRecords
    }
    
    row := r.rows[r.current]
    r.current++
    
    // Create record from row
    key := rowToKey(row)
    data := rowToData(row)
    record := dal.NewRecordWithData(key, data)
    record.SetError(dal.ErrNoError)
    
    return record, nil
}

func (r *myRecordsReader) Cursor() (string, error) {
    if r.current == 0 {
        return "", nil
    }
    return fmt.Sprintf("cursor-%d", r.current), nil
}

func (r *myRecordsReader) Close() error {
    return nil
}
```

---

## Adapter Patterns

### Schema Mapping Pattern

For SQL databases, map keys to columns:

```go
func NewSQLSchema() dal.Schema {
    return dal.NewSchema(
        // KeyToFields: Add key as column
        func(key *dal.Key, data any) ([]dal.ExtraField, error) {
            fields := []dal.ExtraField{
                {Name: "id", Value: key.ID},
            }
            
            // Handle parent key for hierarchical data
            if parent := key.Parent(); parent != nil {
                fields = append(fields, dal.ExtraField{
                    Name:  "parent_id",
                    Value: parent.ID,
                })
            }
            
            return fields, nil
        },
        
        // DataToKey: Extract key from row
        func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
            // Use reflection or type assertion
            v := reflect.ValueOf(data).Elem()
            idField := v.FieldByName("ID")
            id := idField.Interface()
            
            key := dal.NewKeyWithID(incompleteKey.Collection(), id)
            
            // Handle parent if needed
            parentField := v.FieldByName("ParentID")
            if parentField.IsValid() && !parentField.IsNil() {
                parentID := parentField.Interface()
                parentKey := dal.NewKeyWithID("parent", parentID)
                key = dal.NewKeyWithParentAndID(parentKey, key.Collection(), id)
            }
            
            return key, nil
        },
    )
}
```

### Error Translation Pattern

Translate database-specific errors to DALgo errors:

```go
func translateError(err error) error {
    if err == nil {
        return nil
    }
    
    switch {
    case isNotFoundError(err):
        return dal.ErrRecordNotFound
    case isDuplicateKeyError(err):
        return errors.New("duplicate key")
    case isDeadlockError(err):
        return errors.New("transaction deadlock")
    default:
        return err
    }
}
```

### Connection Pooling Pattern

```go
type MyDatabase struct {
    pool ConnectionPool
    // ...
}

func (db *MyDatabase) Get(ctx context.Context, record dal.Record) error {
    conn, err := db.pool.Acquire(ctx)
    if err != nil {
        return err
    }
    defer db.pool.Release(conn)
    
    return conn.Get(ctx, record)
}
```

---

## Testing Adapters

### End-to-End Test Suite

DALgo provides an end-to-end test suite for adapters:

```go
// https://github.com/dal-go/dalgo-end2end-tests

package mydb_test

import (
    "testing"
    "github.com/dal-go/dalgo-end2end-tests"
    "github.com/dal-go/dalgo/dal"
)

func TestMyAdapter(t *testing.T) {
    db := setupTestDatabase(t)
    defer cleanupTestDatabase(t, db)
    
    // Run standard test suite
    dalgotest.TestDatabase(t, db)
}

func setupTestDatabase(t *testing.T) dal.DB {
    // Initialize your test database
    return mydb.NewDatabase(/* ... */)
}

func cleanupTestDatabase(t *testing.T, db dal.DB) {
    // Clean up test data
}
```

### Unit Testing Individual Methods

```go
func TestGet(t *testing.T) {
    db := &MyDatabase{client: mockClient}
    
    key := dal.NewKeyWithID("users", "user123")
    user := &User{}
    record := dal.NewRecordWithData(key, user)
    
    err := db.Get(context.Background(), record)
    if err != nil {
        t.Fatalf("Get failed: %v", err)
    }
    
    if !record.Exists() {
        t.Error("Record should exist")
    }
    
    if user.Name != "Expected Name" {
        t.Errorf("Unexpected name: %s", user.Name)
    }
}
```

---

## Next Steps

- See [Schema Handling](schema.md) for key-column mapping
- Read [Core Interfaces](interfaces.md) to understand requirements
- Check [Examples](examples.md) for complete adapter examples
- Review existing adapters for implementation patterns
