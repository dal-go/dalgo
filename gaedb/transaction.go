package gaedb

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/strongo/log"
	"google.golang.org/appengine/datastore"
	"strings"
)

// RunInTransaction starts new transaction
var RunInTransaction = func(c context.Context, f func(tc context.Context) error, opts *datastore.TransactionOptions) error {
	if LoggingEnabled {
		if opts == nil {
			log.Debugf(c, "gaedb.RunInTransaction(): starting transaction, opts=nil...")
		} else {
			log.Debugf(c, "gaedb.RunInTransaction(): starting transaction, opts=%+v...", *opts)
		}
	}
	attempt := 0
	fWrapped := func(c context.Context) (err error) {
		attempt++
		log.Debugf(c, "tx attempt #%d", attempt)
		if err = f(c); err != nil {
			m := fmt.Sprintf("tx attempt #%d failed: ", attempt)
			if err == datastore.ErrConcurrentTransaction {
				log.Warningf(c, m+err.Error())
			} else {
				log.Errorf(c, m+err.Error())
			}
		}
		return
	}
	if err := dbRunInTransaction(c, fWrapped, opts); err != nil {
		if LoggingEnabled {
			if strings.Contains(err.Error(), "nested transactions are not supported") {
				panic(err)
			}
			log.Errorf(c, errors.WithMessage(err, "transaction failed").Error())
		}
		return err
	}
	if LoggingEnabled {
		log.Debugf(c, "Transaction successful")
	}
	return nil
}
