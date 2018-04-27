package gaedb

import (
	"cloud.google.com/go/datastore"
	"github.com/pkg/errors"
	"github.com/strongo/db"
)

// SaverWrapper used to serialize struct to properties on saving
type SaverWrapper struct {
	entity db.EntityHolder
}

var _ datastore.PropertyLoadSaver = (*SaverWrapper)(nil)

// Load loads props
func (wrapper SaverWrapper) Load([]datastore.Property) (err error) {
	return errors.New("gaedb.SaverWrapper does not support Load() method")
}

// Save save props
func (wrapper SaverWrapper) Save() (props []datastore.Property, err error) {
	return
}
