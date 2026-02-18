# ORM Package

This document covers the Object-Relational Mapping features in DALgo's ORM package.

## Table of Contents

- [Overview](#overview)
- [Field Definitions](#field-definitions)
- [Collection Definitions](#collection-definitions)
- [Type-Safe Queries](#type-safe-queries)
- [Best Practices](#best-practices)

---

## Overview

The `orm` package provides strongly-typed field definitions and collection references for building type-safe queries.

### Package Import

```go
import "github.com/dal-go/dalgo/orm"
```

### Why Use ORM?

```go
// Without ORM: string-based field names (error-prone)
query := dal.From(dal.CollectionRef{Name: "users"}).
    WhereField("eamil", dal.Equal, "alice@example.com") // Typo! "eamil" instead of "email"

// With ORM: compile-time safety
query := dal.From(Users.CollectionRef()).
    Where(Users.Fields.Email.EqualTo("alice@example.com")) // Typo caught at compile time
```

---

## Field Definitions

### Creating Field Definitions

```go
type FieldDefinition[T any] struct {
    name       string
    valueType  string
    isRequired bool
    defaultVal T
}

// Create a field
emailField := orm.NewField[string]("email")

// With options
emailField := orm.NewField[string]("email",
    orm.Required(),
    orm.WithDefault("unknown@example.com"),
)
```

### Field Types

```go
// String field
nameField := orm.NewField[string]("name")

// Integer field
ageField := orm.NewField[int]("age")

// Boolean field
verifiedField := orm.NewField[bool]("verified")

// Time field
createdAtField := orm.NewField[time.Time]("created_at")

// Custom type field
statusField := orm.NewField[UserStatus]("status")
```

### Field Options

```go
type FieldOption[T any] func(FieldDefinition[T]) FieldDefinition[T]

// Mark field as required
orm.Required()

// Set default value
orm.WithDefault(value)

// Example usage
emailField := orm.NewField[string]("email",
    orm.Required(),
)

ageField := orm.NewField[int]("age",
    orm.WithDefault(0),
)
```

### Field Methods

```go
// Get field name
name := emailField.Name() // "email"

// Get field type
fieldType := emailField.Type() // "string"

// Check if required
required := emailField.IsRequired() // true/false

// Get default value
defaultVal := ageField.DefaultValue() // 0
```

---

## Collection Definitions

### Basic Collection

```go
type UserFields struct {
    Name      orm.FieldDefinition[string]
    Email     orm.FieldDefinition[string]
    Age       orm.FieldDefinition[int]
    Verified  orm.FieldDefinition[bool]
    CreatedAt orm.FieldDefinition[time.Time]
}

type UserCollection struct {
    Fields UserFields
}

func (c UserCollection) CollectionRef() dal.CollectionRef {
    return dal.CollectionRef{Name: "users"}
}

// Global instance
var Users = UserCollection{
    Fields: UserFields{
        Name:      orm.NewField[string]("name", orm.Required()),
        Email:     orm.NewField[string]("email", orm.Required()),
        Age:       orm.NewField[int]("age"),
        Verified:  orm.NewField[bool]("verified"),
        CreatedAt: orm.NewField[time.Time]("created_at"),
    },
}
```

### Collection with Methods

```go
func (c UserCollection) Query() *dal.QueryBuilder {
    return dal.From(c.CollectionRef())
}

func (c UserCollection) NewKey(id string) *dal.Key {
    return dal.NewKeyWithID(c.CollectionRef().Name, id)
}

func (c UserCollection) NewRecord(id string, data *User) dal.Record {
    key := c.NewKey(id)
    return dal.NewRecordWithData(key, data)
}

// Usage
query := Users.Query().Where(Users.Fields.Email.EqualTo("alice@example.com"))
key := Users.NewKey("user123")
record := Users.NewRecord("user123", &User{})
```

### Nested Collections

```go
type TeamFields struct {
    Name      orm.FieldDefinition[string]
    CreatedAt orm.FieldDefinition[time.Time]
}

type MemberFields struct {
    Name  orm.FieldDefinition[string]
    Role  orm.FieldDefinition[string]
    Email orm.FieldDefinition[string]
}

type TeamCollection struct {
    Fields  TeamFields
    Members MemberCollection
}

type MemberCollection struct {
    Fields MemberFields
}

func (c TeamCollection) CollectionRef() dal.CollectionRef {
    return dal.CollectionRef{Name: "teams"}
}

func (c MemberCollection) CollectionRef(teamKey *dal.Key) dal.CollectionRef {
    return dal.CollectionRef{
        Name:   "members",
        Parent: teamKey,
    }
}

// Global instance
var Teams = TeamCollection{
    Fields: TeamFields{
        Name:      orm.NewField[string]("name"),
        CreatedAt: orm.NewField[time.Time]("created_at"),
    },
    Members: MemberCollection{
        Fields: MemberFields{
            Name:  orm.NewField[string]("name"),
            Role:  orm.NewField[string]("role"),
            Email: orm.NewField[string]("email"),
        },
    },
}

// Usage
teamKey := dal.NewKeyWithID("teams", "team123")
query := dal.From(Teams.Members.CollectionRef(teamKey)).
    Where(Teams.Members.Fields.Role.EqualTo("admin"))
```

---

## Type-Safe Queries

### Equal Comparison

```go
// Find users with specific email
query := Users.Query().
    Where(Users.Fields.Email.EqualTo("alice@example.com"))

// Multiple conditions
query := Users.Query().
    Where(Users.Fields.Email.EqualTo("alice@example.com")).
    Where(Users.Fields.Verified.EqualTo(true))
```

### CompareTo (Generic Comparisons)

```go
// Greater than
query := Users.Query().
    Where(Users.Fields.Age.CompareTo(dal.GreaterThen, dal.Constant{Value: 18}))

// Less than or equal
query := Users.Query().
    Where(Users.Fields.Age.CompareTo(dal.LessOrEqual, dal.Constant{Value: 65}))
```

### Complex Queries

```go
// Find active users over 18
query := Users.Query().
    Where(Users.Fields.Age.CompareTo(dal.GreaterOrEqual, dal.Constant{Value: 18})).
    Where(Users.Fields.Verified.EqualTo(true)).
    OrderBy(dal.Ascending(Users.Fields.CreatedAt.Name())).
    Limit(100).
    SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })
```

### Field References in Queries

```go
// Compare two fields
query := Users.Query().
    Where(Users.Fields.UpdatedAt.CompareTo(
        dal.GreaterThen,
        dal.Field(Users.Fields.CreatedAt.Name()),
    ))
```

---

## Best Practices

### 1. Define Collections as Globals

```go
// ✅ Good: Single global instance
var Users = UserCollection{
    Fields: UserFields{
        Email: orm.NewField[string]("email"),
        Name:  orm.NewField[string]("name"),
    },
}

// Usage across the codebase
query := Users.Query().Where(Users.Fields.Email.EqualTo(email))

// ❌ Bad: Creating new instances
func GetUser(email string) {
    users := UserCollection{...} // Don't do this
    query := users.Query()...
}
```

### 2. Use Consistent Field Names

```go
// ✅ Good: Field names match database columns
var Users = UserCollection{
    Fields: UserFields{
        Email:     orm.NewField[string]("email"),      // matches "email" column
        FirstName: orm.NewField[string]("first_name"), // matches "first_name" column
    },
}

// ❌ Bad: Mismatched names
var Users = UserCollection{
    Fields: UserFields{
        Email:     orm.NewField[string]("userEmail"),  // database has "email"
        FirstName: orm.NewField[string]("fname"),      // database has "first_name"
    },
}
```

### 3. Group Related Collections

```go
// ✅ Good: Organized structure
package schema

var (
    Users     UserCollection
    Posts     PostCollection
    Comments  CommentCollection
    Teams     TeamCollection
)

// ❌ Bad: Scattered definitions
package models
var Users = ...

package services
var Posts = ...
```

### 4. Add Helper Methods

```go
type UserCollection struct {
    Fields UserFields
}

func (c UserCollection) CollectionRef() dal.CollectionRef {
    return dal.CollectionRef{Name: "users"}
}

// ✅ Good: Helpful methods
func (c UserCollection) Query() *dal.QueryBuilder {
    return dal.From(c.CollectionRef())
}

func (c UserCollection) FindByEmail(email string) dal.StructuredQuery {
    return c.Query().
        Where(c.Fields.Email.EqualTo(email)).
        Limit(1).
        SelectIntoRecord(c.NewRecordFactory())
}

func (c UserCollection) FindVerifiedUsers() dal.StructuredQuery {
    return c.Query().
        Where(c.Fields.Verified.EqualTo(true)).
        SelectIntoRecord(c.NewRecordFactory())
}

func (c UserCollection) NewRecordFactory() func() dal.Record {
    return func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    }
}

// Usage
query := Users.FindByEmail("alice@example.com")
query := Users.FindVerifiedUsers()
```

### 5. Type-Safe Constants

```go
type UserStatus string

const (
    StatusActive   UserStatus = "active"
    StatusInactive UserStatus = "inactive"
    StatusBanned   UserStatus = "banned"
)

type UserFields struct {
    Status orm.FieldDefinition[UserStatus]
}

// Usage with type safety
query := Users.Query().
    Where(Users.Fields.Status.EqualTo(StatusActive)) // Compile-time checked
```

### 6. Document Field Purpose

```go
type UserFields struct {
    // Email is the user's primary email address (required, unique)
    Email orm.FieldDefinition[string]
    
    // EmailVerified indicates if the email has been verified via confirmation link
    EmailVerified orm.FieldDefinition[bool]
    
    // CreatedAt is the timestamp when the user account was created (auto-set)
    CreatedAt orm.FieldDefinition[time.Time]
    
    // LastLogin is the timestamp of the user's most recent login (nullable)
    LastLogin orm.FieldDefinition[*time.Time]
}
```

### 7. Validate Field Definitions

```go
func init() {
    // Validate all fields are defined
    if Users.Fields.Email.Name() == "" {
        panic("Users.Fields.Email not initialized")
    }
    if Users.Fields.Name.Name() == "" {
        panic("Users.Fields.Name not initialized")
    }
}
```

### 8. Query Builder Pattern

```go
type UserQueryBuilder struct {
    *dal.QueryBuilder
    collection UserCollection
}

func (c UserCollection) NewQueryBuilder() *UserQueryBuilder {
    return &UserQueryBuilder{
        QueryBuilder: dal.From(c.CollectionRef()),
        collection:   c,
    }
}

func (qb *UserQueryBuilder) WithEmail(email string) *UserQueryBuilder {
    qb.QueryBuilder = qb.Where(qb.collection.Fields.Email.EqualTo(email))
    return qb
}

func (qb *UserQueryBuilder) OnlyVerified() *UserQueryBuilder {
    qb.QueryBuilder = qb.Where(qb.collection.Fields.Verified.EqualTo(true))
    return qb
}

func (qb *UserQueryBuilder) Build() dal.StructuredQuery {
    return qb.SelectIntoRecord(func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    })
}

// Usage
query := Users.NewQueryBuilder().
    WithEmail("alice@example.com").
    OnlyVerified().
    Build()
```

---

## Complete Example

```go
package schema

import (
    "reflect"
    "time"
    
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo/orm"
)

// Define field structures
type UserFields struct {
    Email     orm.FieldDefinition[string]
    Name      orm.FieldDefinition[string]
    Age       orm.FieldDefinition[int]
    Verified  orm.FieldDefinition[bool]
    CreatedAt orm.FieldDefinition[time.Time]
    UpdatedAt orm.FieldDefinition[time.Time]
}

// Define collection
type UserCollection struct {
    Fields UserFields
}

func (c UserCollection) CollectionRef() dal.CollectionRef {
    return dal.CollectionRef{Name: "users"}
}

func (c UserCollection) Query() *dal.QueryBuilder {
    return dal.From(c.CollectionRef())
}

func (c UserCollection) FindByEmail(email string) dal.StructuredQuery {
    return c.Query().
        Where(c.Fields.Email.EqualTo(email)).
        Limit(1).
        SelectIntoRecord(c.recordFactory())
}

func (c UserCollection) FindVerifiedAdults() dal.StructuredQuery {
    return c.Query().
        Where(c.Fields.Verified.EqualTo(true)).
        Where(c.Fields.Age.CompareTo(dal.GreaterOrEqual, dal.Constant{Value: 18})).
        OrderBy(dal.Ascending(c.Fields.CreatedAt.Name())).
        SelectIntoRecord(c.recordFactory())
}

func (c UserCollection) recordFactory() func() dal.Record {
    return func() dal.Record {
        return dal.NewRecordWithIncompleteKey("users", reflect.String, &User{})
    }
}

// Global instance
var Users = UserCollection{
    Fields: UserFields{
        Email:     orm.NewField[string]("email", orm.Required()),
        Name:      orm.NewField[string]("name", orm.Required()),
        Age:       orm.NewField[int]("age"),
        Verified:  orm.NewField[bool]("verified"),
        CreatedAt: orm.NewField[time.Time]("created_at"),
        UpdatedAt: orm.NewField[time.Time]("updated_at"),
    },
}

// Usage in application code
func FindUserByEmail(ctx context.Context, db dal.DB, email string) (*User, error) {
    query := Users.FindByEmail(email)
    
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

---

## Next Steps

- See [Query Building](queries.md) for advanced query patterns
- Read [Record Management](records.md) for working with results
- Check [Examples](examples.md) for complete ORM usage
- Review [Schema Handling](schema.md) for database integration
