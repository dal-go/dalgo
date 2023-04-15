package dal

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestBeforeSave(t *testing.T) {
	data := struct{}{}
	err := BeforeSave(nil, nil, NewRecordWithIncompleteKey("test", reflect.String, data))
	assert.Nil(t, err)
}
