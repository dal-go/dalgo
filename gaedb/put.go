package gaedb

import (
	"bytes"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"github.com/strongo/log"
	"github.com/pkg/errors"
)

var Put = func(c context.Context, key *datastore.Key, val interface{}) (*datastore.Key, error) {
	if val == nil {
		panic("val == nil")
	}
	var err error
	isPartialKey := key.Incomplete()
	if LoggingEnabled {
		buf := new(bytes.Buffer)
		fmt.Fprintf(buf, "dbPut(%v) => properties:", key2str(key))
		if props, err := datastore.SaveStruct(val); err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("failed to SaveStruct(%v) to properties", val))
		} else {
			var prevPropName string
			for _, prop := range props {
				if prop.Name == prevPropName {
					fmt.Fprintf(buf, ", %v", prop.Value)
				} else {
					fmt.Fprintf(buf, "\n\t%v: %v", prop.Name, prop.Value)
				}
				prevPropName = prop.Name
			}
		}
		log.Debugf(c, buf.String())
	}
	if key, err = dbPut(c, key, val); err != nil {
		return key, errors.WithMessage(err, "failed to put to db "+key2str(key))
	} else if LoggingEnabled && isPartialKey {
		log.Debugf(c, "dbPut() inserted new record with key: "+key2str(key))
	}
	return key, err
}

var PutMulti = func(c context.Context, keys []*datastore.Key, vals interface{}) ([]*datastore.Key, error) {
	if LoggingEnabled {
		//buf := new(bytes.Buffer)
		//buf.WriteString("dbPutMulti(")
		//for i, key := range keys {
		// Need to use reflection...
		//}
		//buf.WriteString(")")
		logKeys(c, "dbPutMulti", keys)
	}
	return dbPutMulti(c, keys, vals)
}

