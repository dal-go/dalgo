package gaedb

import (
	"github.com/pkg/errors"
	"github.com/strongo/db"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"testing"
)

func TestDatabase_RunInTransaction(t *testing.T) {
	dbInstance := gaeDatabase{}
	i, j := 0, 0

	var xg bool

	RunInTransaction = func(c context.Context, f func(c context.Context) error, opts *datastore.TransactionOptions) error {
		if opts == nil {
			if xg {
				t.Error("Expected XG==true")
			}
		} else if opts.XG != xg {
			t.Errorf("Expected XG==%v", xg)
		}
		j += 1
		return f(c)
	}

	xg = true
	err := dbInstance.RunInTransaction(context.Background(), func(c context.Context) error {
		i += 1
		return nil
	}, db.CrossGroupTransaction)

	if err != nil {
		t.Errorf("Got unexpected error: %v", err)
	}

	if i != 1 {
		t.Errorf("Expected 1 exection, got: %d", i)
	}
	if j != 1 {
		t.Errorf("Expected 1 exection, got: %d", i)
	}

	i, j = 0, 0
	xg = false
	err = dbInstance.RunInTransaction(context.Background(), func(c context.Context) error {
		i += 1
		return errors.New("Test1")
	}, db.SingleGroupTransaction)

	if err == nil {
		t.Error("Expected error, got nil")
	} else if err.Error() != "Test1" {
		t.Errorf("Got unexpected error: %v", err)
	}

	if i != 1 {
		t.Errorf("Expected 1 exection, got: %d", i)
	}
	if j != 1 {
		t.Errorf("Expected 1 exection, got: %d", i)
	}
}
