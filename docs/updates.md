# Update Operations

This document covers partial record updates using field paths in DALgo.

## Table of Contents

- [Update Basics](#update-basics)
- [Field Names vs Field Paths](#field-names-vs-field-paths)
- [Update Types](#update-types)
- [Preconditions](#preconditions)
- [Update Patterns](#update-patterns)
- [Best Practices](#best-practices)

---

## Update Basics

Updates allow modifying specific fields without reading the entire record.

### Why Use Updates?

```go
// ❌ Without updates: full read-modify-write
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    record := dal.NewRecordWithData(key, &User{})
    if err := tx.Get(ctx, record); err != nil {
        return err
    }
    user := record.Data().(*User)
    user.LastLogin = time.Now() // Only changing one field
    return tx.Set(ctx, record)  // But writes entire record
})

// ✅ With updates: partial update
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    updates := []update.Update{
        update.ByFieldName("last_login", time.Now()),
    }
    return tx.Update(ctx, key, updates) // Only updates one field
})
```

### Basic Update

```go
import "github.com/dal-go/dalgo/update"

key := dal.NewKeyWithID("users", "user123")
updates := []update.Update{
    update.ByFieldName("email", "newemail@example.com"),
    update.ByFieldName("updated_at", time.Now()),
}

err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Update(ctx, key, updates)
})
```

---

## Field Names vs Field Paths

### Simple Field Names

For top-level fields:

```go
type User struct {
    Name  string
    Email string
    Age   int
}

// Update simple fields
updates := []update.Update{
    update.ByFieldName("name", "Alice Smith"),
    update.ByFieldName("email", "alice@example.com"),
    update.ByFieldName("age", 30),
}
```

### Field Paths (Nested Fields)

For nested structures:

```go
type User struct {
    Name    string
    Email   string
    Address Address
}

type Address struct {
    Street string
    City   string
    State  string
    Zip    string
}

// Update nested field using dot notation
updates := []update.Update{
    update.ByFieldName("address.city", "San Francisco"),
    update.ByFieldName("address.state", "CA"),
}

// Alternative: using explicit field path
updates := []update.Update{
    update.ByFieldPath([]string{"address", "city"}, "San Francisco"),
    update.ByFieldPath([]string{"address", "state"}, "CA"),
}
```

### Array Elements

```go
type User struct {
    Tags []string
}

// Update array element (if supported by database)
updates := []update.Update{
    update.ByFieldPath([]string{"tags", "0"}, "new-tag"),
}
```

### Map Fields

```go
type User struct {
    Metadata map[string]string
}

// Update map value
updates := []update.Update{
    update.ByFieldPath([]string{"metadata", "lastSeen"}, "2024-01-15"),
}
```

---

## Update Types

### Set Field Value

```go
// Set field to a new value
updates := []update.Update{
    update.ByFieldName("status", "active"),
    update.ByFieldName("balance", 100.50),
    update.ByFieldName("verified", true),
}
```

### Delete Field

```go
// Delete/remove a field
updates := []update.Update{
    update.DeleteByFieldName("temporary_token"),
    update.DeleteByFieldPath("metadata", "cached_value"),
}
```

### Increment/Decrement

Note: Support depends on database adapter.

```go
// Increment a numeric field
updates := []update.Update{
    update.ByFieldName("login_count", update.Increment(1)),
    update.ByFieldName("balance", update.Increment(50.0)),
}

// Decrement
updates := []update.Update{
    update.ByFieldName("credits", update.Decrement(10)),
}
```

### Server Timestamp

Some databases support server-side timestamps:

```go
updates := []update.Update{
    update.ByFieldName("updated_at", update.ServerTimestamp),
}
```

### Array Operations

Note: Support depends on database adapter.

```go
// Append to array
updates := []update.Update{
    update.ByFieldName("tags", update.ArrayUnion("new-tag")),
}

// Remove from array
updates := []update.Update{
    update.ByFieldName("tags", update.ArrayRemove("old-tag")),
}
```

---

## Preconditions

Preconditions ensure updates only happen if certain conditions are met.

### Exists Precondition

```go
// Only update if record exists
key := dal.NewKeyWithID("users", "user123")
updates := []update.Update{
    update.ByFieldName("status", "deleted"),
}

err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
})
// Error if record doesn't exist
```

### Last Update Time Precondition

```go
// Only update if not modified since last read
lastUpdateTime := time.Now().Add(-1 * time.Hour)

err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Update(ctx, key, updates, 
        dal.WithLastUpdateTimePrecondition(lastUpdateTime),
    )
})
// Error if record was modified after lastUpdateTime
```

### Combined Preconditions

```go
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Update(ctx, key, updates,
        dal.WithExistsPrecondition(),
        dal.WithLastUpdateTimePrecondition(timestamp),
    )
})
```

---

## Update Patterns

### Atomic Counter

```go
func IncrementLoginCount(ctx context.Context, db dal.DB, userID string) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("login_count", update.Increment(1)),
        update.ByFieldName("last_login", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
    })
}
```

### Conditional Update

```go
func ActivateUser(ctx context.Context, db dal.DB, userID string, lastSeenTime time.Time) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("status", "active"),
        update.ByFieldName("activated_at", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Only update if not modified since last read
        return tx.Update(ctx, key, updates,
            dal.WithExistsPrecondition(),
            dal.WithLastUpdateTimePrecondition(lastSeenTime),
        )
    })
}
```

### Batch Updates

```go
func UpdateMultipleUsers(ctx context.Context, db dal.DB, userIDs []string, status string) error {
    updates := []update.Update{
        update.ByFieldName("status", status),
        update.ByFieldName("updated_at", time.Now()),
    }
    
    keys := make([]*dal.Key, len(userIDs))
    for i, id := range userIDs {
        keys[i] = dal.NewKeyWithID("users", id)
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.UpdateMulti(ctx, keys, updates)
    })
}
```

### Update with Record

```go
// Some adapters support UpdateRecord for caching strategies
func UpdateUserEmail(ctx context.Context, db dal.DB, userID string, newEmail string) error {
    key := dal.NewKeyWithID("users", userID)
    record := dal.NewRecordWithData(key, &User{})
    
    // Get current data (e.g., for cache)
    if err := db.Get(ctx, record); err != nil {
        return err
    }
    
    updates := []update.Update{
        update.ByFieldName("email", newEmail),
        update.ByFieldName("email_updated_at", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // UpdateRecord allows adapter to update caches
        return tx.UpdateRecord(ctx, record, updates)
    })
}
```

### Soft Delete

```go
func SoftDeleteUser(ctx context.Context, db dal.DB, userID string) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("deleted", true),
        update.ByFieldName("deleted_at", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
    })
}
```

### Toggle Boolean

```go
func ToggleUserVerification(ctx context.Context, db dal.DB, userID string) error {
    key := dal.NewKeyWithID("users", userID)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Read current value
        record := dal.NewRecordWithData(key, &User{})
        if err := tx.Get(ctx, record); err != nil {
            return err
        }
        
        user := record.Data().(*User)
        
        // Update with toggled value
        updates := []update.Update{
            update.ByFieldName("verified", !user.Verified),
            update.ByFieldName("verified_at", time.Now()),
        }
        
        return tx.Update(ctx, key, updates)
    })
}
```

### Update Nested Object

```go
func UpdateAddress(ctx context.Context, db dal.DB, userID string, address Address) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("address", address), // Update entire nested object
        update.ByFieldName("address_updated_at", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates)
    })
}

// Or update individual nested fields
func UpdateCity(ctx context.Context, db dal.DB, userID string, city string) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("address.city", city), // Only update one nested field
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates)
    })
}
```

---

## Best Practices

### 1. Use Updates for Partial Changes

```go
// ✅ Good: Use update for single field change
updates := []update.Update{
    update.ByFieldName("last_seen", time.Now()),
}
tx.Update(ctx, key, updates)

// ❌ Bad: Full read-write for single field
record := dal.NewRecordWithData(key, &User{})
tx.Get(ctx, record)
user := record.Data().(*User)
user.LastSeen = time.Now()
tx.Set(ctx, record)
```

### 2. Validate Field Names

```go
// ✅ Good: Validate field names exist
const (
    FieldStatus    = "status"
    FieldUpdatedAt = "updated_at"
)

updates := []update.Update{
    update.ByFieldName(FieldStatus, "active"),
    update.ByFieldName(FieldUpdatedAt, time.Now()),
}

// ❌ Bad: Typos in field names fail silently
updates := []update.Update{
    update.ByFieldName("statsu", "active"), // Typo!
}
```

### 3. Use Preconditions for Safety

```go
// ✅ Good: Ensure record exists before update
tx.Update(ctx, key, updates, dal.WithExistsPrecondition())

// ❌ Bad: Update might succeed on non-existent record
tx.Update(ctx, key, updates) // Creates record if not exists (in some DBs)
```

### 4. Group Related Updates

```go
// ✅ Good: Update related fields together
updates := []update.Update{
    update.ByFieldName("email", newEmail),
    update.ByFieldName("email_verified", false),
    update.ByFieldName("email_updated_at", time.Now()),
}
tx.Update(ctx, key, updates)

// ❌ Bad: Multiple separate updates
tx.Update(ctx, key, []update.Update{update.ByFieldName("email", newEmail)})
tx.Update(ctx, key, []update.Update{update.ByFieldName("email_verified", false)})
tx.Update(ctx, key, []update.Update{update.ByFieldName("email_updated_at", time.Now())})
```

### 5. Handle Update Errors

```go
// ✅ Good: Handle precondition failures
err := tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
if err != nil {
    if dal.IsNotFound(err) {
        return errors.New("user not found")
    }
    if isPreconditionFailed(err) {
        return errors.New("record was modified by another transaction")
    }
    return err
}

// ❌ Bad: Generic error handling
err := tx.Update(ctx, key, updates)
if err != nil {
    return err // Don't know what went wrong
}
```

### 6. Document Field Paths

```go
// ✅ Good: Document nested field paths
// UpdateUserCity updates the city in the user's address.
// Field path: address.city
func UpdateUserCity(ctx context.Context, db dal.DB, userID, city string) error {
    updates := []update.Update{
        update.ByFieldName("address.city", city),
    }
    // ...
}
```

### 7. Test Updates

```go
func TestUpdateUser(t *testing.T) {
    // Create test user
    key := dal.NewKeyWithID("users", "test-user")
    user := &User{Name: "Alice", Email: "old@example.com"}
    record := dal.NewRecordWithData(key, user)
    db.Set(ctx, record)
    
    // Update email
    updates := []update.Update{
        update.ByFieldName("email", "new@example.com"),
    }
    err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates)
    })
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify update
    updatedRecord := dal.NewRecordWithData(key, &User{})
    db.Get(ctx, updatedRecord)
    updatedUser := updatedRecord.Data().(*User)
    
    if updatedUser.Email != "new@example.com" {
        t.Errorf("Email not updated: got %s", updatedUser.Email)
    }
    if updatedUser.Name != "Alice" {
        t.Errorf("Name should not change: got %s", updatedUser.Name)
    }
}
```

---

## Next Steps

- See [Transactions](transactions.md) for update transaction patterns
- Read [Record Management](records.md) for full record operations
- Check [Error Handling](errors.md) for update error patterns
- Review [Examples](examples.md) for complete update examples
