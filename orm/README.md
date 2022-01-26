# DALgo ORM

[Object-Relational Mapping](https://en.wikipedia.org/wiki/Object%E2%80%93relational_mapping) (ORM) is a technique
that lets you query and manipulate data from a database
using an object-oriented paradigm.

This ORM works hand to hand with [DALgo DAL](../dal).

Firs of all you need to define fields of your data objects:

```go
type user struct {
	Email     orm.StringField
	FirstName orm.StringField
	LastName  orm.StringField
}

// This will be used to query data
var User = user{
    FirstName: orm.NewStringField("fist_name"),
    LastName:  orm.NewStringField("last_name"),
    Email:     orm.NewStringField("email"),
}

```

Then indicate where the data stored (_e.g. table or collection name_):
```go
func (v user) Collection() *dal.CollectionRef {
	return &dal.CollectionRef{
		Name: "users",
	}
}
```

Now you can create queries to database using strongly typed code.

For example imaging we'd like to select a user with specific email:
```go
type row struct {
	FirstName string
	LastName string
}

query := dal.Select{
    From:  User.Collection(),
    Where: User.Email.EqualToString(email),
    Columns: query.Columns(User.FirstName.Name(), User.LastName.Name()),
    Into: func() interface{} {
        return &row{}
    },
    Limit: 1,
}
```
