package orm

import (
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

var _ Collection = (*UserCollection)(nil)

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
	return dal.CollectionRef{Name: "Users"}
}

func (v UserCollection) Query() dal.QueryBuilder {
	return dal.From(Users.CollectionRef().Name)
}

func (v UserCollection) IDKind() reflect.Kind {
	return reflect.String
}

func TestFieldDefinitionEqualTo(t *testing.T) {
	var condition dal.Condition
	assert.Nil(t, condition)

	// Direct call
	condition = Users.Field.Email.EqualTo("test@example.com")
	assert.NotNil(t, condition)
	assert.Equal(t, "Email = 'test@example.com'", condition.String())

	// Usage example
	query := Users.Query().
		Where(Users.Field.Email.EqualTo("test@example.com")).
		SelectKeysOnly(Users.IDKind())
	assert.NotNil(t, query)
	assert.Equal(t, "SELECT * FROM [Users] WHERE Email = 'test@example.com'", query.String())
}
