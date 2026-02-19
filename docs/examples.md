# Complete Examples

This document provides end-to-end examples of using DALgo in real applications.

## Table of Contents

- [Basic CRUD Application](#basic-crud-application)
- [Blog with Comments](#blog-with-comments)
- [E-Commerce Orders](#e-commerce-orders)
- [Multi-Tenant Application](#multi-tenant-application)
- [Quick Reference](#quick-reference)

---

## Basic CRUD Application

A simple user management system demonstrating create, read, update, and delete operations.

### Setup

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "reflect"
    "strings"
    "time"
    
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo/update"
)

type User struct {
    ID        string
    Email     string
    Name      string
    Age       int
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (u *User) Validate() error {
    if strings.TrimSpace(u.Email) == "" {
        return errors.New("email is required")
    }
    if !strings.Contains(u.Email, "@") {
        return errors.New("invalid email format")
    }
    if strings.TrimSpace(u.Name) == "" {
        return errors.New("name is required")
    }
    if u.Age < 0 || u.Age > 150 {
        return errors.New("age must be between 0 and 150")
    }
    return nil
}
```

### Create

```go
func CreateUser(ctx context.Context, db dal.DB, user *User) error {
    if err := user.Validate(); err != nil {
        return fmt.Errorf("invalid user: %w", err)
    }
    
    now := time.Now()
    user.CreatedAt = now
    user.UpdatedAt = now
    
    key := dal.NewKeyWithID("users", user.ID)
    record := dal.NewRecordWithData(key, user)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Insert(ctx, record)
    })
}
```

### Read

```go
func GetUser(ctx context.Context, db dal.DB, userID string) (*User, error) {
    key := dal.NewKeyWithID("users", userID)
    user := &User{}
    record := dal.NewRecordWithData(key, user)
    
    err := db.Get(ctx, record)
    if dal.IsNotFound(err) {
        return nil, fmt.Errorf("user %s not found", userID)
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    return user, nil
}
```

### Update

```go
func UpdateUser(ctx context.Context, db dal.DB, user *User) error {
    if err := user.Validate(); err != nil {
        return fmt.Errorf("invalid user: %w", err)
    }
    
    user.UpdatedAt = time.Now()
    
    key := dal.NewKeyWithID("users", user.ID)
    record := dal.NewRecordWithData(key, user)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Set(ctx, record)
    })
}

// Partial update (only specific fields)
func UpdateUserEmail(ctx context.Context, db dal.DB, userID, newEmail string) error {
    key := dal.NewKeyWithID("users", userID)
    updates := []update.Update{
        update.ByFieldName("email", newEmail),
        update.ByFieldName("updated_at", time.Now()),
    }
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Update(ctx, key, updates, dal.WithExistsPrecondition())
    })
}
```

### Delete

```go
func DeleteUser(ctx context.Context, db dal.DB, userID string) error {
    key := dal.NewKeyWithID("users", userID)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Delete(ctx, key)
    })
}
```

### List/Query

```go
func ListUsers(ctx context.Context, db dal.DB, limit int) ([]*User, error) {
    query := dal.From(dal.CollectionRef{Name: "users"}).
        OrderBy(dal.Ascending("created_at")).
        Limit(limit).
        SelectIntoRecord(func() dal.Record {
            return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
        })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to execute query: %w", err)
    }
    defer reader.Close()
    
    users := make([]*User, 0, limit)
    for {
        record, err := reader.Next()
        if err == dal.ErrNoMoreRecords {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("failed to read record: %w", err)
        }
        
        user := record.Data().(*User)
        users = append(users, user)
    }
    
    return users, nil
}

func FindUserByEmail(ctx context.Context, db dal.DB, email string) (*User, error) {
    query := dal.From(dal.CollectionRef{Name: "users"}).
        WhereField("email", dal.Equal, email).
        Limit(1).
        SelectIntoRecord(func() dal.Record {
            return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
        })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to execute query: %w", err)
    }
    defer reader.Close()
    
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        return nil, fmt.Errorf("user with email %s not found", email)
    }
    if err != nil {
        return nil, fmt.Errorf("failed to read record: %w", err)
    }
    
    return record.Data().(*User), nil
}
```

---

## Blog with Comments

Hierarchical data with parent-child relationships.

### Models

```go
type Post struct {
    ID        string
    UserID    string
    Title     string
    Content   string
    Published bool
    CreatedAt time.Time
}

