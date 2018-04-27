package gaedb

import (
	"context"
	"github.com/strongo/log"
	"google.golang.org/appengine/datastore"
)

// Get loads entity from DB by key
var Get = func(c context.Context, key *datastore.Key, val interface{}) error {
	if LoggingEnabled {
		log.Debugf(c, "dbGet(%v)", key2str(key))
	}
	if key.IntID() == 0 && key.StringID() == "" {
		panic("key.IntID() == 0 && key.StringID() is empty string")
	}
	return dbGet(c, key, val)
}

// GetMulti loads multiple entities from DB by key
var GetMulti = func(c context.Context, keys []*datastore.Key, vals interface{}) error {
	if LoggingEnabled {
		logKeys(c, "gaedb.GetMulti", "", keys)
	}
	return dbGetMulti(c, keys, vals)
}
