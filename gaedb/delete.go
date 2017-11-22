package gaedb

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"github.com/strongo/log"
)

var Delete = func(c context.Context, key *datastore.Key) error {
	log.Warningf(c, "gaedb.Delete(%v)", key2str(key))
	return dbDelete(c, key)
}

var DeleteMulti = func(c context.Context, keys []*datastore.Key) error {
	log.Warningf(c, "Deleting %v entities", len(keys))
	logKeys(c, "dbDeleteMulti", keys)
	return dbDeleteMulti(c, keys)
}


