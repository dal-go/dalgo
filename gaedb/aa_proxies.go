package gaedb

import (
	"github.com/strongo/nds"
	"google.golang.org/appengine/datastore"
	"github.com/strongo/db/mockdb"
)

var (
	LoggingEnabled   = true // TODO: move to Context.WithValue()
	mockDB           *mockdb.MockDB
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
	dbGetMulti         = nds.GetMulti
	dbPut              = nds.Put
	dbPutMulti         = nds.PutMulti
	dbDelete           = nds.Delete
	dbDeleteMulti      = nds.DeleteMulti
)
