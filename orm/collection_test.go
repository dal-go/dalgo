package orm

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

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
