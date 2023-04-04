# Dalgo helper package: `record`

Contains helpers to simplify working with dalgo records in strongly typed way.

## Type [`WithID[K comparable]`](./with_id.go)

This helper type can/should be used in your own type that packs strongly typed data & ID of your entity.
This simplifies strongly typed work with your entities.

Like:

```go
package models4user

import (
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
)

// UserDto is a type that holds user data
type UserDto struct {
	Name  string
	Email string
}

// User is a type that holds both user data and user ID
type User struct {
	record.WithID[string] // In our case ID is a string

	Dto *UserDto
}

// NewUser creates an object that hold both user data and user ID
func NewUser(id string, dto *UserDto) (user User) {
	user.ID = id
	user.Dto = dto
	user.Key = dal.NewKey("users", dal.WithID(id))
	user.Record = dal.NewRecordWithData(user.Key, dto)
	return user
}

```

The `User.Dto` provides strongly typed access to user data.
We also can access it via `Data()` method of `record.Record` interface but that will require type assertion like:

```go
email := user.Record.Data().(*UserDto).Email // requires manual casting to *UserDto to access email
```

what is not very convenient. Compare it with:

```go
email := user.Dto.Email // this is strongly typed
```