type Comment struct {
    ID        string
    PostID    string
    UserID    string
    Content   string
    CreatedAt time.Time
}
```

### Create Post with Comments

```go
func CreatePost(ctx context.Context, db dal.DB, post *Post) error {
    post.CreatedAt = time.Now()
    
    key := dal.NewKeyWithID("posts", post.ID)
    record := dal.NewRecordWithData(key, post)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Insert(ctx, record)
    })
}

func AddComment(ctx context.Context, db dal.DB, postID string, comment *Comment) error {
    comment.PostID = postID
    comment.CreatedAt = time.Now()
    
    // Hierarchical key: posts/{postID}/comments/{commentID}
    postKey := dal.NewKeyWithID("posts", postID)
    commentKey := dal.NewKeyWithParentAndID(postKey, "comments", comment.ID)
    record := dal.NewRecordWithData(commentKey, comment)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Verify post exists
        exists, err := tx.Exists(ctx, postKey)
        if err != nil {
            return err
        }
        if !exists {
            return fmt.Errorf("post %s not found", postID)
        }
        
        return tx.Insert(ctx, record)
    })
}
```

### Query Comments for Post

```go
func GetPostComments(ctx context.Context, db dal.DB, postID string) ([]*Comment, error) {
    postKey := dal.NewKeyWithID("posts", postID)
    
    query := dal.From(dal.CollectionRef{
        Name:   "comments",
        Parent: postKey,
    }).
        OrderBy(dal.Ascending("created_at")).
        SelectIntoRecord(func() dal.Record {
            return dal.NewRecordWithIncompleteKey("comments", reflect.String, &Comment{})
        })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    comments := make([]*Comment, 0)
    for {
        record, err := reader.Next()
        if err == dal.ErrNoMoreRecords {
            break
        }
        if err != nil {
            return nil, err
        }
        comments = append(comments, record.Data().(*Comment))
    }
    
    return comments, nil
}
```

---

## E-Commerce Orders

Transactional order processing with stock management.

### Models

```go
type Product struct {
    ID    string
    Name  string
    Price float64
    Stock int
}

type Order struct {
    ID         string
    UserID     string
    Items      []OrderItem
    TotalPrice float64
    Status     string
    CreatedAt  time.Time
}

type OrderItem struct {
    ProductID string
    Quantity  int
    Price     float64
}
```

### Place Order (Atomic)

```go
func PlaceOrder(ctx context.Context, db dal.DB, order *Order) error {
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Calculate total and verify stock
        var total float64
        for _, item := range order.Items {
            productKey := dal.NewKeyWithID("products", item.ProductID)
            product := &Product{}
            productRecord := dal.NewRecordWithData(productKey, product)
            
            if err := tx.Get(ctx, productRecord); err != nil {
                return fmt.Errorf("product %s not found: %w", item.ProductID, err)
            }
            
            if product.Stock < item.Quantity {
                return fmt.Errorf("insufficient stock for product %s", item.ProductID)
            }
            
            // Update stock
            product.Stock -= item.Quantity
            if err := tx.Set(ctx, productRecord); err != nil {
                return err
            }
            
            // Calculate item price
            item.Price = product.Price
            total += product.Price * float64(item.Quantity)
        }
        
        // Create order
        order.TotalPrice = total
        order.Status = "pending"
        order.CreatedAt = time.Now()
        
        orderKey := dal.NewKeyWithID("orders", order.ID)
        orderRecord := dal.NewRecordWithData(orderKey, order)
        
        return tx.Insert(ctx, orderRecord)
    })
}
```

### Cancel Order (Restore Stock)

```go
func CancelOrder(ctx context.Context, db dal.DB, orderID string) error {
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Get order
        orderKey := dal.NewKeyWithID("orders", orderID)
        order := &Order{}
        orderRecord := dal.NewRecordWithData(orderKey, order)
        
        if err := tx.Get(ctx, orderRecord); err != nil {
            return fmt.Errorf("order not found: %w", err)
        }
        
        if order.Status != "pending" {
            return fmt.Errorf("cannot cancel order with status %s", order.Status)
        }
        
        // Restore stock for all items
        for _, item := range order.Items {
            productKey := dal.NewKeyWithID("products", item.ProductID)
            updates := []update.Update{
                update.ByFieldName("stock", update.Increment(item.Quantity)),
            }
            
            if err := tx.Update(ctx, productKey, updates); err != nil {
                return fmt.Errorf("failed to restore stock: %w", err)
            }
        }
        
        // Update order status
        order.Status = "cancelled"
        return tx.Set(ctx, orderRecord)
    })
}
```

---

## Multi-Tenant Application

SaaS application with tenant isolation using composite keys.

### Models with Tenant

```go
type TenantUser struct {
    TenantID  string
    UserID    string
    Name      string
    Email     string
    Role      string
    CreatedAt time.Time
}
```

### Composite Key Operations

```go
func CreateTenantUser(ctx context.Context, db dal.DB, user *TenantUser) error {
    // Create composite key
    fields := []dal.FieldVal{
        {Name: "tenant_id", Value: user.TenantID},
        {Name: "user_id", Value: user.UserID},
    }
    key := dal.NewKeyWithFields("tenant_users", fields...)
    
    user.CreatedAt = time.Now()
    record := dal.NewRecordWithData(key, user)
    
    return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        return tx.Insert(ctx, record)
    })
}

