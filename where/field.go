package where

import (
	"github.com/dal-go/dalgo/dal"
)

// Field creates an expression that represents a FieldRef value
func Field(name string) dal.FieldRef {
	return dal.FieldRef{Name: name}
}
