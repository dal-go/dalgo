# Hooks and Validation

This document covers data validation and operation hooks in DALgo.

## Table of Contents

- [Validation](#validation)
- [Hooks System](#hooks-system)
- [Common Validation Patterns](#common-validation-patterns)
- [Best Practices](#best-practices)

---

## Validation

DALgo automatically validates records before save operations if they implement the `ValidatableRecord` interface.

### ValidatableRecord Interface

```go
type ValidatableRecord interface {
    Validate() error
}
```

### Basic Validation

```go
type User struct {
    ID    string
    Name  string
    Email string
    Age   int
}

func (u *User) Validate() error {
    if u.Name == "" {
        return errors.New("name is required")
    }
    if u.Email == "" {
        return errors.New("email is required")
    }
    if !strings.Contains(u.Email, "@") {
        return errors.New("invalid email format")
    }
    if u.Age < 0 || u.Age > 150 {
        return errors.New("age must be between 0 and 150")
    }
    return nil
}

// Usage
user := &User{Name: "Alice", Email: "alice@example.com", Age: 30}
record := dal.NewRecordWithData(key, user)

err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    // Validate() is called automatically before Set
    return tx.Set(ctx, record)
})
if err != nil {
    // Validation error wrapped in ErrHookFailed
    if errors.Is(err, dal.ErrHookFailed) {
        log.Printf("Validation failed: %v", err)
    }
}
```

### Validation with Multiple Checks

```go
func (u *User) Validate() error {
    errs := make([]error, 0)
    
    if u.Name == "" {
        errs = append(errs, errors.New("name is required"))
    }
    
    if u.Email == "" {
        errs = append(errs, errors.New("email is required"))
    } else if !isValidEmail(u.Email) {
        errs = append(errs, errors.New("invalid email format"))
    }
    
    if u.Age < 0 {
        errs = append(errs, errors.New("age cannot be negative"))
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("validation failed: %w", errors.Join(errs...))
    }
    
    return nil
}

func isValidEmail(email string) bool {
    // Simple validation
    return strings.Contains(email, "@") && strings.Contains(email, ".")
}
```

### Validation with Key

Some records need to validate against their key:

```go
type ValidatableWithKey interface {
    ValidateWithKey(*dal.Key) error
}

type Post struct {
    ID      string
    UserID  string
    Title   string
    Content string
}

func (p *Post) ValidateWithKey(key *dal.Key) error {
    // Ensure ID in data matches key
    if key.ID != p.ID {
        return fmt.Errorf("ID mismatch: key=%s, data=%s", key.ID, p.ID)
    }
    
    // Validate parent relationship
    if parent := key.Parent(); parent != nil {
        if parent.ID != p.UserID {
            return errors.New("post.UserID must match parent key")
        }
    }
    
    return p.Validate()
}

func (p *Post) Validate() error {
    if p.Title == "" {
        return errors.New("title is required")
    }
    if len(p.Title) > 200 {
        return errors.New("title too long (max 200 characters)")
    }
    if p.Content == "" {
        return errors.New("content is required")
    }
    return nil
}
```

---

## Hooks System

DALgo provides a hook system for executing code before/after database operations.

### BeforeSave Hook

```go
import "github.com/dal-go/dalgo/dal"

// Called automatically before Set operations
err := dal.BeforeSave(ctx, db, record)
if err != nil {
    // Validation or hook failed
}
```

### Custom Hooks

Define custom record hooks:

```go
type RecordHook func(ctx context.Context, record dal.Record) error

// Register custom hooks
var customHooks []dal.RecordHook

func init() {
    customHooks = append(customHooks, timestampHook, auditLogHook)
}

func timestampHook(ctx context.Context, record dal.Record) error {
    // Add timestamp to records
    data := record.Data()
    if timestamped, ok := data.(interface{ SetUpdatedAt(time.Time) }); ok {
        timestamped.SetUpdatedAt(time.Now())
    }
    return nil
}

func auditLogHook(ctx context.Context, record dal.Record) error {
    // Log all database operations
    log.Printf("Operation on record: %s", record.Key())
    return nil
}
```

### Wrapping Database with Hooks

```go
// WithHooks wraps a database to add custom hooks
func WithHooks(db dal.DB, hooks ...dal.RecordHook) dal.DB {
    return &dbWithHooks{
        DB:    db,
        hooks: hooks,
    }
}

type dbWithHooks struct {
    dal.DB
    hooks []dal.RecordHook
}

func (db *dbWithHooks) Set(ctx context.Context, record dal.Record) error {
    // Run hooks before operation
    for _, hook := range db.hooks {
        if err := hook(ctx, record); err != nil {
            return fmt.Errorf("%w: %v", dal.ErrHookFailed, err)
        }
    }
    
    // Execute actual operation
    return db.DB.Set(ctx, record)
}

// Implement other methods similarly...
```

---

## Common Validation Patterns

### Required Fields

```go
func (u *User) Validate() error {
    required := map[string]string{
        "name":  u.Name,
        "email": u.Email,
    }
    
    for field, value := range required {
        if strings.TrimSpace(value) == "" {
            return fmt.Errorf("%s is required", field)
        }
    }
    
    return nil
}
```

### Format Validation

```go
import "regexp"

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func (u *User) Validate() error {
    if !emailRegex.MatchString(u.Email) {
        return errors.New("invalid email format")
    }
    return nil
}
```

### Length Validation

```go
func (u *User) Validate() error {
    if len(u.Name) < 2 {
        return errors.New("name too short (min 2 characters)")
    }
    if len(u.Name) > 100 {
        return errors.New("name too long (max 100 characters)")
    }
    if len(u.Bio) > 500 {
        return errors.New("bio too long (max 500 characters)")
    }
    return nil
}
```

### Range Validation

```go
func (p *Product) Validate() error {
    if p.Price < 0 {
        return errors.New("price cannot be negative")
    }
    if p.Price > 1000000 {
        return errors.New("price too high (max $1,000,000)")
    }
    if p.Quantity < 0 {
        return errors.New("quantity cannot be negative")
    }
    return nil
}
```

### Enum Validation

```go
type Status string

const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive"
    StatusPending  Status = "pending"
)

func (u *User) Validate() error {
    validStatuses := map[Status]bool{
        StatusActive:   true,
        StatusInactive: true,
        StatusPending:  true,
    }
    
    if !validStatuses[u.Status] {
        return fmt.Errorf("invalid status: %s", u.Status)
    }
    
    return nil
}
```

### Cross-Field Validation

```go
func (e *Event) Validate() error {
    if e.StartTime.After(e.EndTime) {
        return errors.New("start time must be before end time")
    }
    
    if e.MinParticipants > e.MaxParticipants {
        return errors.New("min participants cannot exceed max participants")
    }
    
    return nil
}
```

### Conditional Validation

```go
func (u *User) Validate() error {
    // Email required for email-verified users
    if u.EmailVerified && u.Email == "" {
        return errors.New("email required for verified users")
    }
    
    // Password required for non-OAuth users
    if u.AuthProvider == "local" && u.PasswordHash == "" {
        return errors.New("password required for local auth")
    }
    
    return nil
}
```

### Nested Validation

```go
type User struct {
    Name    string
    Email   string
    Address *Address
}

type Address struct {
    Street  string
    City    string
    State   string
    ZipCode string
}

func (a *Address) Validate() error {
    if a.Street == "" {
        return errors.New("street is required")
    }
    if a.City == "" {
        return errors.New("city is required")
    }
    if a.State == "" {
        return errors.New("state is required")
    }
    return nil
}

func (u *User) Validate() error {
    if u.Name == "" {
        return errors.New("name is required")
    }
    
    // Validate nested object
    if u.Address != nil {
        if err := u.Address.Validate(); err != nil {
            return fmt.Errorf("address validation failed: %w", err)
        }
    }
    
    return nil
}
```

### Async Validation (External Services)

```go
func (u *User) Validate() error {
    // Synchronous validation
    if u.Email == "" {
        return errors.New("email is required")
    }
    
    // For async validation (like checking email existence),
    // do it in a separate step, not in Validate()
    return nil
}

func ValidateUserEmail(ctx context.Context, email string) error {
    // Check with external service
    exists, err := emailService.CheckExists(ctx, email)
    if err != nil {
        return fmt.Errorf("failed to validate email: %w", err)
    }
    if exists {
        return errors.New("email already registered")
    }
    return nil
}

// Usage
err := ValidateUserEmail(ctx, user.Email)
if err != nil {
    return err
}

err = db.Set(ctx, record) // Now save
```

---

## Best Practices

### 1. Fast Synchronous Validation Only

```go
// ✅ Good: Quick checks
func (u *User) Validate() error {
    if u.Email == "" {
        return errors.New("email required")
    }
    return nil
}

// ❌ Bad: Slow external calls
func (u *User) Validate() error {
    // Don't do this in Validate()!
    exists, _ := database.CheckEmailExists(u.Email)
    if exists {
        return errors.New("email taken")
    }
    return nil
}
```

### 2. Clear Error Messages

```go
// ✅ Good: Descriptive errors
func (u *User) Validate() error {
    if len(u.Name) < 2 {
        return errors.New("name must be at least 2 characters")
    }
    return nil
}

// ❌ Bad: Vague errors
func (u *User) Validate() error {
    if len(u.Name) < 2 {
        return errors.New("invalid")
    }
    return nil
}
```

### 3. Don't Modify Data in Validation

```go
// ✅ Good: Only validate
func (u *User) Validate() error {
    if u.Email == "" {
        return errors.New("email required")
    }
    return nil
}

// ❌ Bad: Modifying in validation
func (u *User) Validate() error {
    u.Email = strings.ToLower(u.Email) // Don't modify!
    return nil
}

// ✅ Better: Use a separate Prepare/Normalize method
func (u *User) Prepare() {
    u.Email = strings.ToLower(u.Email)
    u.Name = strings.TrimSpace(u.Name)
}
```

### 4. Validate Early

```go
// ✅ Good: Validate before transaction
user := &User{Name: "Alice", Email: "alice@example.com"}
if err := user.Validate(); err != nil {
    return fmt.Errorf("invalid user data: %w", err)
}

err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Set(ctx, record) // Validation already done
})

// ❌ Bad: Only validate in transaction
err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
    return tx.Set(ctx, record) // Wastes transaction if validation fails
})
```

### 5. Test Validation Logic

```go
func TestUserValidation(t *testing.T) {
    tests := []struct {
        name    string
        user    *User
        wantErr bool
        errMsg  string
    }{
        {
            name:    "valid user",
            user:    &User{Name: "Alice", Email: "alice@example.com"},
            wantErr: false,
        },
        {
            name:    "missing name",
            user:    &User{Email: "alice@example.com"},
            wantErr: true,
            errMsg:  "name is required",
        },
        {
            name:    "missing email",
            user:    &User{Name: "Alice"},
            wantErr: true,
            errMsg:  "email is required",
        },
        {
            name:    "invalid email",
            user:    &User{Name: "Alice", Email: "not-an-email"},
            wantErr: true,
            errMsg:  "invalid email format",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.user.Validate()
            
            if tt.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                } else if !strings.Contains(err.Error(), tt.errMsg) {
                    t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
                }
            } else {
                if err != nil {
                    t.Errorf("unexpected error: %v", err)
                }
            }
        })
    }
}
```

### 6. Composition for Reusable Validation

```go
// Reusable validators
func validateEmail(email string) error {
    if !emailRegex.MatchString(email) {
        return errors.New("invalid email format")
    }
    return nil
}

func validateLength(field string, value string, min, max int) error {
    length := len(value)
    if length < min {
        return fmt.Errorf("%s too short (min %d characters)", field, min)
    }
    if length > max {
        return fmt.Errorf("%s too long (max %d characters)", field, max)
    }
    return nil
}

// Use in validation
func (u *User) Validate() error {
    if err := validateEmail(u.Email); err != nil {
        return err
    }
    if err := validateLength("name", u.Name, 2, 100); err != nil {
        return err
    }
    return nil
}
```

---

## Next Steps

- See [Error Handling](errors.md) for validation error patterns
- Read [Record Management](records.md) for record lifecycle
- Check [Transactions](transactions.md) for validation in transactions
- Review [Examples](examples.md) for complete validation examples
