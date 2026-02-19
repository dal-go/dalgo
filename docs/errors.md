# Error Handling

This document covers error types and error handling patterns in DALgo.

## Table of Contents

- [Error Types](#error-types)
- [Checking Errors](#checking-errors)
- [Error Patterns](#error-patterns)
- [Best Practices](#best-practices)

---

## Error Types

### Standard Errors

```go
import "github.com/dal-go/dalgo/dal"

// Record not found
dal.ErrRecordNotFound

// Operation not supported by adapter
dal.ErrNotSupported

// Operation not yet implemented
dal.ErrNotImplementedYet

// No more records (from query iterator)
dal.ErrNoMoreRecords

// Limit reached (query result limit)
dal.ErrLimitReached

// Internal marker (not an actual error)
dal.ErrNoError
```

### Checking for Not Found

```go
err := db.Get(ctx, record)
if dal.IsNotFound(err) {
    // Record doesn't exist
    fmt.Println("Record not found")
} else if err != nil {
    // Other error occurred
    return fmt.Errorf("failed to get record: %w", err)
}

// Alternative: check record state
if !record.Exists() {
    fmt.Println("Record doesn't exist")
}
```

### ErrNotFoundByKey

Error that includes the key that wasn't found:

```go
err := db.Get(ctx, record)
if notFoundErr, ok := err.(dal.ErrNotFoundByKey); ok {
    key := notFoundErr.Key()
    cause := notFoundErr.Cause()
    fmt.Printf("Key %s not found: %v\n", key, cause)
}
```

### ErrExceedsMaxNumberOfAttempts

Returned when ID generation exceeds retry attempts:

```go
err := db.Insert(ctx, record, dal.WithRandomStringID())
if errors.Is(err, dal.ErrExceedsMaxNumberOfAttempts) {
    // Failed to generate unique ID after max attempts
    return errors.New("unable to generate unique ID")
}
```

### ErrHookFailed

Returned when a hook fails:

```go
err := db.Set(ctx, record)
if errors.Is(err, dal.ErrHookFailed) {
    // Validation or hook failed
    return fmt.Errorf("data validation failed: %w", err)
}
```

---

## Checking Errors

### Using errors.Is()

```go
import "errors"

err := db.Get(ctx, record)

// Check for specific error
if errors.Is(err, dal.ErrRecordNotFound) {
    // Handle not found
}

if errors.Is(err, dal.ErrNotSupported) {
    // Operation not supported by this adapter
}
```

### Using errors.As()

```go
// Check and extract error details
var notFoundErr dal.ErrNotFoundByKey
if errors.As(err, &notFoundErr) {
    key := notFoundErr.Key()
    fmt.Printf("Not found: %s\n", key)
}
```

### Helper Function: IsNotFound

```go
// Convenience function
if dal.IsNotFound(err) {
    // Record not found
}

// Equivalent to:
if err != nil && errors.Is(err, dal.ErrRecordNotFound) {
    // Record not found
}
```

### Checking Record Errors

```go
// After Get operation
err := db.Get(ctx, record)

// Method 1: Check returned error
if err != nil && !dal.IsNotFound(err) {
    return err
}

// Method 2: Check record error
if err := record.Error(); err != nil {
    return err
}

// Method 3: Check existence
if !record.Exists() {
    fmt.Println("Record not found")
}
```

### Checking Multi-Operation Errors

```go
records := []dal.Record{record1, record2, record3}
err := db.GetMulti(ctx, records)

// Check overall operation error
if err != nil {
    return fmt.Errorf("GetMulti failed: %w", err)
}

// Check individual record errors
for i, record := range records {
    if err := record.Error(); err != nil {
        if dal.IsNotFound(err) {
            fmt.Printf("Record %d not found\n", i)
        } else {
            fmt.Printf("Record %d error: %v\n", i, err)
        }
    }
}

// Helper to check any error
if err := dal.AnyRecordWithError(records...); err != nil {
    return fmt.Errorf("some records have errors: %w", err)
}
```

---

## Error Patterns

### Pattern 1: Graceful Degradation

```go
func GetUserProfile(ctx context.Context, db dal.DB, userID string) (*Profile, error) {
    key := dal.NewKeyWithID("profiles", userID)
    profile := &Profile{}
    record := dal.NewRecordWithData(key, profile)
    
    err := db.Get(ctx, record)
    if dal.IsNotFound(err) {
        // Return default profile instead of error
        return &Profile{
            UserID: userID,
            Theme:  "default",
        }, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get profile: %w", err)
    }
    
    return profile, nil
}
```

### Pattern 2: Create If Not Exists

```go
func GetOrCreateUser(ctx context.Context, db dal.DB, userID string) (*User, error) {
    key := dal.NewKeyWithID("users", userID)
    user := &User{}
    record := dal.NewRecordWithData(key, user)
    
    err := db.Get(ctx, record)
    if dal.IsNotFound(err) {
        // Create new user
        user = &User{
            ID:        userID,
            CreatedAt: time.Now(),
        }
        record = dal.NewRecordWithData(key, user)
        
        err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
            return tx.Insert(ctx, record)
        })
        if err != nil {
            return nil, fmt.Errorf("failed to create user: %w", err)
        }
        
        return user, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    return user, nil
}
```

### Pattern 3: Retry on Transient Errors

```go
func RetryableOperation(ctx context.Context, db dal.DB, maxRetries int) error {
    var lastErr error
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
            // Operation that might fail due to contention
            return tx.Update(ctx, key, updates)
        })
        
        if err == nil {
            return nil
        }
        
        if !isTransientError(err) {
            return err // Not retryable
        }
        
        lastErr = err
        
        // Exponential backoff
        time.Sleep(time.Duration(attempt*100) * time.Millisecond)
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}

func isTransientError(err error) bool {
    // Check for contention, deadlock, timeout, etc.
    // Database-specific logic
    return false
}
```

### Pattern 4: Aggregate Errors

```go
func BatchCreate(ctx context.Context, db dal.DB, users []*User) error {
    records := make([]dal.Record, len(users))
    for i, user := range users {
        key := dal.NewKeyWithID("users", user.ID)
        records[i] = dal.NewRecordWithData(key, user)
    }
    
    err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.InsertMulti(ctx, records)
    })
    
    if err != nil {
        // Check individual record errors
        errs := make([]error, 0)
        for i, record := range records {
            if err := record.Error(); err != nil {
                errs = append(errs, fmt.Errorf("user %d (%s): %w", i, users[i].ID, err))
            }
        }
        
        if len(errs) > 0 {
            return fmt.Errorf("batch create failed: %w", errors.Join(errs...))
        }
    }
    
    return nil
}
```

### Pattern 5: Wrap Errors with Context

```go
func UpdateUserEmail(ctx context.Context, db dal.DB, userID, newEmail string) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("email", newEmail),
    }
    
    err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
    })
    
    if err != nil {
        if dal.IsNotFound(err) {
            return fmt.Errorf("user %s not found", userID)
        }
        return fmt.Errorf("failed to update email for user %s: %w", userID, err)
    }
    
    return nil
}
```

### Pattern 6: Transaction Rollback Handling

```go
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    // ... operations ...
    return someError
})

if err != nil {
    // Check if rollback also failed
    if rbErr, ok := err.(interface {
        OriginalError() error
        RollbackError() error
    }); ok {
        log.Printf("Transaction failed: %v", rbErr.OriginalError())
        log.Printf("Rollback also failed: %v", rbErr.RollbackError())
        return fmt.Errorf("transaction and rollback failed: %w", err)
    }
    
    return fmt.Errorf("transaction failed: %w", err)
}
```

---

## Best Practices

### 1. Always Check Errors

```go
// ✅ Good: Check every error
err := db.Get(ctx, record)
if err != nil && !dal.IsNotFound(err) {
    return fmt.Errorf("failed to get record: %w", err)
}

// ❌ Bad: Ignore errors
db.Get(ctx, record)
// What if Get failed?
```

### 2. Distinguish Not Found from Errors

```go
// ✅ Good: Handle not found separately
err := db.Get(ctx, record)
if dal.IsNotFound(err) {
    // Not found is expected in some cases
    return nil, nil
} else if err != nil {
    // Actual error occurred
    return nil, fmt.Errorf("failed to get record: %w", err)
}

// ❌ Bad: Treat not found as error
err := db.Get(ctx, record)
if err != nil {
    return nil, err // Fails when record simply doesn't exist
}
```

### 3. Wrap Errors with Context

```go
// ✅ Good: Provide context
err := db.Get(ctx, record)
if err != nil {
    return fmt.Errorf("failed to get user %s: %w", userID, err)
}

// ❌ Bad: Return raw error
err := db.Get(ctx, record)
if err != nil {
    return err // No context about what failed
}
```

### 4. Use errors.Is() Not ==

```go
// ✅ Good: Use errors.Is()
if errors.Is(err, dal.ErrRecordNotFound) {
    // Works even if err is wrapped
}

// ❌ Bad: Direct comparison
if err == dal.ErrRecordNotFound {
    // Fails if error is wrapped
}
```

### 5. Log Errors Appropriately

```go
// ✅ Good: Log with appropriate level
err := db.Get(ctx, record)
if dal.IsNotFound(err) {
    log.Debug("User not found: %s", userID) // Debug level
} else if err != nil {
    log.Error("Failed to get user %s: %v", userID, err) // Error level
    return err
}

// ❌ Bad: Log all errors as errors
err := db.Get(ctx, record)
if err != nil {
    log.Error("Error: %v", err) // Not found logged as error!
    return err
}
```

### 6. Don't Swallow Errors

```go
// ✅ Good: Propagate or handle errors
err := db.Get(ctx, record)
if err != nil && !dal.IsNotFound(err) {
    return fmt.Errorf("failed to get record: %w", err)
}

// ❌ Bad: Swallow errors
err := db.Get(ctx, record)
if err != nil {
    log.Printf("Error: %v", err)
    // Continue as if nothing happened
}
```

### 7. Check Precondition Failures

```go
// ✅ Good: Handle precondition failures
err := tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
if err != nil {
    if dal.IsNotFound(err) {
        return errors.New("record does not exist")
    }
    if isPreconditionFailed(err) {
        return errors.New("record was modified concurrently")
    }
    return err
}

// ❌ Bad: Generic error handling
err := tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
if err != nil {
    return err // Don't know why it failed
}
```

### 8. Test Error Paths

```go
func TestGetUser_NotFound(t *testing.T) {
    key := dal.NewKeyWithID("users", "nonexistent")
    record := dal.NewRecordWithData(key, &User{})
    
    err := db.Get(ctx, record)
    
    // Test not found is handled correctly
    if !dal.IsNotFound(err) {
        t.Errorf("Expected not found error, got: %v", err)
    }
    
    if record.Exists() {
        t.Error("Record should not exist")
    }
}

func TestGetUser_DatabaseError(t *testing.T) {
    // Simulate database error (using mock)
    mockDB := &MockDB{
        GetFunc: func(ctx context.Context, record dal.Record) error {
            return errors.New("database connection failed")
        },
    }
    
    key := dal.NewKeyWithID("users", "user123")
    record := dal.NewRecordWithData(key, &User{})
    
    err := mockDB.Get(ctx, record)
    
    // Test error is propagated correctly
    if err == nil {
        t.Error("Expected error, got nil")
    }
    if err.Error() != "database connection failed" {
        t.Errorf("Unexpected error: %v", err)
    }
}
```

---

## Next Steps

- See [Transactions](transactions.md) for transaction error handling
- Read [Record Management](records.md) for record state errors
- Check [Hooks and Validation](hooks.md) for validation errors
- Review [Examples](examples.md) for error handling patterns
