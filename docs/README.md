# DALgo Documentation for AI Agents

**Version:** 1.0  
**Target Audience:** AI agents incorporating dalgo into their projects  
**Repository:** https://github.com/dal-go/dalgo

## Table of Contents

- [Overview](#overview)
- [Design Principles](#design-principles)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Documentation Structure](#documentation-structure)
- [When to Use DALgo](#when-to-use-dalgo)

---

## Overview

DALgo (Database Abstraction Layer in Go) provides a unified, database-agnostic API for 
working with multiple database types in Go. It abstracts the underlying database 
implementation, enabling you to:

- **Switch databases** without rewriting application logic
- **Write cleaner, more maintainable code** with consistent patterns
- **Support multiple databases** (SQL, NoSQL, key-value, file systems)
- **Test easily** by mocking the DAL interface
- **Add logging and hooks** for all database operations

### Key Features

- ✅ **100% test coverage** with rigorous quality assurance
- ✅ **Unified API** for relational and key-value databases
- ✅ **Transaction support** with configurable isolation levels
- ✅ **Query builder** with structured and text-based queries
- ✅ **Record management** with strongly typed helpers
- ✅ **Schema handling** for both SQL tables and NoSQL collections
- ✅ **Multiple adapters** (Firestore, SQL, file system, Git, BadgerDB, BuntDB)
- ✅ **Hook system** for validation, logging, and custom logic

---

## Design Principles

DALgo follows these core design principles:

### 1. **Database Agnosticism**

Write code once, run on any database. The `dal.DB` interface abstracts all 
database-specific details. Adapters handle translation to native APIs.

```go
// Works with Firestore, PostgreSQL, MySQL, file system, etc.
type DB interface {
    Get(ctx context.Context, record Record) error
    Set(ctx context.Context, record Record) error
    // ... other methods
}
```

### 2. **Immutability and Safety**

- Keys are immutable once created
- Records track their state (exists, changed, error)
- Type-safe operations prevent common errors

### 3. **Separation of Concerns**

- **`dal`** package: Core abstractions (DB, Record, Key, Query)
- **`orm`** package: Object-relational mapping helpers
- **`record`** package: Strongly typed record wrappers
- **`update`** package: Field update operations
- **`recordset`** package: Columnar data representation

### 4. **Composability**

Small, focused interfaces compose into larger capabilities:

```go
type ReadSession interface {
    Getter
    MultiGetter
    QueryExecutor
}

type WriteSession interface {
    Setter
    MultiSetter
    Deleter
    MultiDeleter
    Updater
    MultiUpdater
    Inserter
    MultiInserter
}
```

### 5. **Context-First**

All operations accept `context.Context` for cancellation, deadlines, and 
request-scoped values.

### 6. **Schema Flexibility**

Supports both:
- **Key-value stores** (key separate from data)
- **Relational databases** (key columns part of table)

The `Schema` interface bridges this gap.

---

## Quick Start

### Installation

```bash
go get github.com/dal-go/dalgo
```

### Basic Usage

```go
package main

import (
    "context"
    "github.com/dal-go/dalgo/dal"
)

// 1. Define your data structure
type User struct {
    Name  string
    Email string
}

// 2. Create a key and record
func main() {
    ctx := context.Background()
    
    // Create a key for the "users" collection with ID "user123"
    key := dal.NewKeyWithID("users", "user123")
    
    // Create a record with data
    user := &User{Name: "Alice", Email: "alice@example.com"}
    record := dal.NewRecordWithData(key, user)
    
    // 3. Use with any database adapter
    // db := ... (initialize your database adapter)
    
    // Save the record
    // err := db.Set(ctx, record)
    
    // Retrieve the record
    // err = db.Get(ctx, record)
    // if err != nil && !dal.IsNotFound(err) {
    //     // handle error
    // }
    // if record.Exists() {
    //     userData := record.Data().(*User)
    //     // use userData
    // }
}
```

---

## Core Concepts

### Records and Keys

Every database operation works with **Records**. A record consists of:
- **Key**: Identifies the record (collection + ID + optional parent)
- **Data**: The actual record content (usually a struct)
- **State**: Exists, error, changed flags

```go
// Simple key
key := dal.NewKeyWithID("users", "user123")

// Hierarchical key (parent/child relationship)
parentKey := dal.NewKeyWithID("teams", "team456")
childKey := dal.NewKeyWithParentAndID(parentKey, "members", "member789")

// Record with data
record := dal.NewRecordWithData(key, &User{Name: "Bob"})
```

### Sessions and Transactions

Operations can be executed:
- **Directly on DB**: For simple operations
- **In read-only transactions**: For consistent reads
- **In read-write transactions**: For atomic writes

```go
// Simple operation
err := db.Get(ctx, record)

// Read-only transaction
err = db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
    return tx.Get(ctx, record)
})

// Read-write transaction
err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    if err := tx.Get(ctx, record); err != nil {
        return err
    }
    // modify record.Data()
    return tx.Set(ctx, record)
})
```

### Queries

Build queries using the query builder API:

```go
// Query for users with a specific email
query := dal.From(dal.CollectionRef{Name: "users"}).
    WhereField("email", dal.Equal, "alice@example.com").
    Limit(10).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })

reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
defer reader.Close()

for {
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    // process record
}
```

---

## Documentation Structure

This documentation is organized into focused modules:

| Document | Description |
|----------|-------------|
| **[Core Interfaces](interfaces.md)** | The main DAL interfaces: DB, Record, Key, Transaction |
| **[Record Management](records.md)** | Working with records, keys, and strongly typed data |
| **[Query Building](queries.md)** | Building structured and text queries |
| **[Transactions](transactions.md)** | Transaction patterns and isolation levels |
| **[Database Adapters](adapters.md)** | How to choose and implement database adapters |
| **[Schema Handling](schema.md)** | Mapping keys to columns for SQL and NoSQL |
| **[ORM Package](orm.md)** | Object-relational mapping with field definitions |
| **[Update Operations](updates.md)** | Partial updates with field paths |
| **[Error Handling](errors.md)** | Error types and patterns |
| **[Hooks and Validation](hooks.md)** | Pre/post operation hooks and validation |
| **[Examples](examples.md)** | Complete working examples |

---

## When to Use DALgo

### ✅ Use DALgo When:

- Building applications that may need to support multiple databases
- You want database-agnostic business logic
- You need consistent patterns across different storage backends
- You want to easily test database code with mocks
- You need transaction support across different databases
- You want to add logging/monitoring hooks to all DB operations
- Working with hierarchical data (parent/child relationships)

### ❌ Consider Alternatives When:

- You need database-specific features not abstracted by DALgo
- You're building a small project with a single, fixed database
- Performance is critical and you need hand-tuned native queries
- You already have significant investment in a specific ORM (e.g., GORM)

---

## Next Steps

For AI agents incorporating DALgo:

1. **Start with [Core Interfaces](interfaces.md)** to understand the foundation
2. **Read [Record Management](records.md)** for working with data
3. **Study [Database Adapters](adapters.md)** to choose your backend
4. **Review [Examples](examples.md)** for complete patterns
5. **Reference [Query Building](queries.md)** when implementing search

Each document is self-contained but cross-references related concepts. The documentation
assumes familiarity with Go programming and basic database concepts.

---

## Contributing

DALgo is an open-source project. Contributions are welcome:

- **Report issues**: https://github.com/dal-go/dalgo/issues
- **Submit PRs**: Follow [CONTRIBUTING.md](../CONTRIBUTING.md)
- **Write adapters**: Implement adapters for new databases
- **Improve docs**: Help make documentation clearer

## License

DALgo is licensed under the terms specified in [LICENSE](../LICENSE).
