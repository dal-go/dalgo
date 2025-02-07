package orm

import (
	"github.com/dal-go/dalgo/dal"
	"reflect"
)

var _ Collection = (*UserCollection)(nil)

// This will be used to query data
var Users = UserCollection{
	Field: UserFields{
		Email: NewField("Email", Required[string]()),
	},
}

type UserFields struct {
	Email FieldDefinition[string]
}

func (v UserFields) Fields() []Field {
	return []Field{v.Email}
}

type UserCollection struct {
	Field UserFields
}

func (v UserCollection) Fields() []Field {
	return []Field{v.Field.Email}
}

func (v UserCollection) CollectionRef() dal.CollectionRef {
	return dal.NewRootCollectionRef("Users", "")
}

func (v UserCollection) Query() dal.QueryBuilder {
	return dal.From(Users.CollectionRef())
}

func (v UserCollection) IDKind() reflect.Kind {
	return reflect.String
}
