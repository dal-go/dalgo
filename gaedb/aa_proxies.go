package gaedb

import (
	"bytes"
	"fmt"
	//"github.com/strongo/nds"
	"github.com/strongo/log"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"os"
	"github.com/strongo/db"
	"strconv"
	"strings"
	"github.com/pkg/errors"
	"github.com/strongo/nds"
)

var (
	LoggingEnabled   = true // TODO: move to Context.WithValue()
	mockDB           MockDB
	NewIncompleteKey = datastore.NewIncompleteKey
	NewKey           = datastore.NewKey

	//dbRunInTransaction = datastore.RunInTransaction
	//dbGet = datastore.Get
	//dbGetMulti = datastore.GetMulti
	//dbPut = datastore.Put
	//dbPutMulti = datastore.PutMulti
	//dbDelete = datastore.Delete
	//dbDeleteMulti = datastore.DeleteMulti

	dbRunInTransaction = nds.RunInTransaction
	dbGet              = nds.Get
	dbGetMulti         = datastore.GetMulti
	dbPut              = nds.Put
	dbPutMulti         = nds.PutMulti
	dbDelete           = nds.Delete
	dbDeleteMulti      = nds.DeleteMulti
)
