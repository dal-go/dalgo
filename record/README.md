# Dalgo helper package: `record`

This helper package contains types that help to simplify working with dalgo records in strongly typed way.

## Type [`WithID[K comparable]`](./with_id.go)

This helper type can/should be used in your own type that packs strongly typed data & ID of your entity.
This simplifies strongly typed work with your entities.

Like:

```go
package models4user

import (
	"github.com/strongo/dalgo/dal"
	"github.com/strongo/dalgo/record"
)

// UserDto is a type that holds user data
type UserDto struct {
	Name  string
	Email string
}

// User is a type that holds both user data and user ID
type User struct {
	record.WithID[string] // In our case ID is a string
	
	// Dto provides strongly typed access to user data
	Dto *UserDto
}

// NewUser creates an object that hold both user data and user ID
func NewUser(id string, dto *UserDto) (user User) {
	user.ID = id
	user.Dto = dto
	user.Key = dal.NewKey("User", dal.WithID(id))
	user.Record = dal.NewRecordWithData(user.Key, dto)
	return user
}
```
