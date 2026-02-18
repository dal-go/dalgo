# Schema Handling

This document explains how DALgo handles schema mapping between key-value and relational databases.

## Table of Contents

- [The Schema Problem](#the-schema-problem)
- [Schema Interface](#schema-interface)
- [Key-Value Databases](#key-value-databases)
- [Relational Databases](#relational-databases)
- [Schema Patterns](#schema-patterns)
- [Best Practices](#best-practices)

---

## The Schema Problem

DALgo supports both key-value and relational databases, which handle keys differently:

### Key-Value Databases

```
Key: users/user123
Value: {"name": "Alice", "email": "alice@example.com"}
```

The key is **separate** from the value. The ID (`user123`) is not duplicated in the data.

### Relational Databases

```sql
CREATE TABLE users (
    id VARCHAR PRIMARY KEY,
    name VARCHAR,
    email VARCHAR
);
```

The key (`id`) is a **column** in the table, part of the row data.

### The Challenge

When using DALgo with SQL databases, we need to:
1. **Writing**: Inject the key ID into the data as a column
2. **Reading**: Extract the key ID from the column data

The `Schema` interface solves this problem.

---

## Schema Interface

```go
type Schema interface {
    // DataToKey: Extract key from data read from database
    DataToKey(incompleteKey *Key, data any) (key *Key, err error)
    
    // KeyToFields: Map key to fields when writing to database
    KeyToFields(key *Key, data any) (fields []ExtraField, err error)
}

type ExtraField struct {
    Name  string
    Value any
}
```

### Creating a Schema

```go
schema := dal.NewSchema(keyToFieldsFunc, dataToKeyFunc)
```

---

## Key-Value Databases

For key-value stores (Firestore, Datastore, BadgerDB), no schema mapping is needed:

```go
// No mapping required
schema := dal.NewSchema(nil, nil)
```

### Why No Mapping?

```go
// Writing to Firestore
key := dal.NewKeyWithID("users", "user123")
user := &User{Name: "Alice", Email: "alice@example.com"}
record := dal.NewRecordWithData(key, user)

// Firestore stores:
// - Key: users/user123
// - Document: {name: "Alice", email: "alice@example.com"}
// ID is NOT in the document!

// Reading from Firestore
record := dal.NewRecordWithData(key, &User{})
db.Get(ctx, record)
// Key comes from document path, not from data
```

---

## Relational Databases

For SQL databases, schema mapping is **required**.

### Simple ID Column

```go
type User struct {
    ID    string `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

schema := dal.NewSchema(
    // KeyToFields: Inject ID into data
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        return []dal.ExtraField{
            {Name: "id", Value: key.ID},
        }, nil
    },
    
    // DataToKey: Extract ID from data
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        user := data.(*User)
        return dal.NewKeyWithID(incompleteKey.Collection(), user.ID), nil
    },
)
```

### How It Works

#### Writing (Set)

```go
key := dal.NewKeyWithID("users", "user123")
user := &User{Name: "Alice", Email: "alice@example.com"}
record := dal.NewRecordWithData(key, user)

db.Set(ctx, record)

// 1. DALgo calls KeyToFields(key, user)
// 2. Returns: [{Name: "id", Value: "user123"}]
// 3. SQL adapter generates:
//    INSERT INTO users (id, name, email) 
//    VALUES ('user123', 'Alice', 'alice@example.com')
```

#### Reading (Get)

```go
key := dal.NewKeyWithID("users", "user123")
user := &User{}
record := dal.NewRecordWithData(key, user)

db.Get(ctx, record)

// 1. SQL adapter queries:
//    SELECT id, name, email FROM users WHERE id = 'user123'
// 2. Returns: {id: "user123", name: "Alice", email: "alice@example.com"}
// 3. Unmarshals into user struct
// 4. DALgo calls DataToKey(incompleteKey, user)
// 5. Returns: Key with ID "user123"
```

### Composite Keys

For multi-column primary keys:

```go
type TenantUser struct {
    TenantID string `db:"tenant_id"`
    UserID   string `db:"user_id"`
    Name     string `db:"name"`
    Email    string `db:"email"`
}

schema := dal.NewSchema(
    // KeyToFields: Inject both key fields
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        fields := key.ID.([]dal.FieldVal)
        extraFields := make([]dal.ExtraField, len(fields))
        for i, f := range fields {
            extraFields[i] = dal.ExtraField{
                Name:  f.Name,
                Value: f.Value,
            }
        }
        return extraFields, nil
    },
    
    // DataToKey: Extract both key fields
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        user := data.(*TenantUser)
        fields := []dal.FieldVal{
            {Name: "tenant_id", Value: user.TenantID},
            {Name: "user_id", Value: user.UserID},
        }
        return dal.NewKeyWithFields(incompleteKey.Collection(), fields...), nil
    },
)
```

Usage:

```go
// Create composite key
fields := []dal.FieldVal{
    {Name: "tenant_id", Value: "tenant1"},
    {Name: "user_id", Value: "user123"},
}
key := dal.NewKeyWithFields("tenant_users", fields...)

// Use normally
user := &TenantUser{Name: "Alice", Email: "alice@example.com"}
record := dal.NewRecordWithData(key, user)
db.Set(ctx, record)

// Generates SQL:
// INSERT INTO tenant_users (tenant_id, user_id, name, email)
// VALUES ('tenant1', 'user123', 'Alice', 'alice@example.com')
```

### Integer IDs

```go
type Post struct {
    ID      int    `db:"id"`
    Title   string `db:"title"`
    Content string `db:"content"`
}

schema := dal.NewSchema(
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        return []dal.ExtraField{
            {Name: "id", Value: key.ID},
        }, nil
    },
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        post := data.(*Post)
        return dal.NewKeyWithID(incompleteKey.Collection(), post.ID), nil
    },
)
```

### Auto-Increment IDs

For databases with auto-increment IDs:

```go
schema := dal.NewSchema(
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        // For new records, ID might be 0 (to be auto-generated)
        if key.ID.(int) == 0 {
            return nil, nil // Let database generate ID
        }
        return []dal.ExtraField{
            {Name: "id", Value: key.ID},
        }, nil
    },
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        post := data.(*Post)
        // After insert, database sets ID
        return dal.NewKeyWithID(incompleteKey.Collection(), post.ID), nil
    },
)

