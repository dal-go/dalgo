package dal

import (
	"context"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestBeforeSave(t *testing.T) {
	data := struct{}{}
	err := BeforeSave(context.Background(), nil, NewRecordWithIncompleteKey("test", reflect.String, data))
	assert.Nil(t, err)
}
