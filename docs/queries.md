# Query Building

This document covers building and executing queries in DALgo.

## Table of Contents

- [Query Types](#query-types)
- [Query Builder API](#query-builder-api)
- [Structured Queries](#structured-queries)
- [Text Queries](#text-queries)
- [Query Execution](#query-execution)
- [Filtering](#filtering)
- [Ordering and Pagination](#ordering-and-pagination)
- [Query Patterns](#query-patterns)

---

## Query Types

DALgo supports two types of queries:

### Structured Queries

Built using the query builder API with type-safe operations:

```go
query := dal.From(dal.CollectionRef{Name: "users"}).
    WhereField("email", dal.Equal, "alice@example.com").
    OrderBy(dal.Ascending("created_at")).
    Limit(10).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })
```

### Text Queries

Raw SQL or query strings with arguments:

```go
query := dal.NewTextQuery(
    "SELECT * FROM users WHERE email = ? AND status = ?",
    "alice@example.com",
    "active",
)
```

---

## Query Builder API

The query builder provides a fluent API for constructing queries.

### Basic Structure

```go
// 1. Start with From() to specify the source
builder := dal.From(source)

// 2. Add conditions with Where() or WhereField()
builder = builder.WhereField("status", dal.Equal, "active")

// 3. Add ordering with OrderBy()
builder = builder.OrderBy(dal.Ascending("created_at"))

// 4. Add pagination with Limit() and Offset()
builder = builder.Limit(10).Offset(20)

// 5. Finalize with Select*()
query := builder.SelectIntoRecord(recordFactory)
```

### Creating a Query Builder

```go
import "github.com/dal-go/dalgo/dal"

// Method 1: Using NewQueryBuilder
builder := dal.NewQueryBuilder(dal.From(dal.CollectionRef{Name: "users"}))

// Method 2: Using From() directly (shorter)
builder := dal.From(dal.CollectionRef{Name: "users"})

// Method 3: Using collection group
builder := dal.From(dal.CollectionGroupRef{Name: "posts"})
```

---

## Structured Queries

### Query Sources

#### Collection Reference

Query a specific collection:

```go
source := dal.CollectionRef{Name: "users"}
query := dal.From(source).
    WhereField("status", dal.Equal, "active").
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })
```

#### Collection Group Reference

Query across all collections with the same name:

```go
// Query all "comments" collections across all parent posts
source := dal.CollectionGroupRef{Name: "comments"}
query := dal.From(source).
    WhereField("author_id", dal.Equal, "user123").
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("comments", reflect.String, &Comment{})
    })
```

#### Hierarchical Collections

Query a child collection under a specific parent:

```go
teamKey := dal.NewKeyWithID("teams", "engineering")
source := dal.CollectionRef{
    Name:   "members",
    Parent: teamKey,
}
query := dal.From(source).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("members", reflect.String, &Member{})
    })
```

### Select Modes

#### Select Into Records

Returns individual records with full data:

```go
query := dal.From(dal.CollectionRef{Name: "users"}).
    SelectIntoRecord(func() dal.Record {
        // This factory is called for each result row
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })

reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
defer reader.Close()

for {
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    user := record.Data().(*User)
    fmt.Printf("User: %s\n", user.Name)
}
```

#### Select Into Recordset

Returns columnar data (more efficient for analytics):

```go
import "github.com/dal-go/dalgo/recordset"

query := dal.From(dal.CollectionRef{Name: "users"}).
    SelectIntoRecordset()

reader, err := db.ExecuteQueryToRecordsetReader(ctx, query)
defer reader.Close()

for {
    row, rs, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    // Access columnar data
    name := rs.Column("name").String(row)
    email := rs.Column("email").String(row)
    fmt.Printf("%s: %s\n", name, email)
}
```

#### Select Keys Only

Returns only record keys (no data):

```go
query := dal.From(dal.CollectionRef{Name: "users"}).
    WhereField("inactive", dal.Equal, true).
    SelectKeysOnly(reflect.String)

reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
defer reader.Close()

keys := make([]*dal.Key, 0)
for {
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    keys = append(keys, record.Key())
}

// Use keys for batch deletion
err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.DeleteMulti(ctx, keys)
})
```

---

## Filtering

### Simple Conditions

```go
// Equal
builder.WhereField("status", dal.Equal, "active")

// Greater than
builder.WhereField("age", dal.GreaterThen, 18)

// Greater or equal
builder.WhereField("score", dal.GreaterOrEqual, 100)

// Less than
builder.WhereField("price", dal.LessThen, 50.0)

// Less or equal
builder.WhereField("priority", dal.LessOrEqual, 5)
```

### Operators

Available comparison operators:

```go
const (
    Equal          Operator = "=="
    GreaterThen    Operator = ">"
    GreaterOrEqual Operator = ">="
    LessThen       Operator = "<"
    LessOrEqual    Operator = "<="
    In             Operator = "In"
)
```

### Multiple Conditions

```go
// All conditions are ANDed together
query := dal.From(dal.CollectionRef{Name: "products"}).
    WhereField("category", dal.Equal, "electronics").
    WhereField("price", dal.LessThen, 500.0).
    WhereField("in_stock", dal.Equal, true).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("products", reflect.String, &Product{})
    })
```

### Using Where() with Conditions

For more complex conditions, use `Where()` with condition objects:

```go
import "github.com/dal-go/dalgo/dal"

// Create a condition
condition := dal.NewComparison(
    dal.Field("status"),
    dal.Equal,
    dal.Constant{Value: "active"},
)

query := dal.From(dal.CollectionRef{Name: "users"}).
    Where(condition).
    SelectIntoRecord(recordFactory)
```

### Field References

```go
// Compare two fields
condition := dal.NewComparison(
    dal.Field("updated_at"),
    dal.GreaterThen,
    dal.Field("created_at"),
)

query := dal.From(dal.CollectionRef{Name: "users"}).
    Where(condition).
    SelectIntoRecord(recordFactory)
```

### Array Field Queries

Check if a value exists in an array field:

```go
// Check if "tag1" is in the tags array
query := dal.From(dal.CollectionRef{Name: "posts"}).
    WhereInArrayField("tags", "tag1").
    SelectIntoRecord(recordFactory)
```

### Group Conditions (AND/OR)

```go
// AND group (default)
andCondition := dal.GroupCondition{
    Operator: dal.And,
    Conditions: []dal.Condition{
        dal.WhereField("age", dal.GreaterOrEqual, 18),
        dal.WhereField("verified", dal.Equal, true),
    },
}

// OR group
orCondition := dal.GroupCondition{
    Operator: dal.Or,
    Conditions: []dal.Condition{
        dal.WhereField("role", dal.Equal, "admin"),
        dal.WhereField("role", dal.Equal, "moderator"),
    },
}

query := dal.From(dal.CollectionRef{Name: "users"}).
    Where(andCondition).
    SelectIntoRecord(recordFactory)
```

---

## Ordering and Pagination

### Ordering Results

```go
// Single order
query := dal.From(dal.CollectionRef{Name: "users"}).
    OrderBy(dal.Ascending("name")).
    SelectIntoRecord(recordFactory)

// Multiple orders
query := dal.From(dal.CollectionRef{Name: "users"}).
    OrderBy(
        dal.Descending("priority"),
        dal.Ascending("name"),
    ).
    SelectIntoRecord(recordFactory)
```

### Creating Order Expressions

```go
// Ascending order
ascending := dal.Ascending("field_name")

// Descending order
descending := dal.Descending("field_name")

// Using field reference
fieldRef := dal.Field("created_at")
order := dal.OrderExpression{
    Expression: fieldRef,
    Descending: true,
}
```

### Limiting Results

```go
// Limit number of results
query := dal.From(dal.CollectionRef{Name: "users"}).
    Limit(10).
    SelectIntoRecord(recordFactory)
```

### Offset (Skip)

```go
// Skip first 20 results, then return next 10
query := dal.From(dal.CollectionRef{Name: "users"}).
    Offset(20).
    Limit(10).
    SelectIntoRecord(recordFactory)
```

### Cursor-Based Pagination

```go
// First page
query := dal.From(dal.CollectionRef{Name: "users"}).
    OrderBy(dal.Ascending("created_at")).
    Limit(10).
    SelectIntoRecord(recordFactory)

reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
defer reader.Close()

// Get cursor after reading results
cursor, err := reader.Cursor()

// Next page using cursor
nextQuery := dal.From(dal.CollectionRef{Name: "users"}).
    OrderBy(dal.Ascending("created_at")).
    Limit(10).
    StartFrom(dal.Cursor(cursor)).
    SelectIntoRecord(recordFactory)
```

---

## Query Execution

### Executing to Records Reader

Returns records one by one:

```go
query := dal.From(dal.CollectionRef{Name: "users"}).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })

// Execute query
reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
if err != nil {
    return fmt.Errorf("failed to execute query: %w", err)
}
defer reader.Close()

// Iterate through results
for {
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    if err != nil {
        return fmt.Errorf("failed to read record: %w", err)
    }
    
    user := record.Data().(*User)
    fmt.Printf("User: %s (%s)\n", user.Name, user.Email)
}
```

### Executing to Recordset Reader

Returns columnar data:

```go
import "github.com/dal-go/dalgo/recordset"

query := dal.From(dal.CollectionRef{Name: "users"}).
    SelectIntoRecordset(
        recordset.WithColumn("name", recordset.String),
        recordset.WithColumn("email", recordset.String),
        recordset.WithColumn("age", recordset.Int),
    )

reader, err := db.ExecuteQueryToRecordsetReader(ctx, query)
if err != nil {
    return err
}
defer reader.Close()

rs := reader.Recordset()

for {
    row, _, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    
    name := rs.Column("name").String(row)
    email := rs.Column("email").String(row)
    age := rs.Column("age").Int(row)
    
    fmt.Printf("%s (%d): %s\n", name, age, email)
}
```

### Reading All Results

Helper to read all results into a slice:

```go
query := dal.From(dal.CollectionRef{Name: "users"}).
    Limit(100).
    SelectIntoRecord(recordFactory)

reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
if err != nil {
    return err
}
defer reader.Close()

// Read all into slice
records, err := dal.ReadAll(reader)
if err != nil {
    return err
}

for _, record := range records {
    user := record.Data().(*User)
    // process user
}
```

---

## Query Patterns

### Finding a Single Record

```go
func FindUserByEmail(ctx context.Context, db dal.DB, email string) (*User, error) {
    query := dal.From(dal.CollectionRef{Name: "users"}).
        WhereField("email", dal.Equal, email).
        Limit(1).
        SelectIntoRecord(func() dal.Record {
            return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
        })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        return nil, dal.ErrRecordNotFound
    }
    if err != nil {
        return nil, err
    }
    
    return record.Data().(*User), nil
}
```

### Counting Records

```go
func CountActiveUsers(ctx context.Context, db dal.DB) (int, error) {
    query := dal.From(dal.CollectionRef{Name: "users"}).
        WhereField("status", dal.Equal, "active").
        SelectKeysOnly(reflect.String)
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return 0, err
    }
    defer reader.Close()
    
    count := 0
    for {
        _, err := reader.Next()
        if err == dal.ErrNoMoreRecords {
            break
        }
        if err != nil {
            return 0, err
        }
        count++
    }
    
    return count, nil
}
```

### Paginated Query

```go
type Page struct {
    Records    []dal.Record
    NextCursor string
    HasMore    bool
}

func ListUsers(ctx context.Context, db dal.DB, cursor string, pageSize int) (*Page, error) {
    builder := dal.From(dal.CollectionRef{Name: "users"}).
        OrderBy(dal.Ascending("name")).
        Limit(pageSize)
    
    if cursor != "" {
        builder = builder.StartFrom(dal.Cursor(cursor))
    }
    
    query := builder.SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    records := make([]dal.Record, 0, pageSize)
    for {
        record, err := reader.Next()
        if err == dal.ErrNoMoreRecords {
            break
        }
        if err != nil {
            return nil, err
        }
        records = append(records, record)
    }
    
    nextCursor := ""
    if len(records) == pageSize {
        nextCursor, _ = reader.Cursor()
    }
    
    return &Page{
        Records:    records,
        NextCursor: nextCursor,
        HasMore:    len(records) == pageSize,
    }, nil
}
```

### Complex Filtering

```go
func FindProducts(ctx context.Context, db dal.DB, filters ProductFilters) ([]dal.Record, error) {
    builder := dal.From(dal.CollectionRef{Name: "products"})
    
    // Apply optional filters
    if filters.Category != "" {
        builder = builder.WhereField("category", dal.Equal, filters.Category)
    }
    
    if filters.MinPrice > 0 {
        builder = builder.WhereField("price", dal.GreaterOrEqual, filters.MinPrice)
    }
    
    if filters.MaxPrice > 0 {
        builder = builder.WhereField("price", dal.LessOrEqual, filters.MaxPrice)
    }
    
    if filters.InStockOnly {
        builder = builder.WhereField("in_stock", dal.Equal, true)
    }
    
    // Apply ordering
    switch filters.SortBy {
    case "price":
        builder = builder.OrderBy(dal.Ascending("price"))
    case "name":
        builder = builder.OrderBy(dal.Ascending("name"))
    default:
        builder = builder.OrderBy(dal.Descending("created_at"))
    }
    
    // Apply pagination
    builder = builder.Limit(filters.PageSize)
    if filters.Offset > 0 {
        builder = builder.Offset(filters.Offset)
    }
    
    query := builder.SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("products", reflect.String, &Product{})
    })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    return dal.ReadAll(reader)
}
```

---

## Text Queries

For databases that support SQL or custom query languages:

```go
import "github.com/dal-go/dalgo/dal"

// Simple text query
query := dal.NewTextQuery(
    "SELECT * FROM users WHERE status = ?",
    "active",
)

// Multiple parameters
query := dal.NewTextQuery(
    "SELECT * FROM users WHERE email = ? AND age > ?",
    "alice@example.com",
    18,
)

// Execute text query
reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
```

Note: Text query support depends on the database adapter. Not all adapters support text queries.

---

## Next Steps

- See [Transactions](transactions.md) for query execution in transactions
- Read [Record Management](records.md) for working with query results
- Check [Schema Handling](schema.md) for collection/table mapping
- Review [Examples](examples.md) for complete query patterns