// Insert with auto-generated ID
post := &Post{Title: "Hello", Content: "World"}
key := dal.NewIncompleteKey("posts", reflect.Int, nil)
record := dal.NewRecordWithData(key, post)

db.Insert(ctx, record)
// After insert, post.ID is set by database
// record.Key().ID contains the generated ID
```

---

## Schema Patterns

### Reflection-Based Schema

Generic schema that works with any struct:

```go
import "reflect"

func NewReflectionSchema(idFieldName string) dal.Schema {
    return dal.NewSchema(
        func(key *dal.Key, data any) ([]dal.ExtraField, error) {
            return []dal.ExtraField{
                {Name: "id", Value: key.ID},
            }, nil
        },
        func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
            v := reflect.ValueOf(data).Elem()
            idField := v.FieldByName(idFieldName)
            if !idField.IsValid() {
                return nil, fmt.Errorf("field %s not found", idFieldName)
            }
            id := idField.Interface()
            return dal.NewKeyWithID(incompleteKey.Collection(), id), nil
        },
    )
}

// Usage
schema := NewReflectionSchema("ID")
```

### Tag-Based Schema

Use struct tags to define key fields:

```go
type User struct {
    ID    string `db:"id" key:"true"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

func NewTagBasedSchema() dal.Schema {
    return dal.NewSchema(
        func(key *dal.Key, data any) ([]dal.ExtraField, error) {
            fields := []dal.ExtraField{}
            v := reflect.ValueOf(data).Elem()
            t := v.Type()
            
            for i := 0; i < t.NumField(); i++ {
                field := t.Field(i)
                if field.Tag.Get("key") == "true" {
                    name := field.Tag.Get("db")
                    value := v.Field(i).Interface()
                    fields = append(fields, dal.ExtraField{
                        Name:  name,
                        Value: value,
                    })
                }
            }
            
            return fields, nil
        },
        func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
            // Extract key fields from struct
            // Similar reflection logic
            return nil, nil
        },
    )
}
```

### Hierarchical Schema

For parent-child relationships:

```go
type Member struct {
    TeamID   string `db:"team_id"`
    MemberID string `db:"member_id"`
    Name     string `db:"name"`
    Role     string `db:"role"`
}

schema := dal.NewSchema(
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        fields := []dal.ExtraField{
            {Name: "member_id", Value: key.ID},
        }
        
        // Add parent key
        if parent := key.Parent(); parent != nil {
            fields = append(fields, dal.ExtraField{
                Name:  "team_id",
                Value: parent.ID,
            })
        }
        
        return fields, nil
    },
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        member := data.(*Member)
        
        // Create parent key
        teamKey := dal.NewKeyWithID("teams", member.TeamID)
        
        // Create child key with parent
        return dal.NewKeyWithParentAndID(teamKey, "members", member.MemberID), nil
    },
)

// Usage
teamKey := dal.NewKeyWithID("teams", "engineering")
memberKey := dal.NewKeyWithParentAndID(teamKey, "members", "alice")
member := &Member{Name: "Alice", Role: "Engineer"}
record := dal.NewRecordWithData(memberKey, member)

db.Set(ctx, record)
// Generates SQL:
// INSERT INTO members (team_id, member_id, name, role)
// VALUES ('engineering', 'alice', 'Alice', 'Engineer')
```

### UUID Schema

For UUID-based keys:

```go
import "github.com/google/uuid"

type Entity struct {
    ID   uuid.UUID `db:"id"`
    Name string    `db:"name"`
}

schema := dal.NewSchema(
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        return []dal.ExtraField{
            {Name: "id", Value: key.ID.(uuid.UUID).String()},
        }, nil
    },
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        entity := data.(*Entity)
        return dal.NewKeyWithID(incompleteKey.Collection(), entity.ID), nil
    },
)
```

---

## Best Practices

### 1. Match Database Structure

```go
// ✅ Good: Schema matches database structure
// Database: users table with 'user_id' column
schema := dal.NewSchema(
    func(key *dal.Key, data any) ([]dal.ExtraField, error) {
        return []dal.ExtraField{
            {Name: "user_id", Value: key.ID}, // Matches column name
        }, nil
    },
    func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
        user := data.(*User)
        return dal.NewKeyWithID(incompleteKey.Collection(), user.UserID), nil
    },
)

// ❌ Bad: Mismatch between schema and database
// Database has 'user_id', but schema uses 'id'
```

### 2. Handle Nil Keys Gracefully

```go
// ✅ Good: Check for incomplete keys
func(key *dal.Key, data any) ([]dal.ExtraField, error) {
    if key.ID == nil {
        return nil, nil // Let database generate ID
    }
    return []dal.ExtraField{{Name: "id", Value: key.ID}}, nil
}
```

### 3. Type Safety

```go
// ✅ Good: Type assertions with checks
func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
    user, ok := data.(*User)
    if !ok {
        return nil, fmt.Errorf("expected *User, got %T", data)
    }
    return dal.NewKeyWithID(incompleteKey.Collection(), user.ID), nil
}

// ❌ Bad: Unsafe type assertion
func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
    user := data.(*User) // Panics if wrong type
    return dal.NewKeyWithID(incompleteKey.Collection(), user.ID), nil
}
```

### 4. Consistent Field Names

```go
// ✅ Good: Consistent naming across schema functions
const idFieldName = "id"

keyToFields := func(key *dal.Key, data any) ([]dal.ExtraField, error) {
    return []dal.ExtraField{{Name: idFieldName, Value: key.ID}}, nil
}

dataToKey := func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
    // Uses same field name
    return extractKeyFromField(data, idFieldName)
}
```

### 5. Document Schema Assumptions

```go
// ✅ Good: Documented schema
// NewUserSchema creates a schema for the users table.
// Assumes:
// - Table name: "users"
// - Primary key: "id" column (string type)
// - No parent relationships
func NewUserSchema() dal.Schema {
    return dal.NewSchema(
        func(key *dal.Key, data any) ([]dal.ExtraField, error) {
            return []dal.ExtraField{{Name: "id", Value: key.ID}}, nil
        },
        func(incompleteKey *dal.Key, data any) (*dal.Key, error) {
            user := data.(*User)
            return dal.NewKeyWithID("users", user.ID), nil
        },
    )
}
```

### 6. Test Schema Functions

```go
func TestSchema(t *testing.T) {
    schema := NewUserSchema()
    
    // Test KeyToFields
    key := dal.NewKeyWithID("users", "user123")
    user := &User{Name: "Alice"}
    fields, err := schema.KeyToFields(key, user)
    if err != nil {
        t.Fatal(err)
    }
    if len(fields) != 1 || fields[0].Name != "id" || fields[0].Value != "user123" {
        t.Errorf("KeyToFields: unexpected result: %+v", fields)
    }
    
    // Test DataToKey
    user = &User{ID: "user456", Name: "Bob"}
    incompleteKey := dal.NewIncompleteKey("users", reflect.String, nil)
    key, err = schema.DataToKey(incompleteKey, user)
    if err != nil {
        t.Fatal(err)
    }
    if key.ID != "user456" {
        t.Errorf("DataToKey: expected user456, got %v", key.ID)
    }
}
```

---

## Next Steps

- See [Database Adapters](adapters.md) for adapter implementation
- Read [Record Management](records.md) for working with keys
- Check [Examples](examples.md) for complete schema patterns
- Review SQL adapter source code for real-world examples
