package gaedb

import (
	"github.com/strongo/log"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

var Delete = func(c context.Context, key *datastore.Key) error {
	log.Debugf(c, "gaedb.Delete(%v)", key2str(key))
	return dbDelete(c, key)
}

var DeleteMulti = func(c context.Context, keys []*datastore.Key) error {
	if len(keys) == 1 {
		return Delete(c, keys[0])
	}
	logKeys(c, "gaedb.DeleteMulti", "", keys)
	return dbDeleteMulti(c, keys)
}
