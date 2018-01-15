package gaedb

import (
	"github.com/strongo/log"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

var Get = func(c context.Context, key *datastore.Key, val interface{}) error {
	if LoggingEnabled {
		log.Debugf(c, "dbGet(%v)", key2str(key))
	}
	if key.IntID() == 0 && key.StringID() == "" {
		panic("key.IntID() == 0 && key.StringID() is empty string")
	}
	return dbGet(c, key, val)
}

var GetMulti = func(c context.Context, keys []*datastore.Key, vals interface{}) error {
	if LoggingEnabled {
		logKeys(c, "dbGetMulti", "", keys)
	}
	return dbGetMulti(c, keys, vals)
}