func GetTenantUser(ctx context.Context, db dal.DB, tenantID, userID string) (*TenantUser, error) {
    fields := []dal.FieldVal{
        {Name: "tenant_id", Value: tenantID},
        {Name: "user_id", Value: userID},
    }
    key := dal.NewKeyWithFields("tenant_users", fields...)
    
    user := &TenantUser{}
    record := dal.NewRecordWithData(key, user)
    
    err := db.Get(ctx, record)
    if dal.IsNotFound(err) {
        return nil, fmt.Errorf("user not found in tenant %s", tenantID)
    }
    if err != nil {
        return nil, err
    }
    
    return user, nil
}

func ListTenantUsers(ctx context.Context, db dal.DB, tenantID string) ([]*TenantUser, error) {
    query := dal.From(dal.CollectionRef{Name: "tenant_users"}).
        WhereField("tenant_id", dal.Equal, tenantID).
        OrderBy(dal.Ascending("created_at")).
        SelectIntoRecord(func() dal.Record {
            return dal.NewRecordWithIncompleteKey("tenant_users", reflect.String, &TenantUser{})
        })
    
    reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    users := make([]*TenantUser, 0)
    for {
        record, err := reader.Next()
        if err == dal.ErrNoMoreRecords {
            break
        }
        if err != nil {
            return nil, err
        }
        users = append(users, record.Data().(*TenantUser))
    }
    
    return users, nil
}
```

---

## Quick Reference

### Creating Records

```go
// With complete key and data
key := dal.NewKeyWithID("users", "user123")
record := dal.NewRecordWithData(key, &User{})

// With incomplete key (for queries)
record := dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})

// Hierarchical key
parentKey := dal.NewKeyWithID("teams", "team456")
childKey := dal.NewKeyWithParentAndID(parentKey, "members", "member789")
```

### CRUD Operations

```go
// Create (insert)
tx.Insert(ctx, record)

// Read (get)
db.Get(ctx, record)

// Update (full record)
tx.Set(ctx, record)

// Update (partial)
updates := []update.Update{update.ByFieldName("email", "new@example.com")}
tx.Update(ctx, key, updates)

// Delete
tx.Delete(ctx, key)
```

### Queries

```go
// Basic query
query := dal.From(dal.CollectionRef{Name: "users"}).
    WhereField("email", dal.Equal, "alice@example.com").
    Limit(10).
    SelectIntoRecord(recordFactory)

// Execute query
reader, err := db.ExecuteQueryToRecordsReader(ctx, query)
defer reader.Close()

// Iterate results
for {
    record, err := reader.Next()
    if err == dal.ErrNoMoreRecords {
        break
    }
    // Process record
}
```

### Transactions

```go
// Read-write transaction
err := db.RunReadwriteTransaction(ctx, 
    func(ctx context.Context, tx dal.ReadwriteTransaction) error {
        // Operations here
        return nil
    },
)

// Read-only transaction
err := db.RunReadonlyTransaction(ctx,
    func(ctx context.Context, tx dal.ReadTransaction) error {
        // Read operations only
        return nil
    },
)
```

---

## Next Steps

- Review [Core Interfaces](interfaces.md) for comprehensive API reference
- Study [Transactions](transactions.md) for advanced patterns
- Read [Query Building](queries.md) for complex queries
- Check [Error Handling](errors.md) for robust error management

