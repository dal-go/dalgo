# DALgo ORM

[Object-Relational Mapping](https://en.wikipedia.org/wiki/Object%E2%80%93relational_mapping) (ORM) is a technique
that lets you query and manipulate data from a database
using an object-oriented paradigm.

This ORM works hand to hand with [DALgo DAL](../dal).

Firs of all you need to define fields of your data objects:

```go
package schema

import (
	"github.com/strongo/dalgo/dal"
    "github.com/strongo/dalgo/orm"
)

type UserFields struct {
	Email     orm.FieldDefinition[string]
	FirstName orm.FieldDefinition[string]
	LastName  orm.FieldDefinition[string]
}

type UserCollection struct {
    Fields UserFields
}
func (v UserCollection) CollectionRef() dal.CollectionRef {
	return dal.CollectionRef{Name: "Users"}
}

// This will be used to query data
var Users = UserCollection{
	Fields: UserFields{
        Email:     orm.NewFieldDefinition("email", orm.KindString),
        FirstName: orm.NewFieldDefinition("first_name", orm.KindString),
        LastName:  orm.NewFieldDefinition("last_name", orm.KindString),
    },
} 

```

Now you can create queries to database using strongly typed code.

For example imaging we'd like to select a user with specific email:
```go
package example

import (
    "github.com/strongo/dalgo/dal"
    "github.com/strongo/dalgo/orm"
)

func QueryUserIDByEmail(email string) *dal.Query {
    return schema.Users.Query().
        Where(schema.Users.Field.Email.EqualTo("test@example.com")).
        Limit(1).
        SelectKeysOnly(schema.Users.IDKind())
}
```
