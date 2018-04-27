package gaedb

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/log"
	"google.golang.org/appengine/datastore"
)

// Put saves entity to datastore
var Put = func(c context.Context, key *datastore.Key, val interface{}) (*datastore.Key, error) {
	if val == nil {
		panic("val == nil")
	}
	var err error
	isPartialKey := key.Incomplete()
	if LoggingEnabled {
		buf := new(bytes.Buffer)
		logEntityProperties(buf, fmt.Sprintf("dbPut(%v) => properties:", key2str(key)), val)
		log.Debugf(c, buf.String())
	}
	if key, err = dbPut(c, key, val); err != nil {
		return key, errors.WithMessage(err, fmt.Sprintf("failed to put to db (key=%v)", key2str(key)))
	} else if LoggingEnabled && isPartialKey {
		log.Debugf(c, "dbPut() inserted new record with key: "+key2str(key))
	}
	return key, err
}

func logEntityProperties(buf *bytes.Buffer, prefix string, val interface{}) (err error) {
	var props []datastore.Property
	if propertyLoadSaver, ok := val.(datastore.PropertyLoadSaver); ok {
		if props, err = propertyLoadSaver.Save(); err != nil {
			return errors.WithMessage(err, "failed to call val.(datastore.PropertyLoadSaver).Save()")
		}
	} else if props, err = datastore.SaveStruct(val); err != nil {
		return errors.WithMessage(err, fmt.Sprintf("failed to call datastore.SaveStruct()"))
	}
	fmt.Fprint(buf, prefix)
	var prevPropName string
	for _, prop := range props {
		if prop.Name == prevPropName {
			fmt.Fprintf(buf, ", %v", prop.Value)
		} else {
			fmt.Fprintf(buf, "\n\t%v: %v", prop.Name, prop.Value)
		}
		prevPropName = prop.Name
	}
	return
}

// PutMulti saves multipe entities to datastore
var PutMulti = func(c context.Context, keys []*datastore.Key, vals interface{}) ([]*datastore.Key, error) {
	if LoggingEnabled {
		//buf := new(bytes.Buffer)
		//buf.WriteString(" => \n")
		//for i, key := range keys {
		//	logEntityProperties(buf, key2str(key) + ": ", vals[i]) // TODO: Needs use of reflection
		//}
		//logKeys(c, "dbPutMulti", buf.String(), keys)
		logKeys(c, "dbPutMulti", "", keys)
	}
	return dbPutMulti(c, keys, vals)
}
